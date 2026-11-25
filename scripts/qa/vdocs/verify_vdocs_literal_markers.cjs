/* Optional actual pinned reader, geometry, TextX edit and facade-save QA. */
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const {createRequire,stripTypeScriptTypes}=require('node:module');
const root=path.resolve(__dirname,'../../..'),qa=path.join(root,'output/colleaf-qa');
const req=createRequire(path.join(qa,'package.json'));
const core=req('@univerjs/core'),engine=req('@univerjs/engine-render'),docs=req('@univerjs/docs'),drawing=req('@univerjs/drawing'),docDrawing=req('@univerjs/docs-drawing'),{FUniver}=req('@univerjs/core/facade');req('@univerjs/docs/facade');
for(const name of ['core','engine-render','docs','drawing','docs-drawing'])assert.equal(JSON.parse(fs.readFileSync(path.join(qa,'node_modules/@univerjs',name,'package.json'))).version,'1.0.4');
function save(snapshot){const univer=new core.Univer();try{for(const plugin of [docs.UniverDocsPlugin,drawing.UniverDrawingPlugin,docDrawing.UniverDocsDrawingPlugin])univer.registerPlugin(plugin);const api=FUniver.newAPI(univer);return JSON.parse(JSON.stringify(api.createDocument(structuredClone(snapshot)).save()));}finally{univer.dispose();}}
function glyphs(page,out){
 for(const section of page.sections||[])for(const column of section.columns)for(const line of column.lines)for(const divide of line.divides)for(const glyph of divide.glyphGroup)out.push(glyph);
 for(const table of page.skeTables?.values()||[])for(const row of table.rows)for(const cell of row.cells)glyphs(cell,out);
 for(const note of page.notes||[])glyphs(note.page,out);
}
async function main(){
 const snapshot={id:'manydoc-literal-qa',title:'Literal labels',documentStyle:{documentFlavor:1,pageSize:{width:800,height:1000}},...JSON.parse(fs.readFileSync(path.join(qa,'literal-markers.json')))};
 const source=fs.readFileSync(path.join(root,'output/codec/colleaf-format-0.txt'),'utf8');assert.equal(crypto.createHash('sha256').update(source).digest('hex'),'c54b27b8c303713f5d5d5dfdb7cd54ec9b8410f5045613150861855b11c899c0');
 const app=await import('data:text/javascript;base64,'+Buffer.from(stripTypeScriptTypes(source)).toString('base64'));
 const validate=s=>app.validateDocument({type:'vibedada-document',version:3,engine:'univer',engineVersion:'1.0.4',document:s});validate(snapshot);
 const owned=snapshot.body.tables.map(t=>snapshot.tableSource[t.tableId]).filter(t=>t.manydocLiteralItem);
 assert.equal(owned.length,3);assert.deepEqual(owned.map(t=>t.manydocLiteralItem.index),[0,1,2]);
 const model=new core.DocumentDataModel(snapshot),view=new engine.DocumentViewModel(model),skeleton=new engine.DocumentSkeleton(view,new core.LocaleService());skeleton.calculate();
 const tables=skeleton.getSkeletonData().pages.flatMap(p=>[...p.skeTables.values()]);assert.equal(tables.length,3);
 for(const table of tables){assert.ok(table.width>0);assert.equal(table.rows[0].cells.length,2);assert.ok(table.rows[0].cells[1].left>table.rows[0].cells[0].left);}
 const painted=[];for(const page of skeleton.getSkeletonData().pages)glyphs(page,painted);
 const text=painted.map(g=>g.content||'').join('');for(const token of ['α)','β)','FIRST','SECOND','EMPTY','Marker note'])assert.ok(text.includes(token),[token,text]);
 assert.equal(painted.filter(g=>g.glyphType===engine.GlyphType.LIST).length,0,'literal labels must not paint automatic counters');
 assert.ok(painted.some(g=>g.noteId==='manydoc-note-1'),'label citation lost');
 const at=model.getBody().dataStream.indexOf('α)');assert.ok(at>=0);
 const body=core.TextX.apply(structuredClone(model.getBody()),new core.TextX().retain(at).delete(1).insert(1,{dataStream:'Z'}).serialize());model.reset({...model.getSnapshot(),body});
 const saved=save(model.getSnapshot());validate(saved);for(const t of owned)assert.deepEqual(saved.tableSource[t.tableId].manydocLiteralItem,t.manydocLiteralItem);
 fs.writeFileSync(path.join(qa,'literal-markers-saved.json'),JSON.stringify(save(snapshot),null,2));
 fs.writeFileSync(path.join(qa,'literal-markers-edited.json'),JSON.stringify(saved,null,2));
 skeleton.dispose();view.dispose();model.dispose();
 for(const name of ['mixed','nested','stories','rtl','sources','continued']){
  const input=JSON.parse(fs.readFileSync(path.join(root,'output/native-literal',name+'.json')));validate(input);
  const m=new core.DocumentDataModel(input),v=new engine.DocumentViewModel(m),sk=new engine.DocumentSkeleton(v,new core.LocaleService());
  try{
   sk.calculate();const data=sk.getSkeletonData(),all=[];for(const p of data.pages)glyphs(p,all);
   const visible=all.map(g=>g.content||'').join('');
   if(name==='mixed')assert.deepEqual(all.filter(g=>g.glyphType===engine.GlyphType.LIST).map(g=>g.content),['Item 7)','Item 9)']);
   if(name==='nested'||name==='stories')for(const t of ['RICH_LABEL','INNER_TABLE','INNER_LIST','RICH_BODY'])assert.ok(visible.includes(t),[name,t,visible]);
   if(name==='continued')for(let i=0;i<48;i++)for(const kind of ['LABEL','BODY'])assert.equal(visible.split(kind+'_LINE_'+String(i).padStart(2,'0')).length-1,1,[name,kind,i]);
   if(name==='stories'){
    assert.ok(data.skeHeaders.size);const header=[];for(const cache of data.skeHeaders.values())for(const p of cache.values())glyphs(p,header);
    for(const t of ['RICH_LABEL','INNER_TABLE','INNER_LIST','RICH_BODY'])assert.ok(header.map(g=>g.content||'').join('').includes(t),t);
   }
   const output=save(input);validate(output);
   assert.deepEqual(output.tableSource,input.tableSource);fs.writeFileSync(path.join(root,'output/native-literal',name+'-saved.json'),JSON.stringify(output,null,2));
   console.log(name,'actual native glyphs and facade table ownership pass');
  }finally{sk.dispose();v.dispose();m.dispose();}
 }
 console.log('Pinned reader/native label+body glyphs/table geometry/note citation/TextX edit/actual facade save pass; scoped SDK evidence only; broader native fidelity remains unqualified');
}
main().catch(err=>{console.error(err);process.exitCode=1});
