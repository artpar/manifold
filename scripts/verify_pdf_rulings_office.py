"""Targeted DOCX exported by LibreOffice -> untagged PDF -> manydoc SF.

Development-only producer/reader qualification. This generated fixture is not
a representative published real-world document, nor a runtime dependency.
"""
from pathlib import Path
import argparse
import json
import subprocess
from pypdf import PdfReader
from docx import Document
from docx.shared import Pt
from docx.enum.table import WD_ROW_HEIGHT_RULE
import pymupdf

parser=argparse.ArgumentParser()
parser.add_argument('directory',nargs='?',type=Path,default=Path('output/pdf-rulings/office'))
parser.add_argument('--soffice',default='/Applications/LibreOffice.app/Contents/MacOS/soffice')
args=parser.parse_args();out=args.directory.resolve();out.mkdir(parents=True,exist_ok=True)
doc=Document();section=doc.sections[0];section.page_width=Pt(600);section.page_height=Pt(800)
section.top_margin=section.bottom_margin=section.left_margin=section.right_margin=Pt(54)
doc.add_paragraph('Independent Writer export')
table=doc.add_table(rows=3,cols=3);table.style='Table Grid';table.autofit=False
for col in table.columns:col.width=Pt(100)
for row in table.rows:row.height=Pt(40);row.height_rule=WD_ROW_HEIGHT_RULE.EXACTLY
for r,c,value in [(0,0,'MERGED TOP'),(0,2,'Top right'),(1,0,'Two lines\nin this cell'),(1,1,'Middle'),(2,1,'Bottom'),(2,2,'Linked')]:table.cell(r,c).text=value
table.cell(0,0).merge(table.cell(0,1));table.cell(1,0).merge(table.cell(2,0))
for row in table.rows:
    for cell in row.cells:
        for paragraph in cell.paragraphs:
            for run in paragraph.runs:run.font.name='Liberation Sans';run.font.size=Pt(10)
doc.save(out/'writer-source.docx')
profile=(out/'lo-profile').as_uri()
filter_options='pdf:writer_pdf_Export:'+json.dumps({'UseTaggedPDF':{'type':'boolean','value':'false'},'PDFUACompliance':{'type':'boolean','value':'false'}},separators=(',',':'))
subprocess.run([args.soffice,f'-env:UserInstallation={profile}','--headless','--convert-to',filter_options,'--outdir',str(out),str(out/'writer-source.docx')],check=True)
source=out/'writer-source.pdf';reader=PdfReader(source,strict=True)
assert '/StructTreeRoot' not in reader.trailer['/Root'] and len(reader.pages)==1
sf_path=out/'recovered.sf'
subprocess.run(['bin/manydoc','extract','-format','sf','-o',str(sf_path),str(source)],check=True)
sf=json.loads(sf_path.read_text());tables=[n for n in sf['content'] if n['type']=='table'];assert len(tables)==1
rows=tables[0]['content'];assert [len(row['content']) for row in rows]==[2,3,2]
assert rows[0]['content'][0]['attrs']['colspan']==2 and rows[1]['content'][0]['attrs']['rowspan']==2
plain=lambda n:n.get('text','')+''.join(plain(c) for c in n.get('content',[]))
assert [[plain(c) for c in row['content']] for row in rows]==[['MERGED TOP','Top right'],['Two lines in this cell','Middle',''],['Bottom','Linked']]
with pymupdf.open(source) as pdf:
    found=pdf[0].find_tables().tables;assert len(found)==1
    expected={tuple(round(v,3) for v in box) for box in found[0].cells}
    actual=set()
    for row in rows:
        for cell in row['content']:
            a=cell['attrs']['source'];assert a['structure']=='inferred'
            q=a['quad'];actual.add(tuple(round(v,3) for v in (q[0]['x'],q[0]['y'],q[2]['x'],q[2]['y'])))
    assert actual==expected
    pdf[0].get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/'source.png')
for ext in ('docx','pdf'):subprocess.run(['bin/manydoc','convert','-allow-lossy','-pdf-font-map','Liberation Sans=Go','-pdf-font-map','Carlito=Go','-pdf-font-map','Caladea=Go','-o',str(out/f'reflow.{ext}'),str(sf_path)],check=True)
word=Document(out/'reflow.docx');assert len(word.tables)==1
t=word.tables[0];assert t.cell(0,0)._tc is t.cell(0,1)._tc and t.cell(1,0)._tc is t.cell(2,0)._tc and t.cell(1,2).text==''
with pymupdf.open(out/'reflow.pdf') as pdf:
    assert len(pdf)==1
    found=pdf[0].find_tables().tables;assert len(found)==1 and found[0].row_count==3 and found[0].col_count==3 and len(found[0].cells)==7
    expected=''.join(plain(n) for n in sf['content'])
    assert ''.join(pdf[0].get_text().split())==''.join(expected.split())
    pdf[0].get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/'native.png')
lo=out/'lo';lo.mkdir(exist_ok=True)
subprocess.run([args.soffice,f'-env:UserInstallation={profile}','--headless','--convert-to','pdf','--outdir',str(lo),str(out/'reflow.docx')],check=True)
with pymupdf.open(lo/'reflow.pdf') as pdf:
    assert len(pdf)==1
    assert all(word in pdf[0].get_text() for word in ('Independent Writer export','MERGED TOP','Middle','Bottom','Linked'))
    pdf[0].get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(lo/'page.png')
print('Targeted LibreOffice untagged export passed: seven exact MuPDF cell bounds, colspan/rowspan/empty/multiline content; generated DOCX merge identities and native/LibreOffice PDF grids/content')
