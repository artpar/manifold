"""Independent PDF Unicode spacing, glyph, decoration and break checks."""
from pathlib import Path
import argparse
import collections
import json
import math
from pypdf import PdfReader
from pypdf.generic import ContentStream
import pymupdf
import uharfbuzz as hb
from font_oracles import verify_pdf_glyph_program, original_font_name

parser=argparse.ArgumentParser();parser.add_argument('directory',nargs='?',type=Path,default=Path('output/typography'))
a=parser.parse_args();root=a.directory
assert (root/'typography.pdf').read_bytes().startswith(b'%PDF-1.7')
r=PdfReader(root/'typography.pdf',strict=True)
sf=json.loads((root/'typography.sf').read_text())
assert r.metadata.title=='Unicode typography' and len(r.pages)==1
# Derive punctuation/figure widths from independently parsed embedded CID metrics.
font=next(f.get_object() for f in r.pages[0]['/Resources']['/Font'].values() if f.get_object()['/BaseFont']=='/utf8go')
cid=font['/DescendantFonts'][0].get_object();widths={};w=cid['/W'];i=0
while i<len(w):
    start=int(w[i]);i+=1
    if isinstance(w[i],list):
        for j,value in enumerate(w[i]):widths[start+j]=float(value)
        i+=1
    else:
        end=int(w[i]);value=float(w[i+1]);i+=2
        for code in range(start,end+1):widths[code]=value
expected={1:5.5,2:11,3:11/3,4:11/4,5:11/6,6:11*widths[48]/1000,7:11*widths[46]/1000,8:2.2,9:1.1,10:2.2,11:22/9,12:11,13:11*widths[32]/1000,14:0,15:0}
# Ordinary A now uses the shaped original font, while virtual-space widths
# intentionally retain the declared primitive-font policy. Derive A from its
# actual original program instead of assuming it appears in the control subset.
shaped=next(f.get_object() for f in r.pages[0]['/Resources']['/Font'].values() if original_font_name(str(f.get_object()['/BaseFont']))=='GoRegular')
raw=shaped['/DescendantFonts'][0].get_object()['/FontDescriptor']['/FontFile2'].get_data()
verify_pdf_glyph_program(shaped)
face=hb.Face(raw);original=hb.Font(face)
a_advance=11*original.get_glyph_h_advance(original.get_nominal_glyph(65))/face.upem
chars_for=lambda page:[c for b in page.get_text('rawdict')['blocks'] if b['type']==0 for line in b['lines'] for span in line['spans'] for c in span['chars']]
with pymupdf.open(root/'typography.pdf') as doc:
    all_chars=chars_for(doc[0]);rows={}
    for c in all_chars:rows.setdefault(round(c['origin'][1],2),[]).append(c)
    for chars in rows.values():
        text=''.join(c['c'] for c in chars)
        if not text.startswith('S') or len(text)<3 or not text[1:3].isdigit():continue
        number=int(text[1:3]);ca=next(c for c in chars if c['c']=='A');cb=next(c for c in chars if c['c']=='B')
        # A's glyph advance is independent of the virtual-space implementation.
        measured=cb['origin'][0]-ca['origin'][0]-a_advance
        assert abs(measured-expected[number])<.02,(number,measured,expected[number])
        expected.pop(number)
    assert not expected,expected
    text=doc[0].get_text()
    assert 'café' in text and 'cafe\u0301' in text and 'A\u2060B' in text.replace('\n','')
    assert 'interoperability' in text.replace('\n','')
    # Every grapheme/word lies inside the reserved content area; notes remain
    # inside their separate bottom reserve.
    for c in all_chars:
        x0,y0,x1,y1=c['bbox'];assert x0>=29.9 and x1<=330.1 and y0>=29.9 and y1<=590.1,(c['c'],c['bbox'])
    decorated=next(chars for chars in rows.values() if ''.join(c['c'] for c in chars).startswith('Decorated'))
    baseline=decorated[0]['origin'][1]
    strokes=doc[0].get_drawings()
    for y in (baseline+1.1,baseline-4.4):
        assert any(abs(d['rect'].y0-y)<.03 and d['rect'].width>65 for d in strokes),(y,strokes)
    for i,page in enumerate(doc):page.get_pixmap(matrix=pymupdf.Matrix(1.8,1.8)).save(root/f'page-{i+1}.png')
# Read standard ActualText operands, rather than trusting an extractor which may
# deliberately normalize typographic whitespace to ASCII spaces.
actual=[];stack=0
for page in r.pages:
    for operands,operator in ContentStream(page.get_contents(),r).operations:
        if operator==b'BDC':
            stack+=1
            if '/ActualText' in operands[1]:actual.append(str(operands[1]['/ActualText']))
        elif operator==b'EMC':stack-=1;assert stack>=0
assert stack==0
for value in ('\u2003','\u200a','\u2008','\u2060','cafe\u0301'):
    assert any(value in text for text in actual),repr(value)
assert any(a.get_object().get('/A',{}).get('/URI')=='https://example.invalid/typography' for p in r.pages for a in p.get('/Annots',[]))
print('Independent typography passed: 15 measured space/control advances, logical Unicode/NFC, bounds, decorations, URI and standard ActualText')
