"""Independent SF/DOCX/PDF page-setting, geometry, asset and note checks."""
import argparse
import base64
import io
import json
import re
from pathlib import Path
from zipfile import ZipFile

from docx import Document
from lxml import etree
from PIL import Image
from pypdf import PdfReader
import pymupdf

p=argparse.ArgumentParser();p.add_argument('directory',nargs='?',type=Path,default=Path('output/page-layout'));p.add_argument('--office',action='store_true')
a=p.parse_args();out=a.directory
ns={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}


def nodes(n):
    yield n
    for child in n.get('content',[]):
        yield from nodes(child)


def pixels(n):
    return Image.open(io.BytesIO(base64.b64decode(n['attrs']['src'].split(',',1)[1]))).convert('RGBA')


def close(actual,want,tolerance=.03):
    assert len(actual)==len(want) and all(abs(x-y)<tolerance for x,y in zip(actual,want)),(actual,want)


cases={'asymmetric':{'pages':6,'size':(480,420),'edges':(70,60,55,30),'dist':(12,16),'images':13},
       'zero':{'pages':2,'size':(240,300),'edges':(0,0,0,0),'dist':(0,0),'images':1},
       'offsets':{'pages':1,'size':(300,400),'edges':(33,33,33,33),'dist':(0,10),'images':0}}
report={}
for name,want in cases.items():
    source=json.loads((out/(name+'.sf')).read_text())
    assert source==json.loads((out/(name+'-docx.sf')).read_text()),'page/content/SF semantics changed through DOCX'
    word=Document(out/(name+'.docx'));section=word.sections[0]
    close(tuple(v.pt for v in (section.top_margin,section.right_margin,section.bottom_margin,section.left_margin)),want['edges'])
    close((section.header_distance.pt,section.footer_distance.pt),want['dist'])
    close((section.page_width.pt,section.page_height.pt),want['size'])
    with ZipFile(out/(name+'.docx')) as package:
        root=etree.fromstring(package.read('word/document.xml'));margin=root.xpath('//w:sectPr/w:pgMar',namespaces=ns)[0]
        actual=tuple(int(margin.get('{'+ns['w']+'}'+key))/20 for key in ('top','right','bottom','left','header','footer'))
        close(actual,want['edges']+want['dist'])
        if name=='asymmetric':
            assert root.xpath('//w:pgSz[@w:orient="landscape"]',namespaces=ns)
            assert len(word.sections[0].header.tables[0].rows)==2
            table=word.sections[0].header.tables[0]
            assert table.cell(0,0)._tc is table.cell(0,1)._tc
            assert len(word.tables[0].rows)==25
            assert len(root.xpath('//w:pageBreakBefore',namespaces=ns))==2
        if name=='zero':
            # A nonzero ignored legacy fallback survives, while all four actual
            # serialized edges and both running distances remain zero.
            assert source['attrs']['layout']['margin']==41
    recovered=json.loads((out/(name+'-pdf.sf')).read_text())
    assert len(recovered['attrs']['pages'])==want['pages']
    # Canvas geometry/placed content are explicit source facts. PDF does not
    # establish authorial margin settings or running-distance intent.
    assert not recovered['attrs']['layout'].get('margins')
    assert not recovered['attrs']['layout'].get('header_distance')
    assert not recovered['attrs']['layout'].get('footer_distance')
    imgs=[n for n in nodes(recovered) if n['type']=='image']
    assert len(imgs)==want['images']
    reference=None
    for root in source['content']+source['attrs']['headers']+source['attrs']['footers']:
        for n in nodes(root):
            if n['type']=='image':reference=pixels(n);break
    for n in imgs:
        image=pixels(n);assert image.size==reference.size and image.tobytes()==reference.tobytes()
    reader=PdfReader(out/(name+'.pdf'),strict=True);assert len(reader.pages)==want['pages']
    texts=[];positions=[];uri_count=0;internal=[]
    for i,page in enumerate(reader.pages):
        close((float(page.mediabox.width),float(page.mediabox.height)),want['size'])
        texts.append(page.extract_text())
        for image in page.images:
            rgba=image.image.convert('RGBA');assert rgba.size==reference.size and rgba.tobytes()==reference.tobytes()
        for ref in page.get('/Annots',[]):
            annot=ref.get_object()
            if '/A' in annot:
                assert annot['/A']['/URI']=='https://example.invalid/page-header';uri_count+=1
            else:
                dest=annot['/Dest'];target=next(j for j,p in enumerate(reader.pages) if p.indirect_reference.idnum==dest[0].idnum)
                assert i==target==1 and 72<=420-float(dest[3])<=365
                internal.append((i,target))
        with pymupdf.open(out/(name+'.pdf')) as doc:
            p=doc[i];p.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/f'{name}-checked-{i+1}.png')
            images=p.get_image_info(xrefs=True)
            for im in images:positions.append((i+1,im['bbox']))
            words=p.get_text('words')
            if name=='asymmetric':
                assert p.get_text().count('ASYMMETRIC HEADER')==1 and p.get_text().count('ASYMMETRIC FOOTER')==1
                close(images[0]['bbox'],(36,40,66,50));close(images[-1]['bbox'],(30,390,50,396))
                rects=[tuple(item[1]) for path in p.get_drawings() for item in path['items'] if item[0]=='re']
                assert len(rects)>=3
                for actual,expected in zip(rects,[(30,12,420,34),(30,34,225,64),(225,34,420,64)]):close(actual,expected)
                for w in words:
                    assert 29.9<=w[0]<=w[2]<=420.1
                    if 64<=w[1]<380:assert 71.9<=w[1]<=w[3]<=365.1,(i,w)
                if i==1:
                    close(images[1]['bbox'],(30,337.8,54,345.8))
                    separators=[(v[1].x,v[1].y,v[2].x,v[2].y) for path in p.get_drawings() for v in path['items'] if v[0]=='l' and abs(v[1].y-v[2].y)<.01 and abs(v[1].x-330)<.03 and abs(v[2].x-420)<.03]
                    assert separators,'RTL note separator did not follow asymmetric right edge'
                if i==3:assert p.search_for('ROW-12') and not p.search_for('ROW-11')
            elif name=='zero':
                assert words and min(w[0] for w in words)==0
                assert p.search_for('EDGE HEADER')[0].y0<1
                assert 290<=p.search_for('EDGE FOOTER')[0].y0<291
                if i==0:close(images[0]['bbox'],(0,29.8,50,41.8))
            else:
                assert p.search_for('TOP EDGE')[0].y0<1
                assert 33<=p.search_for('OFFSET BODY')[0].y0<34
                assert 380<=p.search_for('BOTTOM EDGE')[0].y0<381
                assert all(abs(w[0]-33)<.02 for w in words if w[4] in ('TOP','OFFSET','BOTTOM'))
    if name=='asymmetric':
        text='\n'.join(texts)
        for n in range(24):
            assert len(re.findall(rf'ROW-{n:02d}(?!\d)',text))==1
            assert text.count(f'VALUE-{n:02d}')==1
        for token in ('BODY START','BODY END','NOTE ABOVE FOOTER','NOTE END'):assert text.count(token)==1
        assert uri_count==6 and internal==[(1,1)]
    sf_positions=[]
    for n in imgs:
        s=n['attrs']['source'];quad=s['quad'];sf_positions.append((s['page'],(min(v['x'] for v in quad),min(v['y'] for v in quad),max(v['x'] for v in quad),max(v['y'] for v in quad))))
    assert len(sf_positions)==len(positions)
    for (page,box),(expected_page,expected_box) in zip(sorted(sf_positions),sorted(positions)):
        assert page==expected_page;close(box,expected_box)
    report[name]={'native_pages':want['pages'],'native_raster_placements':len(positions),'source_sf_docx_exact':True,'source_xml_settings':'exact','native_layout_geometry':'passed','recovered_sf_positions_rgba':'passed'}
    if a.office:
        path=out/'lo'/(name+'.pdf')
        with pymupdf.open(path) as doc:
            assert len(doc)==(3 if name=='asymmetric' else want['pages'])
            text='\n'.join(p.get_text() for p in doc)
            count=sum(len(p.get_image_info()) for p in doc)
            assert count==(7 if name=='asymmetric' else want['images'])
            if name=='asymmetric':
                assert all(text.count(token)==3 for token in ('ASYMMETRIC HEADER','ASYMMETRIC FOOTER'))
                for n in range(24):assert len(re.findall(rf'ROW-{n:02d}(?!\d)',text))==1
                for token in ('BODY START','BODY END','NOTE ABOVE FOOTER','NOTE END'):assert text.count(token)==1
            for page in doc:
                for im in page.get_image_info():
                    box=pymupdf.Rect(im['bbox']);assert box.x0>=-.05 and box.y0>=-.05 and box.x1<=page.rect.width+.05 and box.y1<=page.rect.height+.05
            report[name].update(office_pages=len(doc),office_raster_placements=count,office_visible_content='passed')
        for page in PdfReader(path,strict=True).pages:
            for im in page.images:
                rgba=im.image.convert('RGBA');assert rgba.size==reference.size and rgba.tobytes()==reference.tobytes()
        for i in range(report[name]['office_pages']):
            with pymupdf.open(path) as doc:doc[i].get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/'lo'/f'{name}-checked-{i+1}.png')
(out/'verification.json').write_text(json.dumps(report,indent=2)+'\n')
print('Page layout QA passed: exact SF/DOCX/settings relationships, nine native pages/fourteen exact-pixel placements, asymmetric/zero/independent anchors, merged geometry, RTL note bounds, real destinations and recovered SF positions')
if a.office:print('Six LibreOffice pages/eight placed rasters and all body/running/note content checked separately; producer table-start pagination differs')
