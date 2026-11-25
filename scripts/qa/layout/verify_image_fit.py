"""Optional independent image fitting, pixels, geometry and note-link QA."""
from pathlib import Path
from zipfile import ZipFile
import argparse,base64,io,json
from docx import Document
from lxml import etree
from PIL import Image
from pypdf import PdfReader
import pymupdf

parser=argparse.ArgumentParser();parser.add_argument('directory',nargs='?',type=Path,default=Path('output/image-fit'));parser.add_argument('--office',action='store_true')
args=parser.parse_args();out=args.directory
sf=json.loads((out/'image-fit.sf').read_text())
body=sf['content'][0];nested=sf['content'][2]['content'][0]['content'][0]['content'][0]['content'][0]['content'][0]['content'][0]
first=sf['attrs']['notes'][0]['content'][0]
last=sf['attrs']['notes'][1]['content'][0]['content'][0]['content'][0]['content'][0]['content'][0]['content'][0]['content'][0]
assert [(n['attrs']['width'],n['attrs']['height']) for n in (body,nested,first,last)]==[(0,0),(80,0),(0,2000),(60,600)]
blob=base64.b64decode(body['attrs']['src'].split(',',1)[1]);original=Image.open(io.BytesIO(blob)).convert('RGB');assert original.size==(64,640)
reader=PdfReader(out/'image-fit.pdf',strict=True);assert len(reader.pages)==7
links=[]
for i,page in enumerate(reader.pages):
 for ref in page.get('/Annots',[]):
  a=ref.get_object();assert '/Dest' in a
  target=next(j for j,p in enumerate(reader.pages) if p.indirect_reference.idnum==a['/Dest'][0].idnum)
  links.append((i+1,target+1))
assert links==[(3,4),(3,4),(5,6),(5,6)],links
ns={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main','wp':'http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing'}
with ZipFile(out/'image-fit.docx') as z:
 media=[z.read(n) for n in z.namelist() if n.startswith('word/media/')]
 assert len(media)==4 and all(b==blob for b in media)
 ext=[]
 for name in ('word/document.xml','word/footnotes.xml'):
  n=etree.fromstring(z.read(name));ext.extend((int(v.get('cx'))/12700,int(v.get('cy'))/12700) for v in n.xpath('//wp:extent',namespaces=ns))
 assert ext==[(15.2,152),(12.4,124),(9.6,96),(6.8,68)],ext
word=Document(out/'image-fit.docx');outer=word.tables[0]
assert outer.cell(0,0)._tc is outer.cell(1,0)._tc
assert len(outer.cell(0,0).tables)==1
expected={0:(20,20,35.2,172),1:(32,32,44,152),3:(38,42,51,172),5:(50,54,59,144)}
with pymupdf.open(out/'image-fit.pdf') as doc:
 assert [len(p.get_image_info()) for p in doc]==[1,1,0,1,0,1,0]
 for i,p in enumerate(doc):
  for info in p.get_image_info(xrefs=True):
   assert all(abs(a-b)<.03 for a,b in zip(info['bbox'],expected[i])),(i,info)
   im=Image.open(io.BytesIO(doc.extract_image(info['xref'])['image'])).convert('RGB')
   assert im.size==original.size and im.tobytes()==original.tobytes()
  for token,page in [('SIDE_TOP',1),('SIDE_BOTTOM',1),('IMAGE_NOTE_REF',2),('NESTED_NOTE_REF',4),('FINISH',6)]:
   if i==page:assert p.search_for(token)
  if i in (2,4):
   notice=p.search_for('Continued on a later page');assert len(notice)==1
   assert len(p.get_links())==2 and all(l['page']==i+1 for l in p.get_links())
   rect=next(l['from'] for l in p.get_links() if l['from'].width>100)
   assert rect.contains(notice[0])
  for w in p.get_text('words'):assert w[0]>=19.95 and w[1]>=19.95 and w[2]<=280.05 and w[3]<=180.05,(i,w)
  p.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/f'page-{i+1}.png')
 for page,want in [(1,[(20,20,130,160),(26,26,118,140),(150,20,130,26),(150,46,130,134)]),(5,[(38,42,242,130),(44,48,230,110)])]:
  rects=[]
  for path in doc[page].get_drawings():
   for item in path['items']:
    if item[0]=='re':r=item[1];rects.append((r.x0,r.y0,r.width,r.height))
  assert len(rects)==len(want),(page,rects)
  for a,b in zip(rects,want):assert all(abs(x-y)<.03 for x,y in zip(a,b)),(a,b)
print('Image fitting passed: seven PDF pages, four exact pixel assets/placements, nested merge bounds, linked notices/real note destinations, preferred SF and fitted DOCX extents')
if args.office:
 with pymupdf.open(out/'lo'/'image-fit.pdf') as doc:
  assert sum(len(p.get_image_info()) for p in doc)==4
  text=''.join(p.get_text() for p in doc)
  for token in ['SIDE_TOP','SIDE_BOTTOM','IMAGE_NOTE_REF','NESTED_NOTE_REF','FINISH']:assert text.count(token)==1,token
  for i,p in enumerate(doc):
   for image in p.get_image_info():
    x0,y0,x1,y1=image['bbox'];assert x0>=19.95 and y0>=19.95 and x1<=280.05 and y1<=180.05,(i,image)
   p.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/'lo'/f'page-{i+1}.png')
  print(f'LibreOffice image containment passed: {len(doc)} pages, all four image placements and original tokens; producer pagination differs')
