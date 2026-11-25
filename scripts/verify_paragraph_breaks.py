"""Independent checks for conditional paragraph starts and DOCX break codec."""
from pathlib import Path
from zipfile import ZipFile
import argparse
import json
import subprocess
from docx import Document
from docx.shared import Pt
from docx.enum.style import WD_STYLE_TYPE
from lxml import etree
from pypdf import PdfReader
import pymupdf

parser=argparse.ArgumentParser()
parser.add_argument('directory',nargs='?',type=Path,default=Path('output/paragraph-breaks'))
parser.add_argument('--binary',type=Path,default=Path('bin/manifold'))
parser.add_argument('--office',action='store_true')
args=parser.parse_args();out=args.directory
expected={'FIRST':1,'SECOND':2,'NOTE_FIRST':3,'NOTE_SECOND':4,'ITEM_FIRST':5,'ITEM_CONT':6,'CELL_FIRST':7,'CELL_SECOND':8,'SIDE_ZERO':7,'SIDE_ONE':8,'SIDE_TWO':8,'AFTER_TABLE':8}
ns={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main','md':'urn:manydoc:metadata:1'}
word=Document(out/'paragraph-breaks.docx')
assert word.paragraphs[0].paragraph_format.page_break_before is True
with ZipFile(out/'paragraph-breaks.docx') as z:
    main=etree.fromstring(z.read('word/document.xml'));notes=etree.fromstring(z.read('word/footnotes.xml'))
    assert len(main.xpath('//w:pageBreakBefore',namespaces=ns))==6
    assert len(notes.xpath('//w:pageBreakBefore',namespaces=ns))==2
    assert len(main.xpath('//w:p[@md:breakRole="beforeList"]',namespaces=ns))==1
    assert main.xpath('//w:p[@md:breakRole="beforeList"]/@md:conditionalBefore',namespaces=ns)==['1']
    assert not main.xpath('//w:br[@w:type="page"]',namespaces=ns)
reader=PdfReader(out/'paragraph-breaks.pdf',strict=True);assert len(reader.pages)==8
annots=[a.get_object() for a in reader.pages[1]['/Annots']]
internal=next(a for a in annots if '/Dest' in a)
assert internal['/Dest'][0].idnum==reader.pages[2].indirect_reference.idnum
assert any(a.get('/A',{}).get('/URI')=='https://example.invalid/paragraph' for a in annots)
with pymupdf.open(out/'paragraph-breaks.pdf') as doc:
    for token,page in expected.items():
        found=[i+1 for i,p in enumerate(doc) for w in p.get_text('words') if w[4].rstrip('0123456789')==token]
        assert found==[page],(token,found,page)
    for token,page,x in [('CELL_FIRST',6,26),('CELL_SECOND',7,26),('SIDE_ZERO',6,156),('SIDE_ONE',7,156)]:
        assert abs(doc[page].search_for(token)[0].x0-x)<.04
    spans=[s for b in doc[1].get_text('dict')['blocks'] if b['type']==0 for l in b['lines'] for s in l['spans']]
    assert any(s['text']=='SECOND' and s['flags']&16 for s in spans)
    for i,p in enumerate(doc):
        for w in p.get_text('words'):assert w[0]>=19.95 and w[1]>=19.95 and w[2]<=280.05 and w[3]<=160.05,(i,w)
        p.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/f'page-{i+1}.png')

# Author a separate source with python-docx, not manydoc's own writer.
source=Document();section=source.sections[0]
section.page_width=Pt(300);section.page_height=Pt(180)
section.top_margin=section.bottom_margin=section.left_margin=section.right_margin=Pt(20)
base=source.styles.add_style('StartPage',WD_STYLE_TYPE.PARAGRAPH)
base.paragraph_format.page_break_before=True
inherited=source.styles.add_style('InheritedStart',WD_STYLE_TYPE.PARAGRAPH);inherited.base_style=base
source.add_paragraph('SOURCE_FIRST').paragraph_format.page_break_before=True
source.add_paragraph('SOURCE_BASE','StartPage')
source.add_paragraph('SOURCE_CANCEL','InheritedStart').paragraph_format.page_break_before=False
source.add_paragraph('SOURCE_INHERIT','InheritedStart')
path=out/'independent.docx';source.save(path)
def convert(a,b):subprocess.run([str(args.binary.resolve()),'convert','-allow-lossy','-pdf-font-map','Calibri=Go','-pdf-font-map','Cambria=Go','-o',str(b),str(a)],check=True,capture_output=True)
convert(path,out/'independent.sf');convert(out/'independent.sf',out/'independent.pdf')
sf=json.loads((out/'independent.sf').read_text())
assert [n.get('attrs',{}).get('pageBreakBefore',False) for n in sf['content']]==[True,True,False,True]
with pymupdf.open(out/'independent.pdf') as doc:
    assert len(doc)==3
    for token,page in [('SOURCE_FIRST',0),('SOURCE_BASE',1),('SOURCE_CANCEL',1),('SOURCE_INHERIT',2)]:
        assert sum(len(p.search_for(token)) for p in doc)==1 and doc[page].search_for(token)
        doc[page].get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/f'independent-page-{page+1}.png')
print('Paragraph starts passed: eight native pages, conditional/explicit distinction, list/cell/note ownership, real destination and independently authored style inheritance/cancellation')
if args.office:
    with pymupdf.open(out/'lo'/'paragraph-breaks.pdf') as doc:
        for token in expected:
            assert sum(w[4].rstrip('0123456789')==token for p in doc for w in p.get_text('words'))==1,token
        assert all(p.get_text().strip() for p in doc),'unexpected synthetic blank page'
        for i,p in enumerate(doc):p.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/'lo'/f'page-{i+1}.png')
        print(f'LibreOffice content check passed: {len(doc)} pages, no missing tokens or synthetic blank page; cell/note pagination differs from native')
