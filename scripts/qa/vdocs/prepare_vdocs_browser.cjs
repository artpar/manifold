/* Optional independent browser QA. Production remains Go-only.
 * Installs pinned development packages in ignored output/colleaf-browser, checks
 * original Colleaf source hashes and bundles its unmodified mount/validator.
 * Usage: node scripts/qa/vdocs/prepare_vdocs_browser.cjs [public-example.vdocs]
 */
const fs=require('node:fs');
const path=require('node:path');
const crypto=require('node:crypto');
const {createRequire}=require('node:module');
const {spawnSync}=require('node:child_process');
const root=path.resolve(__dirname,'../../..');
const qa=path.join(root,'output/colleaf-browser');
const commit='5ce2ef507b28b92f5b03443ade87bc0e17f3d8b6';
const dependencies={
 '@univerjs/presets':'1.0.4', '@univerjs/preset-docs-core':'1.0.4',
 '@univerjs/preset-docs-drawing':'1.0.4', '@univerjs/preset-docs-hyper-link':'1.0.4',
 '@univerjs/docs-find-replace':'1.0.4', '@univerjs/find-replace':'1.0.4',
 '@univerjs/icons':'1.43.1', rxjs:'7.8.2', react:'19.0.0', 'react-dom':'19.0.0', esbuild:'0.27.0',
};
const mainSource=`import {IRenderManagerService,DocSkeletonManagerService} from '@univerjs/preset-docs-core';
import {mountDocument} from './integrations/univer/document';
const qa:any = {changes:0, errors:[]};
(window as any).qa=qa;
window.addEventListener('error',e=>qa.errors.push(e.message));
window.addEventListener('unhandledrejection',e=>qa.errors.push(String(e.reason)));
qa.mount=async (content:any,mobile=false)=>{
 qa.runtime?.dispose();
 document.querySelector('#editor')!.innerHTML='';
 qa.runtime=mountDocument(document.querySelector('#editor') as HTMLElement,content,true,mobile,()=>qa.changes++);
 await qa.runtime.ready;
 await document.fonts.ready;
 await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));
 qa.snapshot=()=>qa.runtime.snapshot();
 return {stage:qa.runtime.univerAPI.getCurrentLifecycleStage(),canvases:document.querySelectorAll('canvas').length};
};
qa.renderedNotes=()=>{
 const doc=qa.runtime.univerAPI.getActiveDocument();
 const render=qa.runtime.univer.__getInjector().get(IRenderManagerService).getRenderUnitById(doc.getId());
 const skeleton=render.with(DocSkeletonManagerService).getSkeleton();
 return skeleton.getSkeletonData().pages.map((p:any,index:number)=>({page:index+1,notes:(p.notes||[]).map((n:any)=>({id:n.noteId,type:qa.snapshot().document.notes[n.noteId]?.type,referenceIndex:n.referenceIndex,top:n.top}))}));
};
// Raw native geometry for scoped QA probes; it is never part of application data.
qa.skeletonData=()=>{
 const doc=qa.runtime.univerAPI.getActiveDocument();
 const render=qa.runtime.univer.__getInjector().get(IRenderManagerService).getRenderUnitById(doc.getId());
 return render.with(DocSkeletonManagerService).getSkeleton().getSkeletonData();
};
qa.result=await qa.mount(await (await fetch('/document.vdocs')).json());
qa.ready=true;
document.querySelector('#status')!.textContent='Ready';
`;
const html='<!doctype html><html><head><meta charset="UTF-8"><link rel="stylesheet" href="/dist/main.css"><style>html,body{margin:0;height:100%;font-family:Arial}#status{position:fixed;z-index:10000;right:8px;top:8px;background:white;padding:3px}#editor{height:100vh;width:100vw}</style></head><body><span id="status">Loading</span><div id="editor"></div><script type="module" src="/dist/main.js"></script></body></html>\n';
async function main(){
 const input=path.resolve(process.argv[2]||path.join(root,'output/colleaf-tagged/document.vdocs'));
 const raw=fs.readFileSync(input);const envelope=JSON.parse(raw);
 if(envelope.type!=='vibedada-document'||envelope.version!==3)throw Error('Generate the public VDOCS example first; this harness requires that envelope.');
 fs.mkdirSync(qa,{recursive:true});
 for(const [cache,remote,target,hash]of [
  ['colleaf-1.txt','src/integrations/univer/document.ts','src/integrations/univer/document.ts','11d3fe8caadd351a3871b82ceecba3285cd77e02f777f7f7b1fecc57252b499a'],
  ['colleaf-format-0.txt','src/shared/files/docsFormat.ts','src/shared/files/docsFormat.ts','c54b27b8c303713f5d5d5dfdb7cd54ec9b8410f5045613150861855b11c899c0'],
 ]){
  const cached=path.join(root,'output/codec',cache);let bytes;
  if(fs.existsSync(cached))bytes=fs.readFileSync(cached);
  else{const response=await fetch(`https://raw.githubusercontent.com/artpar/colleaf/${commit}/${remote}`);if(!response.ok)throw Error(`Source download failed: ${response.status}`);bytes=Buffer.from(await response.arrayBuffer());}
  if(crypto.createHash('sha256').update(bytes).digest('hex')!==hash)throw Error(`Source hash mismatch: ${remote}`);
  const destination=path.join(qa,target);fs.mkdirSync(path.dirname(destination),{recursive:true});fs.writeFileSync(destination,bytes);
 }
 fs.writeFileSync(path.join(qa,'package.json'),JSON.stringify({private:true,type:'module',dependencies},null,2)+'\n');
 function matches(){return Object.entries(dependencies).every(([name,version])=>{try{return JSON.parse(fs.readFileSync(path.join(qa,'node_modules',name,'package.json'))).version===version}catch{return false}})}
 if(!matches()){
  const result=spawnSync('npm',['install','--ignore-scripts','--no-audit','--no-fund'],{cwd:qa,stdio:'inherit'});
  if(result.error)throw result.error;if(result.status!==0)throw Error(`npm install failed: ${result.status}`);
 }
 if(!matches())throw Error('Pinned QA dependency versions do not match');
 fs.writeFileSync(path.join(qa,'src/main.ts'),mainSource);
 fs.writeFileSync(path.join(qa,'index.html'),html);
 fs.writeFileSync(path.join(qa,'document.vdocs'),raw);
 const req=createRequire(path.join(qa,'package.json'));
 req('esbuild').buildSync({entryPoints:[path.join(qa,'src/main.ts')],bundle:true,format:'esm',target:'es2022',define:{'process.env.NODE_ENV':'"production"'},loader:{'.svg':'dataurl','.png':'dataurl','.woff':'file','.woff2':'file','.ttf':'file'},outdir:path.join(qa,'dist')});
 console.log('Pinned actual Colleaf browser harness built; no application or production files modified.');
 console.log('Serve: python3 -m http.server 8768 --bind 127.0.0.1 --directory output/colleaf-browser');
 console.log('Open http://127.0.0.1:8768 and inspect window.qa.ready/result/errors/snapshot().');
}
main().catch(error=>{console.error(error);process.exitCode=1});
