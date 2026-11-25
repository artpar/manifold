/* Optional pinned native note-placement acceptance check. A failure is a real
 * open qualification gap; do not replace the first-reference-order oracle.
 * Requires the public notes example and the existing pinned QA packages.
 */
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const {createRequire}=require('node:module');
const root=path.resolve(__dirname,'../../..');
const req=createRequire(path.join(root,'output/colleaf-qa/package.json'));
const core=req('@univerjs/core');
const engine=req('@univerjs/engine-render');
for(const name of ['core','engine-render'])assert.equal(JSON.parse(fs.readFileSync(path.join(root,'output/colleaf-qa/node_modules/@univerjs',name,'package.json'))).version,'1.0.4');
const input=JSON.parse(fs.readFileSync(process.argv[2]||path.join(root,'output/colleaf-tagged/document.vdocs'),'utf8'));
assert.equal(input.type,'vibedada-document');assert.equal(input.version,3);
const d=input.document;
// The native source itself identifies first reference order; no pixel or
// cached-SF oracle supplies the expectation.
const expected=[];
for(const range of [...(d.body.customRanges||[])].sort((a,b)=>a.startIndex-b.startIndex)){
 if(range.rangeType!==core.CustomRangeType.FOOTNOTE)continue;
 const id=range.properties?.noteId;
 if(id&&!expected.includes(id))expected.push(id);
}
assert.equal(expected.length,2,'the fixture must retain both original footnotes');
function observe(snapshot){
 const model=new core.DocumentDataModel(snapshot);
 const view=new engine.DocumentViewModel(model);
 const skeleton=new engine.DocumentSkeleton(view,new core.LocaleService());
 try{
  skeleton.calculate();
  const observed=[],positions=[];
  for(const [page,p]of skeleton.getSkeletonData().pages.entries()){
   for(const n of p.notes||[]){
    if(snapshot.notes[n.noteId]?.type!=='footnote'||n.continued)continue;
    if(!observed.includes(n.noteId)){observed.push(n.noteId);positions.push({id:n.noteId,page:page+1,top:n.top,referenceIndex:n.referenceIndex});}
   }
  }
  return {observed,positions};
 }finally{skeleton.dispose();view.dispose();model.dispose();}
}
const original=observe(d);
// A separate control replaces only later repeated note atoms with literal marks.
// The original source and first-reference-order acceptance oracle stay intact.
const control=structuredClone(d),seen=new Set();let removed=0;
control.body.customRanges=control.body.customRanges.filter(range=>{
 if(range.rangeType!==core.CustomRangeType.FOOTNOTE)return true;
 const id=range.properties?.noteId;if(!id)return true;
 if(!seen.has(id)){seen.add(id);return true;}
 assert.equal(control.body.dataStream[range.startIndex],'\ufffc');
 assert.equal(control.notes[id].customMark,'1','this control requires the unchanged single-digit example');
 control.body.dataStream=control.body.dataStream.slice(0,range.startIndex)+'1'+control.body.dataStream.slice(range.startIndex+1);
 removed++;return false;
});
assert.equal(removed,1,'the original fixture must retain its repeated citation');
const controlResult=observe(control);assert.deepEqual(controlResult.observed,expected);
// Reordering raw ranges is not a stable fix: a normal TextX edit canonicalizes
// them again. Keep this experiment separate from the unchanged acceptance input.
const reordered=structuredClone(d);
const repeats=reordered.body.customRanges.map((r,i)=>[r,i]).filter(([r])=>r.properties?.noteId===expected[0]).map(([,i])=>i);
assert.equal(repeats.length,2);
[reordered.body.customRanges[repeats[0]],reordered.body.customRanges[repeats[1]]]=[reordered.body.customRanges[repeats[1]],reordered.body.customRanges[repeats[0]]];
const beforeEdit=observe(reordered);
const edited=structuredClone(reordered);
edited.body=core.TextX.apply(edited.body,new core.TextX().insert(1,{dataStream:'X'}).serialize());
const afterEdit=observe(edited);
console.log(JSON.stringify({expectedFirstReferenceOrder:expected,observedBodyOrder:original.observed,positions:original.positions,noRepeatControlOrder:controlResult.observed,rawRangeReorderControl:beforeEdit.observed,afterNativeTextXEdit:afterEdit.observed,editedReferenceIndices:afterEdit.positions.map(p=>p.referenceIndex)},null,2));
assert.deepEqual(original.observed,expected,'native footnote bodies must follow first-reference order even with repeated citations');
console.log('Native footnote first-reference placement order passes');
