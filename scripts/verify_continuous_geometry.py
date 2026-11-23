"""Independent geometry, frame ownership, asset and relationship checks."""
import argparse, base64, io, json
from pathlib import Path
from docx import Document
from docx.enum.section import WD_SECTION_START
from PIL import Image
from pypdf import PdfReader
import pymupdf

parser=argparse.ArgumentParser();parser.add_argument('--directory',type=Path,default=Path('output/continuous-geometry'));parser.add_argument('--office',action='store_true')
a=parser.parse_args();out=a.directory

def close(actual,expected):
    assert len(actual)==len(expected) and all(abs(float(x)-float(y))<.03 for x,y in zip(actual,expected)),(actual,expected)
def nodes(n):
    yield n
    for c in n.get('content',[]):yield from nodes(c)
    if n['type']=='section':
        for role in ('headers','footers'):
            for story in n.get('attrs',{}).get(role,{}).values():
                for c in story:yield from nodes(c)
source=json.loads((out/'document.sf').read_text());assert source==json.loads((out/'docx.sf').read_text())
assert len(source['content'])==4
word=Document(out/'document.docx');assert len(word.sections)==4
sizes=[(360,440),(360,440),(480,360),(480,360)]
edges=[(40,40,40,40),(70,20,60,100),(50,60,40,30),(60,30,50,80)]
distances=[(18,18),(12,15),(10,12),(4,6)]
for i,s in enumerate(word.sections):
    close((s.page_width.pt,s.page_height.pt),sizes[i]);close((s.top_margin.pt,s.right_margin.pt,s.bottom_margin.pt,s.left_margin.pt),edges[i])
    close((s.header_distance.pt,s.footer_distance.pt),distances[i])
    assert s.start_type==(WD_SECTION_START.NEW_PAGE if i==0 else WD_SECTION_START.CONTINUOUS)
assert len(word.tables)==1 and len(word.tables[0].rows)==4
assert any(r.target_ref=='https://example.com/frame' for r in word.part.rels.values() if r.reltype.endswith('/hyperlink'))
images=[n for n in nodes(source) if n['type']=='image']+[n for note in source['attrs']['notes'] for block in note['content'] for n in nodes(block) if n['type']=='image']
original=Image.open(io.BytesIO(base64.b64decode(images[0]['attrs']['src'].split(',',1)[1]))).convert('RGBA')
reader=PdfReader(out/'document.pdf',strict=True);assert len(reader.pages)==5
expected=[('OLD HEADER','OLD BODY','NEW BODY','OLD NOTE','NEW NOTE','OLD FOOT'),('NEW HEADER','NEW SECOND','NEW FOOT'),('WIDE HEADER','WIDE BODY','WIDE NOTE','WIDE FOOT'),('WIDE HEADER','WIDE SECOND','LAST BODY','WIDE FOOT'),('LAST HEADER','LAST SECOND','LAST FOOT')]
position_checks=[{'OLD BODY':(40,70.6069),'NEW BODY':(100,90.4069),'OLD FOOT':(40,420.4414)},{'NEW SECOND':(100,70.6069),'NEW FOOT':(100,415.4414)},{'WIDE BODY':(30,60.6069),'WIDE FOOT':(30,338.4414)},{'WIDE SECOND':(30,60.6069),'LAST BODY':(80,80.4069),'WIDE FOOT':(30,338.4414)},{'LAST SECOND':(80,60.6069),'LAST FOOT':(80,344.4414)}]
placements=[];uris=[];destinations=[];texts=[]
with pymupdf.open(out/'document.pdf') as doc:
    for i,page in enumerate(reader.pages):
        close((page.mediabox.width,page.mediabox.height),(360,440) if i<2 else (480,360))
        text=page.extract_text();texts.append(text)
        for token in expected[i]:assert text.count(token)==1,(i,token,text)
        for token in ('OLD HEADER','NEW HEADER','WIDE HEADER','LAST HEADER','OLD FOOT','NEW FOOT','WIDE FOOT','LAST FOOT'):
            if token not in expected[i]:assert token not in text,(i,token,text)
        p=doc[i];p.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/f'checked-{i+1}.png')
        for token,(x,y) in position_checks[i].items():
            rects=p.search_for(token);assert len(rects)==1;close((rects[0].x0,rects[0].y0),(x,y))
        for image in p.get_image_info(xrefs=True):placements.append((i+1,list(image['bbox'])))
        for image in page.images:
            rgba=image.image.convert('RGBA');assert rgba.size==original.size and rgba.tobytes()==original.tobytes()
        for ref in page.get('/Annots',[]):
            ann=ref.get_object()
            if '/A' in ann:uris.append((i+1,str(ann['/A']['/URI'])))
            else:
                target=next(j+1 for j,p in enumerate(reader.pages) if p.indirect_reference.idnum==ann['/Dest'][0].idnum)
                destinations.append((i+1,target));assert i+1==target
    rects=[tuple(item[1]) for path in doc[0].get_drawings() for item in path['items'] if item[0]=='re']
    assert len(rects)==8 and all(abs(r[0]-100)<.03 or abs(r[0]-220)<.03 for r in rects)
    assert all(r[2]<=340.03 for r in rects)
assert uris==[(1,'https://example.com/frame')],uris
assert destinations==[(1,1),(1,1),(3,3)],destinations
assert len(placements)==3
for actual,want in zip(placements,[(1,(58,334,98,342)),(1,(100,364,120,372)),(3,(48,292,78,302))]):
    assert actual[0]==want[0];close(actual[1],want[1])
for i in range(4):assert '\n'.join(texts).count(f'ROW {i}')==1 and '\n'.join(texts).count(f'VALUE {i}')==1
recovered=json.loads((out/'pdf.sf').read_text());assert len(recovered['attrs']['pages'])==5 and all(n['type']!='section' for n in recovered['content'])
report={'native_pages':5,'sizes':[(360,440)]*2+[(480,360)]*3,'pixel_placements':placements,'uri_annotations':uris,'note_destinations':destinations,'source_docx_sf':'exact'}
controls={}
for name in ('stories','left','top','bottom','width','orientation','first','same'):
    path=out/'controls'/(name+'.pdf')
    if not path.exists():continue
    with pymupdf.open(path) as doc:
        want=(500,440) if name=='width' else (440,360) if name=='orientation' else (360,440)
        assert len(doc)==(3 if name in ('width','orientation') else 2)
        for p in list(doc)[1:]:close((p.rect.width,p.rect.height),want)
        new=next(rect for p in doc for rect in p.search_for('NEW BODY'))
        assert abs(new.x0-(100 if name=='left' else 40))<.03
        if name=='top':assert new.y0>=100
        controls[name]={'pages':len(doc),'page_sizes':[list(p.rect)[2:] for p in doc],'new_body':list(new)}
report['native_independent_controls']=controls
if a.office:
    producer=PdfReader(out/'office/document.pdf',strict=True);report['office_pages']=len(producer.pages)
    report['office_texts']=[p.extract_text() for p in producer.pages]
    for token in ('OLD BODY','NEW BODY','NEW SECOND','OLD NOTE','NEW NOTE','WIDE BODY','WIDE SECOND','WIDE NOTE','LAST BODY','LAST SECOND'):
        assert '\n'.join(report['office_texts']).count(token)==1
    for page in producer.pages:
        for image in page.images:
            rgba=image.image.convert('RGBA');assert rgba.size==original.size and rgba.tobytes()==original.tobytes()
    with pymupdf.open(out/'office/document.pdf') as doc:
        report['office_sizes']=[list(p.rect)[2:] for p in doc]
        report['office_image_placements']=sum(len(p.get_image_info(xrefs=True)) for p in doc);assert report['office_image_placements']==3
        report['office_shared_body_positions']={token:[list(r) for p in doc for r in p.search_for(token)] for token in ('NEW BODY','LAST BODY')}
        for i,p in enumerate(doc):p.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/f'office-checked-{i+1}.png')
baselines={}
for name in ('sections','continuous'):
    parent=out/f'parent-{name}.pdf'
    current=out.parent/name/'document.pdf'
    if not parent.exists() or not current.exists():continue
    old,new=PdfReader(parent,strict=True),PdfReader(current,strict=True)
    assert len(old.pages)==len(new.pages)
    for before,after in zip(old.pages,new.pages):assert before.get_contents().get_data()==after.get_contents().get_data()
    baselines[name]=len(new.pages)
report['previous_checkpoint_page_streams']=baselines
(out/'verification.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report,indent=2))
