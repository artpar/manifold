"""Optional independent SF/DOCX/PDF text-style and layout qualification."""
import argparse
import json
from pathlib import Path
from zipfile import ZipFile

from docx import Document
from lxml import etree
from pypdf import PdfReader
import pymupdf
from font_oracles import verify_go_program

p=argparse.ArgumentParser()
p.add_argument('directory',nargs='?',type=Path,default=Path('output/text-style'))
p.add_argument('--office',type=Path)
a=p.parse_args();out=a.directory
sf=json.loads((out/'text-style.sf').read_text())
assert sf['content'][0]['content'][0]['marks'][0]=={'type':'textStyle','attrs':{'fontFamily':'Go','fontSize':24,'color':'#224488'}}
word=Document(out/'text-style.docx')
r=word.paragraphs[0].runs[0]
assert r.font.name=='Go' and r.font.size.pt==24 and str(r.font.color.rgb)=='224488'
r=word.paragraphs[1].runs
assert [(v.font.size.pt,str(v.font.color.rgb)) for v in r[:2]]==[(28,'BB2233'),(10,'333333')]
assert r[1].font.name=='Go Mono'
assert word.tables[0].cell(1,0).paragraphs[0].runs[0].font.size.pt==18
ns={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}
with ZipFile(out/'text-style.docx') as z:
    note=etree.fromstring(z.read('word/footnotes.xml'))
    assert note.xpath('//w:footnote[not(@w:type)]//w:rPr/w:sz/@w:val',namespaces=ns)==['28']
    assert note.xpath('//w:footnote[not(@w:type)]//w:rPr/w:color/@w:val',namespaces=ns)==['006633']
reader=PdfReader(out/'text-style.pdf',strict=True)
assert len(reader.pages)==4
fonts=[v.get_object() for v in reader.pages[0]['/Resources']['/Font'].values()]
glyph_fonts=[v.get_object() for key,v in reader.pages[0]['/Resources']['/Font'].items() if str(key).startswith('/MDG')]
# Four configured faces are used. App Sans deliberately supplies the same Go
# Regular program as the default, so actual physical names correctly coincide.
assert len(glyph_fonts)==4
assert sorted(str(f['/BaseFont']) for f in glyph_fonts)==['/Go-Bold','/GoMono','/GoRegular','/GoRegular']
for f in glyph_fonts:
 desc=f['/DescendantFonts'][0].get_object()['/FontDescriptor']
 verify_go_program(str(f['/BaseFont'])[1:],desc['/FontFile2'].get_data())
assert (out/'app-sans.ttf').read_bytes() in [f['/DescendantFonts'][0].get_object()['/FontDescriptor']['/FontFile2'].get_data() for f in glyph_fonts]
assert all('/FontFile2' in f['/DescendantFonts'][0].get_object()['/FontDescriptor'] for f in fonts)
mono=next(f for f in glyph_fonts if str(f['/BaseFont'])=='/GoMono')
assert int(mono['/DescendantFonts'][0].get_object()['/FontDescriptor']['/Flags'])&1
links=[]
for i,page in enumerate(reader.pages):
    for ref in page.get('/Annots',[]):
        v=ref.get_object()
        if '/Dest' in v:
            dest=next(j for j,pp in enumerate(reader.pages) if pp.indirect_reference.idnum==v['/Dest'][0].idnum)
            links.append((i+1,dest+1))
assert links==[(1,1)]

def approx(x,y): assert abs(x-y)<.035,(x,y)
with pymupdf.open(out/'text-style.pdf') as doc:
    spans=[s for page in doc for b in page.get_text('dict')['blocks'] for line in b.get('lines',[]) for s in line['spans']]
    def one(name):
        found=[s for s in spans if s['text']==name];assert len(found)==1,(name,found);return found[0]
    for name,size,color in [('COLOR_AND_SIZE',24,0x224488),('LARGE ',28,0xBB2233),('small ',10,0x333333),('linked',18,0x2255CC),('LARGE_LIST',18,0x007777),('STYLED_NOTE',14,0x006633),('CUSTOM_FACE',14,0x225588)]:
        s=one(name);approx(s['size'],size);assert s['color']==color,(name,s)
    assert [one(name)['font'] for name in ('LARGE ','small ','CUSTOM_FACE')]==['GoRegular','GoMono','GoRegular']
    for name in ('LARGE ','small ','linked'):approx(one(name)['origin'][1],101.2)
    assert one('COLOR_AND_SIZE')['flags']&16
    for name in ['LARGE_LIST',*(f'CELL_{i:02d}' for i in range(6))]:
        assert one(name)['font']=='GoMono'
    raw=[s for b in doc[0].get_text('rawdict')['blocks'] for line in b.get('lines',[]) for s in line['spans']]
    chars=next(s['chars'] for s in raw if ''.join(c['c'] for c in s['chars'])=='small ')
    for x,y in zip(chars,chars[1:]):approx(y['origin'][0]-x['origin'][0],6)
    alltext=''.join(page.get_text() for page in doc)
    for token in ['COLOR_AND_SIZE','LARGE_LIST','CUSTOM_FACE','STYLED_NOTE',*(f'CELL_{i:02d}' for i in range(6)),*(f'VALUE_{i:02d}' for i in range(6))]:assert alltext.count(token)==1,token
    assert alltext.count('STYLE_HEADER')==2 and alltext.count('WRAPPED_TEXT')==3
    baselines=[s['origin'][1] for s in spans if s['text'].startswith(('boundaries with','advances and larger'))]
    approx(baselines[1]-baselines[0],25.2)
    uri=next(v for v in doc[0].get_links() if v.get('uri'))
    assert uri['uri']=='https://example.invalid/styles' and uri['from'].contains(pymupdf.Rect(one('linked')['bbox']))
    # Decoration inherits the run color and actual baseline/size.
    assert any(abs(d['rect'].y0-103)<.04 and d['color'] and all(abs(c-v)<.005 for c,v in zip(d['color'],(0x22/255,0x55/255,0xCC/255))) for d in doc[0].get_drawings())
    for i,page in enumerate(doc):
        for b in page.get_text('dict')['blocks']:
            for line in b.get('lines',[]):
                for s in line['spans']:
                    if not s['text'].strip():continue
                    x0,y0,x1,y1=s['bbox'];assert x0>=29.95 and x1<=370.05,(i,s)
                    if s['text']=='RUNNING_HEADER':assert 0<=y0<y1<=30
                    elif s['text']=='RUNNING_FOOTER':assert 270<=y0<y1<=300
                    else:assert y0>=29.95 and y1<=270.05,(i,s)
        page.get_pixmap(matrix=pymupdf.Matrix(1.7,1.7)).save(out/f'page-{i+1}.png')
print('Text styles passed: four native pages, exact sizes/colors/baselines, fixed-pitch advances, used embedded fonts/custom face, links/decoration, wrapping, repeated headers and DOCX properties')
if a.office:
    with pymupdf.open(a.office) as doc:
        assert len(doc)==2
        whole=''.join(page.get_text() for page in doc)
        for token in ['COLOR_AND_SIZE','LARGE_LIST','CUSTOM_FACE','STYLED_NOTE',*(f'CELL_{i:02d}' for i in range(6)),*(f'VALUE_{i:02d}' for i in range(6))]:assert whole.count(token)==1,token
        observed=sorted({s['font'] for page in doc for b in page.get_text('dict')['blocks'] for line in b.get('lines',[]) for s in line['spans']})
        (out/'office-observation.json').write_text(json.dumps({'pages':2,'all_original_tokens':'passed','observed_fonts':observed,'same_pagination_as_native':False,'font_substitution':'Go/Go Mono/App Sans are not embedded in DOCX; producer chooses installed faces'},indent=2)+'\n')
        for i,page in enumerate(doc):page.get_pixmap(matrix=pymupdf.Matrix(1.7,1.7)).save(out/f'office-page-{i+1}.png')
    print('LibreOffice retained all tokens in two pages; installed-font substitution and pagination differ, see office-observation.json')
