"""Independent native shaping/GDEF/clip/color/link oracle, QA only."""
import hashlib
from font_oracles import verify_pdf_glyph_program
from pathlib import Path
from collections import defaultdict
import json
from zipfile import ZipFile
from lxml import etree
from docx import Document
from pypdf import PdfReader
from pypdf.generic import ContentStream
import pymupdf
import uharfbuzz as hb

out=Path('output/paint-context');original=Path('testdata/assets/NotoSans-Regular.ttf').read_bytes()
face=hb.Face(original);font=hb.Font(face)
word=Document(out/'paint.docx')
assert [p.text for p in word.paragraphs]==['AV','office']
assert [(r.text,str(r.font.color.rgb),r.underline,r.font.strike) for r in word.paragraphs[1].runs]==[('of','FF0000',True,None),('i','008800',None,None),('ce','000000',None,None)]
assert word.paragraphs[1].hyperlinks[0].text=='f'
assert word.tables[0].cell(0,0).text=='office'
with ZipFile(out/'paint.docx') as z:
 ns={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}
 root=etree.fromstring(z.read('word/document.xml'))
 assert root.xpath('//w:hyperlink/w:r/w:rPr/w:strike',namespaces=ns)
 assert b'https://example.invalid/ligature' in z.read('word/_rels/document.xml.rels')
source=json.loads((out/'paint.sf').read_text());recovered=json.loads((out/'recovered.sf').read_text())
def all_nodes(n):
 yield n
 for child in n.get('content',[]):yield from all_nodes(child)
blue=[n for n in all_nodes(recovered) if n.get('text')=='f' and any(m['type']=='link' for m in n.get('marks',[]))]
assert len(blue)==2
for n in blue:
 marks={m['type']:m.get('attrs',{}) for m in n['marks']}
 assert marks['textStyle']['color']=='#0000FF' and marks['link']['href']=='https://example.invalid/ligature'
 assert marks['source']['precision']=='replacement_span_bounds' and marks['source']['render_mode']==3

rows_checked=0;clips_checked=0
for name in ['paint','emergency']:
 r=PdfReader(out/f'{name}.pdf',strict=True);assert len(r.pages)==1
 page=r.pages[0];fonts={}
 for key,obj in page['/Resources']['/Font'].items():
  if not str(key).startswith('/MDG'):continue
  f=obj.get_object();cid=f['/DescendantFonts'][0].get_object();desc=cid['/FontDescriptor']
  verify_pdf_glyph_program(f,original)
  widths={};values=cid['/W'];j=0
  while j<len(values):
   start=int(values[j]);j+=1
   if isinstance(values[j],list):
    for k,value in enumerate(values[j]):widths[start+k]=float(value)
    j+=1
   else:
    end=int(values[j]);value=float(values[j+1]);j+=2
    for code in range(start,end+1):widths[code]=value
  fonts[str(key)]=(cid['/CIDToGIDMap'].get_data(),widths)
 rows=defaultdict(lambda:{'logical':[],'glyphs':[],'parts':[]});stack=[];clip=None;rgb=(0,0,0);active=None;part=[];which=None;point=None;size=None;render_mode=0;artifact=False;pending_rect=None;pending_clip=False
 for args,op in ContentStream(page.get_contents(),r).operations:
  if op==b'q':stack.append((clip,rgb,render_mode))
  elif op==b'Q':clip,rgb,render_mode=stack.pop()
  elif op==b're':
   x,y,w,h=map(float,args);pending_rect=(x,y,x+w,y+h)
  elif op in (b'W',b'W*'):pending_clip=True
  elif op in (b'n',b'S',b'f',b'f*',b'B',b'b'):
   if pending_clip:clip=pending_rect;clips_checked+=1
   pending_rect=None;pending_clip=False
  elif op==b'rg':rgb=tuple(map(float,args))
  elif op==b'Tr':render_mode=int(args[0])
  elif op==b'BDC':assert active is None;active=str(args[1]['/ActualText']);artifact=str(args[0])=='/Artifact';part=[]
  elif op==b'Tf':which=str(args[0]);size=float(args[1])
  elif op==b'Tm':point=(float(args[4]),float(args[5]))
  elif op==b'Tj' and active is not None:
   raw=args[0].original_bytes if hasattr(args[0],'original_bytes') else bytes(args[0]);assert len(raw)==2
   code=int.from_bytes(raw,'big');mapping,widths=fonts[which]
   part.append((int.from_bytes(mapping[2*code:2*code+2],'big'),point,size,rgb,widths[code]))
  elif op==b'EMC':
   assert active is not None and part
   row=rows[part[0][1][1]]
   if artifact:
    assert active==''
    row['glyphs']+=part;row['parts'].append((None,clip,part))
   elif render_mode==3:
    assert row['parts'][-1][0] is None and len(part)==1 and part[0][0]==3
    previous=row['parts'][-1]
    # The invisible carrier's nominal metric spans its actual logical paint band.
    expected_width=previous[1][2]-previous[1][0] if previous[1][0]>0 and previous[1][2]<float(page.mediabox.width) else None
    if expected_width is not None:assert abs(part[0][4]*size/1000-expected_width)<.000001
    row['parts'][-1]=(active,previous[1],previous[2]);row['logical'].append(active)
   else:
    row['logical'].append(active);row['glyphs']+=part;row['parts'].append((active,clip,part))
   active=None
 assert not stack
 for baseline,row in rows.items():
  text=''.join(row['logical']);buf=hb.Buffer();buf.add_str(text);buf.guess_segment_properties();buf.language='und';hb.shape(font,buf)
  unique=[];seen=set()
  for g in row['glyphs']:
   key=(g[0],round(g[1][0],6),round(g[1][1],6))
   if key not in seen:seen.add(key);unique.append(g)
  assert [g[0] for g in unique]==[g.codepoint for g in buf.glyph_infos],(name,text,unique)
  x=0;origin=unique[0][1][0];point_size=unique[0][2]
  for g,pos in zip(unique,buf.glyph_positions):
   assert abs(g[1][0]-origin-(x+pos.x_offset)*point_size/face.upem)<.000001,(name,text,g,pos)
   x+=pos.x_advance
  if text=='office':
   ffi=next(g for g in unique if g[0]==1654);base=ffi[1][0]
   left,right=base+315*point_size/face.upem,base+631*point_size/face.upem
   # Original Noto GDEF has f_f_i carets 315/631 (format 1 design units).
   p0,p1,p2,p3=row['parts'];assert [p[0] for p in row['parts']]==['of','f','i','ce']
   assert abs(p0[1][2]-left)<.000001
   assert abs(p1[1][0]-left)<.000001 and abs(p1[1][2]-right)<.000001
   assert abs(p2[1][0]-right)<.000001 and p3[1] is None
   for part,expected in zip(row['parts'],[(1,0,0),(0,0,1),(0,136/255,0),(0,0,0)]):assert all(abs(a-b)<.00000001 for a,b in zip(part[2][0][3],expected))
  rows_checked+=1
 if name=='paint':
  assert page.extract_text().count('office')==2
  assert len([a for a in page['/Annots'] if a.get_object().get('/A',{}).get('/URI')=='https://example.invalid/ligature'])==2
 else:assert page.extract_text().replace('\n','')=='ffi'
 with pymupdf.open(out/f'{name}.pdf') as doc:
  if name=='paint':assert doc[0].get_text().count('office')==2
  else:assert doc[0].get_text().replace('\n','')=='ffi'
  pix=doc[0].get_pixmap(matrix=pymupdf.Matrix(2,2));pix.save(out/f'{name}-page.png')
  pixels=pix.samples
  assert any(pixels[i]>200 and pixels[i+1]<30 and pixels[i+2]<30 for i in range(0,len(pixels),pix.n))
  assert any(pixels[i]<30 and pixels[i+1]<30 and pixels[i+2]>200 for i in range(0,len(pixels),pix.n))
print(f'Paint context passed: {rows_checked} native-HarfBuzz rows, {clips_checked} GDEF caret clips, original program/GIDs/positions, partial-ligature colors, both-reader logical text, links and carrier/clip provenance')
