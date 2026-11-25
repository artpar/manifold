/* Optional pinned zero/legal glyphs, source policy and facade-save qualification. */
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const {createRequire}=require('node:module'),root=path.resolve(__dirname,'../../..'),qa=path.join(root,'output/vdocs-qa'),req=createRequire(path.join(qa,'package.json'));
const c=req('@univerjs/core'),e=req('@univerjs/engine-render'),docs=req('@univerjs/docs'),drawing=req('@univerjs/drawing'),dd=req('@univerjs/docs-drawing'),{FUniver}=req('@univerjs/core/facade');req('@univerjs/docs/facade');
for(const n of ['core','engine-render','docs','drawing','docs-drawing'])assert.equal(JSON.parse(fs.readFileSync(path.join(qa,'node_modules/@univerjs',n,'package.json'))).version,'1.0.4');
function markers(document){
 const m=new c.DocumentDataModel(document),v=new e.DocumentViewModel(m),s=new e.DocumentSkeleton(v,new c.LocaleService()),labels=[];
 function walk(p){for(const sec of p.sections)for(const col of sec.columns)for(const l of col.lines)for(const d of l.divides)for(const g of d.glyphGroup)if(g.glyphType===e.GlyphType.LIST)labels.push(g.content);for(const t of p.skeTables.values())for(const r of t.rows)for(const cell of r.cells)walk(cell)}
 try{s.calculate();for(const p of s.getSkeletonData().pages)walk(p);return labels}finally{s.dispose();v.dispose();m.dispose()}
}
const expected=['0%','01%','02%','0%','A%','B%','0%','a%','b%','0%','I%','II%','0%','i%','ii%','0.','1.','2.','0.','0.0','0.i','0.ii','A.','B.','0.0','0.1','0.2','0%','01%','02%'];
function inspect(data,edited=false){
 const wanted=expected.slice();if(edited){wanted.splice(0,3,'5%','06%','07%');wanted.splice(15,3,'00.','01.','02.');wanted.splice(18,6,'5.','5.0','5.i','5.ii','F.','G.')}
 assert.deepEqual(markers(data),wanted);
 const note=Object.values(data.notes)[0];assert.deepEqual(markers({id:'note',body:note.body,lists:note.lists,documentStyle:data.documentStyle}),['0%','I%','II%']);
 const header=Object.values(data.headers)[0];assert.deepEqual(markers({id:'header',body:header.body,lists:data.lists,documentStyle:data.documentStyle}),['0%','i%','ii%']);
 const bullets=data.body.paragraphs.filter(p=>p.bullet).map(p=>p.bullet);assert.equal(bullets[0].listId,bullets[1].listId);assert.notEqual(bullets[0].listType,bullets[1].listType);
 for(const b of bullets)assert.equal(data.lists[b.listType].nestingLevel[b.nestingLevel].glyphSymbol,undefined,'marker strings must not be frozen');
}
function save(data){const u=new c.Univer();try{for(const p of [docs.UniverDocsPlugin,drawing.UniverDrawingPlugin,dd.UniverDocsDrawingPlugin])u.registerPlugin(p);return JSON.parse(JSON.stringify(FUniver.newAPI(u).createDocument(structuredClone(data)).save()))}finally{u.dispose()}}
async function main(){

 const data=JSON.parse(fs.readFileSync(path.join(qa,'zero-lists.json')));inspect(data);
 // Independent foreign snapshots retain the pinned engine's own zero policy,
 // which intentionally differs from the canonical SF zero convention.
 for(const glyph of [3,4,5,6,7])for(const legal of [false,true]){
  const body={dataStream:'X\rY\rZ\r\n',textRuns:[],paragraphs:[1,3,5].map(startIndex=>({startIndex,bullet:{listType:'t',listId:'g',nestingLevel:0}})),sectionBreaks:[{startIndex:6}]};
  const foreign={id:'foreign',body,lists:{t:{listType:'t',nestingLevel:[{glyphFormat:'%1.',glyphType:glyph,startNumber:-1,bulletAlignment:1,isLegal:legal}]}},documentStyle:{documentFlavor:1,pageSize:{width:800,height:1000}}};

  const labels=glyph===3?['00.','01.','02.']:legal?['0.','1.','2.']:glyph===4?['.','A.','B.']:glyph===5?['.','a.','b.']:glyph===6?['.','I.','II.']:['.','i.','ii.'];
  assert.deepEqual(markers(foreign),labels);
 }
 const saved=save(save(data));inspect(saved);fs.writeFileSync(path.join(qa,'zero-lists-saved.json'),JSON.stringify(saved));
 const edited=structuredClone(saved),bullets=edited.body.paragraphs.filter(p=>p.bullet).map(p=>p.bullet);bullets[0].startNumber=4;bullets[18].startNumber=4;
 const legal=edited.lists[bullets[15].listType].nestingLevel[0];assert.equal(legal.isLegal,true);assert.equal(legal.glyphType,2);legal.glyphType=3;
 const final=save(edited);inspect(final,true);fs.writeFileSync(path.join(qa,'zero-lists-edited.json'),JSON.stringify(final));
 console.log('Pinned 30 main/cell zero/legal/ancestor markers plus 3 note and 3 header markers, 10 foreign zero/legal policies, shared automatic counters, native counter/glyph edits and actual facade save/reopen pass; browser/preset interaction unqualified');
}
main().catch(error=>{console.error(error);process.exitCode=1});
