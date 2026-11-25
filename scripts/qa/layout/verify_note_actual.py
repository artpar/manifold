"""Independent exact/opaque structural ActualText note graph and output checks."""
from pathlib import Path
import base64,io,json
from pypdf import PdfReader
from pypdf.generic import ContentStream,IndirectObject
from PIL import Image
from docx import Document
import pymupdf

root=Path('output/note-actual')
def walk(nodes):
    for n in nodes:
        yield n
        yield from walk(n.get('content',[]))
def editable(v):
    if isinstance(v,list):return [editable(x) for x in v if not(isinstance(x,dict) and x.get('type')=='source')]
    if isinstance(v,dict):
        d={k:editable(x) for k,x in v.items() if k not in ('source','label_source')}
        return {k:x for k,x in d.items() if not(k in ('marks','attrs') and not x)}
    return v
def tree(value):
    if isinstance(value,list):
        for child in value:yield from tree(child)
    elif isinstance(value,IndirectObject):
        obj=value.get_object();yield f'({value.idnum} {value.generation} R)',obj
        yield from tree(obj.get('/K'))
def ident(v):return f'({v.idnum} {v.generation} R)'
reader=PdfReader('output/audit/aligned-actual-note.pdf')
elems=dict(tree(reader.trailer['/Root']['/StructTreeRoot'].get('/K')))
assert elems['(7 0 R)']['/ActualText']=='BODY*1MIDDLE*1AFTER'
assert elems['(21 0 R)']['/ActualText']=='*1'
assert [ident(r) for r in elems['(9 0 R)']['/Ref']]==['(8 0 R)','(18 0 R)']
paint=[];stack=[];size=None;color=0
for args,op in ContentStream(reader.pages[0]['/Contents'],reader).operations:
    if op==b'BDC':stack.append(args[1].get('/MCID') if isinstance(args[1],dict) else None)
    elif op==b'BMC':stack.append(None)
    elif op==b'EMC':stack.pop()
    elif op==b'Tf':size=float(args[1])
    elif op==b'rg':color=tuple(float(x) for x in args)
    elif op==b'Tj' and stack and stack[-1]==1:paint.append((str(args[0]),size,color))
assert paint==[('*',10.,0),('1',8.,(1.,0.,0.))]
source=json.loads((root/'source.sf').read_text());word=json.loads((root/'word.sf').read_text())
refs=[n for n in walk(source['content']) if n['type']=='note_reference'];assert len(refs)==2
assert [next(m['attrs']['element'] for m in n['marks'] if m['type']=='source') for n in refs]==['(8 0 R)','(18 0 R)']
for n in refs:
    provenance=next(m['attrs'] for m in n['marks'] if m['type']=='source')
    assert provenance['structure']=='explicit' and 'pdf_structure_actualtext_aligned' in provenance['evidence']
    assert not n['attrs'].get('inferred',False)
rich=refs[0]['attrs']['label_content'];assert [n['text'] for n in rich]==['*','1']
assert [next(m['attrs']['fontSize'] for m in n['marks'] if m['type']=='textStyle') for n in rich]==[10,8]
assert [n['text'] for n in source['content'][0]['content'] if n['type']=='text']==['BODY','MIDDLE','AFTER']
note=source['attrs']['notes'][0];assert len(source['attrs']['notes'])==1 and note['id']=='note-1'
assert [n['type'] for n in note['content']]==['paragraph','ordered_list','table','image']
image=note['content'][-1]['attrs'];im=Image.open(io.BytesIO(base64.b64decode(image['src'].split(',',1)[1]))).convert('RGB')
assert im.size==(2,1) and [im.getpixel((x,0)) for x in range(2)]==[(255,0,0),(0,0,255)]
assert editable(source['content'])==editable(word['content'])
assert editable(source['attrs']['notes'])==editable(word['attrs']['notes'])
Document(root/'word.docx')
opaque=json.loads((root/'opaque.sf').read_text())
assert not opaque['attrs'].get('notes') and not [n for n in walk(opaque['content']) if n['type']=='note_reference']
assert any(n.get('text')=='DIFFERENT*1TEXT*1CONTENT' for n in walk(opaque['content']))
assert any(d['code']=='pdf_note_relationship' for d in opaque['attrs']['diagnostics'])
assert any(n.get('attrs',{}).get('source',{}).get('references') for n in opaque['content'])
for name in ('native','lo/word'):
    doc=pymupdf.open(root/(name+'.pdf'));assert len(doc)==1
    p=doc[0];text=p.get_text()
    assert text.count('BODY')==2 and all(text.count(v)==1 for v in ('MIDDLE','AFTER','NOTE BODY','LIST ITEM','TABLE CELL'))
    red=[s for b in p.get_text('dict')['blocks'] for l in b.get('lines',[]) for s in l['spans'] if s['text']=='1' and s['color']==0xFF0000]
    assert len(red)==1
    if name=='native':
        assert abs(red[0]['size']-5.6)<.001
        links=[l for l in p.get_links() if l['kind']==pymupdf.LINK_GOTO]
        assert len(links)==2 and all(l['to'].y>650 for l in links)
    p.get_pixmap(matrix=pymupdf.Matrix(1.3,1.3)).save(root/('native.png' if name=='native' else 'word.png'))
print('Original nested ActualText/Ref operators, full exact correspondence, two explicit sites, individual source styles/regions, exact Word meaning, list/table/raster, native links and opaque fallback pass')
