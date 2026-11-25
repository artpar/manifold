/* Optional actual pinned literal/rich citation, markerless anchor and facade QA. */
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),{createRequire}=require('node:module');
const qa=path.resolve(__dirname,'../../../output/vdocs-qa'),req=createRequire(path.join(qa,'package.json'));
const c=req('@univerjs/core'),e=req('@univerjs/engine-render'),docs=req('@univerjs/docs'),drawing=req('@univerjs/drawing'),dd=req('@univerjs/docs-drawing'),{FUniver}=req('@univerjs/core/facade');req('@univerjs/docs/facade');
for(const n of ['core','engine-render','docs','drawing','docs-drawing'])assert.equal(JSON.parse(fs.readFileSync(path.join(qa,'node_modules/@univerjs',n,'package.json'))).version,'1.0.4');
function inspect(data,edited=false){
 const m=new c.DocumentDataModel(data),v=new e.DocumentViewModel(m),s=new e.DocumentSkeleton(v,new c.LocaleService());
 try{
  s.calculate();const refs=[],paint=[],placed=new Map();
  function glyphs(p){return p.sections.flatMap(s=>s.columns.flatMap(c=>c.lines.flatMap(l=>l.divides.flatMap(d=>d.glyphGroup))))}
  function walk(p){for(const g of glyphs(p)){if(g.noteId)refs.push([g.noteId,g.content]);paint.push(g)}for(const t of p.skeTables.values())for(const r of t.rows)for(const cell of r.cells)walk(cell)}
  for(const p of s.getSkeletonData().pages){walk(p);for(const n of p.notes||[])placed.set(n.noteId,n.page)}
  assert.equal(refs.length,7);assert.ok(refs.every(([id,label])=>label===''),'generated citations must be markerless');assert.equal(placed.size,2);
  const fragments=data.body.customRanges.filter(r=>r.properties?.profile==='manydoc.note-citation'),groups=new Map();
  for(const f of fragments){assert.ok(f.properties.token);const old=groups.get(f.properties.token);if(old){assert.deepEqual(old.properties,f.properties);old.startIndex=Math.min(old.startIndex,f.startIndex);old.endIndex=Math.max(old.endIndex,f.endIndex)}else groups.set(f.properties.token,structuredClone(f))}
  const owners=[...groups.values()];assert.equal(owners.length,7);if(edited)assert.equal(fragments.length,8,'actual TextX split is a regression case');
  assert.deepEqual(owners.map(r=>data.body.dataStream.slice(r.startIndex+1,r.endIndex)),['1',edited?'III':'II','*A','','*A','II','1']);
  for(const r of owners){assert.equal(r.active,false);assert.equal(r.properties.textColorMode,'text');assert.equal(data.body.dataStream[r.startIndex],'\ufffc');assert.equal(data.body.dataStream[r.endIndex],'\u2060')}
  const stars=paint.filter(g=>g.content==='*');assert.equal(stars.length,2);for(const g of stars){assert.equal(g.ts.fs,edited?14:12);assert.equal(g.ts.bl,edited?0:1);assert.equal(g.ts.cl.rgb,'#FF0000');assert.equal(g.ts.va,c.BaselineOffset.SUPERSCRIPT);assert.notEqual(g.ts.ul?.s,1)}
  const green=paint.filter(g=>g.content==='A'&&g.ts.cl?.rgb==='#006633');assert.equal(green.length,2);for(const g of green){assert.equal(g.ts.fs,8);assert.equal(g.ts.it,1);assert.equal(g.ts.va,c.BaselineOffset.SUPERSCRIPT)}
  for(const [id,prefix]of [['manydoc-note-1','9\tFoot body'],['manydoc-note-2','1\tEnd body']]){assert.equal(data.notes[id].customMark,'');assert.equal(data.notes[id].showBodyReference,false);assert.ok(data.notes[id].body.dataStream.startsWith(prefix));assert.ok(glyphs(placed.get(id)).some(g=>g.content===prefix[0]))}
 }finally{s.dispose();v.dispose();m.dispose()}
}
function save(data){const u=new c.Univer();try{for(const p of [docs.UniverDocsPlugin,drawing.UniverDrawingPlugin,dd.UniverDocsDrawingPlugin])u.registerPlugin(p);return JSON.parse(JSON.stringify(FUniver.newAPI(u).createDocument(structuredClone(data)).save()))}finally{u.dispose()}}
async function main(){
 const data=JSON.parse(fs.readFileSync(path.join(qa,'citations.json')));

 inspect(data);const saved=save(save(data));inspect(saved);fs.writeFileSync(path.join(qa,'citations-saved.json'),JSON.stringify(saved,null,2));
 const edited=structuredClone(saved),owner=edited.body.customRanges.filter(r=>r.properties?.profile==='manydoc.note-citation')[1],at=owner.startIndex+1;
 const style=edited.body.textRuns.find(r=>r.st===at).ts;
 edited.body=c.TextX.apply(edited.body,new c.TextX().retain(at).delete(2).insert(3,{dataStream:'III',textRuns:[{st:0,ed:3,ts:style}]}).serialize());
 for(const r of edited.body.textRuns)if(r.ts.cl?.rgb==='#FF0000'){r.ts.bl=0;r.ts.fs=14}
 const final=save(edited);inspect(final,true);fs.writeFileSync(path.join(qa,'citations-edited.json'),JSON.stringify(final,null,2));
 console.log('Pinned seven markerless anchors with literal/rich/default/empty citation glyphs/styles in sections/cells/lists, two real note bodies, TextX/style edits and facade save pass; browser paint/wrapping unqualified');
}
main().catch(error=>{console.error(error);process.exitCode=1});
