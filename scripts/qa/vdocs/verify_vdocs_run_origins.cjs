/* Optional pinned run-source interval/citation save/edit qualification. */
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const {createRequire}=require('node:module');
const root=path.resolve(__dirname,'../../..'),qa=path.join(root,'output/vdocs-qa'),req=createRequire(path.join(qa,'package.json'));
const c=req('@univerjs/core'),e=req('@univerjs/engine-render'),docs=req('@univerjs/docs'),drawing=req('@univerjs/drawing'),dd=req('@univerjs/docs-drawing'),hyper=req('@univerjs/docs-hyper-link'),{FUniver}=req('@univerjs/core/facade');req('@univerjs/docs/facade');
for(const n of ['core','engine-render','docs','drawing','docs-drawing','docs-hyper-link']) assert.equal(JSON.parse(fs.readFileSync(path.join(qa,'node_modules/@univerjs',n,'package.json'))).version,'1.0.4');
function evidence(data){return JSON.parse(data.body.customRanges.find(r=>r.properties?.profile==='manydoc.native-intent').properties.records.find(r=>r.name==='manydoc.document-properties.v1').data).evidence}
function bodies(data){return [data.body,...Object.values(data.headers).map(s=>s.body),...Object.values(data.footers).map(s=>s.body),...Object.values(data.notes).map(s=>s.body)]}
function save(data){const u=new c.Univer();try{for(const p of[docs.UniverDocsPlugin,drawing.UniverDrawingPlugin,dd.UniverDocsDrawingPlugin,hyper.UniverDocsHyperLinkPlugin])u.registerPlugin(p);return JSON.parse(JSON.stringify(FUniver.newAPI(u).createDocument(structuredClone(data)).save()))}finally{u.dispose()}}
function inspect(data,pool,count=11){
 assert.deepEqual(evidence(data).runOrigins,pool);const refs=[];
 for(const b of bodies(data))for(const r of b.textRuns||[]){const o=r.ts?.manydocOrigin;if(o){assert.deepEqual(Object.keys(o).sort(),['profile','ref','version']);assert.equal(o.profile,'manydoc.run-origin');assert.equal(o.version,1);assert.ok(pool.some(p=>p.ref===o.ref));refs.push(o.ref)}}
 assert.equal(refs.length,count);
 const m=new c.DocumentDataModel(data),v=new e.DocumentViewModel(m),s=new e.DocumentSkeleton(v,new c.LocaleService());
 try{s.calculate();const glyphs=[];for(const p of s.getSkeletonData().pages)for(const sec of p.sections)for(const col of sec.columns)for(const l of col.lines)for(const d of l.divides)glyphs.push(...d.glyphGroup);const emoji=glyphs.find(g=>g.content==='😀');assert.ok(emoji);assert.equal(emoji.ts.bl,1);assert.equal(emoji.ts.fs,13);const citations=glyphs.filter(g=>g.noteId);assert.deepEqual(citations.map(g=>g.content),['','']);assert.ok(glyphs.some(g=>g.content==='*'||g.content==='X'),'projected rich citation remains visible');}finally{s.dispose();v.dispose();m.dispose()}
}
async function main(){

 const data=JSON.parse(fs.readFileSync(path.join(qa,'run-origins.json'))),pool=evidence(data).runOrigins;
 assert.equal(pool.length,11);assert.deepEqual(pool.map(p=>p.source.element).sort(),['body-mark-run','cell-run','citation-label-run','citation-run','default-citation-run','first-run','footer-run','header-run','item-run','note-run','second-run']);
 for(const o of pool){assert.match(o.ref,/^[a-f0-9]{64}$/);assert.match(o.fingerprint,/^[a-f0-9]{64}$/);assert.equal(o.source.format,'pdf');for(const key of['text','body','runs','label_content'])assert.ok(!Object.hasOwn(o,key))}
 inspect(data,pool);const saved=save(save(data));inspect(saved,pool);assert.equal(saved.body.dataStream,data.body.dataStream);fs.writeFileSync(path.join(qa,'run-origins-saved.json'),JSON.stringify(saved));
 const edit=structuredClone(saved);edit.body=c.TextX.apply(edit.body,new c.TextX().retain(3).insert(1,{dataStream:'Q'}).serialize());const edited=save(edit);inspect(edited,pool,12);assert.ok(edited.body.dataStream.startsWith('😀 QFirst\r'));const ref=pool.find(p=>p.source.element==='first-run').ref;assert.deepEqual(edited.body.textRuns.filter(r=>r.ts?.manydocOrigin?.ref===ref).map(r=>[r.st,r.ed]),[[0,3],[4,9]]);fs.writeFileSync(path.join(qa,'run-origins-edited.json'),JSON.stringify(edited));
 const citation=structuredClone(saved),range=citation.body.customRanges.find(r=>r.properties?.profile==='manydoc.note-citation'&&r.properties.mode==='rich'),start=range.startIndex+1;
 const ts=citation.body.textRuns.find(r=>r.st===start).ts;assert.equal(ts.va,3);assert.ok(ts.manydocOrigin);
 citation.body=c.TextX.apply(citation.body,new c.TextX().retain(start).delete(1).insert(1,{dataStream:'X',textRuns:[{st:0,ed:1,ts}]}).serialize());const cited=save(citation);inspect(cited,pool);assert.equal(cited.body.dataStream[start],'X');fs.writeFileSync(path.join(qa,'run-origins-citation.json'),JSON.stringify(cited));
 const detach=structuredClone(saved);delete detach.body.textRuns[0].ts.manydocOrigin;const detached=save(detach);inspect(detached,pool,10);assert.equal(detached.body.dataStream,saved.body.dataStream);fs.writeFileSync(path.join(qa,'run-origins-detached.json'),JSON.stringify(detached));
 console.log('Pinned11 text-free run-source records across stories/cell/list/note/body-mark/rich/default citations, styled emoji, two saves/reopens, actual split and citation TextX edits and source-reference removal pass. Go checks historical/unbound SF; browser paint remains unqualified.');
}
main().catch(error=>{console.error(error);process.exitCode=1});
