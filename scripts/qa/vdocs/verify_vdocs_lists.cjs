/* Optional pinned native list marker/reader/edit/save QA. */
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const {stripTypeScriptTypes}=require('node:module');
const root=path.resolve(__dirname,'../../..'),qa=path.join(root,'output/vdocs-qa');
const core=require(path.join(qa,'node_modules/@univerjs/core')),engine=require(path.join(qa,'node_modules/@univerjs/engine-render'));
for(const name of ['core','engine-render'])assert.equal(JSON.parse(fs.readFileSync(path.join(qa,'node_modules/@univerjs',name,'package.json'))).version,'1.0.4');
function markerMap(snapshot){
 const model=new core.DocumentDataModel(snapshot),view=new engine.DocumentViewModel(model),skeleton=new engine.DocumentSkeleton(view,new core.LocaleService());
 skeleton.calculate();const labels=new Map();
 for(const page of skeleton.getSkeletonData().pages)for(const section of page.sections)for(const column of section.columns)for(const line of column.lines){
  for(const divide of line.divides)for(const glyph of divide.glyphGroup)if(glyph.glyphType===engine.GlyphType.LIST)labels.set(line.paragraphIndex,glyph.content);
 }
 skeleton.dispose();view.dispose();model.dispose();return labels;
}
async function main(){
 const snapshot={id:'manydoc-lists-qa',title:'Native list ownership',documentStyle:{documentFlavor:1,pageSize:{width:800,height:1000}},...JSON.parse(fs.readFileSync(path.join(qa,'lists.json')))};

 const envelope=document=>({type:'manifold-document',version:3,engine:'univer',engineVersion:'1.0.4',document});
 const expected=['Appendix AA.','AA.iv)','AA.v)','Appendix BB.','8.1','2.','3.','•','0.'];
 const marked=snapshot.body.paragraphs.filter(p=>p.bullet);
 const labels=markerMap(snapshot);
 assert.deepEqual(marked.map(p=>labels.get(p.startIndex)),expected);
 const legal=structuredClone(snapshot);legal.lists[marked[1].bullet.listType].nestingLevel[1].isLegal=true;
 const legalLabels=markerMap(legal);assert.equal(legalLabels.get(marked[1].startIndex),'27.4)');assert.equal(legalLabels.get(marked[2].startIndex),'27.5)');
 const note=snapshot.notes['manydoc-note-1'];
 const noteLabels=markerMap({id:'note-labels',documentStyle:snapshot.documentStyle,body:note.body,lists:note.lists});
 assert.deepEqual(note.body.paragraphs.filter(p=>p.bullet).map(p=>noteLabels.get(p.startIndex)),['0.','1.']);
 const ownership=JSON.parse(snapshot.body.customRanges.find(r=>r.rangeType===5&&r.properties.profile==='manydoc.native-intent').properties.records.find(r=>r.name==='manydoc.list-ownership.v1').data);
 assert.equal(ownership.stories[''].length,6);
 const model=new core.DocumentDataModel(snapshot);let view=new engine.DocumentViewModel(model);
 assert.equal(view.getParagraph(marked[0].startIndex).bullet.startNumber,26);
 const originalIds=model.getBody().paragraphs.map(p=>p.paragraphId);
 view.dispose();
 const body=core.TextX.apply(structuredClone(model.getBody()),new core.TextX().delete(6).insert(6,{dataStream:'Edited'}).serialize());
 model.reset({...model.getSnapshot(),body});view=new engine.DocumentViewModel(model);
 assert.ok(model.getBody().dataStream.startsWith('Edited one\r'));
 assert.deepEqual(model.getBody().paragraphs.map(p=>p.paragraphId),originalIds);
 assert.deepEqual(model.getSnapshot().resources,snapshot.resources);
 const editedLabels=markerMap(model.getSnapshot());assert.deepEqual(marked.map(p=>editedLabels.get(p.startIndex)),expected);
 fs.writeFileSync(path.join(qa,'lists-edited.json'),JSON.stringify(model.getSnapshot(),null,2));
 view.dispose();model.dispose();
 console.log('Pinned native compound/independent/restarted/zero/bullet markers, TextX save passed; full browser/resource persistence and visual layout remain unqualified');
}
main().catch(error=>{console.error(error);process.exitCode=1});
