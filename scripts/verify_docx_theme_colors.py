"""Independent DOCX theme color fixture and SF/DOCX/PDF oracle, QA only.

python-docx authors an ordinary full package; XML explicitly selects source
palette, mappings and run properties. Source theme binding intentionally resolves
to RGB in SF. Requires bin/manydoc and optional QA Python dependencies.
"""
import argparse
import json
import subprocess
from pathlib import Path
from zipfile import ZipFile
from lxml import etree
from docx import Document
from docx.oxml import OxmlElement
from docx.shared import Pt
from pypdf import PdfReader
import pymupdf

parser=argparse.ArgumentParser()
parser.add_argument('--office',type=Path,help='optional LibreOffice source rendering for observations')
parser.add_argument('--reflow-office',type=Path,help='optional rendering of exported RGB-only DOCX')
args=parser.parse_args()
out=Path('output/docx-theme-colors');out.mkdir(parents=True,exist_ok=True)
w='http://schemas.openxmlformats.org/wordprocessingml/2006/main'
a='http://schemas.openxmlformats.org/drawingml/2006/main'
# A deliberately stale red cache tests the theme's precedence. Every other
# cache is correct according to the independent expected RGB values below.
cases=[
 ('THEME_BASE','accent1',None,None,'FF0000','4F81BD'),
 ('THEME_TINT','accent1','99',None,'95B3D7','95B3D7'),
 ('THEME_SHADE','accent3',None,'80','800000','800000'),
 ('BOTH_MODIFIERS','accent3','80','00','FF7F7F','FF7F7F'),
 ('GRAY_SHADE','accent4',None,'80','404040','404040'),
 ('MAP_TEXT','text1',None,None,'FF0000','FF0000'),
 ('MAP_BACKGROUND','background2',None,None,'808080','808080'),
 ('LINK_PALETTE','hyperlink',None,None,'0000FF','0000FF'),
 ('DIRECT_DARK','dark2',None,None,'204060','204060'),
 ('TINTOFBLACK','accent6','80',None,'7F7F7F','7F7F7F'),
 ('SHADEOFWHITE','accent5',None,'80','808080','808080'),
]
doc=Document();doc.core_properties.title='Independent theme colors'
doc.styles['Normal'].font.name='Liberation Sans';doc.styles['Normal'].font.size=Pt(14)
for token,theme,tint,shade,cache,rgb in cases:
 run=doc.add_paragraph().add_run(token)
 color=OxmlElement('w:color');color.set('{'+w+'}val',cache);color.set('{'+w+'}themeColor',theme)
 if tint is not None:color.set('{'+w+'}themeTint',tint)
 if shade is not None:color.set('{'+w+'}themeShade',shade)
 run._r.get_or_add_rPr().append(color)
source=out/'source.docx';doc.save(source)
with ZipFile(source) as z:parts={name:z.read(name) for name in z.namelist()}
theme=etree.fromstring(parts['word/theme/theme1.xml'])
palette=theme.find('.//{'+a+'}clrScheme')
values=dict(dk1='102030',lt1='F0E0D0',dk2='204060',lt2='E0C0A0',accent1='4F81BD',accent2='C0504D',accent3='FF0000',accent4='808080',accent5='FFFFFF',accent6='000000',hlink='0000FF',folHlink='800080')
for name,rgb in values.items():
 slot=palette.find('{'+a+'}'+name)
 for c in list(slot):slot.remove(c)
 etree.SubElement(slot,'{'+a+'}srgbClr',val=rgb)
parts['word/theme/theme1.xml']=etree.tostring(theme,xml_declaration=True,encoding='UTF-8',standalone=True)
settings=etree.fromstring(parts['word/settings.xml'])
for child in list(settings):
 if child.tag=='{'+w+'}clrSchemeMapping':settings.remove(child)
map_node=etree.SubElement(settings,'{'+w+'}clrSchemeMapping')
map_node.set('{'+w+'}t1','accent3');map_node.set('{'+w+'}bg2','accent4')
parts['word/settings.xml']=etree.tostring(settings,xml_declaration=True,encoding='UTF-8',standalone=True)
with ZipFile(source,'w') as z:
 for name,data in parts.items():z.writestr(name,data)
subprocess.run(['bin/manydoc','extract','-format','sf','-o',str(out/'source.sf'),str(source)],check=True)
sf=json.loads((out/'source.sf').read_text())
def runs(n):
 if n['type']=='text':yield n
 for c in n.get('content',[]):yield from runs(c)
observed={n['text']:next(m['attrs'] for m in n['marks'] if m['type']=='textStyle') for n in runs(sf)}
assert len(observed)==len(cases)
for token,theme,tint,shade,cache,rgb in cases:
 assert observed[token]['color']=='#'+rgb,(token,observed[token])
 assert observed[token]['fontSize']==14
assert any(d['code']=='color_theme' for d in sf['attrs']['diagnostics'])
for ext in ('docx','pdf'):
 subprocess.run(['bin/manydoc','convert','-allow-lossy','-pdf-font-map','Liberation Sans=Go','-o',str(out/f'reflow.{ext}'),str(out/'source.sf')],check=True)
with ZipFile(out/'reflow.docx') as z:
 root=etree.fromstring(z.read('word/document.xml'))
 for token,theme,tint,shade,cache,rgb in cases:
  run=root.xpath('//w:r[w:t="'+token+'"]',namespaces={'w':w})[0]
  color=run.find('.//{'+w+'}color')
  assert color.get('{'+w+'}val')==rgb and color.get('{'+w+'}themeColor') is None
reader=PdfReader(out/'reflow.pdf',strict=True)
assert len(reader.pages)==1 and reader.metadata.title=='Independent theme colors'
with pymupdf.open(out/'reflow.pdf') as pdf:
 spans={s['text']:s for b in pdf[0].get_text('dict')['blocks'] for l in b.get('lines',[]) for s in l['spans']}
 for token,theme,tint,shade,cache,rgb in cases:
  assert spans[token]['color']==int(rgb,16) and abs(spans[token]['size']-14)<.001
 pdf[0].get_pixmap(matrix=pymupdf.Matrix(1.3,1.3)).save(out/'native.png')
print('Theme color checks passed: eleven source bindings/modifiers/mappings, SF RGB and inherited size, standard DOCX RGB, native PDF colors and metadata')
if args.office:
 # Observation is separate: a producer may retain a stale cached value or use
 # a different tint/shade quantization. Do not rewrite the SF expected colors.
 with pymupdf.open(args.office) as pdf:
  spans={s['text']:s for p in pdf for b in p.get_text('dict')['blocks'] for l in b.get('lines',[]) for s in l['spans']}
  observations={}
  for token,theme,tint,shade,cache,rgb in cases:
   span=spans[token]
   assert abs(span['size']-14)<.01
   if cache==rgb:assert span['color']==int(rgb,16),(token,span)
   else:assert span['color'] in (int(cache,16),int(rgb,16)),(token,span)
   observations[token]={'expected':rgb,'observed':f"{span['color']:06X}",'cache':cache}
  for i,p in enumerate(pdf):p.get_pixmap(matrix=pymupdf.Matrix(1.3,1.3)).save(out/f'office-{i+1}.png')
  (out/'office-observation.json').write_text(json.dumps(observations,indent=2))
 print('Source LibreOffice colors observed separately:',observations)

if args.reflow_office:
 with pymupdf.open(args.reflow_office) as pdf:
  assert len(pdf)==1
  spans={s['text']:s for p in pdf for b in p.get_text('dict')['blocks'] for l in b.get('lines',[]) for s in l['spans']}
  assert len(spans)==len(cases)
  for token,theme,tint,shade,cache,rgb in cases:
   assert spans[token]['color']==int(rgb,16) and abs(spans[token]['size']-14)<.01
  pdf[0].get_pixmap(matrix=pymupdf.Matrix(1.3,1.3)).save(out/'office-reflow.png')
 print('Exported RGB-only DOCX LibreOffice check passed: all eleven exact colors/sizes/tokens on one page')
