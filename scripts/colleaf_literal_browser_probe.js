/* Evaluate this owned QA function in the pinned Colleaf browser harness.
 * Requires generated nested.vdocs/stories.vdocs beside document.vdocs.
 * Strict mode retains the missing-red-image pixel oracle; do not weaken it.
 */
async function probeLiteralBrowser(name = 'nested', strict = true) {
 const wire=await (await fetch('/'+name+'.vdocs')).json();
 await qa.mount(wire);await new Promise(resolve=>setTimeout(resolve,500));
 const documentData=qa.snapshot().document;
 const drawings=Object.values(documentData.drawings||{});
 for(const story of [...Object.values(documentData.headers||{}),...Object.values(documentData.footers||{}),...Object.values(documentData.notes||{})])drawings.push(...Object.values(story.drawings||{}));
 const source=drawings.find(d=>d.description==='RED_LABEL');
 if(!source)throw Error('Expected actual RED_LABEL drawing source');
 const image=new Image();image.src=source.source;await image.decode();
 const sample=document.createElement('canvas');sample.width=sample.height=1;
 sample.getContext('2d').drawImage(image,0,0);const rgba=[...sample.getContext('2d').getImageData(0,0,1,1).data];
 if(rgba.join(',')!=='255,0,0,255')throw Error('Reference source pixel changed');
 let redPixels=0;
 for(const canvas of document.querySelectorAll('canvas')){
  const ctx=canvas.getContext('2d');if(!ctx)continue;
  const p=ctx.getImageData(0,0,canvas.width,canvas.height).data;
  for(let i=0;i<p.length;i+=4)if(p[i]>200&&p[i+1]<60&&p[i+2]<60&&p[i+3]>200)redPixels++;
 }
 const result={name,sourceDecodedRGBA:rgba,redPixels,drawings:drawings.length,errors:qa.errors,notes:qa.renderedNotes()};
 if(strict&&redPixels===0)throw Error('Native literal image was not painted: '+JSON.stringify(result));
 return result;
}
