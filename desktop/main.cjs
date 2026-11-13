const {app,BrowserWindow,shell,Menu,dialog}=require('electron');
const path=require('node:path');
const fs=require('node:fs');
const {startBackend}=require('./backend.cjs');
let backend,closing=false;
async function create(){
 backend=await startBackend(app);
 const base=backend.url,target=new URL(base);
 if(!['http:','https:'].includes(target.protocol))throw new Error('工作台地址必须为HTTP(S)');
 const win=new BrowserWindow({width:1500,height:980,minWidth:900,minHeight:620,title:'砚台 小说创作工作台',backgroundColor:'#f7f8f3',webPreferences:{contextIsolation:true,nodeIntegration:false,sandbox:true,webSecurity:true}});
 // The native package reads its local assets directly. Retire an older PWA
 // shell before loading an upgraded bundle, retaining cookies and author buffers.
 await win.webContents.session.clearStorageData({origin:target.origin,storages:['serviceworkers','cachestorage']});
 win.webContents.setWindowOpenHandler(({url})=>{const destination=new URL(url);if(['https:','http:'].includes(destination.protocol))shell.openExternal(url);return{action:'deny'};});
 win.webContents.on('will-navigate',(event,url)=>{if(new URL(url).origin!==target.origin){event.preventDefault();if(['http:','https:'].includes(new URL(url).protocol))shell.openExternal(url);}});
 win.webContents.on('did-fail-load',(_event,code,_description,url,isMainFrame)=>{if(!isMainFrame||code===-3)return;fs.appendFileSync(path.join(backend.data||app.getPath('userData'),'desktop.log'),JSON.stringify({event:'load-failed',code,url})+'\n');});
 win.webContents.on('before-input-event',(event,input)=>{if(input.control&&input.key.toLowerCase()==='r'){event.preventDefault();win.loadURL(base);}});
 Menu.setApplicationMenu(Menu.buildFromTemplate([{label:'砚台',submenu:[{label:'重新连接',accelerator:'CmdOrCtrl+R',click:()=>win.loadURL(base)},{label:'在浏览器打开',click:()=>shell.openExternal(base)},...(backend.data?[{label:'打开数据与模型配置目录',click:()=>shell.openPath(backend.data)},{label:'查看本机登录信息',click:()=>shell.openPath(path.join(backend.data,'login.txt'))}]:[]),{type:'separator'},{role:'quit'}]},{label:'编辑',submenu:[{role:'undo'},{role:'redo'},{type:'separator'},{role:'cut'},{role:'copy'},{role:'paste'},{role:'selectAll'}]},{label:'视图',submenu:[{role:'resetZoom'},{role:'zoomIn'},{role:'zoomOut'},{role:'togglefullscreen'}]}]));
 if(backend.credentials){
   const response=await win.webContents.session.fetch(base+'/api/session',{method:'POST',headers:{'Content-Type':'application/json','Origin':base},body:JSON.stringify({username:backend.credentials.username,password:backend.credentials.password})});
   if(!response.ok)throw new Error('本机账户自动登录失败，请查看数据目录。');
 }
 await win.loadURL(base);
}
if(!app.requestSingleInstanceLock())app.quit();
else app.whenReady().then(create).catch(error=>{try{fs.appendFileSync(path.join(backend?.data||app.getPath('userData'),'desktop.log'),error.stack+'\n');}catch{}console.error(error.message);if(process.env.NOVEL_DESKTOP_TEST_MODE!=='1')dialog.showErrorBox('砚台启动失败',error.message);app.quit();});
app.on('before-quit',event=>{if(backend&&!closing){event.preventDefault();closing=true;backend.stop().finally(()=>app.quit());}});
app.on('window-all-closed',()=>{if(process.platform!=='darwin')app.quit();});
app.on('activate',()=>{if(!BrowserWindow.getAllWindows().length)create();});
