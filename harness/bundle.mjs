import { build } from 'esbuild';
import fs from 'node:fs/promises';
import path from 'node:path';
const result=await build({entryPoints:['src/main.ts'],bundle:true,platform:'node',target:'node24',format:'esm',outfile:'bundle/main.mjs',sourcemap:false,minify:false,metafile:true,banner:{js:"import { createRequire as __createRequire } from 'node:module'; const require = __createRequire(import.meta.url);"}});
const packages=new Set();
for(const input of Object.keys(result.metafile.inputs)) { const match=/node_modules\/((?:@[^/]+\/)?[^/]+)/.exec(input.replaceAll('\\','/'));if(match)packages.add(match[1]); }
await fs.mkdir('bundle/licenses',{recursive:true});
const notices=[];
for(const name of [...packages].sort()){
 const dir=path.join('node_modules',name),pkg=JSON.parse(await fs.readFile(path.join(dir,'package.json'),'utf8'));
 notices.push(`${name}@${pkg.version} — ${pkg.license||'See license file'}`);
 const dest=path.join('bundle/licenses',name.replaceAll('/','__'));await fs.mkdir(dest,{recursive:true});
 for(const file of await fs.readdir(dir))if(/^(license|licence|copying|notice|copyright)(\.|$)/i.test(file))await fs.copyFile(path.join(dir,file),path.join(dest,file));
 if(name.startsWith('@earendil-works/'))await fs.copyFile('PI-LICENSE.txt',path.join(dest,'LICENSE.txt'));
}
await fs.writeFile('bundle/THIRD-PARTY-NOTICES.txt','Runtime bundle third-party components\n\n'+notices.join('\n')+'\n');
