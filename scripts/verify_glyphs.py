"""Independent CID/glyph/Unicode/position oracle; QA only, no runtime tools."""
import hashlib
from zipfile import ZipFile
from lxml import etree
from docx import Document
from pathlib import Path
from pypdf import PdfReader
import pymupdf
from font_oracles import verify_go_program

out=Path('output/glyphs')
word=Document(out/'glyphs.docx')
assert word.paragraphs[0].text=='B\u0315 note'
assert word.paragraphs[1].text=='SUPPLEMENTARY A😀B'
assert word.sections[0].header.paragraphs[0].text=='HEADER B\u0315'
assert word.sections[0].footer.paragraphs[0].text=='FOOTER 😀'
assert len(word.tables)==1 and [[c.text for c in row.cells] for row in word.tables[0].rows]==[['TABLE B\u0315','😀'],['BODY Z\u035c','END 😀']]
with ZipFile(out/'glyphs.docx') as z:
 ns={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}
 note=etree.fromstring(z.read('word/footnotes.xml'))
 assert ''.join(note.xpath('//w:t/text()',namespaces=ns))=='NOTE Z\u035c'
 assert b'https://example.invalid/glyph' in z.read('word/_rels/document.xml.rels')
r=PdfReader(out/'glyphs.pdf',strict=True)
assert len(r.pages)==2 and r.metadata.title=='Positioned glyphs'
# Full embedding is deliberate; verify real programs, not font names alone.
seen={}
for page in r.pages:
 for key,obj in page['/Resources']['/Font'].items():
  if not str(key).startswith('/MDG'):continue
  f=obj.get_object();assert f['/Subtype']=='/Type0' and f['/Encoding']=='/Identity-H'
  cid=f['/DescendantFonts'][0].get_object();assert cid['/Subtype']=='/CIDFontType2'
  desc=cid['/FontDescriptor'];family=str(desc['/FontFamily'])
  raw=desc['/FontFile2'].get_data()
  if family=='Go':verify_go_program(str(f['/BaseFont'])[1:],raw)
  else:
   source=Path('testdata/assets')/('NotoSans-Regular.ttf' if family=='Noto Sans' else 'DejaVuSans.ttf')
   assert hashlib.sha256(raw).digest()==hashlib.sha256(source.read_bytes()).digest()
  assert desc['/FontFile2']['/Length1']==len(raw)
  mapping=cid['/CIDToGIDMap'].get_data();assert len(mapping)%2==0
  assert len(mapping)>2 and any(mapping[2:])
  unicode_map=f['/ToUnicode'].get_data();assert b'begincodespacerange' in unicode_map and b'beginbfchar' in unicode_map
  if family=='DejaVu Sans':assert b'D83DDE00' in unicode_map
  if family=='Noto Sans':assert b'00420315' in unicode_map and b'005A035C' in unicode_map
  seen[family]=(obj.idnum,len(raw))
assert set(seen)=={'Noto Sans','DejaVu Sans','Go'}
for family in seen:
 refs=[]
 for page in r.pages:
  refs.append({obj.idnum for key,obj in page['/Resources']['/Font'].items() if str(key).startswith('/MDG') and str(obj.get_object()['/DescendantFonts'][0].get_object()['/FontDescriptor']['/FontFamily'])==family})
 assert refs[0]==refs[1] and len(refs[0])==1
# ToUnicode fallback and ActualText-aware extraction both retain exact strings.
with pymupdf.open(out/'glyphs.pdf') as doc:
 for i,page in enumerate(doc):
  text=page.get_text();fallback=r.pages[i].extract_text()
  for token in ['HEADER B\u0315','FOOTER 😀']:
   assert text.count(token)==1 and fallback.count(token)==1,(i,token,text,fallback)
  if i==0:
   for token in ['B\u0315 note1','SUPPLEMENTARY A😀B','NOTE Z\u035c']:
    assert token in text and token in fallback,(token,text,fallback)
  else:
   for token in ['TABLE B\u0315','BODY Z\u035c','END 😀']:assert token in text and token in fallback
  page.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/f'page-{i+1}.png')
 # The actual supplementary GID must come from the original program's cmap.
 expected=pymupdf.Font(fontfile='testdata/assets/DejaVuSans.ttf').has_glyph(0x1F600)
 assert expected>0
 glyph_ids=[c[1] for p in doc for s in p.get_texttrace() if s['font']=='DejaVuSans' for c in s['chars'] if c[0]==0x1F600]
 assert len(glyph_ids)==5 and set(glyph_ids)=={expected}
 # Reference shaping is MuPDF's HTML/Story engine, independent of the Go port.
 # Compare original GIDs and relative GPOS offsets, not merely extracted text.
 ref=pymupdf.open();p=ref.new_page(width=200,height=100)
 css='@font-face {font-family:TestNoto;src:url(NotoSans-Regular.ttf);} body {font-family:TestNoto;font-size:22pt;}'
 p.insert_htmlbox(p.rect,'B\u0315',css=css,archive=pymupdf.Archive('testdata/assets'))
 ref.save(out/'reference.pdf')
 expected=[c for s in p.get_texttrace() for c in s['chars'] if 0<c[1]<3748]
 actual=[c for s in doc[0].get_texttrace() if s['font']=='NotoSans-Regular' and abs(s['size']-22)<.001 for c in s['chars'] if 0<c[1]<3748]
 assert len(expected)==len(actual)==2,(expected,actual)
 assert [c[1] for c in expected]==[c[1] for c in actual]
 for axis in (0,1):
  assert abs((expected[1][2][axis]-expected[0][2][axis])-(actual[1][2][axis]-actual[0][2][axis]))<.01,(expected,actual)
 ref.close()
 ann=[a.get_object() for a in r.pages[0]['/Annots']]
 assert any(a.get('/A',{}).get('/URI')=='https://example.invalid/glyph' for a in ann)
 assert any('/Dest' in a for a in ann)
print('Glyph transport passed: two pages; original full TTF hashes/shared CID resources; correct supplementary GID; exact ActualText/ToUnicode strings; independent MuPDF reference GIDs/GPOS offsets; URI/note links and running content')
