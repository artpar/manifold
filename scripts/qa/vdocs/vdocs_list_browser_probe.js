/* Evaluate in the pinned unmodified app harness. qa.skeletonData is QA-only.
 * Glyph checks include actual table-cell pagination; screenshots qualify paint.
 */
async function probeNativeLists(name='nested',mobile=false) {
 const wire=await(await fetch('/'+name+'.vdocs',{cache:'no-store'})).json();
 await qa.mount(wire,mobile);await new Promise(r=>setTimeout(r,700));
 function glyphs(p,out){
  for(const s of p.sections||[])for(const c of s.columns)for(const l of c.lines)for(const d of l.divides)for(const g of d.glyphGroup)out.push({content:g.content||'',type:g.glyphType,padding:l.paragraphPaddingLeft,advance:g.width,contentWidth:g.bBox?.width||0});
  for(const t of p.skeTables?.values()||[])for(const r of t.rows)for(const c of r.cells)glyphs(c,out);
  for(const n of p.notes||[])glyphs(n.page,out);
 }
 const data=qa.skeletonData(),pages=data.pages.map(p=>{const g=[];glyphs(p,g);return g;});
 const text=pages.flat().map(g=>g.content).join(''),counters=pages.flat().filter(g=>g.type===2);
 if(name==='nested'&&(!text.includes('INNER_LIST')||!counters.some(g=>g.content==='1.'&&g.padding===21)))throw Error('Nested ordinary counter/indent lost');
 if(counters.some(g=>g.advance<g.contentWidth))throw Error('Native counter frame clips its measured glyph');
 if(name==='mixed'&&counters.map(g=>g.content).join('|')!=='Item 7)|Item 9)')throw Error('Automatic counter display changed');
 if(name==='numbering'&&counters.map(g=>g.content).join('|')!=='Appendix AA.|AA.iv)|Appendix BB.|BB.iv)|2.4.7|[MMMM]|(aaa)|(bb)')throw Error('Original rich counter oracle changed');
 if(name==='continued'){
  if(pages.length<2)throw Error('Fixture failed to exercise actual pagination');
  for(let i=0;i<48;i++)for(const kind of ['LABEL','BODY']){
   const token=kind+'_LINE_'+String(i).padStart(2,'0');
   if(text.split(token).length-1!==1)throw Error('Missing/repeated physical token '+token);
  }
  for(const page of pages){const t=page.map(g=>g.content).join('');if(!t.includes('LABEL_LINE_')||!t.includes('BODY_LINE_'))throw Error('Label/body continuation lost');}
 }
 if(qa.errors.length)throw Error(qa.errors.join('\n'));
 return {name,mobile,pages:pages.length,counters: counters.map(g=>g.content),tokens:name==='continued'?96:undefined,errors:qa.errors};
}
