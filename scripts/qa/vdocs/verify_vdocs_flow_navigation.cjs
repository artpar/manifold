/* Optional pinned list/break/section points, selection-request and save QA. */
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),vm=require('node:vm');
const {createRequire,stripTypeScriptTypes}=require('node:module'),root=path.resolve(__dirname,'../../..'),qa=path.join(root,'output/colleaf-qa'),req=createRequire(path.join(qa,'package.json'));
const c=req('@univerjs/core'),e=req('@univerjs/engine-render'),docs=req('@univerjs/docs'),drawing=req('@univerjs/drawing'),dd=req('@univerjs/docs-drawing'),hyper=req('@univerjs/docs-hyper-link'),{FUniver}=req('@univerjs/core/facade');req('@univerjs/docs/facade');
for(const n of ['core','engine-render','docs','drawing','docs-drawing','docs-hyper-link'])assert.equal(JSON.parse(fs.readFileSync(path.join(qa,'node_modules/@univerjs',n,'package.json'))).version,'1.0.4');
function pinned(file,hash){const source=fs.readFileSync(file,'utf8');assert.equal(crypto.createHash('sha256').update(source).digest('hex'),hash);return source}
const ui=pinned(path.join(root,'output/codec/docs-hyper-link-ui-1.0.4/package/lib/cjs/index.js'),'a5055ae57c52a0f593e2e10e10b3ae39612f9df3bcf84ae4c9f8fc77ba7ac6fb'),begin=ui.indexOf('const ClickDocHyperLinkOperation ='),end=ui.indexOf('//#endregion',begin);assert.ok(begin>0&&end>begin);
function save(data,fn){const u=new c.Univer();try{for(const p of [docs.UniverDocsPlugin,drawing.UniverDrawingPlugin,dd.UniverDocsDrawingPlugin,hyper.UniverDocsHyperLinkPlugin])u.registerPlugin(p);const f=FUniver.newAPI(u).createDocument(structuredClone(data));return fn(u,f)}finally{u.dispose()}}
const offsets={'section-1':0,list:8,'list-alias':8,item:8,'nested-list':13,'list-break':18,'section-2':63,'break-2':63,'empty-section':73};
function inspect(u,f,edited=false){
 const data=f.save(),points=data.body.customRanges.filter(a=>a.rangeType===3);assert.equal(points.length,9);
 const starts=new Map();let start=0;for(const s of data.body.sectionBreaks){starts.set(s.sectionId,start);start=s.startIndex+1}
 for(const b of points){const id=b.properties.bookmarkId,offset=offsets[id]+(edited?1:0);assert.equal(b.startIndex,offset,id);assert.equal(b.endIndex,offset);const hint=b.properties.manydocNavigation;if(hint?.role==='list'){const p=data.body.paragraphs.find(p=>p.paragraphId===hint.ownerId);assert.ok(p?.bullet);const previous=data.body.dataStream.lastIndexOf('\r',p.startIndex-1);assert.equal(previous+1,offset,id)}if(hint?.role==='section'&&(!edited||id!=='section-1'))assert.equal(starts.get(hint.ownerId),offset,id);if(id.includes('break'))assert.equal(data.body.dataStream[offset],'\f')}
 for(const [name,source]of [['header-list',Object.values(data.headers)[0]],['note-list',Object.values(data.notes)[0]]]){const b=source.body.customRanges.find(a=>a.rangeType===3&&a.properties.bookmarkId===name);assert.equal(b.startIndex,0);assert.equal(b.endIndex,0);assert.equal(b.properties.manydocNavigation.role,'list');assert.ok(source.body.paragraphs.find(p=>p.paragraphId===b.properties.manydocNavigation.ownerId)?.bullet)}
 const injector=u.__getInjector(),selection=injector.get(docs.DocSelectionManagerService),events=[],sub=selection.refreshSelection$.subscribe(event=>{if(event)events.push(event)}),sandbox={_univerjs_core:c,_univerjs_docs:docs,_univerjs_engine_render:e,_univerjs_docs_ui:{},window:{open:()=>assert.fail('unexpected external open')}};
 vm.runInNewContext(ui.slice(begin,end)+';globalThis.operation=ClickDocHyperLinkOperation;',sandbox);
 const accessor={get:token=>token===e.IRenderManagerService?{getRenderUnitById:()=>null}:injector.get(token)};
 try{for(const link of data.body.customRanges.filter(a=>a.rangeType===0)){assert.equal(sandbox.operation.handler(accessor,{unitId:data.id,linkId:link.rangeId}),true);const event=events.at(-1),range=event.docRanges[0];assert.equal(event.unitId,data.id);assert.equal(event.subUnitId,data.id);assert.equal(range.startOffset,offsets[link.properties.bookmarkId]+(edited?1:0));assert.equal(range.endOffset,range.startOffset);assert.equal(range.collapsed,true)}assert.equal(events.length,9)}finally{sub.unsubscribe()}
 const model=injector.get(c.IUniverInstanceService).getUnit(data.id,c.UniverInstanceType.UNIVER_DOC),view=new e.DocumentViewModel(model),skeleton=new e.DocumentSkeleton(view,new c.LocaleService());try{skeleton.calculate();assert.ok(skeleton.getSkeletonData().pages.length>=3)}finally{skeleton.dispose();view.dispose()}
 return data;
}
async function main(){
 const source=pinned(path.join(root,'output/codec/colleaf-format-0.txt'),'c54b27b8c303713f5d5d5dfdb7cd54ec9b8410f5045613150861855b11c899c0'),app=await import('data:text/javascript;base64,'+Buffer.from(stripTypeScriptTypes(source)).toString('base64'));
 const valid=document=>app.validateDocument({type:'vibedada-document',version:3,engine:'univer',engineVersion:'1.0.4',document}),data=JSON.parse(fs.readFileSync(path.join(qa,'flow-navigation.json')));valid(data);
 // A foreign point at a real page-control cursor needs no placement hint.
 delete data.body.customRanges.find(a=>a.properties?.bookmarkId==='break-2').properties.manydocNavigation;
 const first=save(data,(u,f)=>inspect(u,f)),saved=save(first,(u,f)=>inspect(u,f));valid(saved);fs.writeFileSync(path.join(qa,'flow-navigation-saved.json'),JSON.stringify(saved));
 const edited=structuredClone(saved);edited.body=c.TextX.apply(edited.body,new c.TextX().insert(1,{dataStream:'Q'}).serialize());const final=save(edited,(u,f)=>inspect(u,f,true));valid(final);fs.writeFileSync(path.join(qa,'flow-navigation-edited.json'),JSON.stringify(final));
 console.log('Pinned11 main/header/note list, page-break and section points,9 exact internal selection requests, current paragraph/section/control bindings, save/reopen and TextX moves pass; browser caret/scroll/paint remain unqualified');
}
main().catch(error=>{console.error(error);process.exitCode=1});
