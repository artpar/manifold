"""Reproducible targeted evidence for still-open explicit PDF story semantics."""
from pathlib import Path

out=Path('output/audit');out.mkdir(parents=True,exist_ok=True)
def pdf(objects):
    data=bytearray(b'%PDF-1.7\n');offsets=[0]
    for i,obj in enumerate(objects,1):
        offsets.append(len(data));data.extend(f'{i} 0 obj\n{obj}\nendobj\n'.encode())
    start=len(data);data.extend(f'xref\n0 {len(offsets)}\n0000000000 65535 f \n'.encode())
    for offset in offsets[1:]:data.extend(f'{offset:010d} 00000 n \n'.encode())
    data.extend(f'trailer\n<< /Size {len(offsets)} /Root 1 0 R >>\nstartxref\n{start}\n%%EOF\n'.encode());return data

def stream(content):return f'<< /Length {len(content.encode())} >>\nstream\n{content}\nendstream'
def text(mcid,x,y,value):return f'BT /F 10 Tf /P << /MCID {mcid} >> BDC {x} {y} Td ({value}) Tj EMC ET\n'
base=['<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 6 0 R /MarkInfo << /Marked true >> >>',
      '<< /Type /Pages /Kids [3 0 R] /Count 1 >>',
      '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> >> /Contents 5 0 R >>',
      '<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>']
objects=base+[stream(text(0,20,300,'BODY')+text(1,60,300,'1')+text(2,20,50,'NOTE BODY')),
 '<< /Type /StructTreeRoot /K [7 0 R 9 0 R] >>',
 '<< /Type /StructElem /S /P /P 6 0 R /Pg 3 0 R /K [0 8 0 R] >>',
 '<< /Type /StructElem /S /Reference /P 7 0 R /Pg 3 0 R /Ref [9 0 R] /K 1 >>',
 '<< /Type /StructElem /S /Note /P 6 0 R /Pg 3 0 R /ID (note-1) /K 10 0 R >>',
 '<< /Type /StructElem /S /P /P 9 0 R /Pg 3 0 R /K 2 >>']
(out/'explicit-note.pdf').write_bytes(pdf(objects))
objects=base.copy();objects[0]='<< /Type /Catalog /Pages 2 0 R >>'
objects.append(stream('/Artifact << /Type /Pagination /Subtype /Header /BBox [10 350 220 390] /Attached [/Top] >> BDC BT /F 10 Tf 20 370 Td (HEADER) Tj ET EMC BT /F 10 Tf 20 300 Td (BODY) Tj ET'))
(out/'pagination-artifact.pdf').write_bytes(pdf(objects))
print('Wrote explicit Note/Reference/Ref and Header artifact probes under',out)
