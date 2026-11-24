"""Independent package semantics, native/office paint and positioned recovery."""
import json
from pathlib import Path
from zipfile import ZipFile
from lxml import etree
from pypdf import PdfReader
import pymupdf

out=Path('output/numbering')
a=json.loads((out/'source.sf').read_text());b=json.loads((out/'from-docx.sf').read_text())
assert a==b,'SF/DOCX tree, attributes, stories or note definitions changed'
expected=['H01:','Appendix AA.','AA.iv)','Appendix BB.','BB.iv)','2.4.7','[MMMM]','(aaa)','(bb)','Fix:']
readers={}
for role,path in [('native',out/'numbering.pdf'),('office',out/'lo/numbering.pdf')]:
 pdf=pymupdf.open(path);raw=PdfReader(path,strict=True)
 assert len(pdf)==len(raw.pages)==1
 text=' '.join(pdf[0].get_text().split())
 for label in expected:assert text.count(label)==1,(role,label,text)
 for word in ['HEADER','PARENT','NEXT PARENT','CHILD','LEGAL EXTERNAL','ROMAN 4000','CELL LIST','CELL VALUE','REFERENCE','NOTE LIST','FOOTER']:assert word in text
 # Locate marker shapes inside their appropriate physical story, not just text.
 hits={label:[list(r) for r in pdf[0].search_for(label)] for label in expected}
 assert hits['H01:'][0][1]<40 and hits['Fix:'][0][1]>450
 assert hits['(bb)'][0][1]>pdf[0].search_for('REFERENCE')[0].y1
 pdf[0].get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/f'{role}-page-1.png')
 readers[role]=dict(pages=len(pdf),marker_bounds=hits)
w='{http://schemas.openxmlformats.org/wordprocessingml/2006/main}'
with ZipFile(out/'numbering.docx') as z:
 nr=etree.fromstring(z.read('word/numbering.xml'))
 levels=list(nr.iter(w+'lvl'))
 # Literal strings and declared formats are real OOXML properties.
 assert any(n.find(w+'lvlText').get(w+'val')=='Appendix %1.' and n.find(w+'numFmt').get(w+'val')=='upperLetter' for n in levels)
 legal=[n for n in levels if n.find(w+'isLgl') is not None]
 assert len(legal)==1
 assert legal[0].find(w+'numFmt').get(w+'val')=='decimal'
 assert json.loads(legal[0].get('{urn:manydoc:metadata:1}numberingFormats'))==['upperLetter','lowerRoman','decimalZero']
 # The actual abstract start agrees with the override in every story.
 abstracts={n.get(w+'abstractNumId'):n for n in nr.findall(w+'abstractNum')}
 for n in nr.findall(w+'num'):
  abstract=abstracts[n.find(w+'abstractNumId').get(w+'val')]
  for ov in n.findall(w+'lvlOverride'):
   level=next(v for v in abstract.findall(w+'lvl') if v.get(w+'ilvl')==ov.get(w+'ilvl'))
   assert level.find(w+'start').get(w+'val')==ov.find(w+'startOverride').get(w+'val')
sf=json.loads((out/'from-pdf.sf').read_text());parts=[];positioned=0
for node in sf['content']:
 for span in node.get('content',[]):
  if span['type']=='text':
   parts.append(span['text'])
   sources=[m['attrs'] for m in span.get('marks',[]) if m['type']=='source']
   assert len(sources)==1 and sources[0]['page']==1
   if 'quad' not in sources[0]:
    assert not span['text'].strip() and sources[0]['evidence']=='positioned_word_gap'
   else:
    assert len(sources[0]['quad'])==4
    positioned+=1
text=' '.join(' '.join(parts).split())
for label in expected:assert label in text,(label,text)
report=dict(marker_labels=expected,readers=readers,sf_docx_exact=True,positioned_pdf_spans=positioned,
 scope='six common numbering formats, OOXML multilevel templates/legal decimal display, nested edits, body/cell/header/footer/note; PDF keeps painted labels/positions, original numbering templates are not recoverable from untagged paint')
(out/'independent.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report,indent=2))
