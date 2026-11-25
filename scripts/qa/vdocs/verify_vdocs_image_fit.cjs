/* Optional actual pinned renderer/facade check; no runtime JavaScript. */
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),{createRequire}=require('node:module');
const qa=path.resolve(__dirname,'../../../output/vdocs-qa'),req=createRequire(path.join(qa,'package.json'));
const core=req('@univerjs/core'),engine=req('@univerjs/engine-render'),docs=req('@univerjs/docs'),drawing=req('@univerjs/drawing'),docDrawing=req('@univerjs/docs-drawing'),{FUniver}=req('@univerjs/core/facade');req('@univerjs/docs/facade');
for(const name of ['core','engine-render','docs','drawing','docs-drawing'])assert.equal(JSON.parse(fs.readFileSync(path.join(qa,'node_modules/@univerjs',name,'package.json'))).version,'1.0.4');
const expected=new Map([['Body tall',[191*4/3,382*4/3]],['Nested merged cell',[114,228]],['List continuation',[191*4/3,382*4/3]],['Inherited header',[5*4/3,10*4/3]],['Footer',[10*4/3,20*4/3]],['Shared note',[143*4/3,286*4/3]]]);
function inspect(snapshot,overflow=false){
 const model=new core.DocumentDataModel(snapshot),view=new engine.DocumentViewModel(model),ske=new engine.DocumentSkeleton(view,new core.LocaleService());
 try{
  ske.calculate();const data=ske.getSkeletonData(),seen=new Set();let placedNotes=0;
  function walk(page,scope){
   for(const [id,box]of page.skeDrawings){
    const source=snapshot.drawings?.[id]||Object.values(snapshot.notes||{}).map(n=>n.drawings?.[id]).find(Boolean);assert.ok(source);
    const alt=source.description;seen.add(alt);
    if(overflow&&alt==='Body tall'){assert.equal(box.width,1000);assert.equal(box.height,2000);assert.ok(box.width>page.pageWidth-page.marginLeft-page.marginRight);continue}
    const target=expected.get(alt);assert.ok(target,alt);
    assert.ok(Math.abs(box.width-target[0])<1e-8,alt+' width');assert.ok(Math.abs(box.height-target[1])<1e-8,alt+' height');
    assert.ok(box.width<=page.pageWidth-page.marginLeft-page.marginRight+1e-8,alt+' horizontal fit');
    if(Number.isFinite(page.pageHeight))assert.ok(box.height<=page.pageHeight-page.marginTop-page.marginBottom+1e-8,alt+' vertical fit');
   }
   for(const table of page.skeTables.values())for(const row of table.rows)for(const cell of row.cells)walk(cell,scope+'/cell');
   for(const note of page.notes||[]){placedNotes++;walk(note.page,'note')}
  }
  for(const page of data.pages)walk(page,'body');
  for(const caches of data.skeHeaders.values())for(const page of caches.values())walk(page,'header');
  for(const caches of data.skeFooters.values())for(const page of caches.values())walk(page,'footer');
  assert.deepEqual([...seen].sort(),[...expected.keys()].sort());assert.ok(placedNotes>0,'note image must be actually placed');
 }finally{ske.dispose();view.dispose();model.dispose()}
}
function save(snapshot){const u=new core.Univer();try{for(const p of [docs.UniverDocsPlugin,drawing.UniverDrawingPlugin,docDrawing.UniverDocsDrawingPlugin])u.registerPlugin(p);return JSON.parse(JSON.stringify(FUniver.newAPI(u).createDocument(structuredClone(snapshot)).save()))}finally{u.dispose()}}
async function main(){
const {stripTypeScriptTypes}=require('node:module'),crypto=require('node:crypto');

const snapshot=JSON.parse(fs.readFileSync(path.join(qa,'fitted-images.json')));inspect(snapshot);
const raw=structuredClone(snapshot),id=raw.body.customBlocks[0].blockId;raw.drawings[id].docTransform.size={width:1000,height:2000};inspect(raw,true);
const saved=save(save(snapshot));inspect(saved);fs.writeFileSync(path.join(qa,'fitted-images-saved.json'),JSON.stringify(saved,null,2));
console.log('Pinned body/nested merged-cell/list/shared-note/inherited-running frame fitting and real note placement/facade save pass; overflow without projection reproduced; browser font/paint/total-story height unqualified');

}
main().catch(error=>{console.error(error);process.exitCode=1});
