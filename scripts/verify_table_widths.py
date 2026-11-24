"""Independent original OOXML widths, emitted package and painted PDF geometry."""
import argparse
import hashlib
import json
from pathlib import Path
from zipfile import ZipFile
from lxml import etree
from docx import Document
from pypdf import PdfReader
import pymupdf

parser=argparse.ArgumentParser()
parser.add_argument('--prepare-source',action='store_true')
args=parser.parse_args()
out=Path('output/table-widths')
w='{http://schemas.openxmlformats.org/wordprocessingml/2006/main}'
def sf_tables(sf):
 result=[]
 def walk(n):
  if n['type']=='table':result.append(n)
  for c in n.get('content',[]):walk(c)
 walk(sf)
 return result
def package_tables(path):
 with ZipFile(path) as z:root=etree.fromstring(z.read('word/document.xml'))
 return list(root.iter(w+'tbl'))
def grid(t):return [int(c.get(w+'w')) for c in t.find(w+'tblGrid') if c.tag==w+'gridCol']

source=Path('testdata/real/mqtt-v5.0-os.docx')
assert hashlib.sha256(source.read_bytes()).hexdigest()=='a567582f279f53b300956282b760db95f66e5096d3ca8412321cf1e8403ac58b'
original=package_tables(source);sf=json.loads((out/'mqtt.sf').read_text());tables=sf_tables(sf)
assert len(tables)==len(original)==59
for i,(t,n) in enumerate(zip(original,tables)):
 assert n['attrs']['columnWidths']==[v/20 for v in grid(t)],i
# The isolated original table stays rich; do not regenerate its oracle from output.
sample=json.loads(json.dumps(sf))
sample['content']=[tables[51]]
sample['attrs']['headers']=[];sample['attrs']['footers']=[]
(out/'mqtt-table.sf').write_text(json.dumps(sample,ensure_ascii=False)+'\n')
if args.prepare_source:
 print('All 59 source width grids exact; wrote isolated original table SF')
 raise SystemExit(0)

src=sf_tables(json.loads((out/'source.sf').read_text()))
back=sf_tables(json.loads((out/'from-docx.sf').read_text()))
assert len(src)==len(back)==2
assert [t['attrs']['columnWidths'] for t in src]==[t['attrs']['columnWidths'] for t in back]==[[45,180,75],[90,210]]
word=Document(out/'widths.docx')
assert len(word.tables)==1 and len(word.tables[0].rows)==26
emit=package_tables(out/'widths.docx')
assert grid(emit[0])==[900,3600,1500]
assert grid(emit[1])==[1482,3458] # fit 300pt preference into 255-8pt inner area
assert int(emit[0].find(w+'tblPr/'+w+'tblW').get(w+'w'))==6000
reader=PdfReader(out/'widths.pdf',strict=True);pdf=pymupdf.open(out/'widths.pdf')
assert len(reader.pages)==len(pdf)==4
texts=[p.get_text() for p in pdf]
assert all(t.count('COMBINED HEADER')==1 for t in texts)
assert sum(t.count('Session Expiry Interval (0)') for t in texts)==1
assert sum(t.count('A wide description column retains its') for t in texts)==22
assert sum(t.count('FITTED INNER') for t in texts)==1
# All body column boundaries, including continued/repeated pages, are physical
# 40,85,265,340pt. Nested boundaries fit the merged right-hand cell on page 4.
vertical=[]
for page in pdf:
 xs=set()
 for path in page.get_drawings():
  for item in path['items']:
   if item[0]=='re':xs.update((round(item[1].x0,3),round(item[1].x1,3)))
   elif item[0]=='l' and abs(item[1].x-item[2].x)<.001:xs.add(round(item[1].x,3))
 assert {40.,85.,265.,340.}<=xs,xs
 vertical.append(sorted(xs))
assert {91.0,163.9,334.0}.issubset(set(vertical[-1]))
original_pdf=pymupdf.open(out/'mqtt-table.pdf')
assert len(original_pdf)==1
assert original_pdf[0].get_text().count('Session Expiry Interval (0)')==1
rectangles=[item[1] for path in original_pdf[0].get_drawings() for item in path['items'] if item[0]=='re']
assert any(abs(r.width-180.35)<.001 for r in rectangles)
assert {91.,163.9,334.}<=set(vertical[-1])
original_pdf=pymupdf.open(out/'mqtt-table.pdf')
assert len(original_pdf)==1
assert original_pdf[0].get_text().count('Session Expiry Interval (0)')==1
rectangles=[item[1] for path in original_pdf[0].get_drawings() for item in path['items'] if item[0]=='re']
assert any(abs(r.width-180.35)<.01 and r.height>50 for r in rectangles)
recovered=sf_tables(json.loads((out/'from-pdf.sf').read_text()))
assert recovered
assert any(all(abs(a-b)<.02 for a,b in zip(t['attrs']['columnWidths'],[45,180,75])) and len(t['attrs']['columnWidths'])==3 for t in recovered)
for t in recovered:
 s=t['attrs']['source'];assert s['table_widths']['evidence']=='painted_grid_adjacency'
 assert s['table_widths']['confidence']==.7
report=dict(original_docx_tables=59,original_table_51_pages=1,original_description_cell_points=180.35,source_widths='all exact original OOXML twips / 20',synthetic_pages=4,
 synthetic_docx_grid_twips=[grid(t) for t in emit],pdf_vertical_boundaries=vertical,
 recovered_tables=len(recovered),scope='physical widths, merges, wrapping, repeated and nested grids; not Word layout equivalence')
(out/'independent.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report,indent=2))
