/* Optional pinned native container bookmark/save/edit QA; no browser claim. */
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),vm=require('node:vm');
const {createRequire}=require('node:module'),root=path.resolve(__dirname,'../../..'),qa=path.join(root,'output/vdocs-qa'),req=createRequire(path.join(qa,'package.json'));
const c=req('@univerjs/core'),e=req('@univerjs/engine-render'),docs=req('@univerjs/docs'),drawing=req('@univerjs/drawing'),dd=req('@univerjs/docs-drawing'),hyper=req('@univerjs/docs-hyper-link'),{FUniver}=req('@univerjs/core/facade');req('@univerjs/docs/facade');
for(const n of ['core','engine-render','docs','drawing','docs-drawing','docs-hyper-link'])assert.equal(JSON.parse(fs.readFileSync(path.join(qa,'node_modules/@univerjs',n,'package.json'))).version,'1.0.4');
function pinned(file,hash){const source=fs.readFileSync(file,'utf8');assert.equal(crypto.createHash('sha256').update(source).digest('hex'),hash);return source}
const ui=pinned(path.join(root,'output/codec/docs-hyper-link-ui-1.0.4/package/lib/cjs/index.js'),'a5055ae57c52a0f593e2e10e10b3ae39612f9df3bcf84ae4c9f8fc77ba7ac6fb'),begin=ui.indexOf('const ClickDocHyperLinkOperation ='),end=ui.indexOf('//#endregion',begin);assert.ok(begin>0&&end>begin);
function save(data,fn){const u=new c.Univer();try{for(const p of [docs.UniverDocsPlugin,drawing.UniverDrawingPlugin,dd.UniverDocsDrawingPlugin,hyper.UniverDocsHyperLinkPlugin])u.registerPlugin(p);const f=FUniver.newAPI(u).createDocument(structuredClone(data));return fn(u,f)}finally{u.dispose()}}
const expected={table:8,'table-alias':8,'cell-picture':16,nested:18,'nested-alias':18,picture:38,'list-table':45,'list-table-alias':45,'list-picture':64,'header-table':0,'header-table-alias':0,'header-picture':10,'footer-picture':0,'note-table':0,'note-table-alias':0,'note-picture':8};
function inspect(u,f,edited=false){
 const data=f.save(),stories=[['main',data.body],...Object.entries(data.headers).map(([id,s])=>[id,s.body]),...Object.entries(data.footers).map(([id,s])=>[id,s.body]),...Object.entries(data.notes).map(([id,s])=>[id,s.body])],seen=new Set();
 for(const [story,body]of stories)for(const b of body.customRanges||[])if(b.rangeType===3){const id=b.properties.bookmarkId,offset=expected[id]+(edited&&story==='main'?1:0);assert.ok(Object.hasOwn(expected,id));assert.ok(!seen.has(id));seen.add(id);assert.equal(b.startIndex,offset,id);assert.equal(b.endIndex,offset);const picture=id.includes('picture'),token=picture?'\b':'\x1a';assert.equal(body.dataStream[offset],token,id);const owner=picture?body.customBlocks.find(a=>a.startIndex===offset)?.blockId:body.tables.find(t=>t.startIndex===offset)?.tableId;assert.ok(owner);if(b.properties.manydocNavigation)assert.equal(b.properties.manydocNavigation.ownerId,owner,id)}
 assert.equal(seen.size,16);
 const injector=u.__getInjector(),events=[],selection=injector.get(docs.DocSelectionManagerService),sub=selection.refreshSelection$.subscribe(event=>{if(event)events.push(event)}),sandbox={_univerjs_core:c,_univerjs_docs:docs,_univerjs_engine_render:e,_univerjs_docs_ui:{},window:{open:()=>assert.fail('internal bookmark opened a window')}};
 vm.runInNewContext(ui.slice(begin,end)+';globalThis.operation=ClickDocHyperLinkOperation;',sandbox);
 const accessor={get:token=>token===e.IRenderManagerService?{getRenderUnitById:()=>null}:injector.get(token)};
 try{for(const link of data.body.customRanges.filter(a=>a.rangeType===0)){assert.equal(sandbox.operation.handler(accessor,{unitId:data.id,linkId:link.rangeId}),true);const event=events.at(-1),range=event.docRanges[0];assert.equal(event.unitId,data.id);assert.equal(event.subUnitId,data.id);assert.equal(event.options.preserveCaret,true);assert.equal(range.startOffset,expected[link.properties.bookmarkId]+(edited?1:0));assert.equal(range.endOffset,range.startOffset);assert.equal(range.collapsed,true)}assert.equal(events.length,7)}finally{sub.unsubscribe()}
 const model=injector.get(c.IUniverInstanceService).getUnit(data.id,c.UniverInstanceType.UNIVER_DOC),view=new e.DocumentViewModel(model),skeleton=new e.DocumentSkeleton(view,new c.LocaleService());try{skeleton.calculate();assert.ok(skeleton.getSkeletonData().pages.length>0)}finally{skeleton.dispose();view.dispose()}
 return data;
}
async function main(){

 const data=JSON.parse(fs.readFileSync(path.join(qa,'container-navigation.json')));
 // Current native structural tokens are sufficient without manydoc placement hints.
 for(const b of data.body.customRanges)if(b.rangeType===3&&['table-alias','picture'].includes(b.properties.bookmarkId))delete b.properties.manydocNavigation;
 const first=save(data,(u,f)=>inspect(u,f)),saved=save(first,(u,f)=>inspect(u,f));fs.writeFileSync(path.join(qa,'container-navigation-saved.json'),JSON.stringify(saved));
 const edited=structuredClone(saved);edited.body=c.TextX.apply(edited.body,new c.TextX().insert(1,{dataStream:'Q'}).serialize());const final=save(edited,(u,f)=>inspect(u,f,true));fs.writeFileSync(path.join(qa,'container-navigation-edited.json'),JSON.stringify(final));
 console.log('Pinned16 container points across main/nested/cell/list/header/footer/note bodies,7 exact internal selection-refresh requests, current token/source bindings, hint-free native points, save/reopen and TextX edits pass; browser caret/scroll/paint remain unqualified');
}
main().catch(error=>{console.error(error);process.exitCode=1});
