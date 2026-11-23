"""Optional independent continuous-section controls and rich-output checks."""
import argparse, base64, io, json
from pathlib import Path
from docx import Document
from docx.enum.section import WD_SECTION_START
from docx.shared import Pt
from PIL import Image
from pypdf import PdfReader
import pymupdf

parser=argparse.ArgumentParser()
parser.add_argument('--directory',type=Path,default=Path('output/continuous'))
parser.add_argument('--make-controls',action='store_true')
parser.add_argument('--office',action='store_true')
a=parser.parse_args();out=a.directory;out.mkdir(parents=True,exist_ok=True)
if a.make_controls:
    for change in ('stories','left','top','bottom','width','orientation','first','same'):
        d=Document();s=d.sections[0];s.page_width,s.page_height=Pt(360),Pt(440)
        for k in ('left_margin','right_margin','top_margin','bottom_margin'):setattr(s,k,Pt(40))
        s.header_distance,s.footer_distance=Pt(12),Pt(14)
        s.header.paragraphs[0].text='OLD HEADER';s.footer.paragraphs[0].text='OLD FOOTER'
        d.add_paragraph('OLD BODY');s=d.add_section(WD_SECTION_START.CONTINUOUS)
        if change=='stories':
            s.header.is_linked_to_previous=False;s.header.paragraphs[0].text='NEW HEADER'
            s.footer.is_linked_to_previous=False;s.footer.paragraphs[0].text='NEW FOOTER'
        if change=='left':s.left_margin=Pt(100)
        if change=='top':s.top_margin=Pt(100)
        if change=='bottom':s.bottom_margin=Pt(100)
        if change=='width':s.page_width=Pt(500)
        if change=='orientation':s.page_width,s.page_height=Pt(440),Pt(360)
        if change=='first':
            s.different_first_page_header_footer=True;s.first_page_header.paragraphs[0].text='NEW FIRST'
        d.add_paragraph('NEW BODY');d.add_page_break();d.add_paragraph('NEXT PAGE BODY')
        d.save(out/(change+'.docx'))
    print('Wrote eight independent continuous-section controls');raise SystemExit

def nodes(n):
    yield n
    for c in n.get('content',[]):yield from nodes(c)
    if n['type']=='section':
        for role in ('headers','footers'):
            for story in n.get('attrs',{}).get(role,{}).values():
                for c in story:yield from nodes(c)
def close(actual,expected):
    assert len(actual)==len(expected) and all(abs(float(x)-float(y))<.03 for x,y in zip(actual,expected)),(actual,expected)

source=json.loads((out/'document.sf').read_text());assert source==json.loads((out/'docx.sf').read_text())
assert len(source['content'])==3 and source['content'][1]['attrs']['start']=='continuous'
word=Document(out/'document.docx');assert len(word.sections)==3
assert all(s.start_type==WD_SECTION_START.CONTINUOUS for s in list(word.sections)[1:])
assert word.sections[1].different_first_page_header_footer
header=word.sections[1].even_page_header.tables[0];assert header.cell(0,0)._tc is header.cell(0,1)._tc
assert len(word.tables)==1 and len(word.tables[0].rows)==6
assert any(rel.target_ref=='https://example.com/continuous' for rel in word.part.rels.values() if rel.reltype.endswith('/hyperlink'))
original=next(n for n in nodes(source) if n['type']=='image')
rgba=Image.open(io.BytesIO(base64.b64decode(original['attrs']['src'].split(',',1)[1]))).convert('RGBA')
reader=PdfReader(out/'document.pdf',strict=True);assert len(reader.pages)==5
expected=[('A DEFAULT','A ONE','A FOOT'),('A EVEN','A SHARED PAGE','B SHARED','SHARED NOTE','A FOOT'),('B EVEN','B TWO','B FOOT'),('B DEFAULT','B THREE','C SHARED','B FOOT'),('C EVEN','C TWO','C FOOT')]
placements=[];links=[];destinations=[];texts=[];bounds=[]
with pymupdf.open(out/'document.pdf') as doc:
    for i,page in enumerate(reader.pages):
        close((page.mediabox.width,page.mediabox.height),(360,440))
        text=page.extract_text();texts.append(text)
        for token in expected[i]:assert text.count(token)==1,(i,token,text)
        for token in ('A DEFAULT','A EVEN','A FOOT','B UNUSED FIRST','B DEFAULT','B EVEN','B FOOT','C DEFAULT','C EVEN','C FOOT'):
            if token not in expected[i]:assert token not in text,(i,token,text)
        p=doc[i];p.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/f'checked-{i+1}.png')
        for im in p.get_image_info(xrefs=True):placements.append((i+1,list(im['bbox'])))
        for im in page.images:
            actual=im.image.convert('RGBA');assert actual.size==rgba.size and actual.tobytes()==rgba.tobytes()
        for body in ('A ONE','A SHARED PAGE','B TWO','B THREE','C TWO'):
            for rect in p.search_for(body):
                assert abs(rect.x0-40)<.03 and abs(rect.y0-82.6072)<.03,(body,rect)
                bounds.append((body,list(rect)))
        for ref in page.get('/Annots',[]):
            ann=ref.get_object()
            if '/A' in ann:links.append((i+1,str(ann['/A']['/URI'])))
            else:
                dest=ann['/Dest'];target=next(j+1 for j,p in enumerate(reader.pages) if p.indirect_reference.idnum==dest[0].idnum)
                destinations.append((i+1,target));assert i+1==target==2
    assert doc[1].search_for('A FOOT') and not doc[1].search_for('B FOOT')
    assert doc[3].search_for('B FOOT') and not doc[3].search_for('C FOOT')
    rects=[tuple(item[1]) for path in doc[2].get_drawings() for item in path['items'] if item[0]=='re']
    assert rects and max(r[3] for r in rects if r[1]<82)<doc[0].search_for('A ONE')[0].y0
assert links==[(2,'https://example.com/continuous')],links
assert destinations==[(2,2)],destinations
assert len(placements)==2;close(placements[0][1],(58,380,82,392));close(placements[1][1],(46,48,70,60))
for i in range(6):assert '\n'.join(texts).count(f'ROW {i}')==1 and '\n'.join(texts).count(f'VALUE {i}')==1
recovered=json.loads((out/'pdf.sf').read_text());assert len(recovered['attrs']['pages'])==5 and all(n['type']!='section' for n in recovered['content'])
report={'native_pages':5,'pixel_placements':placements,'uri_annotations':links,'note_destinations':destinations,'body_bounds':bounds}
source_controls={}
for name in ('stories','left','top','bottom','width','orientation','first','same'):
    path=out/f'control-{name}.sf'
    if not path.exists():continue
    tree=json.loads(path.read_text());assert len(tree['content'])==2
    first,second=tree['content'];assert first['type']==second['type']=='section'
    assert second['attrs']['start']=='continuous'
    layout=second['attrs']['layout'];source_controls[name]=layout
    if name in ('left','top','bottom'):assert layout['margins'][name]==100
    if name=='width':assert layout['width']==500
    if name=='orientation':assert (layout['width'],layout['height'])==(440,360)
report['source_controls']=source_controls
if a.office:
    producer=PdfReader(out/'office/document.pdf',strict=True)
    report['office_pages']=len(producer.pages);report['office_texts']=[p.extract_text() for p in producer.pages]
    office_images=0
    for page in producer.pages:
        for im in page.images:
            actual=im.image.convert('RGBA');assert actual.size==rgba.size and actual.tobytes()==rgba.tobytes();office_images+=1
    report['office_image_resources']=office_images
    with pymupdf.open(out/'office/document.pdf') as doc:
        report['office_image_placements']=sum(len(p.get_image_info(xrefs=True)) for p in doc)
        for i,p in enumerate(doc):p.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/f'office-checked-{i+1}.png')
    for token in ('A ONE','A SHARED PAGE','B SHARED','SHARED NOTE','B TWO','B THREE','C SHARED','C TWO'):
        assert '\n'.join(report['office_texts']).count(token)==1
    controls={}
    for name in ('stories','left','top','bottom','width','orientation','first','same'):
        path=out/'office'/(name+'.pdf')
        if not path.exists():continue
        with pymupdf.open(path) as doc:
            controls[name]={'pages':len(doc),'page_sizes':[list(p.rect)[2:] for p in doc],'texts':[p.get_text() for p in doc],'body_rects':[{token:[list(r) for r in p.search_for(token)] for token in ('OLD BODY','NEW BODY','NEXT PAGE BODY')} for p in doc]}
        assert controls[name]['pages']==2
        assert all('OLD HEADER' in text and 'OLD FOOTER' in text for text in controls[name]['texts'][:1])
        if name=='stories':assert 'NEW HEADER' in controls[name]['texts'][1] and 'NEW FOOTER' in controls[name]['texts'][1]
    report['producer_controls']=controls
source_controls={}
for name in ('stories','left','top','bottom','width','orientation','first','same'):
    path=out/f'control-{name}.sf'
    if not path.exists():continue
    tree=json.loads(path.read_text());assert len(tree['content'])==2
    first,second=tree['content'];assert first['type']==second['type']=='section'
    assert second['attrs']['start']=='continuous'
    layout=second['attrs']['layout'];source_controls[name]=layout
    if name in ('left','top','bottom'):assert layout['margins'][name]==100
    if name=='width':assert layout['width']==500
    if name=='orientation':assert (layout['width'],layout['height'])==(440,360)
    if name=='first':assert second['attrs']['firstPage']
report['source_control_layouts']=source_controls
(out/'verification.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report,indent=2))
