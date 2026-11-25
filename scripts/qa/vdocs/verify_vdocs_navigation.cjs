/* Optional pinned bookmark/heading handler, selection-request and save QA. */
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),vm=require('node:vm');
const {createRequire,stripTypeScriptTypes}=require('node:module'),root=path.resolve(__dirname,'../../..'),qa=path.join(root,'output/colleaf-qa'),req=createRequire(path.join(qa,'package.json'));
const c=req('@univerjs/core'),e=req('@univerjs/engine-render'),docs=req('@univerjs/docs'),drawing=req('@univerjs/drawing'),dd=req('@univerjs/docs-drawing'),hyper=req('@univerjs/docs-hyper-link'),{FUniver}=req('@univerjs/core/facade');req('@univerjs/docs/facade');
for(const n of ['core','engine-render','docs','drawing','docs-drawing','docs-hyper-link'])assert.equal(JSON.parse(fs.readFileSync(path.join(qa,'node_modules/@univerjs',n,'package.json'))).version,'1.0.4');
function pinned(file,hash){const source=fs.readFileSync(file,'utf8');assert.equal(crypto.createHash('sha256').update(source).digest('hex'),hash);return source}
const ui=pinned(path.join(root,'output/codec/docs-hyper-link-ui-1.0.4/package/lib/cjs/index.js'),'a5055ae57c52a0f593e2e10e10b3ae39612f9df3bcf84ae4c9f8fc77ba7ac6fb'),begin=ui.indexOf('const ClickDocHyperLinkOperation ='),end=ui.indexOf('//#endregion',begin);assert.ok(begin>0&&end>begin);
function save(data,fn){const u=new c.Univer();try{for(const p of [docs.UniverDocsPlugin,drawing.UniverDrawingPlugin,dd.UniverDocsDrawingPlugin,hyper.UniverDocsHyperLinkPlugin])u.registerPlugin(p);const f=FUniver.newAPI(u).createDocument(structuredClone(data));return fn(u,f)}finally{u.dispose()}}
const offsets={paragraph:0,before:0,middle:3,end:7,heading:8,empty:16,list:17,cell:28,'cell-run':28};
function inspect(u,f,edited=false){
 const data=f.save(),bookmarks=data.body.customRanges.filter(r=>r.rangeType===3),names=new Set(bookmarks.map(r=>r.properties.bookmarkId));assert.equal(names.size,9);
 for(const [id,offset]of Object.entries(offsets)){const b=bookmarks.find(r=>r.properties.bookmarkId===id);assert.equal(b.startIndex,offset+(edited?1:0),id);assert.equal(b.startIndex,b.endIndex);assert.ok(!names.has(b.rangeId),'transport ID must not alias a logical target')}
 const opened=[],sandbox={_univerjs_core:c,_univerjs_docs:docs,_univerjs_engine_render:e,_univerjs_docs_ui:{},window:{open:(...args)=>opened.push(args)}};
 vm.runInNewContext(ui.slice(begin,end)+';globalThis.operation=ClickDocHyperLinkOperation;',sandbox);
 const injector=u.__getInjector(),selection=injector.get(docs.DocSelectionManagerService),events=[],sub=selection.refreshSelection$.subscribe(event=>{if(event)events.push(event)});
 const accessor={get:token=>token===e.IRenderManagerService?{getRenderUnitById:()=>null}:injector.get(token)};
 try{
  const stories=[[undefined,data.body],...Object.entries(data.headers).map(([id,s])=>[id,s.body]),...Object.entries(data.footers).map(([id,s])=>[id,s.body]),...Object.entries(data.notes).map(([id,s])=>[id,s.body])];let jumps=0;
  for(const [segmentId,body]of stories)for(const r of body.customRanges||[])if(r.rangeType===0){const id=r.properties.bookmarkId??(r.properties.headingId?"heading":undefined),count=events.length,ok=sandbox.operation.handler(accessor,{unitId:data.id,linkId:r.rangeId,segmentId});if(id==='missing'){assert.equal(ok,false);assert.equal(events.length,count);continue}assert.equal(ok,true);jumps++;const event=events.at(-1);assert.equal(event.unitId,data.id);assert.equal(event.subUnitId,data.id);assert.equal(event.isEditing,false);assert.equal(event.options.preserveCaret,true);assert.equal(event.docRanges.length,1);const range=event.docRanges[0];assert.equal(range.startOffset,offsets[id]+(edited?1:0));assert.equal(range.endOffset,range.startOffset);assert.equal(range.collapsed,true)}
  assert.equal(jumps,12);assert.equal(opened.length,0,'internal points must not request window.open');
 }finally{sub.unsubscribe()}
 const model=injector.get(c.IUniverInstanceService).getUnit(data.id,c.UniverInstanceType.UNIVER_DOC),view=new e.DocumentViewModel(model),skeleton=new e.DocumentSkeleton(view,new c.LocaleService());
 try{skeleton.calculate();const glyphs=[];for(const p of skeleton.getSkeletonData().pages)for(const s of p.sections)for(const col of s.columns)for(const l of col.lines)for(const d of l.divides)glyphs.push(...d.glyphGroup);assert.ok(glyphs.some(g=>g.content==='😀'));const emoji=glyphs.find(g=>g.content==='😀');assert.equal(emoji.ts.bl,1);assert.equal(emoji.ts.fs,13);assert.equal(emoji.ts.cl.rgb,'#123456');assert.notEqual(emoji.ts.ul?.s,1)}finally{skeleton.dispose();view.dispose()}
 return data;
}
async function main(){
 const source=pinned(path.join(root,'output/codec/colleaf-format-0.txt'),'c54b27b8c303713f5d5d5dfdb7cd54ec9b8410f5045613150861855b11c899c0'),app=await import('data:text/javascript;base64,'+Buffer.from(stripTypeScriptTypes(source)).toString('base64'));
 const valid=document=>app.validateDocument({type:'vibedada-document',version:3,engine:'univer',engineVersion:'1.0.4',document}),data=JSON.parse(fs.readFileSync(path.join(qa,'navigation.json')));valid(data);
 const first=save(data,(u,f)=>inspect(u,f));valid(first);const saved=save(first,(u,f)=>inspect(u,f));valid(saved);fs.writeFileSync(path.join(qa,'navigation-saved.json'),JSON.stringify(saved));
 const foreign=structuredClone(saved),hp=foreign.body.paragraphs.find(p=>p.paragraphStyle?.outlineLevel===0);hp.paragraphStyle.headingId='foreign-heading';
 for(const body of [foreign.body,...Object.values(foreign.footers).map(s=>s.body)])for(const r of body.customRanges||[])if(r.rangeType===0&&r.properties.bookmarkId==='heading'){delete r.properties.bookmarkId;r.properties.headingId='foreign-heading';r.properties.url='#foreign-heading'}
 // Resource targets must agree with the intended current heading target too.
 const current=new Map();for(const body of [foreign.body,...Object.values(foreign.headers).map(s=>s.body),...Object.values(foreign.footers).map(s=>s.body)])for(const r of body.customRanges||[])if(r.rangeType===0)current.set(r.rangeId,r.properties.url||'');
 for(const r of foreign.resources||[])if(r.name==='DOC_HYPER_LINK_PLUGIN'){const data=JSON.parse(r.data);for(const x of data.links)x.payload=current.get(x.id)||'';r.data=JSON.stringify(data)}
 const foreignSaved=save(foreign,(u,f)=>inspect(u,f));valid(foreignSaved);fs.writeFileSync(path.join(qa,'navigation-heading.json'),JSON.stringify(foreignSaved));
 const edited=structuredClone(saved);edited.body=c.TextX.apply(edited.body,new c.TextX().insert(1,{dataStream:'Q'}).serialize());
 const final=save(edited,(u,f)=>inspect(u,f,true));valid(final);fs.writeFileSync(path.join(qa,'navigation-edited.json'),JSON.stringify(final));
 console.log('Pinned nine main bookmark points,12 internal selection-refresh requests (including header/footer/note links), foreign headingId selection, missing-target false outcome, emoji/style/save/reopen and TextX cursor edits pass; renderer/scroll/browser interactions remain unqualified');
}
main().catch(error=>{console.error(error);process.exitCode=1});
