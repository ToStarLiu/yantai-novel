const fs=require('node:fs/promises'),path=require('node:path');
(async()=>{
 const {packager}=await import('@electron/packager');
 await import('../scripts/collect-runtime-licenses.mjs');
 const root=path.resolve(__dirname,'..'),backend=path.join(__dirname,'resources','backend');
 const expected=path.resolve(__dirname,'resources','backend');
 if(path.resolve(backend)!==expected||!expected.startsWith(path.resolve(__dirname)+path.sep))throw new Error('Invalid generated output directory');
 await fs.rm(expected,{recursive:true,force:true});
 await fs.mkdir(backend,{recursive:true});
 await fs.copyFile(path.join(root,'.tools','api-advanced-next.exe'),path.join(backend,'api.exe'));
 for(const item of ['LICENSE','NOTICE.md'])await fs.copyFile(path.join(root,item),path.join(backend,item));
 for(const item of ['web/dist','assets/fonts','assets/licenses'])await fs.cp(path.join(root,item),path.join(backend,item),{recursive:true,force:true});
 await fs.cp(path.join(root,'harness','bundle'),path.join(backend,'harness'),{recursive:true,force:true});
 await fs.mkdir(path.join(backend,'node'),{recursive:true});
 for(const item of ['node.exe','LICENSE'])await fs.copyFile(path.join(root,'.tools','node-v24.22.3-win-x64',item),path.join(backend,'node',item));
 for(const item of ['bin','lib','share'])await fs.cp(path.join(root,'.tools','pgsql',item),path.join(backend,'pgsql',item),{recursive:true,force:true});
 for(const item of ['server_license.txt','commandlinetools_3rd_party_licenses.txt'])await fs.copyFile(path.join(root,'.tools','pgsql',item),path.join(backend,'pgsql',item));
 const cache=path.join(root,'.local','electron-zip');await fs.mkdir(cache,{recursive:true});
 async function findZip(dir){for(const item of await fs.readdir(dir,{withFileTypes:true})){const p=path.join(dir,item.name);if(item.isDirectory()){const found=await findZip(p);if(found)return found;}else if(item.name==='electron-v45.3.1-win32-x64.zip')return p;}}
 const zip=await findZip(path.join(process.env.LOCALAPPDATA,'electron','Cache'));
 if(zip)await fs.copyFile(zip,path.join(cache,'electron-v45.3.1-win32-x64.zip'));
 const outputs=await packager({dir:__dirname,name:'Yantai',platform:'win32',arch:'x64',out:path.join(__dirname,'dist'),overwrite:true,prune:true,asar:true,ignore:[/^\/dist($|\/)/,/^\/resources($|\/)/,/^\/package\.cjs$/],extraResource:[backend],electronZipDir:zip?cache:undefined});
 console.log(outputs.join('\n'));
})().catch(error=>{console.error(error);process.exitCode=1;});
