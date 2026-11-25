/* Optional pinned literal-marker glyph/counter and actual facade-save QA. */
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const {createRequire,stripTypeScriptTypes}=require('node:module');
const root=path.resolve(__dirname,'../../..'),qa=path.join(root,'output/colleaf-qa'),req=createRequire(path.join(qa,'package.json'));
const c=req('@univerjs/core'),e=req('@univerjs/engine-render'),docs=req('@univerjs/docs'),drawing=req('@univerjs/drawing'),dd=req('@univerjs/docs-drawing'),{FUniver}=req('@univerjs/core/facade');req('@univerjs/docs/facade');
for(const n of ['core','engine-render','docs','drawing','docs-drawing'])assert.equal(JSON.parse(fs.readFileSync(path.join(qa,'node_modules/@univerjs',n,'package.json'))).version,'1.0.4');
function markers(document){
 const m=new c.DocumentDataModel(document),v=new e.DocumentViewModel(m),s=new e.DocumentSkeleton(v,new c.LocaleService()),labels=[];
 function walk(page){for(const sec of page.sections)for(const col of sec.columns)for(const line of col.lines)for(const d of line.divides)for(const g of d.glyphGroup)if(g.glyphType===e.GlyphType.LIST)labels.push(g.content);for(const table of page.skeTables.values())for(const row of table.rows)for(const cell of row.cells)walk(cell)}
 try{s.calculate();for(const p of s.getSkeletonData().pages)walk(p);return labels}finally{s.dispose();v.dispose();m.dispose()}
}
const expected=['%2%','2/iv%0','2/v%0','%3%','10','20','50% literal','50% literal','%1.2.3.4.5.6.7.8.9%','%1.2.3.4.5.6.7.8.10%','12','22'];
function inspect(data,edited=false){
 const labels=expected.slice();if(edited)labels.splice(0,4,'‰5‰','5/iv%0','5/v%0','‰6‰');assert.deepEqual(markers(data),labels);
 const note=Object.values(data.notes)[0];assert.deepEqual(markers({id:'note',body:note.body,lists:note.lists,documentStyle:data.documentStyle}),['note% 0%','note% 1%']);
 const header=Object.values(data.headers)[0];assert.deepEqual(markers({id:'header',body:header.body,lists:data.lists,documentStyle:data.documentStyle}),['header% 1','header% 2']);
 for(const p of data.body.paragraphs.filter(p=>p.bullet))assert.ok(p.bullet.nestingLevel<=8,'helpers must never be active levels');
 assert.ok(Object.values(data.lists).some(l=>l.nestingLevel.length>9));
}
function save(data){const u=new c.Univer();try{for(const p of [docs.UniverDocsPlugin,drawing.UniverDrawingPlugin,dd.UniverDocsDrawingPlugin])u.registerPlugin(p);return JSON.parse(JSON.stringify(FUniver.newAPI(u).createDocument(structuredClone(data)).save()))}finally{u.dispose()}}
async function main(){
 const source=fs.readFileSync(path.join(root,'output/codec/colleaf-format-0.txt'),'utf8');assert.equal(crypto.createHash('sha256').update(source).digest('hex'),'c54b27b8c303713f5d5d5dfdb7cd54ec9b8410f5045613150861855b11c899c0');const app=await import('data:text/javascript;base64,'+Buffer.from(stripTypeScriptTypes(source)).toString('base64'));
 const valid=document=>app.validateDocument({type:'vibedada-document',version:3,engine:'univer',engineVersion:'1.0.4',document});
 const data=JSON.parse(fs.readFileSync(path.join(qa,'literal-lists.json')));valid(data);inspect(data);
 const saved=save(save(data));valid(saved);inspect(saved);fs.writeFileSync(path.join(qa,'literal-lists-saved.json'),JSON.stringify(saved));
 const edited=structuredClone(saved),p=edited.body.paragraphs.find(p=>p.bullet),list=edited.lists[p.bullet.listType];p.bullet.startNumber=4;assert.equal(list.nestingLevel[9].glyphSymbol,'%');list.nestingLevel[9].glyphSymbol='‰';
 const final=save(edited);valid(final);inspect(final,true);fs.writeFileSync(path.join(qa,'literal-lists-edited.json'),JSON.stringify(final));
 console.log('Pinned literal percent/digit suffix/constant/nine-level/native-auto-counter glyphs in main/nested/cell/note/header stories, helper/counter edits and actual facade save/reopen pass; browser/preset controls unqualified');
}
main().catch(error=>{console.error(error);process.exitCode=1});
