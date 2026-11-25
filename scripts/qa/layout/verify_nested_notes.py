"""Independent nested definition/Ref ownership, source hierarchy and output QA."""
from pathlib import Path
from zipfile import ZipFile
import json,base64,io
from pypdf import PdfReader
from pypdf.generic import IndirectObject
from docx import Document
from PIL import Image
import pymupdf
root=Path('output/nested-notes')
def identity(v):return f'({v.idnum} {v.generation} R)'
def tree(v):
    if isinstance(v,list):
        for child in v:yield from tree(child)
    elif isinstance(v,IndirectObject):
        obj=v.get_object();yield identity(v),obj;yield from tree(obj.get('/K'))
def walk(nodes):
    for n in nodes:
        yield n;yield from walk(n.get('content',[]))
def editable(v):
    if isinstance(v,list):return [editable(x) for x in v if not(isinstance(x,dict) and x.get('type')=='source')]
    if isinstance(v,dict):
        out={k:editable(x) for k,x in v.items() if k not in ('source','label_source')}
        return {k:x for k,x in out.items() if not(k in ('marks','attrs') and not x)}
    return v
pages=0
for name in ['nested-definition','redundant-wrapper','opaque-wrapper']:
    original=PdfReader('output/audit/'+name+'-note.pdf');elems=dict(tree(original.trailer['/Root']['/StructTreeRoot'].get('/K')))
    sf=json.loads((root/(name+'.sf')).read_text());word=json.loads((root/(name+'-word.sf')).read_text())
    Document(root/(name+'.docx'))
    assert editable(sf['content'])==editable(word['content'])
    assert editable(sf['attrs']['notes'])==editable(word['attrs']['notes'])
    notes=sf['attrs']['notes'];refs=[n for n in walk(sf['content']) if n['type']=='note_reference']
    for note in notes:
        source=note['source'];obj=elems[source['element']]
        assert source['parent_element']==identity(obj.raw_get('/P'))
        assert base64.b64decode(source['structure_id'])==obj['/ID'].original_bytes
        for edge in source['references']:assert edge['targets']==[identity(t) for t in elems[edge['element']]['/Ref']]
    if name=='nested-definition':
        assert [n['id'] for n in notes]==['note-1','note-2']
        assert [n['attrs']['id'] for n in refs]==['note-1','note-2']
        assert [n['kind'] for n in notes]==['footnote','endnote']
        assert notes[1]['source']['parent_element']==notes[0]['source']['element']=='(9 0 R)'
        assert [n['type'] for n in notes[0]['content']]==['paragraph','ordered_list','table','image']
        assert [n.get('text') for n in walk(notes[1]['content']) if n['type']=='text']==['CHILD BODY']
        im=Image.open(io.BytesIO(base64.b64decode(notes[0]['content'][-1]['attrs']['src'].split(',',1)[1]))).convert('RGB')
        assert im.size==(2,1) and [im.getpixel((x,0)) for x in range(2)]==[(255,0,0),(0,0,255)]
    else:
        assert len(notes)==1 and len(refs)==(2 if name=='redundant-wrapper' else 1)
        edges=sf['content'][0]['attrs']['source']['references']
        assert {e['element'] for e in edges}=={'(8 0 R)','(21 0 R)'}
        for edge in edges:assert edge['targets']==[identity(t) for t in elems[edge['element']]['/Ref']]
        if name=='redundant-wrapper':
            source=next(m['attrs'] for m in refs[0]['marks'] if m['type']=='source')
            assert source['parent_element']=='(21 0 R)'
            assert {e['element'] for e in source['references']}=={'(8 0 R)','(21 0 R)'}
            assert not any(d['code']=='pdf_note_relationship' for d in sf['attrs']['diagnostics'])
        else:
            assert any(n.get('text')=='DIFFERENTMARK' for n in walk(sf['content']))
            assert any(d['code']=='pdf_note_relationship' for d in sf['attrs']['diagnostics'])
    for kind,path in [('native',root/(name+'.pdf')),('word',root/'lo'/(name+'.pdf'))]:
        doc=pymupdf.open(path);pages+=len(doc);text=''.join(p.get_text() for p in doc)
        assert all(text.count(s)==1 for s in ['NOTE BODY','LIST ITEM','TABLE CELL'])
        if name=='nested-definition':assert text.count('CHILD BODY')==1 and len(doc)==2
        if name=='opaque-wrapper':assert 'DIFFERENTMARK' in text
        if kind=='native':
            links=[l for p in doc for l in p.get_links() if l['kind']==pymupdf.LINK_GOTO]
            assert len(links)==len(refs)
            assert [l['page'] for l in links]==([0,1] if name=='nested-definition' else [0]*len(refs))
            assert len(doc[0].get_images())==1
        for i,p in enumerate(doc):p.get_pixmap(matrix=pymupdf.Matrix(1.25,1.25)).save(root/f'{name}-{kind}-{i+1}.png')
print('Original parent/ID/Ref graph, independent nested foot/endnote sites and body ownership/order, exact SF/Word, list/table/pixels, wrapper alias/opaque evidence and native link targets pass;',pages,'rendered pages')
