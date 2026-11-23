"""Optional independent section/story, geometry, relationship and asset QA.
Run examples/sections first. --make-control creates an independent python-docx
reference; --office additionally checks an optional LibreOffice export.
"""
import argparse, base64, io, json
from pathlib import Path
from zipfile import ZipFile
from docx import Document
from docx.enum.section import WD_SECTION_START
from docx.shared import Pt
from lxml import etree
from PIL import Image
from pypdf import PdfReader
import pymupdf

parser=argparse.ArgumentParser()
parser.add_argument('--directory',type=Path,default=Path('output/sections'))
parser.add_argument('--make-control',action='store_true')
parser.add_argument('--office',action='store_true')
a=parser.parse_args();out=a.directory;out.mkdir(parents=True,exist_ok=True)
if a.make_control:
    d=Document();d.settings.odd_and_even_pages_header_footer=True
    s=d.sections[0];s.page_width,s.page_height=Pt(360),Pt(440)
    for edge in ('top_margin','right_margin','bottom_margin','left_margin'):setattr(s,edge,Pt(40))
    s.header.paragraphs[0].text='CONTROL DEFAULT'
    s.even_page_header.paragraphs[0].text='CONTROL EVEN'
    d.add_paragraph('CONTROL ONE');d.add_page_break();d.add_paragraph('CONTROL TWO')
    s=d.add_section(WD_SECTION_START.EVEN_PAGE);s.page_width,s.page_height=Pt(480),Pt(360)
    d.add_paragraph('CONTROL THREE');d.add_page_break();d.add_paragraph('CONTROL FOUR')
    d.save(out/'control.docx')
    print('Wrote independent python-docx control')
    raise SystemExit

def nodes(n):
    yield n
    for c in n.get('content',[]):yield from nodes(c)
    if n['type']=='section':
        for role in ('headers','footers'):
            for story in n.get('attrs',{}).get(role,{}).values():
                for c in story:yield from nodes(c)
def close(actual,expected):
    assert len(actual)==len(expected) and all(abs(float(x)-float(y))<.03 for x,y in zip(actual,expected)),(actual,expected)
source=json.loads((out/'document.sf').read_text())
assert source==json.loads((out/'docx.sf').read_text()),'SF/DOCX changed section semantics'
assert len(source['content'])==3
word=Document(out/'document.docx');assert len(word.sections)==3
assert word.settings.odd_and_even_pages_header_footer
for i,s in enumerate(word.sections):
    close((s.page_width.pt,s.page_height.pt),(360,440) if i==0 else (480,360))
    assert s.start_type==(WD_SECTION_START.ODD_PAGE if i==1 else WD_SECTION_START.NEW_PAGE)
    assert s.different_first_page_header_footer==(i<2)
    assert s.header.is_linked_to_previous==(i==1)
    assert s.first_page_header.is_linked_to_previous==(i==2)
    assert s.even_page_header.is_linked_to_previous==(i>0)
    assert s.footer.is_linked_to_previous==(i>0)
assert word.tables[0].cell(0,0)._tc is word.tables[0].cell(0,1)._tc
ns={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main','r':'http://schemas.openxmlformats.org/officeDocument/2006/relationships'}
with ZipFile(out/'document.docx') as z:
    for name in z.namelist():
        if name.endswith('.xml') or name.endswith('.rels'):etree.fromstring(z.read(name))
    root=etree.fromstring(z.read('word/document.xml'))
    assert len(root.xpath('//w:sectPr',namespaces=ns))==3
    assert len(root.xpath('//w:sectPr/w:headerReference',namespaces=ns))==5
    assert len(root.xpath('//w:sectPr/w:footerReference',namespaces=ns))==3
    assert len(root.xpath('//w:footnoteReference',namespaces=ns))==1
    content=etree.fromstring(z.read('[Content_Types].xml'))
    assert any(c.get('PartName')=='/word/settings.xml' and c.get('ContentType').endswith('.settings+xml') for c in content)
    rels=etree.fromstring(z.read('word/_rels/header2.xml.rels'))
    assert any(c.get('Target')=='https://example.com/first' for c in rels)
reference=next(n for n in nodes(source) if n['type']=='image')
image=Image.open(io.BytesIO(base64.b64decode(reference['attrs']['src'].split(',',1)[1]))).convert('RGBA')
reader=PdfReader(out/'document.pdf',strict=True);assert len(reader.pages)==8
expected=[('A FIRST','A PAGE ONE'),('A EVEN','A PAGE TWO','A EVEN FOOT'),('A DEFAULT','A PAGE THREE','MERGED SECTION CELL','A FOOT'),(),('B PAGE ONE',),('A EVEN','B PAGE TWO','A EVEN FOOT'),('A DEFAULT','B LIST','SECTION NOTE','A FOOT'),('C BODY','A FOOT')]
texts=[];placed=[];uris=[];destinations=[]
with pymupdf.open(out/'document.pdf') as native:
    for i,page in enumerate(reader.pages):
        close((page.mediabox.width,page.mediabox.height),(360,440) if i<4 else (480,360))
        text=page.extract_text();texts.append(text)
        for token in expected[i]:assert text.count(token)==(2 if token=='A EVEN' else 1),(i,token,text)
        for token in ('A FIRST','A DEFAULT','A EVEN FOOT','A EVEN','A FOOT'):
            if token not in expected[i] and not (token=='A EVEN' and 'A EVEN FOOT' in expected[i]):assert token not in text,(i,token,text)
        if i==3:assert not text.strip()
        p=native[i];p.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/f'checked-{i+1}.png')
        for im in p.get_image_info(xrefs=True):
            placed.append((i+1,list(im['bbox'])))
            assert 0<=im['bbox'][0]<im['bbox'][2]<=p.rect.width and 0<=im['bbox'][1]<im['bbox'][3]<=p.rect.height
        for im in page.images:
            rgba=im.image.convert('RGBA');assert rgba.size==image.size and rgba.tobytes()==image.tobytes()
        for ref in page.get('/Annots',[]):
            ann=ref.get_object()
            if '/A' in ann:uris.append((i+1,str(ann['/A']['/URI'])))
            else:
                dest=ann['/Dest'];target=next(j+1 for j,p in enumerate(reader.pages) if p.indirect_reference.idnum==dest[0].idnum)
                destinations.append((i+1,target));assert i+1==target==7
        if i==2:
            assert p.search_for('A FOOT')[0].y0>400,'old footer used new short geometry'
        if i==6:assert 330<p.search_for('A FOOT')[0].y0<345
assert len(placed)==4,placed
assert uris==[(1,'https://example.com/first')],uris
assert destinations==[(7,7)],destinations
report={'native_pages':8,'exact_pixel_placements':len(placed),'placements':placed,'uri_annotations':uris,'internal_destinations':destinations}
recovered=json.loads((out/'pdf.sf').read_text());assert len(recovered['attrs']['pages'])==8
assert all(n['type']!='section' for n in recovered['content']),'untagged PDF invented authored sections'
control=out/'control.sf'
if control.exists():
    external=json.loads(control.read_text())
    assert external['attrs']['evenAndOddHeaders']
    first,second=external['content'];assert first['type']==second['type']=='section'
    assert first['attrs']['layout']['width']==360 and second['attrs']['layout']['width']==480
    assert second['attrs']['start']=='evenPage' and not second['attrs'].get('headers')
    for variant,token in [('default','CONTROL DEFAULT'),('even','CONTROL EVEN')]:
        assert first['attrs']['headers'][variant][0]['content'][0]['text']==token
    report['independent_control_sections']=2
if a.office:
    reader=PdfReader(out/'office/document.pdf',strict=True)
    report['office_pages']=len(reader.pages)
    report['office_texts']=[p.extract_text() for p in reader.pages]
    # Producer pagination is recorded separately from the native oracle.
    alltext='\n'.join(report['office_texts'])
    for token in ('A PAGE ONE','A PAGE TWO','A PAGE THREE','MERGED SECTION CELL','B PAGE ONE','B PAGE TWO','B LIST','B BODY','SECTION NOTE','C BODY'):assert alltext.count(token)==1,(token,alltext)
    count=0
    with pymupdf.open(out/'office/document.pdf') as office:
        for i,p in enumerate(office):
            p.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/f'office-checked-{i+1}.png')
            count+=len(p.get_image_info(xrefs=True))
    report['office_placements']=count
(out/'verification.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report,indent=2))
