"""QA only: independent OOXML language/direction inheritance and native locl GIDs."""
import json
import subprocess
from pathlib import Path
from zipfile import ZipFile
from lxml import etree
from docx import Document
from docx.enum.style import WD_STYLE_TYPE
from docx.oxml import OxmlElement
from docx.shared import Pt
from pypdf import PdfReader
import pymupdf
import uharfbuzz as hb

out=Path('output/language');out.mkdir(parents=True,exist_ok=True)
w='http://schemas.openxmlformats.org/wordprocessingml/2006/main'
ns={'w':w}
def prop(pr,name,attrs):
 for n in list(pr):
  if n.tag=='{'+w+'}'+name:pr.remove(n)
 n=OxmlElement('w:'+name)
 for key,value in attrs.items():n.set('{'+w+'}'+key,str(value))
 pr.append(n)
# Independent source: defaults -> paragraph style -> derived -> character -> direct.
doc=Document();doc.core_properties.title='Independent language inheritance'
section=doc.sections[0];section.page_width=Pt(595.3);section.page_height=Pt(841.9)
section.top_margin=section.bottom_margin=section.left_margin=section.right_margin=Pt(54)
root=doc.styles.element
rpr=root.find('w:docDefaults/w:rPrDefault/w:rPr',ns)
prop(rpr,'lang',{'val':'en-US','eastAsia':'ja-JP','bidi':'ar-SA'});prop(rpr,'rtl',{})
prop(rpr,'rFonts',dict(ascii='Noto Sans',hAnsi='Noto Sans',eastAsia='Noto Sans',cs='Noto Sans'))
normal=doc.styles['Normal'];prop(normal.element.get_or_add_pPr(),'bidi',{'val':'0'});prop(normal.element.get_or_add_rPr(),'lang',{'val':'fr-CA'})
derived=doc.styles.add_style('LanguageDerived',WD_STYLE_TYPE.PARAGRAPH);derived.base_style=normal
prop(derived.element.get_or_add_pPr(),'bidi',{});prop(derived.element.get_or_add_rPr(),'lang',{'eastAsia':'zh-TW'})
char=doc.styles.add_style('LanguageCharacter',WD_STYLE_TYPE.CHARACTER)
prop(char.element.get_or_add_rPr(),'lang',{'bidi':'he-IL'});prop(char.element.get_or_add_rPr(),'rtl',{'val':'false'})
doc.add_paragraph('DEFAULT')
p=doc.add_paragraph(style=derived);prop(p._p.get_or_add_pPr(),'bidi',{'val':'off'})
r=p.add_run('OVERRIDE');r.style=char;prop(r._r.get_or_add_rPr(),'lang',{'val':'sr-RS'})
doc.add_paragraph('INHERITED',style=derived)
p=doc.add_paragraph('FORCED');prop(p.runs[0]._r.get_or_add_rPr(),'cs',{});prop(p.runs[0]._r.get_or_add_rPr(),'lang',{'val':'ru-RU','bidi':'sr-RS'})
source=out/'source.docx';doc.save(source)
subprocess.run(['bin/manifold','extract','-format','sf','-o',str(out/'source.sf'),str(source)],check=True)
sf=json.loads((out/'source.sf').read_text())
expected=[('DEFAULT','ltr','rtl','fr-CA','ja-JP','ar-SA',False),('OVERRIDE','ltr','ltr','sr-RS','zh-TW','he-IL',False),('INHERITED','rtl','rtl','fr-CA','zh-TW','ar-SA',False),('FORCED','ltr','rtl','ru-RU','ja-JP','sr-RS',True)]
for para,(text,pdir,rdir,lang,ea,cs,complex_script) in zip(sf['content'],expected):
 assert para['attrs']['dir']==pdir
 assert len(para['content'])==1 and para['content'][0]['text']==text
 style=next(m['attrs'] for m in para['content'][0]['marks'] if m['type']=='textStyle')
 assert (style['dir'],style['lang'],style['eastAsiaLanguage'],style['complexLanguage'])==(rdir,lang,ea,cs),style
 assert style.get('complexScript',False)==complex_script
assert len(sf['content'])==4
assert not {'run_language','run_direction'} & {d['code'] for d in sf['attrs'].get('diagnostics',[])}
subprocess.run(['bin/manifold','convert','-allow-lossy','-o',str(out/'reflow.docx'),str(out/'source.sf')],check=True)
with ZipFile(out/'reflow.docx') as z:
 body=etree.fromstring(z.read('word/document.xml')).find('w:body',ns)
 for para,(_,pdir,rdir,lang,ea,cs,complex_script) in zip(body.findall('w:p',ns),expected):
  assert para.find('w:pPr/w:bidi',ns).get('{'+w+'}val')==str(pdir=='rtl').lower()
  pr=para.find('w:r/w:rPr',ns)
  assert pr.find('w:rtl',ns).get('{'+w+'}val')==str(rdir=='rtl').lower()
  assert (pr.find('w:cs',ns) is not None)==complex_script
  assert dict(pr.find('w:lang',ns).attrib)=={'{'+w+'}val':lang,'{'+w+'}eastAsia':ea,'{'+w+'}bidi':cs}
# All stories in manydoc's example retain the same language triplet and directions.
with ZipFile(out/'direction.docx') as z:
 for name in ['word/document.xml','word/header1.xml','word/footer1.xml','word/footnotes.xml']:
  tree=etree.fromstring(z.read(name))
  found=tree.xpath('//w:p[.//w:t[contains(.,"Paragraph and run")]]',namespaces=ns)
  assert found,name
  for p in found:
   assert p.find('w:pPr/w:bidi',ns).get('{'+w+'}val')=='true'
   assert p.find('w:hyperlink/w:r/w:rPr/w:cs',ns) is not None
   assert p.find('w:hyperlink/w:r/w:rPr/w:rtl',ns).get('{'+w+'}val')=='false' if p.find('w:hyperlink',ns) is not None else True
# Native C HarfBuzz supplies actual glyphs and advances, independently of Go port.
raw=Path('testdata/assets/NotoSans-Regular.ttf').read_bytes();face=hb.Face(raw);font=hb.Font(face)
r=PdfReader(out/'localized.pdf',strict=True);assert len(r.pages)==1
assert r.pages[0].extract_text().count('бгдпт')==4
with pymupdf.open(out/'localized.pdf') as pdf:
 page=pdf[0];trace=[c for span in page.get_texttrace() for c in span['chars']]
 rows={}
 for c in trace:rows.setdefault(round(c[2][1],2),[]).append(c)
 assert len(rows)==4,rows
 gids=[]
 for (_,chars),locale in zip(sorted(rows.items()),['ru-RU','sr-RS','mk-MK','sr-RS']):
  buf=hb.Buffer();buf.add_str('бгдпт');buf.guess_segment_properties();buf.language=locale;hb.shape(font,buf)
  expected_ids=[g.codepoint for g in buf.glyph_infos];assert [c[1] for c in chars]==expected_ids,(locale,chars,expected_ids)
  gids.append(expected_ids)
  advance=0
  for c,pos in zip(chars,buf.glyph_positions):
   assert abs((c[2][0]-chars[0][2][0])-(advance+pos.x_offset)*28/face.upem)<.011,(locale,c,pos)
   advance+=pos.x_advance
 assert gids[0][0]==457 and gids[1][0]==gids[2][0]==gids[3][0]==2090
 page.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/'localized-page.png')
print('Language QA passed: four inherited paragraph/run/complex bindings; all-story OOXML; four localized native glyph rows against C HarfBuzz',hb.version_string())

# Qualify every painted run against native HarfBuzz after line selection.
# The independent oracle includes kerning and ligatures, not isolated-glyph widths.
from pypdf.generic import ContentStream
for name,expected_text,expected_runs in [('wrap','office AV To office AV To ffi ffi',5),('emergency','ffi',3)]:
 reader=PdfReader(out/f'{name}.pdf',strict=True);assert len(reader.pages)==1
 page=reader.pages[0];fonts=page['/Resources']['/Font']
 mappings={key:f.get_object()['/DescendantFonts'][0].get_object()['/CIDToGIDMap'].get_data() for key,f in fonts.items() if str(key).startswith('/MDG')}
 logical=[];painted=[];active=None;which=None;size=None;point=None
 for args,op in ContentStream(page.get_contents(),reader).operations:
  if op==b'BDC':
   assert active is None
   active=str(args[1]['/ActualText']);painted=[]
  elif op==b'Tf':which=str(args[0]);size=float(args[1])
  elif op==b'Tm':point=(float(args[4]),float(args[5]))
  elif op==b'Tj' and active is not None and which in mappings:
   value=args[0];encoded=value.original_bytes if hasattr(value,'original_bytes') else bytes(value)
   assert len(encoded)==2
   cid=int.from_bytes(encoded,'big');mapping=mappings[which]
   painted.append((int.from_bytes(mapping[2*cid:2*cid+2],'big'),point,size))
  elif op==b'EMC':
   assert active is not None and painted
   b=hb.Buffer();b.add_str(active);b.guess_segment_properties();b.language='en-US';hb.shape(font,b)
   assert [g[0] for g in painted]==[g.codepoint for g in b.glyph_infos],(name,active,painted)
   advance=0
   for (gid,point,size),pos in zip(painted,b.glyph_positions):
    assert size==20
    assert abs((point[0]-painted[0][1][0])-(advance+pos.x_offset)*size/face.upem)<.001,(name,active,gid,point,pos)
    advance+=pos.x_advance
   assert painted[0][1][0]+advance*20/face.upem<=float(page.mediabox.width)-10+.001
   logical.append(active);active=None
 assert len(logical)==expected_runs and ''.join(logical)==expected_text,(name,logical)
 if name=='wrap':
  assert any(b'006600660069' in f.get_object()['/ToUnicode'].get_data() for key,f in fonts.items() if str(key).startswith('/MDG'))
  assert 'office' in page.extract_text() and page.extract_text().count('ffi')>=4
 with pymupdf.open(out/f'{name}.pdf') as doc:
  doc[0].get_pixmap(matrix=pymupdf.Matrix(2,2)).save(out/f'{name}-page.png')
print('Wrapped shaping QA passed: five kerned/ligated runs and three re-shaped emergency lines; cluster ToUnicode mappings')
