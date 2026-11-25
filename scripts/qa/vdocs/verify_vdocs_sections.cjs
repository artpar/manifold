/* Optional pinned section/page/story reader and actual facade-save QA. */
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const {createRequire,stripTypeScriptTypes}=require('node:module');
const root=path.resolve(__dirname,'../../..'),qa=path.join(root,'output/colleaf-qa'),req=createRequire(path.join(qa,'package.json'));
const core=req('@univerjs/core'),engine=req('@univerjs/engine-render'),docs=req('@univerjs/docs'),drawing=req('@univerjs/drawing'),docDrawing=req('@univerjs/docs-drawing'),{FUniver}=req('@univerjs/core/facade');req('@univerjs/docs/facade');
for(const name of ['core','engine-render','docs','drawing','docs-drawing'])assert.equal(JSON.parse(fs.readFileSync(path.join(qa,'node_modules/@univerjs',name,'package.json'))).version,'1.0.4');
function inspect(data){
 const model=new core.DocumentDataModel(data),view=new engine.DocumentViewModel(model),skeleton=new engine.DocumentSkeleton(view,new core.LocaleService());
 try{
  skeleton.calculate();const pages=skeleton.getSkeletonData().pages;
  assert.equal(pages.length,7); // explicit breaks plus odd-page section's blank page
  assert.deepEqual(pages.map(p=>p.headerId),['manydoc-header-2','manydoc-header-3','manydoc-header-1','manydoc-header-3','manydoc-header-5','manydoc-header-3','manydoc-header-6']);
  assert.deepEqual(pages.map(p=>p.footerId),['','','manydoc-footer-4','','manydoc-footer-4','','manydoc-footer-7']);
  for(let i=0;i<5;i++){assert.equal(pages[i].pageWidth,450.125*4/3);assert.equal(pages[i].pageHeight,600.5*4/3);assert.equal(pages[i].marginLeft,40)}
  for(let i=5;i<7;i++){assert.equal(pages[i].pageWidth,400*4/3);assert.equal(pages[i].pageHeight,550*4/3);assert.equal(pages[i].marginLeft,0)}
  assert.equal(model.headerModelMap.get('manydoc-header-5').getBody().dataStream,'\r\n');
  const header=view.getHeaderFooterTreeMap().headerTreeMap.get('manydoc-header-1');
  const table=header.getBody().tables[0];assert.equal(header.getTableByStartIndex(table.startIndex).tableSource.tableRows[0].tableCells[0].columnSpan,2);
  const caches=skeleton.getSkeletonData().skeHeaders.get('manydoc-header-1');let tested=false;
  for(const page of caches.values()){
   assert.equal(page.skeTables.size,1);assert.equal(page.skeDrawings.size,1);
   const box=[...page.skeDrawings.values()][0];assert.deepEqual([box.width,box.height],[32,16]);
   const markers=[];for(const s of page.sections)for(const c of s.columns)for(const l of c.lines)for(const d of l.divides)for(const g of d.glyphGroup)if(g.glyphType===engine.GlyphType.LIST)markers.push(g.content);assert.deepEqual(markers,['•']);tested=true;
  }
  assert.ok(tested);
  assert.equal(Object.keys(docDrawing.collectDocDrawings(data).drawings).length,1);
  assert.equal(data.headerFooterDrawingsOrder.length,1);
 }finally{skeleton.dispose();view.dispose();model.dispose()}
}
function save(data){const univer=new core.Univer();try{for(const plugin of[docs.UniverDocsPlugin,drawing.UniverDrawingPlugin,docDrawing.UniverDocsDrawingPlugin])univer.registerPlugin(plugin);return JSON.parse(JSON.stringify(FUniver.newAPI(univer).createDocument(structuredClone(data)).save()))}finally{univer.dispose()}}
async function main(){
 const data=JSON.parse(fs.readFileSync(path.join(qa,'sections.json')));inspect(data);
 const source=fs.readFileSync(path.join(root,'output/codec/colleaf-format-0.txt'),'utf8');assert.equal(crypto.createHash('sha256').update(source).digest('hex'),'c54b27b8c303713f5d5d5dfdb7cd54ec9b8410f5045613150861855b11c899c0');const app=await import('data:text/javascript;base64,'+Buffer.from(stripTypeScriptTypes(source)).toString('base64'));
 const saved=save(save(data));inspect(saved);app.validateDocument({type:'vibedada-document',version:3,engine:'univer',engineVersion:'1.0.4',document:saved});fs.writeFileSync(path.join(qa,'sections-saved.json'),JSON.stringify(saved,null,2));
 console.log('Pinned seven-page section geometry, first/even/default/inherited/empty stories, header table/list/image and actual facade save/reopen pass; font metrics/browser paint unqualified');
}
main().catch(error=>{console.error(error);process.exitCode=1});
