/* Optional pinned actual app/native reader/mutation QA; no runtime JS. */
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const crypto=require('node:crypto');
const {stripTypeScriptTypes}=require('node:module');
const root=path.resolve(__dirname,'..');
const qa=path.join(root,'output/colleaf-qa');
const core=require(path.join(qa,'node_modules/@univerjs/core'));
const engine=require(path.join(qa,'node_modules/@univerjs/engine-render'));
for(const name of ['core','engine-render'])assert.equal(JSON.parse(fs.readFileSync(path.join(qa,'node_modules/@univerjs',name,'package.json'))).version,'1.0.4');
async function main(){
 const snapshot={id:'manydoc-notes-qa',title:'Native note authority',documentStyle:{documentFlavor:1,pageSize:{width:800,height:1000}},...JSON.parse(fs.readFileSync(path.join(qa,'notes.json')))};
 const source=fs.readFileSync(path.join(root,'output/codec/colleaf-format-0.txt'),'utf8');
 assert.equal(crypto.createHash('sha256').update(source).digest('hex'),'c54b27b8c303713f5d5d5dfdb7cd54ec9b8410f5045613150861855b11c899c0');
 const app=await import('data:text/javascript;base64,'+Buffer.from(stripTypeScriptTypes(source)).toString('base64'));
 const envelope=document=>({type:'vibedada-document',version:3,engine:'univer',engineVersion:'1.0.4',document});
 app.validateDocument(envelope(snapshot));
 const model=new core.DocumentDataModel(snapshot);
 let view=new engine.DocumentViewModel(model);
 const noteViews=view.getNoteTreeMap();
 assert.equal(noteViews.size,3);
 const refs=model.getBody().customBlocks;
 assert.equal(refs.length,4);
 assert.equal(refs[0].startIndex,3); // after A + astral scalar
 assert.equal(view.getCustomBlock(3).noteId,'manydoc-note-2');
 assert.equal(view.getTextRun(3).ts.bl,1);
 assert.equal(view.getTextRun(3).ts.fs,12);
 assert.equal(view.getTextRun(3).ts.lang,'en-US');
 assert.equal(refs[0].noteId,refs[2].noteId); // repeated citation, one definition
 assert.equal(refs[1].noteType,'endnote');
 assert.equal(noteViews.get('manydoc-note-1').getBody().dataStream,'End body 😀\r\n');
 assert.ok(noteViews.get('manydoc-note-2').getBody().dataStream.startsWith('Foot body\r'));
 const noteTable=snapshot.notes['manydoc-note-2'].body.tables[0];
 assert.equal(noteViews.get('manydoc-note-2').getTableByStartIndex(noteTable.startIndex).tableSource.tableRows[0].tableCells[0].columnSpan,2);
 assert.equal(snapshot.notes['manydoc-note-3'].customMark,'§');
 assert.equal(snapshot.notes['manydoc-note-3'].showBodyReference,false);
 const identities=JSON.parse(snapshot.resources.find(r=>r.name==='manydoc.note-identities.v1').data);
 assert.deepEqual(identities.bindings.map(b=>[b.nativeId,b.kind,b.id]),[['manydoc-note-1','endnote','same'],['manydoc-note-2','footnote','same'],['manydoc-note-3','footnote','custom']]);
 view.dispose();
 const saved=structuredClone(model.getSnapshot());
 saved.notes['manydoc-note-2'].body=core.TextX.apply(saved.notes['manydoc-note-2'].body,new core.TextX().delete(4).insert(4,{dataStream:'Edit'}).serialize());
 saved.notes['manydoc-note-3'].customMark='†';
 model.reset(saved);
 view=new engine.DocumentViewModel(model);
 assert.ok(view.getNoteTreeMap().get('manydoc-note-2').getBody().dataStream.startsWith('Edit body\r'));
 assert.deepEqual(model.getSnapshot().resources,snapshot.resources);
 app.validateDocument(envelope(model.getSnapshot()));
 fs.writeFileSync(path.join(qa,'notes-edited.json'),JSON.stringify(model.getSnapshot(),null,2));
 view.dispose();model.dispose();
 console.log('Pinned app validator, native foot/endnote identities/references/bodies/tables and TextX note edit/save passed; note placement/browser rendering not qualified');
}
main().catch(error=>{console.error(error);process.exitCode=1});
