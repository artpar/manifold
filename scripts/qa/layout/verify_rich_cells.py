"""Independent image/nested-cell/footnote layout verification. QA only."""
from pathlib import Path
from zipfile import ZipFile
import argparse
import base64
import io
import json
from pypdf import PdfReader
from docx import Document
from lxml import etree
from PIL import Image
import pymupdf

parser=argparse.ArgumentParser();parser.add_argument('directory',nargs='?',type=Path,default=Path('output/rich-cells'));parser.add_argument('--continued',action='store_true')
args=parser.parse_args();out=args.directory
sf=json.loads((out/'rich-cells.sf').read_text());reader=PdfReader(out/'rich-cells.pdf',strict=True)
plain=lambda n:n.get('text','')+''.join(plain(c) for c in n.get('content',[]))
ns={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main','wp':'http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing','r':'http://schemas.openxmlformats.org/package/2006/relationships'}
images=[]
def assets(nodes):
    for n in nodes:
        if n['type']=='image':images.append(base64.b64decode(n['attrs']['src'].split(',',1)[1]))
        assets(n.get('content',[]))
assets(sf['content'])
for note in sf['attrs']['notes']:assets(note['content'])

def page_index(ref):
    return next(i for i,p in enumerate(reader.pages) if p.indirect_reference.idnum==ref.idnum)

def annotation_targets():
    links=[];uris=[]
    for i,page in enumerate(reader.pages):
        for ref in page.get('/Annots',[]):
            a=ref.get_object()
            if '/Dest' in a:links.append((i,page_index(a['/Dest'][0]),a['/Rect']))
            if a.get('/A',{}).get('/S')=='/URI':uris.append((i,str(a['/A']['/URI'])))
    return links,uris

with ZipFile(out/'rich-cells.docx') as z:
    main=etree.fromstring(z.read('word/document.xml'));notes=etree.fromstring(z.read('word/footnotes.xml'))
    assert all(blob in [z.read(name) for name in z.namelist() if name.startswith('word/media/')] for blob in images)
    if not args.continued:
        extents=main.xpath('//wp:extent',namespaces=ns)
        assert [(int(n.get('cx'))/12700,int(n.get('cy'))/12700) for n in extents]==[(232,116),(30,20)]
        assert len(notes.xpath('//wp:extent',namespaces=ns))==1
        rels=etree.fromstring(z.read('word/_rels/document.xml.rels'))
        assert 'https://example.invalid/inner' in rels.xpath('//r:Relationship/@Target',namespaces=ns)
    else:
        assert len(notes.xpath('//w:tbl/w:tr',namespaces=ns))==12

if not args.continued:
    assert len(reader.pages)==2 and reader.metadata.title=='Images and nested cells'
    word=Document(out/'rich-cells.docx');outer=word.tables[0]
    assert outer.cell(0,0)._tc is outer.cell(0,1)._tc and outer.cell(0,0)._tc is outer.cell(1,0)._tc
    inner=outer.cell(0,0).tables[0];assert inner.cell(0,0)._tc is inner.cell(1,0)._tc
    links,uris=annotation_targets();assert len(links)==2 and all(source==target==1 for source,target,_ in links)
    assert uris==[(1,'https://example.invalid/inner')]
    expected_rectangles=[(30,30,240,224),(36,172,114,54),(150,172,114,26),(150,198,114,28),(270,30,120,26),(270,56,120,198)]
    expected_images={(12,6):(36,50,264,164),(3,2):(42,192,72,212),(2,1):(48,336,60,342)}
    with pymupdf.open(out/'rich-cells.pdf') as doc:
        assert doc[0].get_text().count('PREFACE')==3 and 'CELL_START' not in doc[0].get_text()
        page=doc[1];found=[]
        for path in page.get_drawings():
            for item in path['items']:
                if item[0]=='re':
                    r=item[1];found.append((r.x0,r.y0,r.width,r.height))
        assert len(found)==6
        for a,b in zip(found,expected_rectangles):assert all(abs(x-y)<.03 for x,y in zip(a,b)),(a,b)
        info=page.get_image_info(xrefs=True);assert len(info)==3
        original={(Image.open(io.BytesIO(blob)).width,Image.open(io.BytesIO(blob)).height):Image.open(io.BytesIO(blob)).convert('RGB').tobytes() for blob in images}
        for item in info:
            size=(item['width'],item['height']);assert all(abs(a-b)<.03 for a,b in zip(item['bbox'],expected_images[size]))
            extracted=Image.open(io.BytesIO(doc.extract_image(item['xref'])['image'])).convert('RGB')
            assert extracted.tobytes()==original[size]
        body=page.get_text()
        for word in ('CELL_START','CELL_END','INNER_A','INNER_B','INNER_C','TOP_RIGHT','BOTTOM_RIGHT','AFTER_TABLE','NOTE_CELL','NOTE_NESTED'):assert body.count(word)==1
        # Measured body ends before the independently visible note separator.
        separators=[p for p in page.get_drawings() if any(i[0]=='l' and abs(i[1].y-i[2].y)<.01 and abs(i[1].y-296)<.03 for i in p['items'])]
        assert len(separators)==1
        for i,p in enumerate(doc):p.get_pixmap(matrix=pymupdf.Matrix(1.6,1.6)).save(out/f'page-{i+1}.png')
else:
    assert len(reader.pages)==4
    links,uris=annotation_targets();assert len(links)==1 and links[0][0]==links[0][1]==0 and not uris
    with pymupdf.open(out/'rich-cells.pdf') as doc:
        assert [len(p.get_image_info()) for p in doc]==[1,0,0,0]
        assert [p.get_text().count('NOTE_ROW_') for p in doc]==[3,5,4,0]
        whole=''.join(p.get_text() for p in doc)
        for i in range(12):assert whole.count(f'NOTE_ROW_{i:02d}')==1
        assert 'BODY_AFTER' in doc[3].get_text() and 'BODY_AFTER' not in ''.join(p.get_text() for p in list(doc)[:3])
        assert len(doc[0].get_links())==1 and doc[0].get_links()[0]['page']==0
        for i,p in enumerate(doc):p.get_pixmap(matrix=pymupdf.Matrix(1.6,1.6)).save(out/f'page-{i+1}.png')
with pymupdf.open(out/'rich-cells.pdf') as doc:
    margin=sf['attrs']['layout']['margin']
    for p in doc:
        for b in p.get_text('dict')['blocks']:
            for line in b.get('lines',[]):
                x0,y0,x1,y1=line['bbox'];assert x0>=margin-.1 and y0>=margin-.1 and x1<=p.rect.width-margin+.1 and y1<=p.rect.height-margin+.1,(line['bbox'],plain(sf))
print('Rich cells passed: independent DOCX relationships/merges/assets, PDF geometry/pixels/links/note reservations/continuations/content/bounds'+(' (continued)' if args.continued else ''))
