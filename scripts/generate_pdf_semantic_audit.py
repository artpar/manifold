"""Reproducible independent PDF note/artifact semantic probes."""
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

# A second independent hand-authored source exercises a custom repeated mark,
# explicit subtype, a list, a table, exact raster pixels and a note-body view.
content=(text(0,20,300,'BODY')+text(1,60,300,'*1')+text(2,20,80,'*1')+
         text(3,40,80,'NOTE BODY')+text(4,40,65,'LIST ITEM')+text(5,40,50,'TABLE CELL')+
         text(6,80,300,'MIDDLE')+text(7,125,300,'*1')+text(8,145,300,'AFTER')+text(9,20,65,'1.')+
         '/Figure << /MCID 10 >> BDC q 20 0 0 10 40 30 cm BI /W 2 /H 1 /BPC 8 /CS /RGB /F /AHx ID FF00000000FF> EI Q EMC\n')
objects=base.copy()
objects[0]='<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 6 0 R /Dests << /note [3 0 R /XYZ 40 80 null] >> /MarkInfo << /Marked true >> >>'
objects[3]='<< /Type /Font /Subtype /Type1 /BaseFont /Courier-Bold >>'
objects += [stream(content), '<< /Type /StructTreeRoot /K [7 0 R 9 0 R] >>',
 '<< /Type /StructElem /S /P /P 6 0 R /Pg 3 0 R /K [0 8 0 R 6 18 0 R 8] >>',
 '<< /Type /StructElem /S /Reference /P 7 0 R /Pg 3 0 R /Ref [9 0 R] /K 1 >>',
 '<< /Type /StructElem /S /FENote /A << /O /FENote /NoteType /Footnote >> /P 6 0 R /Pg 3 0 R /ID (note-1) /Ref [8 0 R 18 0 R] /K [10 0 R 11 0 R 12 0 R 15 0 R 20 0 R] >>',
 '<< /Type /StructElem /S /Lbl /P 9 0 R /K 2 >>',
 '<< /Type /StructElem /S /P /P 9 0 R /K 3 >>',
 '<< /Type /StructElem /S /L /A << /O /List /ListNumbering /Decimal >> /P 9 0 R /K 13 0 R >>',
 '<< /Type /StructElem /S /LI /P 12 0 R /K [19 0 R 14 0 R] >>',
 '<< /Type /StructElem /S /LBody /P 13 0 R /K 4 >>',
 '<< /Type /StructElem /S /Table /P 9 0 R /K 16 0 R >>',
 '<< /Type /StructElem /S /TR /P 15 0 R /K 17 0 R >>',
 '<< /Type /StructElem /S /TD /P 16 0 R /K 5 >>',
 '<< /Type /StructElem /S /Span /P 7 0 R /Pg 3 0 R /K 7 >>',
 '<< /Type /StructElem /S /Lbl /P 13 0 R /K 9 >>',
 '<< /Type /StructElem /S /Figure /P 9 0 R /Alt (RED BLUE) /K 10 >>']
(out/'rich-explicit-note.pdf').write_bytes(pdf(objects))
print('Wrote rich explicit Footnote/custom marks/reverse Ref/list/table/raster/navigation probe')
