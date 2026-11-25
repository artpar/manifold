/* Optional independent pinned drawing ownership/geometry/edit-save QA. */
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const {stripTypeScriptTypes}=require('node:module');
const root=path.resolve(__dirname,'../../..'),qa=path.join(root,'output/vdocs-qa');
const core=require(path.join(qa,'node_modules/@univerjs/core')),engine=require(path.join(qa,'node_modules/@univerjs/engine-render')),drawing=require(path.join(qa,'node_modules/@univerjs/docs-drawing'));
for(const name of ['core','engine-render','docs-drawing','drawing'])assert.equal(JSON.parse(fs.readFileSync(path.join(qa,'node_modules/@univerjs',name,'package.json'))).version,'1.0.4');
function boxes(snapshot){
 const model=new core.DocumentDataModel(snapshot),view=new engine.DocumentViewModel(model),skeleton=new engine.DocumentSkeleton(view,new core.LocaleService());skeleton.calculate();
 const result=new Map();
 function walk(page){for(const [id,box]of page.skeDrawings)result.set(id,[box.width,box.height]);for(const table of page.skeTables.values())for(const row of table.rows)for(const cell of row.cells)walk(cell)}
 for(const page of skeleton.getSkeletonData().pages)walk(page);
 skeleton.dispose();view.dispose();model.dispose();return result;
}
async function main(){
 const snapshot={...JSON.parse(fs.readFileSync(path.join(qa,'images.json'))),title:'Native embedded assets',documentStyle:{documentFlavor:1,pageSize:{width:800,height:1000},marginLeft:40,marginRight:40}};

 const envelope=document=>({type:'manifold-document',version:3,engine:'univer',engineVersion:'1.0.4',document});
 const flat=drawing.collectDocDrawings(snapshot);assert.equal(Object.keys(flat.drawings).length,5);assert.equal(new Set(flat.drawingsOrder).size,5);
 const topBoxes=boxes(snapshot);assert.equal(topBoxes.size,4);
 for(const [id,source]of Object.entries(flat.drawings)){
  const found=drawing.findDocDrawing(snapshot,id);assert.ok(found);assert.equal(found.drawing.source,source.source);assert.equal(source.unitId,snapshot.id);
  assert.equal(source.imageSourceType,'BASE64');assert.equal(source.drawingType,core.DrawingTypeEnum.DRAWING_IMAGE);assert.equal(source.layoutType,core.PositionedObjectLayoutType.INLINE);
  const raw=Buffer.from(source.source.split(',')[1],'base64');
  if(source.source.startsWith('data:image/jpeg;'))assert.equal(crypto.createHash('sha256').update(raw).digest('hex'),'fe9737d0cb4f905917054c7938fc3ccf4b83f0623df9cd5d62192dbaae26b510');
  if(found.segmentId===''){assert.deepEqual(topBoxes.get(id),[source.docTransform.size.width,source.docTransform.size.height])}else{
   assert.equal(found.segmentId,'manydoc-note-1');assert.deepEqual(found.path.slice(0,3),['notes','manydoc-note-1','drawings']);
   const note=snapshot.notes[found.segmentId];const noteBoxes=boxes({id:snapshot.id,documentStyle:snapshot.documentStyle,body:note.body,drawings:note.drawings,drawingsOrder:note.drawingsOrder});assert.deepEqual(noteBoxes.get(id),[9.5,9.25*4/3]);
  }
 }
 assert.deepEqual(snapshot.drawingsOrder,snapshot.body.customBlocks.filter(b=>b.blockType===0).map(b=>b.blockId));
 const id=snapshot.body.customBlocks[0].blockId;
 assert.deepEqual(topBoxes.get(id),[32,16]);
 const model=new core.DocumentDataModel(snapshot);const saved=structuredClone(model.getSnapshot());
 saved.drawings[id].docTransform.size={width:64,height:32};saved.drawings[id].description='Resized PNG';model.reset(saved);
 const edited=model.getSnapshot();assert.deepEqual(boxes(edited).get(id),[64,32]);assert.equal(drawing.findDocDrawing(edited,id).drawing.description,'Resized PNG');
 assert.deepEqual(edited.resources,snapshot.resources);fs.writeFileSync(path.join(qa,'images-edited.json'),JSON.stringify(edited,null,2));model.dispose();
 console.log('Pinned native main/cell/list/note image ownership, original JPEG bytes, drawing box geometry and resize/save passed; browser bitmap paint/resource persistence unqualified');
}
main().catch(error=>{console.error(error);process.exitCode=1});
