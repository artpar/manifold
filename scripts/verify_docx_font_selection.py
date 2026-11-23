"""Independent DOCX script/theme font slots -> SF -> DOCX/PDF, QA only.

The native PDF explicitly maps selected families to Go/Go Mono. Latin, Greek
and Cyrillic text exercise distinct font slots without claiming broader shaping.
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

parser=argparse.ArgumentParser();parser.add_argument('--office',type=Path,help='optional rendering of exported DOCX');args=parser.parse_args()
out=Path('output/docx-font-selection');out.mkdir(parents=True,exist_ok=True)
w='http://schemas.openxmlformats.org/wordprocessingml/2006/main';a='http://schemas.openxmlformats.org/drawingml/2006/main'
slots=dict(ascii='Liberation Sans',hAnsi='Liberation Mono',eastAsia='Liberation Serif',cs='Liberation Serif')
def property(run,name,attrs):
 n=OxmlElement('w:'+name)
 for key,value in attrs.items():n.set('{'+w+'}'+key,str(value))
 run._r.get_or_add_rPr().append(n)
def fonts(run,attrs):property(run,'rFonts',attrs)
doc=Document();doc.core_properties.title='Independent font slot selection';doc.styles['Normal'].font.size=Pt(14)
# Exact source partition oracles, independent of manydoc's run segmentation.
cases=[]
def add(text,attrs,expected,extra=()):
 r=doc.add_paragraph().add_run(text);fonts(r,attrs)
 for name,values in extra:property(r,name,values)
 cases.append(expected)
add('ASCIIΩЖé',slots,[('ASCII','Liberation Sans',14,False,False),('ΩЖé','Liberation Mono',14,False,False)])
add('HINTΩЖ',dict(slots,hint='eastAsia'),[('HINT','Liberation Sans',14,False,False),('ΩЖ','Liberation Serif',14,False,False)])
add('THEME_LANGUAGE',dict(slots,asciiTheme='majorAscii'),[('THEME_LANGUAGE','Liberation Mono',14,False,False)])
add('COMPLEX_SLOT',slots,[('COMPLEX_SLOT','Liberation Serif',18,True,True)],(('cs',{}),('bCs',{}),('iCs',{}),('szCs',{'val':'36'})))
add('THEMEΩ',dict(slots,eastAsiaTheme='minorEastAsia',hint='eastAsia'),[('THEME','Liberation Sans',14,False,False),('Ω','Liberation Serif',14,False,False)])
source=out/'source.docx';doc.save(source)
with ZipFile(source) as z:parts={name:z.read(name) for name in z.namelist()}
theme=etree.fromstring(parts['word/theme/theme1.xml'])
for group,family in [('majorFont','Liberation Mono'),('minorFont','Liberation Serif')]:
 node=theme.find('.//{'+a+'}'+group)
 for n in list(node):
  if n.tag=='{'+a+'}font' and n.get('script')=='Jpan':node.remove(n)
 etree.SubElement(node,'{'+a+'}font',script='Jpan',typeface=family)
parts['word/theme/theme1.xml']=etree.tostring(theme,xml_declaration=True,encoding='UTF-8',standalone=True)
settings=etree.fromstring(parts['word/settings.xml'])
for n in list(settings):
 if n.tag=='{'+w+'}themeFontLang':settings.remove(n)
etree.SubElement(settings,'{'+w+'}themeFontLang',{'{'+w+'}val':'ja-JP','{'+w+'}eastAsia':'ja-JP','{'+w+'}bidi':'ar-SA'})
parts['word/settings.xml']=etree.tostring(settings,xml_declaration=True,encoding='UTF-8',standalone=True)
with ZipFile(source,'w') as z:
 for name,data in parts.items():z.writestr(name,data)
subprocess.run(['bin/manydoc','extract','-format','sf','-o',str(out/'source.sf'),str(source)],check=True)
sf=json.loads((out/'source.sf').read_text());assert len(sf['content'])==len(cases)
for block,expected in zip(sf['content'],cases):
 runs=block['content'];assert len(runs)==len(expected),(runs,expected)
 for run,(text,family,size,bold,italic) in zip(runs,expected):
  assert run['text']==text
  marks={m['type']:m.get('attrs',{}) for m in run['marks']}
  assert marks['textStyle']['fontFamily']==family and marks['textStyle']['fontSize']==size,(text,marks)
  assert ('strong' in marks)==bold and ('em' in marks)==italic
assert {'font_theme','font_selectors'} <= {d['code'] for d in sf['attrs']['diagnostics']}
for ext in ('docx','pdf'):
 subprocess.run(['bin/manydoc','convert','-allow-lossy','-pdf-font-map','Liberation Sans=Go','-pdf-font-map','Liberation Serif=Go','-pdf-font-map','Liberation Mono=Go Mono','-o',str(out/f'reflow.{ext}'),str(out/'source.sf')],check=True)
with ZipFile(out/'reflow.docx') as z:
 root=etree.fromstring(z.read('word/document.xml'))
 for para,expected in zip(root.findall('.//{'+w+'}body/{'+w+'}p'),cases):
  runs=para.findall('{'+w+'}r');assert len(runs)==len(expected)
  for run,(text,family,size,bold,italic) in zip(runs,expected):
   assert run.find('{'+w+'}t').text==text
   props=run.find('{'+w+'}rPr')
   for key in slots:assert props.find('{'+w+'}rFonts').get('{'+w+'}'+key)==family
   assert props.find('{'+w+'}sz').get('{'+w+'}val')==str(size*2)
   assert props.find('{'+w+'}szCs').get('{'+w+'}val')==str(size*2)
   for key,enabled in [('b',bold),('bCs',bold),('i',italic),('iCs',italic)]:assert (props.find('{'+w+'}'+key) is not None)==enabled
reader=PdfReader(out/'reflow.pdf',strict=True);assert len(reader.pages)==1 and reader.metadata.title=='Independent font slot selection'
with pymupdf.open(out/'reflow.pdf') as pdf:
 # Native readers may coalesce adjacent spans after explicit family mapping
 # chooses the same face. Preserve the source SF's exact eight-span oracle above
 # and compare every native glyph's effective face/size/style here.
 chars=[(ch['c'],span['size'],span['flags'],ch['origin']) for b in pdf[0].get_text('rawdict')['blocks'] for line in b.get('lines',[]) for span in line['spans'] for ch in span['chars']]
 expected=[(ch,family,size,bold,italic) for p in cases for text,family,size,bold,italic in p for ch in text]
 assert len(chars)==len(expected)
 for i,((ch,size,flags,origin),(want,family,want_size,bold,italic)) in enumerate(zip(chars,expected)):
  assert ch==want and abs(size-want_size)<.001
  assert bool(flags&8)==(family=='Liberation Mono')
  assert bool(flags&16)==bold and bool(flags&2)==italic
  if family=='Liberation Mono' and i and expected[i-1][1]==family and abs(chars[i-1][3][1]-origin[1])<.001:
   assert abs(origin[0]-chars[i-1][3][0]-size*.6)<.02,(ch,origin,chars[i-1])
 pdf[0].get_pixmap(matrix=pymupdf.Matrix(1.4,1.4)).save(out/'native.png')
print('Font selection passed: five independently authored paragraphs/eight exact spans; source slot/theme/complex properties, SF families/marks/sizes, DOCX four slots and complex twins, native PDF fixed-pitch/size/style evidence')
if args.office:
 with pymupdf.open(args.office) as pdf:
  assert len(pdf)==1
  spans=[s for b in pdf[0].get_text('dict')['blocks'] for l in b.get('lines',[]) for s in l['spans']]
  flat=[v for p in cases for v in p]
  assert ''.join(s['text'] for s in spans)==''.join(v[0] for v in flat)
  # Check characters rather than trusting producer span partitioning.
  actual=[(ch['c'],span['size'],span['flags']) for b in pdf[0].get_text('rawdict')['blocks'] for line in b.get('lines',[]) for span in line['spans'] for ch in span['chars']]
  expected=[(ch,size,bold,italic) for text,family,size,bold,italic in flat for ch in text]
  for (ch,size,flags),(want,want_size,bold,italic) in zip(actual,expected):
   assert ch==want and abs(size-want_size)<.01 and bool(flags&16)==bold and bool(flags&2)==italic
  pdf[0].get_pixmap(matrix=pymupdf.Matrix(1.4,1.4)).save(out/'office.png')
  (out/'office-observation.json').write_text(json.dumps({'fonts':sorted({s['font'] for s in spans}),'pages':len(pdf)},indent=2))
 print('Exported DOCX LibreOffice check passed: every character, complex size/bold/italic and one-page output')
