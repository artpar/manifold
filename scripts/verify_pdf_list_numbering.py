"""Independent tagged-PDF/SF/OOXML numbering checks; optional QA only.
Run after MANYDOC_PDF_LIST_NUMBERING_QA=output/pdf-list-numbering go test ...
"""
from pathlib import Path
from collections import Counter
import json
import re
import zipfile
import xml.etree.ElementTree as E
from pypdf import PdfReader
from pypdf.generic import IndirectObject

root = Path('output/pdf-list-numbering')
cases = {
 'lowerRoman-false': ('LowerRoman', 'lowerRoman', '%1.', 4, ['iv.', 'v.', 'vi.']),
 'upperRoman-false': ('UpperRoman', 'upperRoman', '(%1)', 4, ['(IV)', '(V)', '(VI)']),
 'upperLetter-false': ('UpperAlpha', 'upperLetter', 'Annex %1)', 26, ['Annex Z)', 'Annex AA)', 'Annex BB)']),
 'lowerLetter-false': ('LowerAlpha', 'lowerLetter', '%1:', 27, ['aa:', 'bb:', 'cc:']),
 'decimalZero-false': ('Decimal', 'decimalZero', '%1)', 8, ['08)', '09)', '10)']),
 'upperRoman-true': ('Decimal', 'decimal', '%1.', 4, ['4.', '5.', '6.']),
}

def tree(value):
 if isinstance(value, list):
  for child in value:
   yield from tree(child)
 elif isinstance(value, IndirectObject):
  node = value.get_object()
  if node.get('/Type') == '/StructElem':
   yield node
   yield from tree(node.raw_get('/K'))

ns = {'w': 'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}
attr = '{'+ns['w']+'}'
for name, (pdf_format, word_format, template, start, labels) in cases.items():
 pdf = PdfReader(root/(name+'.pdf'), strict=True)
 nodes = list(tree(pdf.trailer['/Root']['/StructTreeRoot'].raw_get('/K')))
 roles = Counter(n['/S'] for n in nodes)
 assert roles['/L'] == 1 and roles['/LI'] == roles['/Lbl'] == roles['/LBody'] == 3
 lists = [n for n in nodes if n['/S'] == '/L']
 assert lists[0]['/A']['/ListNumbering'] == '/'+pdf_format
 visible = '\n'.join(p.extract_text() for p in pdf.pages)
 for i, label in enumerate(labels):
  assert sum(bool(re.fullmatch(re.escape(label) + r'\s*BODY_' + str(i), line.strip())) for line in visible.splitlines()) == 1, (name, label, visible)
  assert visible.count('BODY_'+str(i)) == 1
 sf = json.loads((root/(name+'.sf')).read_text())
 node = sf['content'][0]
 assert node['type'] == 'ordered_list' and node['attrs']['order'] == start
 numbering = node['attrs'].get('numbering')
 if word_format == 'decimal' and template == '%1.':
  assert numbering is None
 else:
  assert numbering['template'] == template and numbering['formats'] == [word_format]
  assert numbering['level'] == 0
 assert node['attrs']['source']['structure'] == 'explicit'
 assert node['attrs']['source']['role'] == 'L'
 for i, item in enumerate(node['content']):
  assert item['type'] == 'list_item' and item['attrs']['source']['label'] == labels[i]
  text = item['content'][0]['content']
  assert ''.join(r.get('text', '') for r in text) == 'BODY_'+str(i)
  assert all(any(m['type'] == 'strong' for m in r.get('marks', [])) for r in text)
  assert all(any(m['type'] == 'link' and m['attrs']['href'] == 'https://example.org/list' for m in r.get('marks', [])) for r in text)
 with zipfile.ZipFile(root/(name+'.docx')) as package:
  numbering_xml = E.fromstring(package.read('word/numbering.xml'))
  body = E.fromstring(package.read('word/document.xml'))
  paragraphs = [p for p in body.findall('.//w:p', ns) if p.find('w:pPr/w:numPr', ns) is not None]
  assert len(paragraphs) == 3
  instances = {n.get(attr+'numId'): n for n in numbering_xml.findall('w:num', ns)}
  abstracts = {n.get(attr+'abstractNumId'): n for n in numbering_xml.findall('w:abstractNum', ns)}
  for paragraph in paragraphs:
   identity = paragraph.find('w:pPr/w:numPr/w:numId', ns).get(attr+'val')
   instance = instances[identity]
   abstract = abstracts[instance.find('w:abstractNumId', ns).get(attr+'val')]
   level = abstract.find('w:lvl', ns)
   assert level.find('w:numFmt', ns).get(attr+'val') == word_format
   assert level.find('w:lvlText', ns).get(attr+'val') == template
   override = instance.find('w:lvlOverride/w:startOverride', ns)
   actual_start = override.get(attr+'val') if override is not None else level.find('w:start', ns).get(attr+'val')
   assert int(actual_start) == start
 print(name, 'actual L/LI/Lbl/LBody, rendered labels, SF style/links/source, OOXML format/template/start pass')
print('6 independent format cases / 18 editable items pass; no browser/Word-fidelity or arbitrary counter-alphabet claim.')
