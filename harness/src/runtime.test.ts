import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createInterface } from 'node:readline';
import { createServer, type IncomingMessage, type ServerResponse } from 'node:http';
import type { AddressInfo } from 'node:net';
import { summaryMessage, textOf, userMessage } from './context.js';
import { fauxAssistantMessage, type Message } from '@earendil-works/pi-ai';

async function harness(start:Record<string,unknown>,onTool?:(message:any)=>unknown){
  const child=spawn(process.execPath,[process.env.HARNESS_TEST_ENTRY||new URL('./main.js',import.meta.url).pathname.replace(/^\/(\w:)/,'$1')],{stdio:['pipe','pipe','pipe']});
  const records:any[]=[],errors:string[]=[];
  child.stderr.on('data',data=>errors.push(data.toString()));
  const lines=createInterface({input:child.stdout});
  const result=new Promise<any[]>((resolve,reject)=>{
    const timer=setTimeout(()=>{child.kill();reject(new Error('harness timeout: '+errors.join('')));},20000);
    lines.on('line',line=>{
      const message=JSON.parse(line);records.push(message);
      if(message.requestId){let value:unknown={ok:true};if(message.type==='authorize')value={attemptId:'attempt-'+message.requestId};else if(message.type==='tool')value=onTool?.(message)||{content:{revisionId:'revision1',text:'她没有透露秘密。'}};child.stdin.write(JSON.stringify({type:'response',requestId:message.requestId,result:value})+'\n');}
      if(message.type==='done'||message.type==='error'){clearTimeout(timer);resolve(records);}
    });
    child.once('exit',code=>{if(!records.some(r=>r.type==='done'||r.type==='error')){clearTimeout(timer);reject(new Error('early exit '+code+':'+errors.join('')));}});
  });
  child.stdin.write(JSON.stringify(start)+'\n');
  try{return await result;}finally{child.stdin.end();child.kill();}
}
function start(overrides:Record<string,unknown>={}){
  return {type:'start',turnId:'turn1',mode:'chat',text:'查一下她知道什么',systemPrompt:'你是小说助手，只用授权工具。',messages:[],historyCurrent:true,context:{chapterId:'chapter1'},tools:[{name:'read_chapter',description:'读取当前章',parameters:{type:'object',properties:{maxChars:{type:'integer'}},additionalProperties:false}},{name:'search_story_memory',description:'检索确认来源',parameters:{type:'object',properties:{query:{type:'string'},limit:{type:'integer'}},required:['query'],additionalProperties:false}}],model:{model:'test-model',api:'responses',fake:true,baseUrl:'',apiKey:'',headers:{},maxOutputTokens:500,contextWindow:32000},maxSteps:6,maxContextTokens:12000,...overrides};
}
const body=async(req:IncomingMessage)=>{let raw='';for await(const chunk of req)raw+=chunk;return JSON.parse(raw);};
function sse(res:ServerResponse,type:string,data:Record<string,unknown>){res.write(`event: ${type}\ndata: ${JSON.stringify({type,...data})}\n\n`);}

test('real Pi loop executes tools and continues, fake mode is clearly identified and usage stays unknown',async()=>{
  const records=await harness(start());
  assert.equal(records.at(-1).type,'done');
  assert.deepEqual(records.filter(r=>r.type==='tool').map(r=>r.name),['read_chapter','search_story_memory']);
  assert.equal(records.filter(r=>r.type==='authorize').length,3);
  assert.ok(records.filter(r=>r.type==='delta').map(r=>r.text).join('').includes('演示模型'));
  assert.equal(records.filter(r=>r.type==='usage').every(r=>r.usage.reported===false),true);
  assert.equal(records.at(-1).messages.some((m:any)=>m.role==='system'),false);
});

test('explicit compaction uses an accounted model call and retains recent turns plus derived notes',async()=>{
  const history=[];for(let i=0;i<6;i++)history.push(userMessage('作者要求'+i),fauxAssistantMessage('候选'+i));
  const records=await harness(start({mode:'compact',text:'',messages:history}));
  assert.equal(records.at(-1).type,'done');assert.equal(records.filter(r=>r.type==='authorize')[0].kind,'compaction');
  const messages=records.at(-1).messages;
  assert.ok(messages[0].content.includes('派生参考'));assert.equal(messages.at(-2).content,'作者要求5');
  assert.ok(records.at(-1).contextStats.compactedMessages>0);
});

test('a long single request compacts completed source reads and preserves exact intent without orphan tool results',async()=>{
  const intent='核对她知道什么，保持秘密，任何候选都不要采用。';
  const records=await harness(start({text:intent}),message=>({content:{revisionId:'source-'+message.name,text:(message.name==='read_chapter'?'旧页标记':'当前页标记')+'字'.repeat(4500)}}));
  assert.equal(records.at(-1).type,'done',JSON.stringify(records.filter(r=>r.type!=='delta').map(r=>({type:r.type,error:r.error,kind:r.kind,stats:r.contextStats}))));
  assert.ok(records.filter(r=>r.type==='authorize').some(r=>r.kind==='compaction'));
  const messages=records.at(-1).messages as Message[];
  assert.ok(messages.some(m=>m.role==='user'&&textOf(m)===intent));
  assert.equal(JSON.stringify(messages).includes('旧页标记'),false);
  assert.ok(JSON.stringify(messages).includes('当前页标记'));
  for(let i=0;i<messages.length;i++)if(messages[i].role==='toolResult'){
    const result=messages[i] as any;
    assert.ok(messages.slice(0,i).some(m=>m.role==='assistant'&&Array.isArray(m.content)&&m.content.some(c=>c.type==='toolCall'&&c.id===result.toolCallId)));
  }
});

test('Responses stream performs real function-call/result roundtrip, store=false and actual usage',async()=>{
  const requests:any[]=[];
  const server=createServer(async(req,res)=>{
    assert.equal(req.url,'/v1/responses');assert.equal(req.headers['x-test-actor'],'actor-test');
    const input=await body(req);requests.push(input);assert.equal(input.store,false);
    res.writeHead(200,{'Content-Type':'text/event-stream'});
    const id='resp_'+requests.length;
    sse(res,'response.created',{response:{id,status:'in_progress',output:[]}});
    let item:any;
    if(requests.length===1){
      item={type:'function_call',id:'fc_1',call_id:'call_1',name:'read_chapter',arguments:'{"maxChars":1000}',status:'completed'};
      sse(res,'response.output_item.added',{output_index:0,item:{...item,arguments:''}});
      sse(res,'response.function_call_arguments.delta',{output_index:0,delta:item.arguments});
      sse(res,'response.output_item.done',{output_index:0,item});
    }else{
      assert.ok(input.input.some((x:any)=>x.type==='function_call_output'&&x.call_id==='call_1'&&x.output.includes('没有透露秘密')));
      item={type:'message',id:'msg_2',role:'assistant',status:'completed',content:[{type:'output_text',text:'她的身份仍未透露。',annotations:[]}]};
      sse(res,'response.output_item.added',{output_index:0,item:{...item,content:[]}});
      sse(res,'response.content_part.added',{output_index:0,content_index:0,part:{type:'output_text',text:'',annotations:[]}});
      sse(res,'response.output_text.delta',{output_index:0,content_index:0,delta:'她的身份仍未透露。'});
      sse(res,'response.output_item.done',{output_index:0,item});
    }
    sse(res,'response.completed',{response:{id,status:'completed',output:[item],usage:{input_tokens:35,output_tokens:12,total_tokens:47,input_tokens_details:{cached_tokens:5}}}});res.end();
  });
  await new Promise<void>(resolve=>server.listen(0,'127.0.0.1',resolve));
  try{
    const records=await harness(start({model:{model:'test-model',api:'responses',fake:false,baseUrl:`http://127.0.0.1:${(server.address() as AddressInfo).port}/v1`,apiKey:'test-key',headers:{'x-test-actor':'actor-test'},maxOutputTokens:500,contextWindow:32000}}));
    assert.equal(records.at(-1).type,'done');assert.equal(requests.length,2);
    assert.ok(records.some(r=>r.type==='delta'&&r.text.includes('身份')));
    const usage=records.filter(r=>r.type==='usage');assert.equal(usage.length,2);assert.equal(usage[1].usage.inputTokens,35);assert.equal(usage[1].usage.cachedInputTokens,5);assert.equal(usage[1].usage.reported,true);
  }finally{server.closeAllConnections();await new Promise<void>(resolve=>server.close(()=>resolve()));}
});

test('an HTTP failure is never retried automatically',async()=>{
  let count=0;const server=createServer((req,res)=>{count++;res.writeHead(429,{'Content-Type':'application/json'});res.end('{"error":{"message":"retry later"}}');});
  await new Promise<void>(resolve=>server.listen(0,'127.0.0.1',resolve));
  try{const records=await harness(start({model:{model:'test-model',api:'responses',fake:false,baseUrl:`http://127.0.0.1:${(server.address() as AddressInfo).port}/v1`,apiKey:'test-key',headers:{},maxOutputTokens:500,contextWindow:32000}}));assert.equal(count,1);assert.equal(records.at(-1).type,'error');}
  finally{server.closeAllConnections();await new Promise<void>(resolve=>server.close(()=>resolve()));}
});

test('Chat Completions receives tool results and reports provider usage through the same budgeted loop',async()=>{
  let count=0;
  const server=createServer(async(req,res)=>{
    assert.equal(req.url,'/v1/chat/completions');const input=await body(req);count++;assert.equal(input.store,false);
    if(count===2)assert.ok(input.messages.some((m:any)=>m.role==='tool'&&m.content.includes('没有透露秘密')));
    res.writeHead(200,{'Content-Type':'text/event-stream'});
    const delta=count===1?{tool_calls:[{index:0,id:'call_chat',type:'function',function:{name:'read_chapter',arguments:'{"maxChars":1000}'}}]}:{content:'已根据原文保留未知边界。'};
    res.write('data: '+JSON.stringify({id:'chat_'+count,object:'chat.completion.chunk',created:1,model:'test-model',choices:[{index:0,delta,finish_reason:null}]})+'\n\n');
    res.write('data: '+JSON.stringify({id:'chat_'+count,object:'chat.completion.chunk',choices:[{index:0,delta:{},finish_reason:count===1?'tool_calls':'stop'}],usage:{prompt_tokens:24,completion_tokens:8,total_tokens:32}})+'\n\n');
    res.end('data: [DONE]\n\n');
  });
  await new Promise<void>(resolve=>server.listen(0,'127.0.0.1',resolve));
  try{const records=await harness(start({model:{model:'test-model',api:'chat',fake:false,baseUrl:`http://127.0.0.1:${(server.address() as AddressInfo).port}/v1`,apiKey:'test',headers:{},maxOutputTokens:500,contextWindow:32000}}));assert.equal(records.at(-1).type,'done');assert.equal(count,2);assert.equal(records.filter(r=>r.type==='usage')[1].usage.reported,true);}
  finally{server.closeAllConnections();await new Promise<void>(resolve=>server.close(()=>resolve()));}
});

test('a closed stream with no terminal response becomes unknown and is not replayed',async()=>{
  let count=0;const server=createServer((req,res)=>{count++;res.writeHead(200,{'Content-Type':'text/event-stream'});sse(res,'response.created',{response:{id:'resp_incomplete',status:'in_progress',output:[]}});res.end();});
  await new Promise<void>(resolve=>server.listen(0,'127.0.0.1',resolve));
  try{const records=await harness(start({model:{model:'test-model',api:'responses',fake:false,baseUrl:`http://127.0.0.1:${(server.address() as AddressInfo).port}/v1`,apiKey:'test',headers:{},maxOutputTokens:500,contextWindow:32000}}));assert.equal(count,1);assert.equal(records.at(-1).type,'error');assert.equal(records.filter(r=>r.type==='usage')[0].status,'NeedsReconciliation');}
  finally{server.closeAllConnections();await new Promise<void>(resolve=>server.close(()=>resolve()));}
});

test('automatic compaction persists the replaced canonical context instead of re-expanding old history',async()=>{
 const messages=[];for(let i=0;i<8;i++)messages.push(userMessage('作者目标'+i+'：保留左臂伤势。'),fauxAssistantMessage('旧候选建议'.repeat(170)));
 const records=await harness(start({messages,maxContextTokens:3000,text:'继续讨论三个方向'}));
 const final=records.at(-1);assert.equal(final.type,'done',JSON.stringify(records));
 assert.ok(records.some(r=>r.type==='compaction'));
 assert.ok(final.messages.length<messages.length);
 assert.ok(JSON.stringify(final.messages).includes('派生参考'));
 assert.ok(JSON.stringify(final.messages).includes('继续讨论三个方向'));
});

test('book planning starts from the book brief and creates a proposal without a chapter',async()=>{
 const records=await harness(start({text:'先帮我规划这本悬疑小说',context:{scope:'book'},tools:[{name:'read_book_brief',description:'读取书籍规划',parameters:{type:'object',properties:{},additionalProperties:false}},{name:'propose_book_plan',description:'保存待采用的书籍规划',parameters:{type:'object',properties:{text:{type:'string'},title:{type:'string'}},required:['text'],additionalProperties:false}}]}));
 assert.equal(records.at(-1).type,'done');assert.deepEqual(records.filter(r=>r.type==='tool').map(r=>r.name),['read_book_brief','propose_book_plan']);
});

test('explicit keyless provider sends no dummy Authorization header',async()=>{
 let requests=0;const server=createServer(async(req,res)=>{requests++;assert.equal(req.headers.authorization,undefined);await body(req);res.writeHead(200,{'Content-Type':'text/event-stream'});const item={type:'message',id:'msg_noauth',role:'assistant',status:'completed',content:[{type:'output_text',text:'本地服务已连接',annotations:[]}]};sse(res,'response.output_item.added',{output_index:0,item:{...item,content:[]}});sse(res,'response.output_item.done',{output_index:0,item});sse(res,'response.completed',{response:{id:'r_noauth',status:'completed',output:[item],usage:{input_tokens:10,output_tokens:3,total_tokens:13}}});res.end();});
 await new Promise<void>(resolve=>server.listen(0,'127.0.0.1',resolve));
 try{const records=await harness(start({model:{model:'test',api:'responses',fake:false,noAuth:true,baseUrl:`http://127.0.0.1:${(server.address() as AddressInfo).port}/v1`,apiKey:'',headers:{},maxOutputTokens:500,contextWindow:32000}}));assert.equal(records.at(-1).type,'done');assert.equal(requests,1);}
 finally{server.closeAllConnections();await new Promise<void>(resolve=>server.close(()=>resolve()));}
});

test('enabled writing skill is read through the real Pi tool loop before source and candidate work',async()=>{
 const base=start() as any;
 const records=await harness(start({context:{chapterId:'chapter1',writingSkills:{items:[{id:'skill1',name:'voice',version:2,hash:'frozen',preview:'Keep actions concrete.'}]}},tools:[...base.tools,{name:'read_writing_skill',description:'读取写作技能',parameters:{type:'object',properties:{skillId:{type:'string'},path:{type:'string'},maxChars:{type:'integer'}},required:['skillId'],additionalProperties:false}}]}),message=>({content:{source:'frozen-author-guide',text:message.name==='read_writing_skill'?'Keep actions concrete.':'她没有透露秘密。'}}));
 assert.equal(records.at(-1).type,'done');
 assert.deepEqual(records.filter(r=>r.type==='tool').map(r=>r.name),['read_writing_skill','read_chapter','search_story_memory']);
 assert.equal(records.find(r=>r.type==='tool').args.skillId,'skill1');
 assert.ok(JSON.stringify(records.filter(r=>r.type==='authorize').at(-1).messages).includes('Keep actions concrete.'));
});


const memoryReadTool={name:'read_author_memory',description:'读取本轮私有作者偏好',parameters:{type:'object',properties:{memoryId:{type:'string'},maxChars:{type:'integer'}},required:['memoryId'],additionalProperties:false}};
const writingSkillReadTool={name:'read_writing_skill',description:'读取冻结写作技能',parameters:{type:'object',properties:{skillId:{type:'string'},path:{type:'string'},maxChars:{type:'integer'}},required:['skillId'],additionalProperties:false}};

test('author memory is read through Pi before writing skill, current prose and candidate work',async()=>{
 const base=start() as any;
 const records=await harness(start({
   text:'把这段改写得含蓄一些',
   context:{chapterId:'chapter1',authorMemory:{items:[{id:'memory1',scope:'author',kind:'voice',preview:'动作承担潜台词。'}]},writingSkills:{items:[{id:'skill1',name:'dialogue',preview:'简洁对白。'}]}},
   tools:[...base.tools,memoryReadTool,writingSkillReadTool,{name:'create_revision_candidate',description:'创建待采用候选',parameters:{type:'object',properties:{text:{type:'string'},mode:{type:'string'},title:{type:'string'}},required:['text','mode'],additionalProperties:false}},{name:'show_diff',description:'查看差异',parameters:{type:'object',properties:{runId:{type:'string'}},required:['runId'],additionalProperties:false}}],
   maxSteps:8,
 }),message=>({content:{source:message.name==='read_author_memory'?'private-author-memory':message.name==='read_writing_skill'?'frozen-author-guide':'saved-source',text:message.name==='read_author_memory'?'PRIVATE-VOICE: 动作承担潜台词。':message.name==='read_writing_skill'?'SKILL-DIALOGUE: 避免重复解释。':'她没有透露秘密。'},...(message.name==='create_revision_candidate'?{artifact:{kind:'revision-candidate',runId:'candidate1'}}:{})}));
 assert.equal(records.at(-1).type,'done');
 assert.deepEqual(records.filter(r=>r.type==='tool').map(r=>r.name),['read_author_memory','read_writing_skill','read_chapter','create_revision_candidate','show_diff']);
 assert.equal(records.find(r=>r.type==='tool').args.memoryId,'memory1');
 const candidateRequest=records.filter(r=>r.type==='authorize')[3];
 assert.ok(JSON.stringify(candidateRequest.messages).includes('PRIVATE-VOICE: 动作承担潜台词。'));
 assert.ok(JSON.stringify(candidateRequest.messages).includes('SKILL-DIALOGUE: 避免重复解释。'));
 assert.equal(records.filter(r=>r.type==='usage').every(r=>r.usage.reported===false),true);
});

test('book planning reads private author preferences before current book brief and plan proposal',async()=>{
 const records=await harness(start({text:'先规划书籍主线',context:{scope:'book',authorMemory:{items:[{id:'memory-book',scope:'book',kind:'workflow',preview:'先给三个取舍。'}]}},tools:[memoryReadTool,{name:'read_book_brief',description:'读取书籍规划',parameters:{type:'object',properties:{},additionalProperties:false}},{name:'propose_book_plan',description:'保存待采用全书规划',parameters:{type:'object',properties:{text:{type:'string'},title:{type:'string'}},required:['text'],additionalProperties:false}}]}));
 assert.equal(records.at(-1).type,'done');
 assert.deepEqual(records.filter(r=>r.type==='tool').map(r=>r.name),['read_author_memory','read_book_brief','propose_book_plan']);
});

function oldPreferenceHistory():Message[]{
 return [userMessage('OLD-USER-VOICE: 我喜欢每段结尾解释人物情绪。'),fauxAssistantMessage('OLD-ASSISTANT-VOICE: 以后每段解释情绪。'),summaryMessage('OLD-SUMMARY-VOICE: 解释所有情绪。'),{role:'toolResult',toolCallId:'old-memory',toolName:'read_author_memory',content:[{type:'text',text:'OLD-TOOL-VOICE: 解释情绪'}],isError:false,timestamp:1}];
}

test('a memory reset never dispatches old user preference, answer, tool or summary even when the source baseline matches',async()=>{
 for(const historyCurrent of [true,false]){
   const history=oldPreferenceHistory();
   const archive=JSON.stringify(history);
   const records=await harness(start({text:'本轮只讨论人物动作',messages:history,historyCurrent,historyReplayAllowed:false}));
   assert.equal(records.at(-1).type,'done');
   assert.equal(records.at(-1).contextStats.historyRebased,true);
   for(const auth of records.filter(r=>r.type==='authorize'))assert.equal(JSON.stringify(auth.messages).includes('OLD-'),false);
   assert.equal(JSON.stringify(records.at(-1).messages).includes('OLD-'),false);
   assert.ok(JSON.stringify(records.at(-1).messages).includes('本轮只讨论人物动作'));
   assert.deepEqual(records.filter(r=>r.type==='tool').map(r=>r.name),['read_chapter','search_story_memory']);
   assert.equal(records.filter(r=>r.type==='delta').map(r=>r.text).join('').includes('继续刚才的讨论'),false);
   assert.equal(JSON.stringify(history),archive);
 }
});

test('a memory reset precedes explicit compaction so forgotten material is never summarized again',async()=>{
 const records=await harness(start({mode:'compact',text:'',messages:oldPreferenceHistory(),historyCurrent:false,historyReplayAllowed:false}));
 assert.equal(records.at(-1).type,'done');
 assert.equal(records.filter(r=>r.type==='authorize').length,0);
 assert.deepEqual(records.at(-1).messages,[]);
 assert.equal(JSON.stringify(records).includes('OLD-'),false);
});

test('an ordinary prose baseline change still retains original author intent while removing old model derivations',async()=>{
 const history:Message[]=[userMessage('INTENT-KEEP: 只核对动作，不生成候选。'),fauxAssistantMessage('DROP-OLD-ANSWER: 她的秘密身份已确定。'),summaryMessage('DROP-OLD-SUMMARY: 她是王女。')];
 const records=await harness(start({text:'检查新保存的正文',messages:history,historyCurrent:false,historyReplayAllowed:true}));
 assert.equal(records.at(-1).type,'done');
 const first=JSON.stringify(records.find(r=>r.type==='authorize').messages);
 assert.ok(first.includes('INTENT-KEEP'));
 assert.equal(first.includes('DROP-OLD'),false);
});
