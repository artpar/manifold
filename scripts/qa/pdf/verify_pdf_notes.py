"""Independent PDF structure-edge, editable note, raster and output checks.

Run generate_pdf_semantic_audit.py and the commands in docs/PDF_NOTES.md first.
No production runtime dependency.
"""
from pathlib import Path
import base64
import io
import json
from pypdf import PdfReader
from pypdf.generic import IndirectObject
from PIL import Image
from docx import Document
import pymupdf

root=Path('output/audit')

def identity(value):
    return f'({value.idnum} {value.generation} R)'

def tree(value):
    if isinstance(value,list):
        for child in value:yield from tree(child)
    elif isinstance(value,IndirectObject):
        obj=value.get_object()
        yield identity(value),obj
        yield from tree(obj.get('/K'))

def sfwalk(nodes):
    for node in nodes:
        yield node
        yield from sfwalk(node.get('content',[]))

for name in ('explicit-note','rich-explicit-note'):
    pdf=PdfReader(root/(name+'.pdf'))
    elems=dict(tree(pdf.trailer['/Root']['/StructTreeRoot'].get('/K')))
    definitions={key:obj for key,obj in elems.items() if obj.get('/S') in ('/Note','/FENote')}
    sf=json.loads((root/(name+'.sf')).read_text())
    notes=sf['attrs']['notes']
    assert len(notes)==len(definitions)==1
    note=notes[0]
    source=note['source']
    original=definitions[source['element']]
    assert base64.b64decode(source['structure_id'])==original['/ID'].original_bytes
    assert source['structure']=='explicit' and source['page']==1 and source['quad']
    forward={key for key,obj in elems.items() if any(identity(target)==source['element'] for target in obj.get('/Ref',[]))}
    reverse={identity(ref) for ref in original.get('/Ref',[])}
    expected=forward|reverse
    refs=[node for node in sfwalk(sf['content']) if node['type']=='note_reference']
    assert len(refs)==len(expected)
    recovered=set()
    for ref in refs:
        assert ref['attrs']=={'id':note['id'],'kind':note['kind']}
        provenance=next(mark['attrs'] for mark in ref['marks'] if mark['type']=='source')
        assert provenance['structure']=='explicit' and provenance['evidence']=='pdf_note_explicit_ref'
        recovered.add(provenance['element'])
        for edge in provenance.get('references',[]):
            assert edge['targets']==[identity(t) for t in elems[edge['element']]['/Ref']]
    assert recovered==expected
    for edge in source.get('references',[]):
        assert edge['targets']==[identity(t) for t in original['/Ref']]
    if name=='explicit-note':
        assert note['label']=='1' and note['kind']=='endnote' and note['kind_inferred']
    else:
        assert note['label']=='*1' and note['kind']=='footnote' and not note.get('kind_inferred',False)
        assert [node['type'] for node in note['content']]==['paragraph','ordered_list','table','image']
        texts=[node for node in sfwalk(note['content']) if node['type']=='text']
        assert [n['text'] for n in texts]==['NOTE BODY','LIST ITEM','TABLE CELL']
        assert all(any(mark['type']=='strong' for mark in n['marks']) for n in texts)
        assert note['label_source']['role']=='Lbl' and note['label_source']['font']=='Courier-Bold' and note['label_source']['quad']
        figure=note['content'][-1]
        assert figure['attrs']['src'].startswith('data:image/png;base64,')
        im=Image.open(io.BytesIO(base64.b64decode(figure['attrs']['src'].split(',',1)[1]))).convert('RGB')
        assert im.size==(2,1) and [im.getpixel((x,0)) for x in range(2)]==[(255,0,0),(0,0,255)]
        assert figure['attrs']['alt']=='RED BLUE'
        view=sf['attrs']['pdfNavigation']['destinations'][0]
        assert view['flow_binding']=='nearest_content' and view['id'] in note['content'][0]['attrs']['anchors']
    print(name, ': original PDF object graph, note identity/kind/mark/positions, exact reference sites and rich body pass')

# Source provenance is an acknowledged DOCX reflow loss. Compare every other
# canonical note value, removing only source records/marks and empty defaults.
def editable(value):
    if isinstance(value,list):
        return [editable(v) for v in value if not (isinstance(v,dict) and v.get('type')=='source')]
    if isinstance(value,dict):
        out={k:editable(v) for k,v in value.items() if k not in ('source','label_source')}
        return {k:v for k,v in out.items() if not (k in ('attrs','marks') and not v)}
    return value
before=json.loads((root/'rich-explicit-note.sf').read_text())['attrs']['notes']
after=json.loads((root/'rich-note-docx.sf').read_text())['attrs']['notes']
assert editable(before)==editable(after)
print('DOCX round trip: all editable rich-note values, identities, marks, anchors and raster bytes match')

Document(root/'rich-note.docx')
for path in (root/'rich-note-native.pdf',root/'lo/rich-note.pdf'):
    pdf=PdfReader(path)
    text='\n'.join(p.extract_text() for p in pdf.pages)
    assert len(pdf.pages)==1 and text.count('*1')==3
    assert '2.' not in text, 'spurious second list marker before table'
    for marker in ('NOTE BODY','LIST ITEM','TABLE CELL'):
        assert text.count(marker)==1
    doc=pymupdf.open(path)
    links=[link for page in doc for link in page.get_links() if link['kind']==pymupdf.LINK_GOTO]
    assert len(links)>=2 and all(link['page']==0 for link in links)
    images=[pix for page in doc for pix in page.get_images()]
    assert images
    raster=[picture.image.convert('RGB') for page in pdf.pages for picture in page.images]
    assert any(im.size==(2,1) and [im.getpixel((x,0)) for x in range(2)]==[(255,0,0),(0,0,255)] for im in raster)
    for i,page in enumerate(doc):page.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(str(path)+f'.{i}.png')
    print(path, ': one page, exact reference/body counts, linked notes and embedded image pass')
