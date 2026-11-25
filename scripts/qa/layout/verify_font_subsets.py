"""Independent embedding/outlines and pinned-before/after raster checks. QA only."""
import argparse
import json
from pathlib import Path
from pypdf import PdfReader
import pymupdf
from font_oracles import verify_pdf_glyph_program

parser=argparse.ArgumentParser()
parser.add_argument('--baseline',type=Path,default=Path('output/font-subset/before'))
parser.add_argument('--han-font',type=Path)
args=parser.parse_args()
out=Path('output/font-subset')
cases=['glyphs/glyphs.pdf','default-shaping/default.pdf','default-shaping/emergency.pdf','default-shaping/marked-space.pdf']
results=[]
for case in cases:
    old=args.baseline/'output'/case;new=Path('output')/case
    a,b=PdfReader(old,strict=True),PdfReader(new,strict=True)
    assert len(a.pages)==len(b.pages)
    before,after=pymupdf.open(old),pymupdf.open(new)
    for i,(pa,pb) in enumerate(zip(a.pages,b.pages)):
        assert pa.get_contents().get_data()==pb.get_contents().get_data(),(case,i,'page commands changed')
        assert before[i].get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).samples==after[i].get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).samples,(case,i,'raster changed')
        after[i].get_pixmap(matrix=pymupdf.Matrix(1,1)).save(out/f"comparison-{Path(case).stem}-{i+1}.png")
    results.append(dict(case=case,pages=len(a.pages),before_bytes=old.stat().st_size,after_bytes=new.stat().st_size,page_commands='exact',raster='exact at 1.5x'))

if args.han_font:
    original=args.han_font.read_bytes()
    old=out/'han-before.pdf';new=out/'han-after.pdf'
    a,b=pymupdf.open(old),pymupdf.open(new)
    assert len(a)==len(b)==1
    assert a[0].get_pixmap(matrix=pymupdf.Matrix(2,2)).samples==b[0].get_pixmap(matrix=pymupdf.Matrix(2,2)).samples
    reader=PdfReader(new,strict=True)
    assert reader.pages[0].extract_text().strip()==b[0].get_text().strip()=='𪛔'
    programs=[]
    for key,ref in reader.pages[0]['/Resources']['/Font'].items():
        if str(key).startswith('/MDG'):programs.append(verify_pdf_glyph_program(ref.get_object(),original))
    assert len(programs)==1 and new.stat().st_size<old.stat().st_size/20
    b[0].get_pixmap(matrix=pymupdf.Matrix(2,2)).save(out/'han-after.png')
    results.append(dict(case='Han U+2A6D4',before_bytes=old.stat().st_size,after_bytes=new.stat().st_size,raster='exact at 2x',text='exact in pypdf and MuPDF',program=programs[0]))
(out/'independent-subsets.json').write_text(json.dumps(results,indent=2)+'\n')
print(json.dumps(results,indent=2))
