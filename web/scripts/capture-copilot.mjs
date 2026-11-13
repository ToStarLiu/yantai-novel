import { chromium } from 'playwright';
import fs from 'node:fs/promises';
const origin=process.env.E2E_BASE_URL||'http://127.0.0.1:8089';
const browser=await chromium.launch({channel:process.env.PLAYWRIGHT_CHANNEL||'chrome'});
try{
  const page=await browser.newPage({viewport:{width:1440,height:1000}});
  await page.request.post(origin+'/api/session',{headers:{Origin:origin},data:{username:process.env.E2E_USERNAME||'e2e-author',password:process.env.E2E_PASSWORD||'test-only-novel-2026'}});
  const projects=await(await page.request.get(origin+'/api/projects')).json();
  let project;
  for(const value of projects.filter(p=>p.title.startsWith('Copilot 对话验收 ·'))){
    const sessions=await(await page.request.get(origin+`/api/projects/${value.id}/copilot/sessions`)).json();
    if(!sessions.length)continue;
    const detail=await(await page.request.get(origin+`/api/copilot/sessions/${sessions[0].id}`)).json();
    if(detail.turns.length>=2&&detail.turns.some(t=>t.input.text.includes('改写'))){project=value;break;}
  }
  if(!project)throw new Error('No existing Copilot browser fixture');
  await page.goto(origin);
  await page.evaluate(id=>localStorage.setItem('novel-project',id),project.id);await page.reload();
  await page.getByRole('button',{name:'灵感助手',exact:true}).click();
  await page.locator('.copilot-tool').first().waitFor();
  await page.locator('.copilot-log').evaluate(el=>{el.scrollTop=0;});
  await page.screenshot({path:'../docs/release/screenshots/copilot-tools.png',fullPage:true});
  const choices=page.locator('.copilot-artifact[data-artifact-kind="revision-candidate"]');
  for(let i=0;i<await choices.count();i++){const compare=choices.nth(i).getByRole('button',{name:'查看 Diff',exact:true});if(await compare.isEnabled().catch(()=>false)){await compare.click();await choices.nth(i).scrollIntoViewIfNeeded();await page.screenshot({path:'../docs/release/screenshots/copilot-diff.png',fullPage:true});break;}}
  await page.setViewportSize({width:390,height:844});
  await page.locator('.copilot-log').evaluate(el=>{el.scrollTop=0;});
  await page.screenshot({path:'../docs/release/screenshots/copilot-mobile-conversation.png',fullPage:true});
  console.log('Captured existing conversation without creating messages or changing manuscripts.');
}finally{await browser.close();}
