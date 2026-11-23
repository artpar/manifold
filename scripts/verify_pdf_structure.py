"""Independent tagged-PDF structure -> SF -> DOCX/PDF checks (optional QA)."""
from pathlib import Path
import argparse
import json
import subprocess
from zipfile import ZipFile
from pypdf import PdfReader
from docx import Document
from lxml import etree
import pymupdf

parser=argparse.ArgumentParser()
parser.add_argument('directory',nargs='?',type=Path,default=Path('output/pdf-structure'))
args=parser.parse_args();root=args.directory;root.mkdir(parents=True,exist_ok=True)

def stream(text,attrs=''):
    if isinstance(text,str):text=text.encode('ascii')
    return f'<< {attrs} /Length {len(text)} >>\nstream\n'.encode()+text+b'\nendstream'
def marked(i,x,y,text):
    text=text.replace('\\','\\\\').replace('(','\\(').replace(')','\\)')
    return f'BT /F 10 Tf /Span << /MCID {i} >> BDC 1 0 0 1 {x} {y} Tm ({text}) Tj EMC ET\n'

# Fixed object references are intentional, making containment and expected
# ownership independently reviewable rather than derived from manydoc output.
objects=[
'<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 11 0 R /MarkInfo << /Marked true >> >>',
'<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>',
'<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 5 0 R >> /XObject << /Img 9 0 R >> >> /Contents 6 0 R /StructParents 0 /Annots [10 0 R] >>',
'<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 5 0 R >> /XObject << /X 8 0 R >> >> /Contents 7 0 R /StructParents 1 >>',
'<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>',
stream(marked(1,20,310,'Before')+marked(0,20,350,'Tagged heading')+marked(2,20,270,'3.')+marked(3,45,270,'First item')+marked(4,20,250,'4.')+marked(5,45,250,'Second item')+marked(6,20,140,'MERGED')+marked(7,200,140,'TOP')+'q 30 0 0 20 20 40 cm /Figure << /MCID 8 >> BDC /Img Do EMC Q\n'+marked(9,20,200,'Example link')+'/Artifact BMC BT /F 8 Tf 20 15 Td (PAGE ONE FOOTER) Tj ET EMC'),
stream(marked(0,20,330,'after')+marked(1,200,350,'BOTTOM')+'q 1 0 0 1 20 120 cm /X Do Q\n/Artifact BMC BT /F 8 Tf 20 15 Td (PAGE TWO FOOTER) Tj ET EMC'),
stream(marked(0,10,20,'FORM TEXT'),'/Type /XObject /Subtype /Form /BBox [0 0 150 60] /Resources << /Font << /F 5 0 R >> >> /StructParents 2'),
stream(bytes([255,0,0]),'/Type /XObject /Subtype /Image /Width 1 /Height 1 /BitsPerComponent 8 /ColorSpace /DeviceRGB'),
'<< /Type /Annot /Subtype /Link /P 3 0 R /Rect [20 197 95 210] /A << /S /URI /URI (https://example.invalid/structure) >> /StructParent 3 >>',
'<< /Type /StructTreeRoot /K 12 0 R /RoleMap << /Chapter /H2 >> /ClassMap << /Merged << /O /Table /RowSpan 2 /ColSpan 2 >> >> /ParentTree 32 0 R >>',
'<< /Type /StructElem /S /Document /P 11 0 R /K [13 0 R 14 0 R 15 0 R 22 0 R 28 0 R 29 0 R 30 0 R] >>',
'<< /Type /StructElem /S /Chapter /P 12 0 R /Pg 3 0 R /K 0 >>',
'<< /Type /StructElem /S /P /P 12 0 R /K [<< /Type /MCR /Pg 3 0 R /MCID 1 >> << /Type /MCR /Pg 4 0 R /MCID 0 >>] >>',
'<< /Type /StructElem /S /L /P 12 0 R /Pg 3 0 R /A << /O /List /ListNumbering /Decimal >> /K [16 0 R 17 0 R] >>',
'<< /Type /StructElem /S /LI /P 15 0 R /K [18 0 R 19 0 R] >>',
'<< /Type /StructElem /S /LI /P 15 0 R /K [20 0 R 21 0 R] >>',
'<< /Type /StructElem /S /Lbl /P 16 0 R /K 2 >>',
'<< /Type /StructElem /S /LBody /P 16 0 R /K 3 >>',
'<< /Type /StructElem /S /Lbl /P 17 0 R /K 4 >>',
'<< /Type /StructElem /S /LBody /P 17 0 R /K 5 >>',
'<< /Type /StructElem /S /Table /P 12 0 R /K [23 0 R 24 0 R] >>',
'<< /Type /StructElem /S /TR /P 22 0 R /Pg 3 0 R /K [25 0 R 26 0 R] >>',
'<< /Type /StructElem /S /TR /P 22 0 R /Pg 4 0 R /K 27 0 R >>',
'<< /Type /StructElem /S /TH /P 23 0 R /C /Merged /K 6 >>',
'<< /Type /StructElem /S /TD /P 23 0 R /K 7 >>',
'<< /Type /StructElem /S /TD /P 24 0 R /K 1 >>',
'<< /Type /StructElem /S /Figure /P 12 0 R /Pg 3 0 R /Alt (Red square) /K 8 >>',
'<< /Type /StructElem /S /P /P 12 0 R /Pg 4 0 R /K << /Type /MCR /Stm 8 0 R /MCID 0 >> >>',
'<< /Type /StructElem /S /P /P 12 0 R /Pg 3 0 R /K 31 0 R >>',
'<< /Type /StructElem /S /Link /P 30 0 R /K [9 << /Type /OBJR /Obj 10 0 R >>] >>',
'<< /Nums [0 [13 0 R 14 0 R 18 0 R 19 0 R 20 0 R 21 0 R 25 0 R 26 0 R 28 0 R 31 0 R] 1 [14 0 R 27 0 R] 2 [29 0 R] 3 31 0 R] >>',
]
data=bytearray(b'%PDF-1.7\n');offsets=[0]
for i,obj in enumerate(objects,1):
    if isinstance(obj,str):obj=obj.encode('ascii')
    offsets.append(len(data));data.extend(f'{i} 0 obj\n'.encode()+obj+b'\nendobj\n')
xref=len(data);data.extend(f'xref\n0 {len(offsets)}\n0000000000 65535 f \n'.encode())
for offset in offsets[1:]:data.extend(f'{offset:010d} 00000 n \n'.encode())
data.extend(f'trailer\n<< /Size {len(offsets)} /Root 1 0 R >>\nstartxref\n{xref}\n%%EOF\n'.encode())
source=root/'tagged.pdf';source.write_bytes(data)
r=PdfReader(source,strict=True);assert len(r.pages)==2
assert r.trailer['/Root']['/StructTreeRoot']['/ClassMap']['/Merged']['/RowSpan']==2
sf_path=root/'tagged.sf';subprocess.run(['bin/manydoc','extract','-format','sf','-o',str(sf_path),str(source)],check=True)
sf=json.loads(sf_path.read_text());bs=sf['content'];assert len(sf['attrs']['pages'])==2
text=lambda n: ''.join(x.get('text','') for x in n.get('content',[]))
assert bs[0]['type']=='heading' and bs[0]['attrs']['level']==2 and text(bs[0])=='Tagged heading'
assert text(bs[1])=='Before after' and len(bs[1]['attrs']['source']['regions'])==2
assert bs[1]['attrs']['source']['structure']=='explicit'
assert bs[2]['type']=='ordered_list' and bs[2]['attrs']['order']==3 and len(bs[2]['content'])==2
assert [i['attrs']['source']['label'] for i in bs[2]['content']]==['3.','4.']
table=bs[3];assert table['type']=='table' and len(table['content'])==2
cell=table['content'][0]['content'][0];assert cell['attrs']['rowspan']==2 and cell['attrs']['colspan']==2 and cell['attrs']['source']['role']=='TH'
assert text(cell['content'][0])=='MERGED' and len(table['content'][1]['content'])==1
assert bs[4]['type']=='image' and bs[4]['attrs']['alt']=='Red square'
assert text(bs[5])=='FORM TEXT' and text(bs[6])=='Example link'
assert any(m['type']=='link' and m['attrs']['href']=='https://example.invalid/structure' for m in bs[6]['content'][0]['marks'])
assert [text(b) for b in bs[-2:]]==['PAGE ONE FOOTER','PAGE TWO FOOTER']
for ext in ('docx','pdf'):
    subprocess.run(['bin/manydoc','convert','-allow-lossy','-pdf-font-map','Courier=Go Mono','-o',str(root/f'reflow.{ext}'),str(sf_path)],check=True)
word=Document(root/'reflow.docx');assert len(word.tables)==1 and len(word.inline_shapes)==1
wt=word.tables[0];assert wt.cell(0,0)._tc is wt.cell(1,1)._tc and wt.cell(0,0).text=='MERGED'
with ZipFile(root/'reflow.docx') as z:
    ns={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}
    numbering=etree.fromstring(z.read('word/numbering.xml'))
    assert '3' in numbering.xpath('//w:startOverride/@w:val',namespaces=ns)
    assert b'https://example.invalid/structure' in z.read('word/_rels/document.xml.rels')
pdf=PdfReader(root/'reflow.pdf',strict=True);assert len(pdf.pages)==1
body=''.join(p.extract_text() for p in pdf.pages)
assert all(word in body for word in ('Tagged heading','Before after','3.','4.','MERGED','TOP','BOTTOM','FORM TEXT','PAGE ONE FOOTER','PAGE TWO FOOTER'))
assert sum(len(p.get_image_info()) for p in pymupdf.open(root/'reflow.pdf'))==1
for filename in ('tagged.pdf','reflow.pdf'):
    with pymupdf.open(root/filename) as doc:
        for i,page in enumerate(doc):page.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(root/f'{filename[:-4]}-{i+1}.png')
print('Tagged tree, logical order, cross-page paragraph, list starts/labels, merged table, image/alt, Form scope, URI, unclaimed content and rendered outputs passed independent checks')
