import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync } from 'node:child_process';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const output=path.join(root,'assets','licenses');
await fs.mkdir(output,{recursive:true});
const lines=['Runtime third-party licenses. Generated from pinned Go modules and production npm lock entries.',''];
const missing=[];
async function copyNotices(source,destination,label,license){
  await fs.mkdir(destination,{recursive:true});
  let found=0;
  for(const file of await fs.readdir(source))if(/^(license|licence|copying|notice|copyright|patents)([._-]|$)/i.test(file)){
    const sourceFile=path.join(source,file);if(!(await fs.stat(sourceFile)).isFile())continue;
    const targetFile=path.join(destination,file);
    try{await fs.chmod(targetFile,0o644);}catch(error){if(error.code!=='ENOENT')throw error;}
    await fs.copyFile(sourceFile,targetFile);await fs.chmod(targetFile,0o644);found++;
  }
  if(!found)missing.push(label);
  lines.push(label+' — '+license);
}
const portable=path.join(root,'.tools','go','bin','go.exe');
const go=await fs.stat(portable).then(()=>portable,()=> 'go');
const modules=[...new Set(execFileSync(go,['list','-deps','-f','{{with .Module}}{{.Path}}|{{.Version}}|{{.Dir}}{{end}}','./cmd/api','./cmd/worker'],{cwd:root,encoding:'utf8',windowsHide:true}).trim().split(/\r?\n/))];
for(const line of modules){const[name,version,dir]=line.split('|');if(!version||!dir)continue;await copyNotices(dir,path.join(output,'go',name.replaceAll('/','__')),name+'@'+version,'see license file');}
const goRoot=execFileSync(go,['env','GOROOT'],{cwd:root,encoding:'utf8',windowsHide:true}).trim();
await copyNotices(goRoot,path.join(output,'go','go-runtime'),'Go runtime','BSD-3-Clause and third-party notices');
const lock=JSON.parse(await fs.readFile(path.join(root,'web','package-lock.json'),'utf8'));
for(const[relative,item]of Object.entries(lock.packages)){
  if(!relative||item.dev||!relative.includes('node_modules/'))continue;
  const dir=path.join(root,'web',relative);
  let pkg;try{pkg=JSON.parse(await fs.readFile(path.join(dir,'package.json'),'utf8'));}catch{if(item.optional)continue;throw new Error('Missing runtime package '+relative);}
  await copyNotices(dir,path.join(output,'web',relative.replaceAll('/','__')),pkg.name+'@'+pkg.version,pkg.license||'see license file');
}
if(missing.length)throw new Error('Missing runtime license source: '+missing.join(', '));
await fs.writeFile(path.join(output,'THIRD-PARTY-NOTICES.txt'),lines.join('\n')+'\n');
console.log('Runtime notices retained for '+(lines.length-2)+' Go/Web components.');
