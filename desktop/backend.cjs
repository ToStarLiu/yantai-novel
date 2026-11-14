const {spawn,execFileSync}=require('node:child_process');
const fs=require('node:fs');
const path=require('node:path');
const crypto=require('node:crypto');
const net=require('node:net');

const pause=ms=>new Promise(resolve=>setTimeout(resolve,ms));
const port=()=>new Promise((resolve,reject)=>{const server=net.createServer();server.once('error',reject);server.listen(0,'127.0.0.1',()=>{const value=server.address().port;server.close(()=>resolve(value));});});
const command=(exe,args,options={})=>execFileSync(exe,args,{windowsHide:true,stdio:'pipe',timeout:60000,...options});

async function startBackend(app){
  if(process.env.NOVEL_DESKTOP_URL)return{url:process.env.NOVEL_DESKTOP_URL,stop:async()=>{}};
  const root=app.isPackaged?path.join(process.resourcesPath,'backend'):path.join(__dirname,'resources','backend');
  if(!fs.existsSync(path.join(root,'api.exe'))){if(app.isPackaged)throw new Error('桌面包缺少本地服务，请重新下载并完整解压便携包。');return{url:'http://127.0.0.1:8088',stop:async()=>{}};}
  const data=process.env.NOVEL_DESKTOP_DATA||path.join(app.getPath('userData'),'workbench');
  fs.mkdirSync(data,{recursive:true});
  const configPath=path.join(data,'config.json');
  let config;
  if(fs.existsSync(configPath))config=JSON.parse(fs.readFileSync(configPath,'utf8'));
  else {config={apiPort:await port(),pgPort:await port(),username:'author',password:crypto.randomBytes(24).toString('base64url')};fs.writeFileSync(configPath,JSON.stringify(config,null,2),{mode:0o600});}
  const database=path.join(data,'pgdata'),pg=path.join(root,'pgsql','bin'),log=path.join(data,'postgres.log');
  const pgExe=name=>path.join(pg,name+'.exe');
  if(!fs.existsSync(path.join(database,'PG_VERSION')))command(pgExe('initdb'),['-D',database,'-U','novel','-A','trust','-E','UTF8','--locale=C']);
  if(fs.readFileSync(path.join(database,'PG_VERSION'),'utf8').trim()!=='18')throw new Error('数据目录的数据库版本不兼容，原数据已保留。');
  try{command(pgExe('pg_ctl'),['-D',database,'status']);}catch{command(pgExe('pg_ctl'),['-D',database,'-l',log,'-w','-t','30','start','-o',`-h 127.0.0.1 -p ${config.pgPort}`],{stdio:'ignore'});}
  try {
  const connection=['-X','-h','127.0.0.1','-p',String(config.pgPort),'-U','novel'];
  const exists=command(pgExe('psql'),[...connection,'-d','postgres','-tAc',"SELECT 1 FROM pg_database WHERE datname='novel'",'-v','ON_ERROR_STOP=1']).toString().trim();
  if(exists!=='1')command(pgExe('createdb'),['-h','127.0.0.1','-p',String(config.pgPort),'-U','novel','-E','UTF8','novel']);
  const url=`http://127.0.0.1:${config.apiPort}`;
  const modelConfig=path.join(data,'model.env');
  const extra={};
  if(fs.existsSync(modelConfig))for(const line of fs.readFileSync(modelConfig,'utf8').split(/\r?\n/)){const match=/^([A-Z][A-Z0-9_]*)=(.*)$/.exec(line.trim());if(match&&match[2].trim()&&/^(LLM_|MEDIA_|IMAGE_MODEL$|TTS_MODEL$|MCP_SERVER_TOKEN$|RESEARCH_)/.test(match[1]))extra[match[1]]=match[2].trim();}
  const env={...process.env,...extra,DATABASE_URL:`postgres://novel@127.0.0.1:${config.pgPort}/novel?sslmode=disable`,AUTHOR_USERNAME:config.username,AUTHOR_PASSWORD:config.password,APP_ADDR:`127.0.0.1:${config.apiPort}`,PUBLIC_ORIGIN:url,WEB_ROOT:path.join(root,'web','dist'),PDF_FONT_PATH:path.join(root,'assets','fonts','NotoSansSC-Regular.ttf'),HARNESS_NODE_PATH:path.join(root,'node','node.exe'),HARNESS_ENTRY_PATH:path.join(root,'harness','main.mjs'),RUN_WORKER:'true',APP_PROVIDER:extra.LLM_BASE_URL||process.env.LLM_BASE_URL?'configured':'fake'};
  const api=path.join(root,'api.exe');
  env.PROVIDER_MASTER_KEY_PATH=path.join(data,'provider-master.key');
  command(api,['migrate','up'],{env,cwd:root});
  fs.writeFileSync(path.join(data,'login.txt'),`地址：${url}\n用户名：${config.username}\n密码：${config.password}\n\n模型配置：优先在页面“更多 → AI 供应商”设置；model.env为兼容入口，修改文件后重启。\n`,{mode:0o600});
  if(!fs.existsSync(modelConfig))fs.writeFileSync(modelConfig,'# 配置兼容接口后重启；未配置时明确使用本地演示模型。\nLLM_BASE_URL=\nLLM_MODEL=\nLLM_API_KEY=\nLLM_WIRE_API=responses\nLLM_ACTOR_AUTHORIZATION=\n');
  const output=fs.openSync(path.join(data,'api.log'),'a');
  const child=spawn(api,[],{env,cwd:root,windowsHide:true,stdio:['ignore',output,output]});
  let launchError;child.on('error',error=>{launchError=error;});
  for(let i=0;i<100;i++){if(launchError)throw launchError;if(child.exitCode!==null)throw new Error('本地服务启动失败，请查看数据目录 api.log。');try{if((await fetch(url+'/healthz')).ok)return{url,credentials:config,data,stop:async()=>{child.kill();await pause(400);try{command(pgExe('pg_ctl'),['-D',database,'-m','fast','-w','stop'],{stdio:'ignore'});}catch{}}};}catch{}await pause(100);}
  child.kill();throw new Error('本地服务启动超时，数据已保留。');
  } catch(error) { try{command(pgExe('pg_ctl'),['-D',database,'-m','fast','-w','stop'],{stdio:'ignore'});}catch{} throw error; }
}
module.exports={startBackend};
