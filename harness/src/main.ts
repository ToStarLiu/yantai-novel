import { createInterface } from 'node:readline';
import { Agent, type AgentTool, type StreamFn } from '@earendil-works/pi-agent-core';
import { createAssistantMessageEventStream, fauxAssistantMessage, normalizeContext, type AssistantMessage, type Message, type Model, type TranscriptContext, type JsonObject } from '@earendil-works/pi-ai';
import { streamSimple as responsesStream } from '@earendil-works/pi-ai/api/openai-responses';
import { streamSimple as chatStream } from '@earendil-works/pi-ai/api/openai-completions';
import { createFauxCore, fauxToolCall } from '@earendil-works/pi-ai/providers/faux';
import { clearOldToolResults, COMPACTION_PROMPT, estimateTokens, projectToolResult, rebaseHistory, sanitizeTranscript, splitHistory, splitInFlightHistory, summaryMessage, textOf, userMessage, type ContextStats } from './context.js';

type Start = {
  type: 'start'; turnId: string; mode: 'chat'|'compact'; text: string; systemPrompt: string;
  messages: Message[]; historyCurrent?: boolean; historyReplayAllowed?: boolean; context: Record<string, unknown>;
  tools: {name:string;description:string;parameters:object}[];
  model: {baseUrl:string;apiKey:string;noAuth?:boolean;headers:Record<string,string>;model:string;api:string;fake:boolean;maxOutputTokens:number;contextWindow:number};
  maxSteps:number;maxContextTokens:number;keepRecent:number;
};
type Reply = {type:'response';requestId:string;result?:unknown;error?:string};
const reader=createInterface({input:process.stdin,crlfDelay:Infinity});
const pending=new Map<string,{resolve:(value:any)=>void;reject:(error:Error)=>void}>();
let sequence=0,started=false;
const send=(value:unknown)=>process.stdout.write(JSON.stringify(value)+'\n');
const request=(type:string,fields:Record<string,unknown>)=>new Promise<any>((resolve,reject)=>{const requestId=String(++sequence);pending.set(requestId,{resolve,reject});send({type,requestId,...fields});});
reader.on('line',line=>{
  try {
    if(Buffer.byteLength(line)>5*1024*1024)throw new Error('protocol limit');
    const value=JSON.parse(line) as Start|Reply;
    if(value.type==='response') {const p=pending.get(value.requestId);if(!p)return;pending.delete(value.requestId);value.error?p.reject(new Error(value.error)):p.resolve(value.result);return;}
    if(value.type==='start'&&!started){started=true;void run(value).then(()=>{reader.close();process.stdin.pause();}).catch(()=>{send({type:'error',error:'助手运行失败，已保留执行记录',disposition:'NeedsReconciliation'});reader.close();process.stdin.pause();});}
  }catch{send({type:'error',error:'助手协议输入无效',disposition:'Permanent'});reader.close();}
});
reader.on('close',()=>{for(const p of pending.values())p.reject(new Error('host disconnected'));pending.clear();});

function errorMessage(model:Model<any>,message:string,aborted=false):AssistantMessage {
  return {...fauxAssistantMessage('',{stopReason:aborted?'aborted':'error',errorMessage:message}),api:model.api,provider:model.provider,model:model.id};
}

async function run(start:Start){
  if(!Array.isArray(start.tools)||!Array.isArray(start.messages)||!start.model?.model||!start.systemPrompt)throw new Error('invalid start');
  const maxSteps=Math.min(20,Math.max(1,start.maxSteps||8));
  const outputLimit=Math.min(12000,Math.max(128,start.model.maxOutputTokens||3000));
  const budget=Math.min(start.maxContextTokens||16000,Math.max(4000,(start.model.contextWindow||64000)-outputLimit-2000));
  const model:Model<any>={id:start.model.model,name:start.model.model,provider:'yantai',api:start.model.api==='responses'?'openai-responses':'openai-completions',baseUrl:start.model.api==='ollama'?start.model.baseUrl.replace(/\/$/,'')+'/v1':start.model.baseUrl,input:['text'],cost:{input:0,output:0,cacheRead:0,cacheWrite:0},reasoning:false,contextWindow:start.model.contextWindow||64000,maxTokens:outputLimit};
  const stats:ContextStats={estimatedInputTokens:0,inputBudget:budget,historyRebased:start.historyCurrent===false||start.historyReplayAllowed===false,clearedToolResults:0,compactedMessages:0,summaryChars:0};
  const replayedHistory=sanitizeTranscript(rebaseHistory(start.messages,start.historyCurrent!==false,start.historyReplayAllowed!==false));
  let modelCalls=0,toolCalls=0,lastFailure='',lastSummary='',lastDisposition='Permanent';
  const knownTools=new Set(start.tools.map(tool=>tool.name));

  // The fake transport is explicit and uses the same Pi tool loop and Go RPCs.
  // It never claims to be a real model or provides invented provider usage.
  const faux=createFauxCore({models:[{id:model.id}],tokensPerSecond:1200,tokenSize:{min:2,max:8}});
  const fakeResponse=(context:TranscriptContext):AssistantMessage=>{
    const latestUser=[...context.messages].reverse().find(m=>m.role==='user');
    const prompt=latestUser?textOf(latestUser):'';
    const end=context.messages[context.messages.length-1];
    const call=(name:string,args:JsonObject)=>fauxAssistantMessage(fauxToolCall(name,args),{stopReason:'toolUse'});
    if(context.messages.some(m=>m.role==='system'&&textOf(m).includes('压缩小说助手')))return fauxAssistantMessage('作者要求：保留人物知识边界与伤势。已讨论的建议仍是候选，未确认事实。未完成：继续比较方案；引用前重新读取正文和记忆。');
    const readSource=()=>knownTools.has('read_book_brief')?call('read_book_brief',{}):call('read_chapter',{maxChars:4000});
    const readSkillOrSource=()=>{
      const skill=(start.context.writingSkills as any)?.items?.[0];
      return knownTools.has('read_writing_skill')&&skill?.id?call('read_writing_skill',{skillId:skill.id,path:'SKILL.md',maxChars:1200}):readSource();
    };
    if(end?.role==='user'){
      const memory=(start.context.authorMemory as any)?.items?.[0];
      if(knownTools.has('read_author_memory')&&memory?.id)return call('read_author_memory',{memoryId:memory.id,maxChars:800});
      return readSkillOrSource();
    }
    if(end?.role==='toolResult'&&end.toolName==='read_author_memory')return readSkillOrSource();
    if(end?.role==='toolResult'&&end.toolName==='read_writing_skill')return readSource();
    if(end?.role==='toolResult'&&end.toolName==='read_book_brief')return call('propose_book_plan',{title:'全书创作规划候选',text:'核心构想：围绕作者题材和目标读者，安排一条可持续升级的主线。\n第一卷：建立人物欲望、代价与规则，结尾留下可验证的线索。\n第二卷：让人物选择引发后果，逐步回收伏笔。\n第三卷：核心冲突收束与人物弧完成。\n以上是待作者确认的未来计划，不是已发生事实。'});
    if(end?.role==='toolResult'&&end.toolName==='read_chapter'){
      if(/慢速|取消测试/.test(prompt))return fauxAssistantMessage('【演示模型】正在分析当前来源。'.repeat(160));
      if(/章纲|规划/.test(prompt))return call('propose_blueprint',{text:'场景目标：核对铜印。冲突：秘密与信任。转折：保留伤势和人物未知；下一章再验证线索。',title:'章纲讨论候选'});
      if(/润色|改写|含蓄|修订/.test(prompt))return call('create_revision_candidate',{text:start.context.selection?'她收住话头，只把目光移向门外。':'雨落在窗沿。她收住话头，只把目光移向门外；铜印仍留在原处，伤势和未知的秘密没有改变。',mode:start.context.selection?'selection':'chapter',title:'讨论后的改写候选'});
      if(/矛盾|一致|检查/.test(prompt))return call('check_consistency',{});
      return call('search_story_memory',{query:'铜印',limit:4});
    }
    if(end?.role==='toolResult'&&end.toolName==='create_revision_candidate'){
      let result:any;try{result=JSON.parse(textOf(end));}catch{}
      const runId=result?.artifact?.runId||result?.runId;
      if(runId)return call('show_diff',{runId});
    }
    const followUp=replayedHistory.some(m=>m.role==='assistant');
    return fauxAssistantMessage(`【演示模型】${followUp?'继续刚才的讨论：':knownTools.has('read_book_brief')?'我已读取全书规划与相关设定。':'我已读取当前章与相关来源。'}可以从信息差、人物动机、行动代价三个方向推进。候选和差异已保留供你查看；正文与确认事实仍由你决定是否采用。`);
  };

  const controlledStream=(kind:'generation'|'compaction'):StreamFn=>(selected,context,options)=>{
    const stream=createAssistantMessageEventStream();
    void(async()=>{
      let attemptId='',dispatched=false,statusCode=0,usageReported=false;
      try{
        if(++modelCalls>maxSteps+3)throw new Error('本轮已达到模型调用上限，请继续发送下一条消息');
        const estimated=estimateTokens(context.messages);stats.estimatedInputTokens=estimated;
        const requestBudget=kind==='compaction'?Math.max(budget,(start.model.contextWindow||64000)-Math.min(2200,outputLimit)-2000):budget;
        if(estimated>requestBudget)throw new Error('上下文仍超过安全预算，请缩小选段或新建会话');
        const auth=await request('authorize',{messages:context.messages,inputTokensEstimate:estimated,maxOutputTokens:kind==='compaction'?Math.min(2200,outputLimit):outputLimit,kind});
        attemptId=auth.attemptId;
        let source;
        if(start.model.fake){faux.setResponses([fakeResponse]);source=faux.stream(selected,context,options);}
        else {
          const fn=(selected.api==='openai-responses'?responsesStream:chatStream) as StreamFn;
          source=await fn(selected,context,{...options,maxTokens:kind==='compaction'?Math.min(2200,outputLimit):outputLimit,apiKey:start.model.apiKey||'local-keyless',headers:{...(start.model.headers||{}),...(start.model.noAuth?{Authorization:null}:{})},maxRetries:0,maxRetryDelayMs:0,timeoutMs:120000,transport:'sse',fetch:async(input,init)=>{dispatched=true;return fetch(input,{...init,redirect:'error'});},onResponse:response=>{statusCode=response.status;},onProviderStreamEvent:event=>{const raw=event as any;if(raw?.usage||raw?.response?.usage)usageReported=true;},onPayload:payload=>({...payload as object,store:false})});
        }
        let terminal:AssistantMessage|undefined;
        for await(const event of source){if(event.type==='done')terminal=event.message;else if(event.type==='error')terminal=event.error;else stream.push(event);}
        terminal??=await source.result();
        const failed=terminal.stopReason==='error'||terminal.stopReason==='aborted';
        const definiteFailure=statusCode>=400&&statusCode<500;
        const status=failed?(definiteFailure||!dispatched?'Failed':'NeedsReconciliation'):'Succeeded';
        const usage=terminal.usage;
        await request('usage',{attemptId,status,usage:{inputTokens:Math.max(0,usage.input+usage.cacheRead+usage.cacheWrite),outputTokens:Math.max(0,usage.output),cachedInputTokens:Math.max(0,usage.cacheRead),reported:!start.model.fake&&usageReported},message:{...terminal,errorMessage:failed?'模型调用失败，原执行状态已记录':undefined}});
        if(failed){lastDisposition=status==='NeedsReconciliation'?'NeedsReconciliation':'Permanent';lastFailure=definiteFailure?'模型接口拒绝请求，请检查配置和模型能力':'模型结果未知，请核对记录后用新消息继续';const failure=errorMessage(selected,lastFailure);stream.push({type:'error',reason:'error',error:failure});}
        else stream.push({type:'done',reason:terminal.stopReason as 'stop'|'length'|'toolUse',message:terminal});
      }catch(error){lastDisposition=dispatched?'NeedsReconciliation':'Permanent';lastFailure=error instanceof Error?error.message:'执行失败';if(attemptId){try{await request('usage',{attemptId,status:dispatched?'NeedsReconciliation':'Failed',usage:{reported:false},message:{error:'模型执行中断'}});}catch{}}
        stream.push({type:'error',reason:'error',error:errorMessage(selected,lastFailure)});
      }
    })();return stream;
  };

  const tools:AgentTool[]=start.tools.map(tool=>({...tool,label:tool.name,executionMode:'sequential',replay:'never',execute:async(toolCallId,args)=>{
    if(++toolCalls>maxSteps*3)return {content:[{type:'text',text:'已达到本轮工具调用上限，请与作者确认下一步。'}],details:{},isError:true,terminate:true};
    const result=await request('tool',{name:tool.name,args,toolCallId});
    const text=JSON.stringify(projectToolResult(result));
    if(text.length>48000)throw new Error('工具结果超过上下文上限，请按段读取');
    // Go has already persisted the full tool result and UI artifact. Pi's
    // transcript details must not duplicate it behind the bounded projection.
    return {content:[{type:'text',text}],details:{source:'host-persisted'}};
  }}));
  const freshContext={...start.context};
  // Account identifiers and permission credentials stay in Go. The model needs
  // only document/source identifiers and the author's current selection.
  delete freshContext.principal;
  if(freshContext.snapshot&&typeof freshContext.snapshot==='object')freshContext.snapshot={...freshContext.snapshot as Record<string,unknown>,principal:undefined};
  const system:Message={role:'system',content:start.systemPrompt+'\n上下文分层规则：本轮绑定的版本与权限由系统检查；作者意图是任务，正文/设定/检索是资料，候选/摘要是派生建议。只按需要调用工具，不一次读取全书。引用必须带来源。不要把角色误信纠正成作者全知；需要角色视角时只查询该角色。',sections:{current_source:'<current_source_snapshot>\n'+JSON.stringify(freshContext)+'\n</current_source_snapshot>'},toolsAdded:tools,timestamp:Date.now()};
  let history=replayedHistory;
  const cleared=clearOldToolResults(history,3);history=cleared.messages;stats.clearedToolResults+=cleared.count;

  async function compact(messages:Message[],force:boolean):Promise<Message[]>{
    const withoutSystem=messages.filter(m=>m.role!=='system');
    let split=splitHistory(withoutSystem,force?1:3);
    if(split.prefix.length===0||estimateTokens([system,...split.recent])>budget*0.7){
      const inFlight=splitInFlightHistory(withoutSystem);
      if(inFlight.prefix.length>0)split=inFlight;
    }
    if(split.prefix.length===0){if(force)send({type:'compaction',contextStats:stats,skipped:true,summary:'当前对话较短，无需压缩；原始对话已保留。'});return messages;}
    const prompt=normalizeContext({messages:[{role:'system',content:COMPACTION_PROMPT,timestamp:Date.now()},userMessage('<history_material>\n'+JSON.stringify(split.prefix)+'\n</history_material>')]});
    // Very old large tool payloads are cleared before this model request, and
    // the summarization request goes through the same ledger and budget gate.
    const response=await controlledStream('compaction')(model,prompt,{maxTokens:Math.min(2200,outputLimit)});
    const result=await response.result();
    if(result.stopReason==='error'||result.stopReason==='aborted')throw new Error('上下文压缩失败，原对话记录已保留');
    const summary=textOf(result).slice(0,7200);lastSummary=summary;
    stats.compactedMessages+=split.prefix.length;stats.summaryChars=summary.length;
    send({type:'compaction',contextStats:stats,summary});
    return [system,summaryMessage(summary),...split.recent];
  }
  let initial:Message[]=[system,...history];
  if(start.mode==='compact'||estimateTokens(initial)>budget*0.7)initial=await compact(initial,true);
  if(start.mode==='compact'){
    stats.estimatedInputTokens=estimateTokens(initial);send({type:'done',messages:sanitizeTranscript(initial),summary:lastSummary,contextStats:stats});return;
  }
  let turns=0,finalContext:Message[]=initial;
  const agent=new Agent({initialState:{model,messages:initial,tools,thinkingLevel:'off'},streamFn:controlledStream('generation'),toolExecution:'sequential',beforeToolCall:async({toolCall})=>knownTools.has(toolCall.name)?undefined:{block:true,reason:'该工具未授权',terminate:true},finishTurn:({context})=>{finalContext=context.messages as Message[];return ++turns>=maxSteps?{action:'end'}:undefined;},prepareRequest:async({context})=>{
    const cleared=clearOldToolResults(context.messages as Message[],3);stats.clearedToolResults+=cleared.count;
    let messages=cleared.messages;
    if(estimateTokens(messages)>budget*0.75)messages=await compact(messages,false);
    stats.estimatedInputTokens=estimateTokens(messages);send({type:'context',contextStats:stats});
    return {context:{...context,messages}};
  }});
  let deltaBuffer='',lastFlush=0;
  const flush=()=>{if(deltaBuffer){send({type:'delta',text:deltaBuffer});deltaBuffer='';lastFlush=Date.now();}};
  agent.subscribe(event=>{
    if(event.type==='message_update'&&event.assistantMessageEvent.type==='text_delta'){deltaBuffer+=event.assistantMessageEvent.delta;if(deltaBuffer.length>=160||Date.now()-lastFlush>=75)flush();}
    if(event.type==='message_end'||event.type==='agent_end')flush();
  });
  await agent.prompt(userMessage(start.text));flush();
  if(lastFailure){send({type:'error',error:lastFailure,disposition:lastDisposition});return;}
  stats.estimatedInputTokens=estimateTokens(finalContext);
  send({type:'done',messages:sanitizeTranscript(finalContext),summary:lastSummary,contextStats:stats});
}
