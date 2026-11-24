"""Independent adjacency and original-publication note/view recovery audit.

Optional guide inputs stay ignored: this script does not redistribute the PDF.
"""
from pathlib import Path
import argparse
import base64
import gzip
import hashlib
import json
from pypdf import PdfReader
from pypdf.generic import ContentStream,TextStringObject,ByteStringObject
from pypdf._cmap import get_encoding

parser=argparse.ArgumentParser()
parser.add_argument('--guide',type=Path)
parser.add_argument('--guide-sf',type=Path)
args=parser.parse_args()
root=Path('output/note-inference')

def load(path):
    with (gzip.open(path,'rt') if str(path).endswith('.gz') else open(path)) as f:return json.load(f)
def walk(nodes):
    for node in nodes:
        yield node
        yield from walk(node.get('content',[]))
def source(node):return next(m['attrs'] for m in node['marks'] if m['type']=='source')
def painted_text(reader,page_number,ids):
    page=reader.pages[page_number-1]
    fonts={str(k):get_encoding(v.get_object()) for k,v in page['/Resources']['/Font'].items()}
    stack=[];font=None;output={}
    for operands,operator in ContentStream(page['/Contents'],reader).operations:
        if operator==b'Tf':font=str(operands[0])
        elif operator==b'BDC':stack.append(operands[1].get('/MCID') if isinstance(operands[1],dict) else None)
        elif operator==b'BMC':stack.append(None)
        elif operator==b'EMC':stack.pop()
        elif operator in (b'Tj',b'TJ') and stack and stack[-1] in ids:
            for value in ([operands[0]] if operator==b'Tj' else operands[0]):
                if not isinstance(value,(TextStringObject,ByteStringObject)):continue
                raw=value.original_bytes if isinstance(value,TextStringObject) else bytes(value)
                encoding,mapping=fonts[font]
                decoded=raw.decode(encoding) if isinstance(encoding,str) else ''.join(encoding.get(c,chr(c)) for c in raw)
                output[int(stack[-1])]=output.get(int(stack[-1]),'')+''.join(mapping.get(c,c) for c in decoded)
    return output

pdf=PdfReader('output/audit/adjacent-note.pdf')
paragraph=pdf.get_object(7);note=pdf.get_object(9)
assert paragraph['/K'][1].idnum==8 and paragraph['/K'][2].idnum==9
assert pdf.get_object(8)['/S']=='/Reference' and note['/S']=='/Note'
assert '/Ref' not in note and '/Ref' not in pdf.get_object(8)
paint=painted_text(pdf,1,{1,2,3})
assert paint[1]==paint[2]=='*1'
sf=load(root/'adjacent.sf');notes=sf['attrs']['notes'];refs=[n for n in walk(sf['content']) if n['type']=='note_reference']
assert len(notes)==len(refs)==1 and refs[0]['attrs']['inferred']
assert refs[0]['attrs']['id']==notes[0]['id'] and notes[0]['kind_inferred']
src=source(refs[0]);assert src['structure']=='explicit' and src['evidence']=='pdf_note_adjacent_reference' and src['confidence']==.85 and not src.get('references')
assert [n['type'] for n in notes[0]['content']]==['paragraph','ordered_list','table','image']
assert ''.join(n['text'] for n in walk(notes[0]['content'][0:1]) if n['type']=='text')==paint[3]
roundtrip=load(root/'adjacent-docx.sf');rs=[n for n in walk(roundtrip['content']) if n['type']=='note_reference']
assert len(rs)==1 and rs[0]['attrs']==refs[0]['attrs']
print('Adjacent probe: original roles/order/paint, separate relationship/kind inference, rich body and DOCX provenance pass')

if args.guide:
    assert args.guide_sf
    digest=hashlib.sha256(args.guide.read_bytes()).hexdigest()
    assert digest=='8185a30793869379ea4c2d82f214a930368b3fa7520faba61df38c52a502335e'
    reader=PdfReader(args.guide);sf=load(args.guide_sf)
    notes={n['id']:n for n in sf['attrs']['notes']};refs=[n for n in walk(sf['content']) if n['type']=='note_reference']
    assert set(notes)=={'NOTE_1','NOTE_2'} and len(refs)==2
    for name,objid,page,label_mcid,body_mcid,annotation in [('NOTE_1',2363,8,130,131,84),('NOTE_2',2367,22,671,672,113)]:
        original=reader.get_object(objid);actual=notes[name]
        assert original['/S']=='/Note' and '/Ref' not in original
        assert base64.b64decode(actual['source']['structure_id'])==original['/ID'].original_bytes
        paint=painted_text(reader,page,{label_mcid,body_mcid})
        assert actual['label']==paint[label_mcid]
        body=''.join(n['text'] for n in walk(actual['content']) if n['type']=='text')
        assert body==paint[body_mcid],(name,'body differs from independently decoded original MCID paint')
        ref=next(n for n in refs if n['attrs']['id']==name);src=source(ref)
        assert ref['attrs']['inferred'] and actual['kind_inferred']
        assert src['structure']=='explicit' and src['evidence']=='pdf_note_link_view' and src['confidence']==.95 and not src.get('references')
        explicit=next(n for n in sf['attrs']['pdfNavigation']['links'] if n['object']==f'({annotation} 0 R)')
        view=next(n for n in sf['attrs']['pdfNavigation']['destinations'] if n['id']==explicit['destination'])
        annot=reader.get_object(annotation)
        dest=annot['/Dest'] if '/Dest' in annot else annot['/A']['/D']
        assert view['parameters']==[None if v is None else float(v) for v in dest[2:]]
        assert view['page']==actual['source']['page']==page
        q=actual['label_source']['quad'];x0=min(p['x'] for p in q);x1=max(p['x'] for p in q);y0=min(p['y'] for p in q);y1=max(p['y'] for p in q)
        assert x0-2<=view['point']['x']<=x1+2 and y0-2<=view['point']['y']<=y1+2
        print(name,': original ID/label/body MCID text hash',hashlib.sha256(body.encode()).hexdigest(),'; exact physical view/region and inferred typed site pass')
    print('Original 71-page guide: both real notes and citation sites pass independent object/paint/view checks; raw source bytes pinned')
