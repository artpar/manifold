"""Optional independent signed-margin geometry, pixels and paint-order QA."""
import argparse, base64, io, json
from pathlib import Path
from docx import Document
from PIL import Image
from pypdf import PdfReader
import pymupdf

parser=argparse.ArgumentParser();parser.add_argument('--directory',type=Path,default=Path('output/signed-overlap'));parser.add_argument('--office',action='store_true')
a=parser.parse_args();out=a.directory
cases={'fixed':(-24,-24,1),'minimum':(24,24,3),'fixed-top':(-24,24,2),'fixed-bottom':(24,-24,2),'large-fixed':(-24,-24,1)}

def close(actual,expected,tolerance=.03):
    assert len(actual)==len(expected) and all(abs(float(x)-float(y))<tolerance for x,y in zip(actual,expected)),(actual,expected)
def rgba(node):return Image.open(io.BytesIO(base64.b64decode(node['attrs']['src'].split(',',1)[1]))).convert('RGBA')
def crop_pixels(pix,rect):
    return [pix.pixel(x,y) for y in range(int(rect.y0*2),int(rect.y1*2)) for x in range(int(rect.x0*2),int(rect.x1*2))]
def near(pixel,want,tolerance=4):return all(abs(v-w)<=tolerance for v,w in zip(pixel,want))
report={}
for name,(top,bottom,pagecount) in cases.items():
    source=json.loads((out/(name+'.sf')).read_text());assert source==json.loads((out/(name+'-docx.sf')).read_text())
    assert source['attrs']['layout']['margins']['top']==top and source['attrs']['layout']['margins']['bottom']==bottom
    originals=[rgba(source['attrs'][role][0]) for role in ('headers','footers')]
    word=Document(out/(name+'.docx'));s=word.sections[0]
    assert word.core_properties.title==source['attrs']['metadata']['title']
    close((s.page_width.pt,s.page_height.pt,s.top_margin.pt,s.bottom_margin.pt,s.left_margin.pt,s.right_margin.pt,s.header_distance.pt,s.footer_distance.pt),(300,400,top,bottom,30,30,12,12))
    assert any(r.target_ref=='https://example.com/overlap' for r in word.part.rels.values() if r.reltype.endswith('/hyperlink'))
    reader=PdfReader(out/(name+'.pdf'),strict=True);assert len(reader.pages)==pagecount
    texts=[];uris=[];destinations=[];placements=[];header_overlap=False;footer_overlap=False;pixels_checked={}
    with pymupdf.open(out/(name+'.pdf')) as doc:
        for i,page in enumerate(reader.pages):
            close((page.mediabox.width,page.mediabox.height),(300,400));text=page.extract_text();texts.append(text)
            p=doc[i];pix=p.get_pixmap(matrix=pymupdf.Matrix(2,2));pix.save(out/f'{name}-checked-{i+1}.png')
            collect=[]
            for im in p.get_image_info(xrefs=True):collect.append(pymupdf.Rect(im['bbox']));placements.append((i+1,list(im['bbox'])))
            assert len(collect)==2
            expected_boxes=((30,12,130,382),(30,10,130,380)) if name=='large-fixed' else ((30,12,230,122),(30,260,230,380))
            for actual,expected in zip(collect,expected_boxes):close(actual,expected)
            for image in page.images:
                actual=image.image.convert('RGBA');assert any(actual.size==original.size and actual.tobytes()==original.tobytes() for original in originals)
            for n in range(12):
                for rect in p.search_for(f'BODY {n:02d}'):
                    assert 29.99<=rect.x0<rect.x1<=270.01
                    header_overlap|=rect.intersects(collect[0]);footer_overlap|=rect.intersects(collect[1])
                    if top>0:assert rect.y0>=129.99
                    if bottom>0:assert rect.y1<=260.01
            if i==0:
                body=p.search_for('BODY 00')[0];close((body.x0,body.y0),(30,24.7724 if top<0 else 130.7724))
            for ref in page.get('/Annots',[]):
                ann=ref.get_object()
                if '/A' in ann:uris.append((i+1,str(ann['/A']['/URI'])))
                else:
                    target=next(j+1 for j,p in enumerate(reader.pages) if p.indirect_reference.idnum==ann['/Dest'][0].idnum)
                    destinations.append((i+1,target));assert i+1==target
            if name=='fixed':
                body=p.search_for('BODY 00')[0];ink=crop_pixels(pix,body)
                assert sum(max(pixel)<12 for pixel in ink)>10,'body was painted beneath header'
                covered=p.search_for('BODY 10')[0];ink=crop_pixels(pix,covered)
                assert sum(max(pixel)<12 for pixel in ink)>10,'fixed footer obscured foreground body text'
                note=p.search_for('NOTE IN BODY FRAME')[0];assert sum(max(pixel)<12 for pixel in crop_pixels(pix,note))>10,'fixed footer obscured foreground note text'
                assert near(pix.pixel(400,40),(137,177,217)) and near(pix.pixel(400,748),(227,142,152))
                pixels_checked={'body_over_header_black_ink':True,'body_and_note_over_footer_black_ink':True,'alpha_samples':[list(pix.pixel(400,40)),list(pix.pixel(400,748))]}
    alltext='\n'.join(texts)
    for n in range(12):assert alltext.count(f'BODY {n:02d}')==1,(name,n,alltext)
    assert alltext.count('NOTE IN BODY FRAME')==1
    assert header_overlap==(top<0) and footer_overlap==(bottom<0),(name,header_overlap,footer_overlap)
    assert len(uris)==1 and uris[0][1]=='https://example.com/overlap';assert len(destinations)==1
    recovered=json.loads((out/(name+'-pdf.sf')).read_text());assert len(recovered['attrs']['pages'])==pagecount
    def tree_nodes(n):
        yield n
        for child in n.get('content',[]):yield from tree_nodes(child)
    recovered_images=[n for n in tree_nodes(recovered) if n['type']=='image']
    assert len(recovered_images)==2*pagecount
    for image in recovered_images:
        actual=rgba(image);assert any(actual.size==original.size and actual.tobytes()==original.tobytes() for original in originals)
        assert image['attrs']['source']['format']=='pdf' and image['attrs']['source'].get('quad')
    result={'native_pages':pagecount,'native_placements':placements,'uri_annotations':uris,'note_destinations':destinations,'pixels':pixels_checked}
    if a.office:
        producer=PdfReader(out/'office'/(name+'.pdf'),strict=True)
        result['office_pages']=len(producer.pages);result['office_texts']=[p.extract_text() for p in producer.pages]
        actual='\n'.join(result['office_texts'])
        for n in range(12):assert actual.count(f'BODY {n:02d}')==1,(name,n,actual)
        assert actual.count('NOTE IN BODY FRAME')==1
        for page in producer.pages:
            for image in page.images:
                actual=image.image.convert('RGBA');assert any(actual.size==original.size and actual.tobytes()==original.tobytes() for original in originals)
        with pymupdf.open(out/'office'/(name+'.pdf')) as doc:
            result['office_placements']=sum(len(p.get_image_info(xrefs=True)) for p in doc)
            assert result['office_placements']==2*len(doc)
            for i,p in enumerate(doc):p.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/f'{name}-office-checked-{i+1}.png')
            result['office_first_body']=list(doc[0].search_for('BODY 00')[0])
    report[name]=result
compatibility={}
for name in ('sections','continuous','continuous-geometry'):
    current=out.parent/name
    parent_pdf=out/f'parent-{name}.pdf';parent_docx=out/f'parent-{name}.docx'
    if not parent_pdf.exists() or not parent_docx.exists():continue
    before,after=PdfReader(parent_pdf,strict=True),PdfReader(current/'document.pdf',strict=True)
    assert len(before.pages)==len(after.pages)
    for old,new in zip(before.pages,after.pages):assert old.get_contents().get_data()==new.get_contents().get_data()
    assert parent_docx.read_bytes()==(current/'document.docx').read_bytes()
    compatibility[name]={'page_streams':len(after.pages),'docx_package':'exact'}
report['_previous_checkpoint_17c8b6c']=compatibility
(out/'verification.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report,indent=2))
