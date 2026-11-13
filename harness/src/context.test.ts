import test from 'node:test';
import assert from 'node:assert/strict';
import { clearOldToolResults, estimateTokens, projectToolResult, rebaseHistory, sanitizeTranscript, splitHistory, splitInFlightHistory, summaryMessage, textOf, userMessage } from './context.js';
import { fauxAssistantMessage } from '@earendil-works/pi-ai';
import { fauxToolCall } from '@earendil-works/pi-ai/providers/faux';
import type { Message } from '@earendil-works/pi-ai';

test('baseline rebase discards old secrets from tools, answers and summaries while keeping author intent', () => {
  const history: Message[] = [userMessage('她应当保持犹豫，不解释身份'),fauxAssistantMessage('旧版秘密：她是王女'),summaryMessage('她是王女'),{role:'toolResult',toolCallId:'x',toolName:'read_chapter',content:[{type:'text',text:'旧秘密'}],isError:false,timestamp:1}];
  const rebased = rebaseHistory(history,false);
  assert.equal(JSON.stringify(rebased).includes('王女'),false);
  assert.equal(JSON.stringify(rebased).includes('旧秘密'),false);
  assert.ok(JSON.stringify(rebased).includes('保持犹豫'));
});

test('large candidate artifacts stay full for the UI while model sees a bounded non-duplicated projection',()=>{
 const result={content:{runId:'run1',text:'原稿'.repeat(30000)},artifact:{runId:'run1',text:'原稿'.repeat(30000),diff:Array(50).fill({before:'旧段'.repeat(1000),after:'新段'.repeat(1000),changed:true})}};
 const projected=projectToolResult(result);
 assert.ok(JSON.stringify(projected).length<2200);
 assert.equal(result.artifact.text.length,60000);
 assert.ok(JSON.stringify(projected).includes('uiHasFullContent'));
});

test('source paging remains exact; projection never silently discards characters between offsets',()=>{
 const source={content:{text:'字'.repeat(4000),offset:0,nextOffset:4000,totalChars:8000,truncated:true}};
 assert.equal((projectToolResult(source) as any).content.text.length,4000);
});

test('old transcript details cannot reintroduce full UI artifacts behind the model projection',()=>{
  const source: Message={role:'toolResult',toolCallId:'call1',toolName:'show_diff',content:[{type:'text',text:'bounded projection'}],isError:false,timestamp:1,details:{artifact:{text:'隐含的完整候选'.repeat(10000)}}};
  const clean=sanitizeTranscript([source]);
  assert.equal(textOf(clean[0]),'bounded projection');
  assert.equal(JSON.stringify(clean).includes('隐含的完整候选'),false);
  assert.ok(JSON.stringify(source).includes('隐含的完整候选'));
});

test('trimming preserves complete recent turns and tool-call/result pairing', () => {
  const history: Message[]=[];
  for(let i=0;i<6;i++) history.push(userMessage('问题'+i),fauxAssistantMessage('回答'+i),{role:'toolResult',toolCallId:String(i),toolName:'read_chapter',content:[{type:'text',text:JSON.stringify({revisionId:'revision-'+i,text:'旧正文'.repeat(1000)})}],isError:false,timestamp:i});
  const {prefix,recent}=splitHistory(history,3);
  assert.equal(prefix.length,9); assert.equal(textOf(recent[0]),'问题3');
  const cleared=clearOldToolResults(history,3);
  assert.equal(cleared.count,3);
  assert.ok(textOf(cleared.messages[2]).includes('revision-0'));
  assert.deepEqual(cleared.messages.slice(9),recent);
  assert.ok(estimateTokens(cleared.messages)<estimateTokens(history));
});

test('in-flight compaction keeps the exact author request and complete latest tool-call group', () => {
  const history: Message[]=[userMessage('只读资料；不要揭示她的身份，也不要采用候选。')];
  for(let i=0;i<3;i++){
    const call=fauxToolCall('read_chapter',{offset:i*4000});
    history.push(fauxAssistantMessage(call,{stopReason:'toolUse'}),{role:'toolResult',toolCallId:call.id,toolName:'read_chapter',content:[{type:'text',text:'第'+i+'页来源'}],isError:false,timestamp:i});
  }
  const {prefix,recent}=splitInFlightHistory(history);
  assert.equal(prefix.length,5);
  assert.equal(textOf(recent[0]),textOf(history[0]));
  assert.equal(recent.length,3);
  assert.equal(recent[1].role,'assistant');
  assert.equal(recent[2].role,'toolResult');
  assert.deepEqual(recent.slice(1),history.slice(-2));
  assert.equal(splitInFlightHistory(history.slice(0,3)).prefix.length,0);
});

test('many long diff hunks fit the model budget while full UI changes remain intact',()=>{
  const result={content:{sourceRevisionId:'rev1',diff:Array.from({length:30},()=>({before:'旧'.repeat(1200),after:'新'.repeat(1200),changed:true}))},artifact:{kind:'diff',diff:Array(30).fill({changed:true}),text:'候选'.repeat(30000)}};
  const projection=projectToolResult(result) as any;
  assert.equal(projection.content.diff.length,6);
  assert.equal(projection.content.diffTotalHunks,30);
  assert.equal(projection.content.diffTruncated,true);
  assert.equal(projection.content.diff[0].beforeTruncated,true);
  assert.ok(estimateTokens(projection)<6500);
  assert.equal(result.content.diff[0].before.length,1200);
  assert.equal(result.content.diff.length,30);
});


test('cleared writing-skill pages retain versioned paths and exact read ranges for rereading',()=>{
  const page:Message={role:'toolResult',toolCallId:'skill-page',toolName:'read_writing_skill',content:[{type:'text',text:JSON.stringify({content:{id:'skill-1',version:4,hash:'sha256hash',path:'references/dialogue.md',offset:1200,nextOffset:2400,text:'对白保留潜台词'.repeat(400)}})}],isError:false,timestamp:1};
  const messages:Message[]=[userMessage('旧请求'),page,userMessage('请求1'),userMessage('请求2'),userMessage('请求3')];
  const cleared=clearOldToolResults(messages,3);const text=textOf(cleared.messages[1]);
  assert.equal(cleared.count,1);for(const value of ['skill-1','"version":4','sha256hash','references/dialogue.md','"offset":1200','"nextOffset":2400'])assert.ok(text.includes(value),value);
  assert.ok(text.includes('应用前须'));assert.equal(text.includes('对白保留潜台词'),false);
});
