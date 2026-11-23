"""Independent ruled-PDF grid -> SF -> DOCX/PDF checks. Optional QA only."""
from pathlib import Path
from collections import Counter
from zipfile import ZipFile
import argparse
import json
import subprocess
from pypdf import PdfReader
from docx import Document
import pymupdf

parser=argparse.ArgumentParser();parser.add_argument('directory',nargs='?',type=Path,default=Path('output/pdf-rulings'))
out=parser.parse_args().directory;out.mkdir(parents=True,exist_ok=True)
edges=[(40,480,340,480),(40,600,340,600),(40,560,340,560),(140,520,340,520),(40,480,40,600),(340,480,340,600),(240,480,240,600),(140,480,140,560)]
def text(font,x,y,value):return f'BT /{font} 10 Tf 1 0 0 1 {x} {y} Tm ({value}) Tj ET\n'
body=text('F',250,500,'Linked')+text('B',50,580,'MERGED TOP')+text('F',50,542,'Two lines')+text('F',50,528,'in this cell')+text('F',250,580,'Top right')+text('F',150,540,'Middle')+text('F',150,500,'Bottom')
def stream(content,attrs=''):
    if isinstance(content,str):content=content.encode('ascii')
    return f'<< {attrs} /Length {len(content)} >>\nstream\n'.encode()+content+b'\nendstream'
paint=[]
for filled in (False,True):
    p='/Opaque gs\n0 0 600 800 re W n\n'
    for x0,y0,x1,y1 in edges:
        if filled:
            p+=f'{x0} {y0-.5} {x1-x0} 1 re f\n' if y0==y1 else f'{x0-.5} {y0} 1 {y1-y0} re f\n'
        else:p+=f'{x0} {y0} m {x1} {y1} l\n'
    if not filled:p+='S\n'
    paint.append(stream(p+body))
resources='/Resources << /Font << /F 5 0 R /B 6 0 R >> /ExtGState << /Opaque 11 0 R >> >>'
objects=[
    '<< /Type /Catalog /Pages 2 0 R >>','<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>',
    f'<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] {resources} /Contents 7 0 R /Annots [9 0 R] >>',
    f'<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] {resources} /Contents 8 0 R /Annots [10 0 R] >>',
    '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>','<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>',*paint,
    '<< /Type /Annot /Subtype /Link /P 3 0 R /Rect [249 497 330 512] /A << /S /URI /URI (https://example.invalid/cell) >> >>',
    '<< /Type /Annot /Subtype /Link /P 4 0 R /Rect [249 497 330 512] /A << /S /URI /URI (https://example.invalid/cell) >> >>',
    '<< /Type /ExtGState /CA 1 /ca 1 /BM /Normal /SMask /None >>',
]
pdf=bytearray(b'%PDF-1.7\n');offsets=[0]
for i,obj in enumerate(objects,1):
    if isinstance(obj,str):obj=obj.encode('ascii')
    offsets.append(len(pdf));pdf.extend(f'{i} 0 obj\n'.encode()+obj+b'\nendobj\n')
xref=len(pdf);pdf.extend(f'xref\n0 {len(offsets)}\n0000000000 65535 f \n'.encode())
for offset in offsets[1:]:pdf.extend(f'{offset:010d} 00000 n \n'.encode())
pdf.extend(f'trailer\n<< /Size {len(offsets)} /Root 1 0 R >>\nstartxref\n{xref}\n%%EOF\n'.encode())
source=out/'ruled.pdf';source.write_bytes(pdf)
reader=PdfReader(source,strict=True);assert len(reader.pages)==2 and '/StructTreeRoot' not in reader.trailer['/Root']
expected_bounds={(40,200,240,240),(240,200,340,240),(40,240,140,320),(140,240,240,280),(240,240,340,280),(140,280,240,320),(240,280,340,320)}
with pymupdf.open(source) as doc:
    original=''.join(page.get_text() for page in doc)
    for i,page in enumerate(doc):
        tables=page.find_tables().tables;assert len(tables)==1
        t=tables[0];assert t.row_count==3 and t.col_count==3
        assert {tuple(cell) for cell in t.cells if cell is not None}==expected_bounds
        page.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/f'source-{i+1}.png')
sf_path=out/'ruled.sf'
subprocess.run(['bin/manydoc','extract','-format','sf','-o',str(sf_path),str(source)],check=True)
sf=json.loads(sf_path.read_text());bs=sf['content'];assert [n['type'] for n in bs]==['table','page_break','table']
plain=lambda n:n.get('text','')+''.join(plain(c) for c in n.get('content',[]))
for table in (bs[0],bs[2]):
    rows=table['content'];assert [len(r['content']) for r in rows]==[2,3,2]
    assert rows[0]['content'][0]['attrs']['colspan']==2 and rows[1]['content'][0]['attrs']['rowspan']==2
    assert [[plain(c) for c in r['content']] for r in rows]==[['MERGED TOP','Top right'],['Two lines in this cell','Middle',''],['Bottom','Linked']]
    assert any(m['type']=='strong' for m in rows[0]['content'][0]['content'][0]['content'][0]['marks'])
    assert any(m['type']=='link' and m['attrs']['href']=='https://example.invalid/cell' for m in rows[2]['content'][1]['content'][0]['content'][0]['marks'])
    bounds=set()
    for row in rows:
        for cell in row['content']:
            a=cell['attrs']['source'];assert a['structure']=='inferred' and a['precision']=='ruling_boundary_bounds'
            q=a['quad'];bounds.add((q[0]['x'],q[0]['y'],q[2]['x'],q[2]['y']))
    assert bounds==expected_bounds
norm=lambda s:Counter(''.join(s.split()))
assert norm(plain(sf))==norm(original)
assert any(d['code']=='pdf_table_rulings_inferred' for d in sf['attrs']['diagnostics'])
for ext in ('docx','pdf'):subprocess.run(['bin/manydoc','convert','-allow-lossy','-pdf-font-map','Helvetica=Go','-o',str(out/f'reflow.{ext}'),str(sf_path)],check=True)
word=Document(out/'reflow.docx');assert len(word.tables)==2
for t in word.tables:
    assert t.cell(0,0)._tc is t.cell(0,1)._tc and t.cell(1,0)._tc is t.cell(2,0)._tc
    assert t.cell(1,2).text=='' and t.cell(1,0).text=='Two lines in this cell'
with ZipFile(out/'reflow.docx') as z:assert b'https://example.invalid/cell' in z.read('word/_rels/document.xml.rels')
flow=PdfReader(out/'reflow.pdf',strict=True);assert len(flow.pages)==2
with pymupdf.open(out/'reflow.pdf') as doc:
    assert norm(''.join(page.get_text() for page in doc))==norm(original)
    for i,page in enumerate(doc):
        tables=page.find_tables().tables;assert len(tables)==1
        t=tables[0];assert t.row_count==3 and t.col_count==3 and len(t.cells)==7
        for b in page.get_text('dict')['blocks']:
            for line in b.get('lines',[]):
                x0,y0,x1,y1=line['bbox'];assert x0>=53.9 and y0>=53.9 and x1<=page.rect.width-53.9 and y1<=page.rect.height-53.9
        page.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/f'reflow-{i+1}.png')
print('Ruled PDF passed: independent MuPDF grid/merges/empty-cell bounds, stroked/filled paths and clipping/opaque state; exact SF cells/marks/links/pages/content; DOCX merge identities; two-page native PDF grids/content/bounds')
