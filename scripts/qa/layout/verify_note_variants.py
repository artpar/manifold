"""Independent variant-note object, package, model, navigation and pixel QA."""
from pathlib import Path
from zipfile import ZipFile
import json
import base64
import io
from lxml import etree
from docx import Document
from pypdf import PdfReader
from pypdf.generic import ContentStream
from PIL import Image
import pymupdf

root=Path('output/note-variants')
ns={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main','md':'urn:manydoc:metadata:1'}
def walk(nodes):
    for node in nodes:
        yield node
        yield from walk(node.get('content',[]))
def editable(value):
    if isinstance(value,list):return [editable(v) for v in value if not(isinstance(v,dict) and v.get('type')=='source')]
    if isinstance(value,dict):
        out={k:editable(v) for k,v in value.items() if k not in ('source','label_source')}
        return {k:v for k,v in out.items() if not(k in ('attrs','marks') and not v)}
    return value
for prefix in ('variant','markerless'):
    pdf=PdfReader(f'output/audit/{prefix}-explicit-note.pdf')
    paint={};active=[]
    for operands,operator in ContentStream(pdf.pages[0]['/Contents'],pdf).operations:
        if operator==b'BDC':active.append(operands[1].get('/MCID'))
        elif operator==b'BMC':active.append(None)
        elif operator==b'EMC':active.pop()
        elif operator==b'Tj' and active and active[-1] is not None:paint.setdefault(int(active[-1]),'');paint[int(active[-1])]+=str(operands[0])
    sf=json.loads((root/f'{prefix}-pdf.sf').read_text());note=sf['attrs']['notes'][0]
    source=note['source'];assert source['role']=='FENote' and source['structure']=='explicit'
    original=pdf.get_object(9)
    assert base64.b64decode(source['structure_id'])==original['/ID'].original_bytes
    refs=[node for node in walk(sf['content']) if node['type']=='note_reference']
    assert len(refs)==2 and len(sf['attrs']['notes'])==1
    expected={f'({ref.idnum} {ref.generation} R)' for ref in original['/Ref']}
    recovered=set()
    for ref in refs:
        assert ref['attrs']['id']==note['id'] and ref['attrs']['kind']==note['kind']
        provenance=next(mark['attrs'] for mark in ref['marks'] if mark['type']=='source')
        assert provenance['structure']=='explicit' and provenance['evidence']=='pdf_note_explicit_ref'
        recovered.add(provenance['element'])
        for edge in provenance.get('references',[]):
            owner=pdf.get_object(int(edge['element'].split()[0][1:]))
            assert edge['targets']==[f'({r.idnum} {r.generation} R)' for r in owner['/Ref']]
    assert recovered==expected
    assert source['references'][0]['element']=='(9 0 R)' and set(source['references'][0]['targets'])==expected
    if prefix=='variant':
        assert note['label']==paint[1]=='*1' and note['body_label']==paint[2]=='N-1'
        assert refs[0]['attrs'].get('label') is None and refs[1]['attrs']['label']==paint[7]=='OTHER'
        assert note['label_source']['element']=='(10 0 R)' and note['label_source']['quad'] and note['label_source']['font']=='Courier-Bold'
    else:
        assert all(ref['attrs']['label']=='' for ref in refs) and note['body_label']==''
        assert not any(mcid in paint for mcid in (1,2,7))
        assert all(pdf.get_object(obj)['/K']==[] for obj in (8,10,18))
        assert not any(d['code']=='pdf_structure_missing_content' for d in sf['attrs']['diagnostics'])
    assert [node['type'] for node in note['content']]==['paragraph','ordered_list','table','image']
    image=note['content'][-1]
    im=Image.open(io.BytesIO(base64.b64decode(image['attrs']['src'].split(',',1)[1]))).convert('RGB')
    assert im.size==(2,1) and [im.getpixel((x,0)) for x in range(2)]==[(255,0,0),(0,0,255)]
    got=json.loads((root/f'{prefix}-docx.sf').read_text())
    assert editable(sf['attrs']['notes'])==editable(got['attrs']['notes'])
    assert editable(sf['content'])==editable(got['content'])
    Document(root/f'{prefix}.docx')
    print(prefix, ': original paint/graph, distinct versus empty values, rich bodies/pixels and editable DOCX round trip pass')

Document(root/'notes.docx')
with ZipFile(root/'notes.docx') as z:
    body=etree.fromstring(z.read('word/document.xml'))
    anchors=body.xpath('//w:r[@md:noteAnchor]',namespaces=ns)
    assert len(anchors)==2 and all(n.xpath('string(w:t)',namespaces=ns)=='\u200b' for n in anchors)
    citations=body.xpath('//w:hyperlink[@md:noteCitation]',namespaces=ns)
    assert [''.join(n.xpath('w:r/w:t/text()',namespaces=ns)) for n in citations]==['*α','E','see α']
    for n in citations:
        assert n.get('{'+ns['w']+'}anchor')==n.get('{'+ns['md']+'}noteCitation')
        assert n.xpath('w:r/w:rPr/w:rStyle',namespaces=ns)
    for part,label in [('footnotes','N-1 '),('endnotes','§E ')]:
        tree=etree.fromstring(z.read('word/'+part+'.xml'))
        assert tree.xpath('//w:r[@md:noteBodyLabel]/w:t/text()',namespaces=ns)==[label]
for path in (root/'notes.pdf',root/'lo/notes.pdf'):
    pdf=PdfReader(path);text='\n'.join(p.extract_text() for p in pdf.pages)
    assert len(pdf.pages)==2
    for mark in ('*α','see α','N-1','§E','FIRST_NOTE_BODY','SECOND_NOTE_BODY','END_NOTE_BODY'):
        assert text.count(mark)==1,(path,mark,text)
    assert '\u200b' not in text
    doc=pymupdf.open(path)
    first=doc[0].search_for('FIRST_NOTE_BODY')[0];end=doc[1].search_for('END_NOTE_BODY')[0]
    # Check the visible first/repeated citations, not just invisible anchor links.
    for mark,page,target in [('*α',0,first),('see α',0,first),('E.',1,end)]:
        regions=doc[0].search_for(mark)
        links=[link for link in doc[0].get_links() if link['kind']==pymupdf.LINK_GOTO and link['page']==page and any(link['from'].intersects(rect) for rect in regions)]
        assert links and any(abs(link['to'].y-target.y0)<20 for link in links),(path,mark,links)
    for i,page in enumerate(doc):page.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(str(path)+f'.{i}.png')
    print(path, ': exact visible marks/bodies, no anchor glyphs, actual visible citation-to-body coordinates pass')
