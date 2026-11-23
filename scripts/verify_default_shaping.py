"""Independent native-HarfBuzz/reader oracle for untagged default shaping. QA only."""
import hashlib
import json
from pathlib import Path
from zipfile import ZipFile
from lxml import etree
from docx import Document
from pypdf import PdfReader
from pypdf.generic import ContentStream
import pymupdf
import uharfbuzz as hb
from font_oracles import verify_go_program

out=Path('output/default-shaping')
sf=json.loads((out/'default.sf').read_text())
def walk(node):
 for mark in node.get('marks',[]):
  if mark['type']=='textStyle':
   assert not any(mark.get('attrs',{}).get(k) for k in ['lang','eastAsiaLanguage','complexLanguage'])
 for child in node.get('content',[]):walk(child)
walk(sf)
for node in sf['attrs']['headers']+sf['attrs']['footers']:walk(node)
for note in sf['attrs']['notes']:
 for node in note['content']:walk(node)
word=Document(out/'default.docx')
assert [p.text for p in word.paragraphs[:3]]==['office AV To','Default AV To office','Latin ΩЖ']
assert [[cell.text for cell in row.cells] for row in word.tables[0].rows]==[['office AV','Mono office'],['ffi café','TEXT']]
with ZipFile(out/'default.docx') as z:
 ns={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}
 notes=etree.fromstring(z.read('word/footnotes.xml'))
 assert ''.join(notes.xpath('//w:t/text()',namespaces=ns))=='Default note office'
 assert b'https://example.invalid/shaping' in z.read('word/_rels/document.xml.rels')
originals={
 'GoRegular':(out/'Go-Regular.ttf').read_bytes(),
 'GoMono':(out/'Go-Mono.ttf').read_bytes(),
 'NotoSans-Regular':Path('testdata/assets/NotoSans-Regular.ttf').read_bytes(),
}
for key in ['GoRegular','GoMono']:verify_go_program(key,originals[key])
checked=0;ligatures=0;used=set()
for name,pages in [('default',2),('emergency',1),('marked-space',1)]:
 reader=PdfReader(out/f'{name}.pdf',strict=True);assert len(reader.pages)==pages
 text=''.join(page.extract_text() for page in reader.pages)
 if name=='default':
  assert text.count('DEFAULT HEADER')==text.count('DEFAULT FOOTER')==2
  for token in ['Default AV To office','Latin ΩЖ','Default note office','Mono office','ffi café','TEXT']:assert token in text,(token,text)
  ann=[a.get_object() for a in reader.pages[0]['/Annots']]
  assert any(a.get('/A',{}).get('/URI')=='https://example.invalid/shaping' for a in ann)
  assert any('/Dest' in a for a in ann)
 elif name=='emergency':assert text.replace('\n','').strip()=='ffi'
 else:assert text.replace('\n','')=='A\u2007\u0315B'
 shared={};actual_runs=[]
 for page in reader.pages:
  fonts={}
  for key,obj in page['/Resources']['/Font'].items():
   if not str(key).startswith('/MDG'):continue
   f=obj.get_object();ps=str(f['/BaseFont'])[1:];cid=f['/DescendantFonts'][0].get_object();desc=cid['/FontDescriptor']
   raw=desc['/FontFile2'].get_data();assert hashlib.sha256(raw).digest()==hashlib.sha256(originals[ps]).digest()
   assert desc['/FontFile2']['/Length1']==len(raw)
   if ps=='GoMono':assert int(desc['/Flags'])&1
   fonts[str(key)]=(cid['/CIDToGIDMap'].get_data(),ps)
   if ps in shared:assert shared[ps]==obj.idnum
   shared[ps]=obj.idnum
   used.add(ps)
  active=None;painted=[];which=None;size=None;point=None
  for args,op in ContentStream(page.get_contents(),reader).operations:
   if op==b'BDC':
    assert active is None
    active=str(args[1]['/ActualText']);painted=[]
   elif op==b'Tf':which=str(args[0]);size=float(args[1])
   elif op==b'Tm':point=(float(args[4]),float(args[5]))
   elif op==b'Tj' and active is not None and which in fonts:
    token=args[0];raw=token.original_bytes if hasattr(token,'original_bytes') else bytes(token)
    assert len(raw)==2
    mapping,ps=fonts[which];code=int.from_bytes(raw,'big')
    painted.append((int.from_bytes(mapping[2*code:2*code+2],'big'),point,size,ps))
   elif op==b'EMC':
    assert active is not None
    if painted:
     ps=painted[0][3];assert all(g[3]==ps for g in painted)
     face=hb.Face(originals[ps]);font=hb.Font(face)
     b=hb.Buffer();b.add_str(active);b.guess_segment_properties();b.language='und';hb.shape(font,b)
     assert [g[0] for g in painted]==[g.codepoint for g in b.glyph_infos],(name,active,painted)
     x=0
     for (_,point,size,_),pos in zip(painted,b.glyph_positions):
      assert size==painted[0][2]
      assert abs(point[0]-(painted[0][1][0]+(x+pos.x_offset)*size/face.upem))<.000001,(name,active,point,pos)
      assert abs(point[1]-(painted[0][1][1]+pos.y_offset*size/face.upem))<.000001
      x+=pos.x_advance
     actual_runs.append(active);checked+=1
     if len(painted)<len(active):ligatures+=1
    else:assert not active.strip(),(name,active)
    active=None
 if name=='emergency':assert actual_runs==['f','f','i'],actual_runs
 with pymupdf.open(out/f'{name}.pdf') as doc:
  for i,page in enumerate(doc):
   page.get_pixmap(matrix=pymupdf.Matrix(1.6,1.6)).save(out/f'{name}-page-{i+1}.png')
assert used==set(originals) and ligatures>=3
print(f'Default shaping passed: {checked} untagged runs against native HarfBuzz {hb.version_string()}, {ligatures} ligature runs; three original font programs, shared resources, native positions, emergency re-shaping and DOCX/relationships/running content')
