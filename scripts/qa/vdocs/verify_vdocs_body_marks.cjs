/* Optional pinned literal/rich body-mark ownership, editor and facade QA. */
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),{createRequire,stripTypeScriptTypes}=require('node:module');
const qa=path.resolve(__dirname,'../../../output/colleaf-qa'),req=createRequire(path.join(qa,'package.json'));
const core=req('@univerjs/core'),engine=req('@univerjs/engine-render'),docs=req('@univerjs/docs'),drawing=req('@univerjs/drawing'),dd=req('@univerjs/docs-drawing'),{FUniver}=req('@univerjs/core/facade');req('@univerjs/docs/facade');
for(const n of ['core','engine-render','docs','drawing','docs-drawing'])assert.equal(JSON.parse(fs.readFileSync(path.join(qa,'node_modules/@univerjs',n,'package.json'))).version,'1.0.4');
function inspect(data,edited=false){
 const m=new core.DocumentDataModel(data),v=new engine.DocumentViewModel(m),s=new engine.DocumentSkeleton(v,new core.LocaleService());
 try{
  s.calculate();const citations=[],placed=new Map();
  const glyphs=p=>p.sections.flatMap(s=>s.columns.flatMap(c=>c.lines.flatMap(l=>l.divides.flatMap(d=>d.glyphGroup))));
  for(const p of s.getSkeletonData().pages){for(const g of glyphs(p))if(g.noteId)citations.push(g.content);for(const n of p.notes||[])placed.set(n.noteId,n.page)}
  assert.deepEqual(citations,['1','2','3','4','5','6']);assert.equal(placed.size,6);
  for(let i=1;i<=6;i++){
   const id='manydoc-note-'+i,note=data.notes[id],p=placed.get(id);assert.equal(note.showBodyReference,false);assert.ok(p);
   assert.equal(glyphs(p).filter(g=>g.count===0&&g.raw==='').length,0,'generated marker must stay suppressed');
   assert.equal(note.body.customRanges.filter(r=>r.properties?.profile==='manydoc.note-body-mark').length,1);
  }
  const first=glyphs(placed.get('manydoc-note-1')).find(g=>g.content===(edited?'8':'9'));assert.ok(first,'real literal body mark');assert.notEqual(first.ts.ul?.s,1,'ownership annotation must not decorate text');
  const rich=glyphs(placed.get('manydoc-note-2'));const star=rich.find(g=>g.content==='*'),two=rich.find(g=>g.content==='2');assert.ok(star&&two);assert.equal(star.ts.fs,edited?14:12);assert.equal(star.ts.bl,edited?0:1);assert.equal(star.ts.cl.rgb,'#FF0000');assert.notEqual(star.ts.ul?.s,1);assert.notEqual(two.ts.ul?.s,1);assert.equal(two.ts.fs,8);assert.equal(two.ts.it,1);
  const noteView=v.getNoteTreeMap().get('manydoc-note-2');assert.equal(noteView.getCustomRangeRaw(1).properties.url,'https://example.org/note');
  const style=glyphs(placed.get('manydoc-note-3')).find(g=>g.content==='3');assert.ok(style);assert.equal(style.ts.bl,1);assert.equal(style.ts.ul.s,1);assert.equal(style.ts.cl.rgb,'#006633');
  assert.equal(placed.get('manydoc-note-4').skeTables.size,1);assert.ok(glyphs(placed.get('manydoc-note-4')).some(g=>g.content==='9'));
  assert.ok(glyphs(placed.get('manydoc-note-5')).some(g=>g.content==='9'),'mark on otherwise empty body');
  assert.ok(!data.notes['manydoc-note-6'].body.dataStream.startsWith('\t'),'empty style does not insert a visible gap');
 }finally{s.dispose();v.dispose();m.dispose()}
}
function save(data){const u=new core.Univer();try{for(const p of[docs.UniverDocsPlugin,drawing.UniverDrawingPlugin,dd.UniverDocsDrawingPlugin])u.registerPlugin(p);return JSON.parse(JSON.stringify(FUniver.newAPI(u).createDocument(structuredClone(data)).save()))}finally{u.dispose()}}
async function main(){
 const data=JSON.parse(fs.readFileSync(path.join(qa,'body-marks.json')));
 const source=fs.readFileSync(path.resolve(__dirname,'../../../output/codec/colleaf-format-0.txt'),'utf8');assert.equal(crypto.createHash('sha256').update(source).digest('hex'),'c54b27b8c303713f5d5d5dfdb7cd54ec9b8410f5045613150861855b11c899c0');const app=await import('data:text/javascript;base64,'+Buffer.from(stripTypeScriptTypes(source)).toString('base64'));
 const validate=document=>app.validateDocument({type:'vibedada-document',version:3,engine:'univer',engineVersion:'1.0.4',document});
 // Reproduce the original failure: unneutralized CUSTOM annotations decorate
 // ordinary marker text, regardless of the authored red/color/underline fields.
 const raw=structuredClone(data),annotation=raw.notes['manydoc-note-2'].body.customRanges.find(r=>r.properties?.profile==='manydoc.note-body-mark');delete annotation.active;delete annotation.properties.textColorMode;
 const rm=new core.DocumentDataModel(raw),rv=new engine.DocumentViewModel(rm),rs=new engine.DocumentSkeleton(rv,new core.LocaleService());try{
  rs.calculate();let star;for(const p of rs.getSkeletonData().pages)for(const n of p.notes||[])if(n.noteId==='manydoc-note-2')for(const sec of n.page.sections)for(const c of sec.columns)for(const l of c.lines)for(const d of l.divides)star ||= d.glyphGroup.find(g=>g.content==='*');
  assert.ok(star);assert.equal(star.ts.cl.rgb,'blue.600');assert.equal(star.ts.ul.s,1);
 }finally{rs.dispose();rv.dispose();rm.dispose()}
 validate(data);inspect(data);const saved=save(save(data));validate(saved);inspect(saved);fs.writeFileSync(path.join(qa,'body-marks-saved.json'),JSON.stringify(saved,null,2));
 const edit=structuredClone(saved);edit.notes['manydoc-note-1'].body=core.TextX.apply(edit.notes['manydoc-note-1'].body,new core.TextX().delete(1).insert(1,{dataStream:'8'}).serialize());
 const first=edit.notes['manydoc-note-2'].body.textRuns.find(r=>r.st===0);first.ts.bl=0;first.ts.fs=14;
 const edited=save(edit);validate(edited);inspect(edited,true);fs.writeFileSync(path.join(qa,'body-marks-edited.json'),JSON.stringify(edited,null,2));
 console.log('Pinned real literal/rich/styled/separate/table/empty note body marks, actual glyph styles/link/body placement and TextX/style edits plus facade save pass; browser paint and wrapping unqualified');
}
main().catch(e=>{console.error(e);process.exitCode=1});
