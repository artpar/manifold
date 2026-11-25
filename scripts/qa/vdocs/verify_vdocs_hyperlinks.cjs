/* Optional pinned hyperlink plugin/resource and unmodified external command QA. */
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),vm=require('node:vm');
const {createRequire,stripTypeScriptTypes}=require('node:module'),root=path.resolve(__dirname,'../../..'),qa=path.join(root,'output/colleaf-qa'),req=createRequire(path.join(qa,'package.json'));
const c=req('@univerjs/core'),e=req('@univerjs/engine-render'),docs=req('@univerjs/docs'),drawing=req('@univerjs/drawing'),dd=req('@univerjs/docs-drawing'),hyper=req('@univerjs/docs-hyper-link'),{FUniver}=req('@univerjs/core/facade');req('@univerjs/docs/facade');
for(const n of ['core','engine-render','docs','drawing','docs-drawing','docs-hyper-link'])assert.equal(JSON.parse(fs.readFileSync(path.join(qa,'node_modules/@univerjs',n,'package.json'))).version,'1.0.4');
function pinned(file,hash){const source=fs.readFileSync(file,'utf8');assert.equal(crypto.createHash('sha256').update(source).digest('hex'),hash);return source}
pinned(path.join(qa,'node_modules/@univerjs/docs-hyper-link/lib/cjs/index.js'),'28566260eb368abfc45fad97045943dc23bbaa54654cd9f955e51926168a4da4');
const ui=pinned(path.join(root,'output/codec/docs-hyper-link-ui-1.0.4/package/lib/cjs/index.js'),'a5055ae57c52a0f593e2e10e10b3ae39612f9df3bcf84ae4c9f8fc77ba7ac6fb');
const commandStart=ui.indexOf('const ClickDocHyperLinkOperation ='),commandEnd=ui.indexOf('//#endregion',commandStart);assert.ok(commandStart>0&&commandEnd>commandStart);
const expected=new Map([['Main 😀','https://example.org/main'],['Cell','https://example.org/cell'],['List','https://example.org/list'],['Header','https://example.org/header'],['Footer','https://example.org/footer']]);
function links(data){const all=[];for(const [segmentId,body]of [[undefined,data.body],...Object.entries(data.headers||{}).map(([id,s])=>[id,s.body]),...Object.entries(data.footers||{}).map(([id,s])=>[id,s.body])])for(const r of body.customRanges||[])if(r.rangeType===0)all.push({segmentId,r,label:body.dataStream.slice(r.startIndex,r.endIndex+1)});return all}
function withDocument(data,fn){const u=new c.Univer();try{for(const p of [docs.UniverDocsPlugin,drawing.UniverDrawingPlugin,dd.UniverDocsDrawingPlugin,hyper.UniverDocsHyperLinkPlugin])u.registerPlugin(p);const f=FUniver.newAPI(u).createDocument(structuredClone(data));return fn(u,f)}finally{u.dispose()}}
function inspectLoaded(u,f,edited){
 const data=f.save(),instance=u.__getInjector().get(c.IUniverInstanceService),model=instance.getUnit(data.id,c.UniverInstanceType.UNIVER_DOC),records=links(data);
 assert.equal(records.length,5);assert.equal(new Set(records.map(x=>x.r.rangeId)).size,5);
 const opened=[],sandbox={_univerjs_core:c,_univerjs_docs:docs,_univerjs_engine_render:e,_univerjs_docs_ui:{},window:{open:(...args)=>opened.push(args)}};
 // Execute the exact hash-pinned command/helper region, without loading UI widgets
 // or substituting a reimplementation of target selection. window.open is a stub.
 vm.runInNewContext(ui.slice(commandStart,commandEnd)+';globalThis.operation=ClickDocHyperLinkOperation;',sandbox);
 const accessor={get:token=>u.__getInjector().get(token)};
 for(const {segmentId,r,label}of records){const target=edited&&label==='Header'?'https://example.org/header-edited':expected.get(label);assert.equal(r.properties.url,target);assert.equal(r.properties.textStyleMode,'text');assert.equal(sandbox.operation.handler(accessor,{unitId:data.id,linkId:r.rangeId,segmentId}),true);assert.deepEqual(opened.at(-1),[target,'_blank','noopener noreferrer'])}
 const resource=data.resources.find(r=>r.name==='DOC_HYPER_LINK_PLUGIN');assert.ok(resource);const entries=JSON.parse(resource.data).links;assert.equal(entries.length,5);
 for(const {r}of records)assert.equal(entries.find(e=>e.id===r.rangeId).payload,r.properties.url);
 const note=Object.values(data.notes)[0],noteLink=note.body.customRanges.find(r=>r.rangeType===0);assert.equal(noteLink.properties.url,'https://example.org/note');assert.equal(entries.some(x=>x.payload==='https://example.org/note'),false,'pinned plugin does not visit note bodies');
 const view=new e.DocumentViewModel(model),skeleton=new e.DocumentSkeleton(view,new c.LocaleService());
 try{skeleton.calculate();const glyphs=[];for(const p of skeleton.getSkeletonData().pages)for(const s of p.sections)for(const col of s.columns)for(const l of col.lines)for(const d of l.divides)glyphs.push(...d.glyphGroup);const authored=glyphs.find(g=>g.content==='M');assert.ok(authored);assert.equal(authored.ts.fs,13);assert.equal(authored.ts.bl,1);assert.equal(authored.ts.cl.rgb,'#123456');assert.equal(authored.ts.ul.s,0)}finally{skeleton.dispose();view.dispose()}
 return data;
}
async function main(){
 const source=pinned(path.join(root,'output/codec/colleaf-format-0.txt'),'c54b27b8c303713f5d5d5dfdb7cd54ec9b8410f5045613150861855b11c899c0'),app=await import('data:text/javascript;base64,'+Buffer.from(stripTypeScriptTypes(source)).toString('base64'));
 const valid=document=>app.validateDocument({type:'vibedada-document',version:3,engine:'univer',engineVersion:'1.0.4',document}),data=JSON.parse(fs.readFileSync(path.join(qa,'hyperlinks.json')));valid(data);
 const first=withDocument(data,(u,f)=>inspectLoaded(u,f,false));valid(first);
 const saved=withDocument(first,(u,f)=>inspectLoaded(u,f,false));valid(saved);fs.writeFileSync(path.join(qa,'hyperlinks-saved.json'),JSON.stringify(saved));
 const resourceEdited=structuredClone(saved),header=links(resourceEdited).find(x=>x.label==='Header'),resource=resourceEdited.resources.find(r=>r.name==='DOC_HYPER_LINK_PLUGIN'),payload=JSON.parse(resource.data);payload.links.find(e=>e.id===header.r.rangeId).payload='https://example.org/header-edited';resource.data=JSON.stringify(payload);
 assert.equal(header.r.properties.url,'https://example.org/header');fs.writeFileSync(path.join(qa,'hyperlinks-resource-edited.json'),JSON.stringify(resourceEdited));
 const editedSaved=withDocument(resourceEdited,(u,f)=>inspectLoaded(u,f,true));valid(editedSaved);fs.writeFileSync(path.join(qa,'hyperlinks-edited-saved.json'),JSON.stringify(editedSaved));
 console.log('Pinned five main/cell/list/header/footer links, note-local URL, authored glyph style, actual hyperlink resource save/reopen/URL-only edit and unmodified external handler targets pass; window.open is stubbed, browser/UI/internal navigation unqualified');
}
main().catch(error=>{console.error(error);process.exitCode=1});
