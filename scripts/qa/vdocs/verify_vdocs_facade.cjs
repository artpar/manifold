/* Optional actual pinned FDocument.save/reopen regression, not getSnapshot-only. */
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const {createRequire}=require('node:module');
const root=path.resolve(__dirname,'../../..'),qa=path.join(root,'output/vdocs-qa');
const req=createRequire(path.join(qa,'package.json'));
const core=req('@univerjs/core'),docs=req('@univerjs/docs'),drawing=req('@univerjs/drawing'),docDrawing=req('@univerjs/docs-drawing'),{FUniver}=req('@univerjs/core/facade');req('@univerjs/docs/facade');
for(const name of ['core','docs','drawing','docs-drawing'])assert.equal(JSON.parse(fs.readFileSync(path.join(qa,'node_modules/@univerjs',name,'package.json'))).version,'1.0.4');
const carrier=body=>(body.customRanges||[]).find(r=>r.rangeType===5&&r.properties?.profile==='manydoc.native-intent');
function operate(snapshot,edit){
 const univer=new core.Univer();
 try{
  for(const plugin of [docs.UniverDocsPlugin,drawing.UniverDrawingPlugin,docDrawing.UniverDocsDrawingPlugin])univer.registerPlugin(plugin);
  const api=FUniver.newAPI(univer),document=api.createDocument(structuredClone(snapshot));
  if(edit){const model=document.getDocumentDataModel();const body=core.TextX.apply(structuredClone(model.getBody()),new core.TextX().insert(1,{dataStream:'X'}).serialize());model.reset({...model.getSnapshot(),body});}
  return JSON.parse(JSON.stringify(document.save()));
 }finally{univer.dispose()}
}
async function main(){
 const cases=JSON.parse(fs.readFileSync(path.join(qa,'facade-cases.json')));

 const envelope=document=>({type:'manifold-document',version:3,engine:'univer',engineVersion:'1.0.4',document});
 const saved={};
 for(const [name,input]of Object.entries(cases)){
  assert.equal(input.resources?.some(r=>r.name.startsWith('manydoc.'))||false,false);
  const intent=carrier(input.body);assert.ok(intent);assert.equal(intent.startIndex,input.body.dataStream.length-1);
  // Reproduce the old bug through the public save API, so this regression cannot
  // silently fall back to testing the lower-level model snapshot instead.
  const legacy=structuredClone(input);legacy.resources=intent.properties.records;legacy.body.customRanges=legacy.body.customRanges.filter(r=>r.rangeId!==intent.rangeId);
  const dropped=operate(legacy);assert.equal(dropped.resources.some(r=>r.name.startsWith('manydoc.')),false);
  const first=operate(input);assert.deepEqual(carrier(first.body).properties,intent.properties);
  saved[name]=operate(first);assert.deepEqual(carrier(saved[name].body).properties,intent.properties);assert.equal(saved[name].body.dataStream,input.body.dataStream);
 }
 fs.writeFileSync(path.join(qa,'facade-saved.json'),JSON.stringify(saved,null,2));
 const edited=operate(saved.lists,true);const c=carrier(edited.body);assert.equal(c.startIndex,edited.body.dataStream.length-1);assert.equal(c.endIndex,c.startIndex);assert.deepEqual(c.properties,carrier(saved.lists.body).properties);
 fs.writeFileSync(path.join(qa,'facade-edited.json'),JSON.stringify(operate(edited),null,2));
 console.log('Actual pinned Univer.createDocument/FDocument.save/reopen retain checked custom-range intent across notes/lists/images and TextX edits; old unregistered-resource loss reproduced; browser rendering remains unqualified');
}
main().catch(error=>{console.error(error);process.exitCode=1});
