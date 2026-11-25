/* Optional pinned native counter/glyph/body-mark and real facade-save QA. */
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),{createRequire,stripTypeScriptTypes}=require('node:module');
const qa=path.resolve(__dirname,'../../../output/colleaf-qa'),req=createRequire(path.join(qa,'package.json'));
const core=req('@univerjs/core'),engine=req('@univerjs/engine-render'),docs=req('@univerjs/docs'),drawing=req('@univerjs/drawing'),dd=req('@univerjs/docs-drawing'),{FUniver}=req('@univerjs/core/facade');req('@univerjs/docs/facade');
for(const name of ['core','engine-render','docs','drawing','docs-drawing'])assert.equal(JSON.parse(fs.readFileSync(path.join(qa,'node_modules/@univerjs',name,'package.json'))).version,'1.0.4');
const expected=[['f1','III'],['e1','AA'],['shared','IV'],['shared','V'],['custom','§'],['f2','VI'],['f3','00'],['e2','BB'],['shared','01'],['silent','']];
function inspect(input){
 const m=new core.DocumentDataModel(input),v=new engine.DocumentViewModel(m),s=new engine.DocumentSkeleton(v,new core.LocaleService());
 try{
  assert.equal(m.documentStyle.marginHeader,30);assert.equal(m.documentStyle.marginFooter,30);
  s.calculate();const refs=[],bodies=new Map();
  function glyphs(p){const out=[];for(const sec of p.sections)for(const c of sec.columns)for(const l of c.lines)for(const d of l.divides)out.push(...d.glyphGroup);return out}
  function main(p){for(const g of glyphs(p))if(g.noteId)refs.push([g.noteId,g.content]);for(const t of p.skeTables.values())for(const r of t.rows)for(const c of r.cells)main(c)}
  for(const p of s.getSkeletonData().pages){main(p);for(const n of p.notes||[]){const marker=glyphs(n.page).find(g=>g.count===0&&g.raw==='');assert.ok(marker,'generated note body mark');const seen=bodies.get(n.noteId)||[];seen.push(marker.content);bodies.set(n.noteId,seen)}}
  assert.deepEqual(refs,expected);
  for(const [id,label]of [['f1','III'],['e1','AA'],['shared','01'],['custom','§'],['f2','VI'],['f3','00'],['e2','BB'],['silent','']]){assert.ok(bodies.has(id),id+' body placement');assert.ok(bodies.get(id).every(x=>x===label),id+' generated body mark: '+JSON.stringify(bodies.get(id)))}
 }finally{s.dispose();v.dispose();m.dispose()}
}
function save(data){const u=new core.Univer();try{for(const plugin of[docs.UniverDocsPlugin,drawing.UniverDrawingPlugin,dd.UniverDocsDrawingPlugin])u.registerPlugin(plugin);return JSON.parse(JSON.stringify(FUniver.newAPI(u).createDocument(structuredClone(data)).save()))}finally{u.dispose()}}
async function main(){
 const raw=JSON.parse(fs.readFileSync(path.join(qa,'note-numbering.json')));
 const source=fs.readFileSync(path.resolve(__dirname,'../../../output/codec/colleaf-format-0.txt'),'utf8');assert.equal(crypto.createHash('sha256').update(source).digest('hex'),'c54b27b8c303713f5d5d5dfdb7cd54ec9b8410f5045613150861855b11c899c0');const app=await import('data:text/javascript;base64,'+Buffer.from(stripTypeScriptTypes(source)).toString('base64'));
 const validate=document=>app.validateDocument({type:'vibedada-document',version:3,engine:'univer',engineVersion:'1.0.4',document});
 // Pinned ideographEnclosedCircle uses parenthesized ideographs despite its name.
 for(const [format,value,label]of [['decimalZero',0,'00'],['lowerRoman',0,''],['upperLetter',0,''],['upperRoman',14,'XIV'],['lowerLetter',27,'aa'],['chicago',7,'**'],['decimalFullWidth',12,'１２'],['decimalFullWidth2',0,'０'],['ideographDigital',10,'一〇'],['taiwaneseDigital',0,'〇'],['decimalEnclosedCircle',36,'㊱'],['decimalEnclosedCircle',51,'51'],['ideographEnclosedCircle',10,'㈩'],['ideographTraditional',10,'癸'],['ideographZodiac',12,'亥']]){
  const data={...structuredClone(raw),noteSettings:{footnote:{numberFormat:format,startNumber:value}},notes:{n:{noteId:'n',type:'footnote',body:{dataStream:'Body\r\n',textRuns:[],paragraphs:[{startIndex:4}]}}},body:{dataStream:'\ufffc\r\n',textRuns:[],paragraphs:[{startIndex:1}],sectionBreaks:[{startIndex:2}],customRanges:[{rangeId:'ref',rangeType:8,startIndex:0,endIndex:0,wholeEntity:true,properties:{noteId:'n'}}]}};
  const m=new core.DocumentDataModel(data),v=new engine.DocumentViewModel(m),s=new engine.DocumentSkeleton(v,new core.LocaleService());try{
   s.calculate();const labels=[];for(const p of s.getSkeletonData().pages)for(const sec of p.sections)for(const c of sec.columns)for(const l of c.lines)for(const d of l.divides)for(const g of d.glyphGroup)if(g.noteId)labels.push(g.content);
   assert.deepEqual(labels,[label],format+' native counter outcome');
  }finally{s.dispose();v.dispose();m.dispose()}
 }
 validate(raw);inspect(raw);const saved=save(save(raw));validate(saved);inspect(saved);fs.writeFileSync(path.join(qa,'note-numbering-saved.json'),JSON.stringify(saved,null,2));
 console.log('Pinned native automatic Roman/letter/zero/section restart/repeated/markerless citations and actual body marks plus facade save pass; browser paint and per-page automatic numbering unqualified');
}
main().catch(error=>{console.error(error);process.exitCode=1});
