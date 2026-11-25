"""Independent citation source operators, SF/Word fragments and PDF box/link QA."""
from pathlib import Path
from zipfile import ZipFile
import json
from lxml import etree
from docx import Document
from pypdf import PdfReader
from pypdf.generic import ContentStream
import pymupdf

root=Path('output/rich-citations')
ns={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main','md':'urn:manydoc:metadata:1'}
def walk(nodes):
    for n in nodes:
        yield n
        yield from walk(n.get('content',[]))
def editable(v):
    if isinstance(v,list):return [editable(x) for x in v if not(isinstance(x,dict) and x.get('type')=='source')]
    if isinstance(v,dict):
        out={k:editable(x) for k,x in v.items() if k not in ('source','label_source')}
        return {k:x for k,x in out.items() if not(k in ('marks','attrs') and not x)}
    return v

def word_fragments(path,count):
    Document(path)
    with ZipFile(path) as z:
        tree=etree.fromstring(z.read('word/document.xml'))
        citations=tree.xpath('//w:hyperlink[@md:noteOverride="rich"]',namespaces=ns)
        assert len(citations)==count
        for citation in citations:
            assert citation.get('{'+ns['w']+'}anchor')==citation.get('{'+ns['md']+'}noteCitation')
            assert citation.get('{'+ns['md']+'}noteIsolation')=='ltr'
            allruns=citation.findall('w:r',ns)
            assert [allruns[i].find('w:t',ns).text for i in (0,-1)]==['\u2066','\u2069']
            runs=[r for r in allruns if r.get('{'+ns['md']+'}noteIsolation') is None];assert len(runs)==2
            assert all(run.find('w:rPr/w:rStyle',ns) is not None for run in runs)
            assert all(run.find('w:t',ns) is not None for run in runs)

def sf_equal(before,after):
    assert editable(before['content'])==editable(after['content'])
    assert editable(before['attrs']['notes'])==editable(after['attrs']['notes'])

def spans(page):return [s for b in page.get_text('dict')['blocks'] for line in b.get('lines',[]) for s in line['spans']]
def covers(rect,box):
    rect=pymupdf.Rect(rect);box=pymupdf.Rect(box)
    return rect.x0<=box.x0+.12 and rect.x1>=box.x1-.12 and rect.y0<=box.y0+.12 and rect.y1>=box.y1-.12

reader=PdfReader('output/audit/rich-citation-note.pdf');stack=[];size=None;color=(0.,0.,0.);paint=[]
for args,op in ContentStream(reader.pages[0]['/Contents'],reader).operations:
    if op==b'BDC':stack.append(args[1].get('/MCID') if isinstance(args[1],dict) else None)
    elif op==b'BMC':stack.append(None)
    elif op==b'EMC':stack.pop()
    elif op==b'Tf':size=float(args[1])
    elif op==b'rg':color=tuple(float(v) for v in args)
    elif op==b'Tj' and stack and stack[-1]==1:paint.append((str(args[0]),size,color))
assert paint==[('*',10.,(0.,0.,0.)),('1',8.,(1.,0.,0.))]
source=json.loads((root/'source.sf').read_text());word=json.loads((root/'word.sf').read_text())
refs=[n for n in walk(source['content']) if n['type']=='note_reference'];assert len(refs)==2
rich=refs[0]['attrs']['label_content'];assert [n['text'] for n in rich]==['*','1'] and 'label' not in refs[0]['attrs']
for i,node in enumerate(rich):
    style=next(m['attrs'] for m in node['marks'] if m['type']=='textStyle')
    assert style['fontSize']==paint[i][1] and style['color']==('#000000' if i==0 else '#FF0000')
    assert any(m['type']=='source' for m in node['marks'])
sf_equal(source,word);word_fragments(root/'word.docx',1)
for filename in ('native.pdf','lo/word.pdf'):
    doc=pymupdf.open(root/filename);assert len(doc)==1
    red=[s for s in spans(doc[0]) if s['text']=='1' and s['color']==0xFF0000];assert len(red)==1
    if filename=='native.pdf':
        assert abs(red[0]['size']-5.6)<.001
        links=[l for l in doc[0].get_links() if l['kind']==pymupdf.LINK_GOTO]
        assert len(links)==2 and sum(covers(l['from'],red[0]['bbox']) for l in links)==1
        assert all(l['page']==0 and l['to'].y>red[0]['bbox'][3]+100 for l in links)
    doc[0].get_pixmap(matrix=pymupdf.Matrix(1.2,1.2)).save(root/('source-native.png' if filename=='native.pdf' else 'source-word.png'))
print('Original citation MCID operators, separate 10pt/8pt source fragments, exact SF/Word meaning and one native link box per note reference pass')

before=json.loads((root/'example/notes.sf').read_text());after=json.loads((root/'example/word.sf').read_text());sf_equal(before,after);word_fragments(root/'example/notes.docx',4)
for filename in ('notes.pdf','lo/notes.pdf'):
    doc=pymupdf.open(root/'example'/filename);assert len(doc)==2
    first=doc[0];red=[s for s in spans(first) if s['text'] in ('-1','-2','-3') and s['color']==0xFF0000]
    assert len(red)==4
    if filename=='notes.pdf':
        links=[l for l in first.get_links() if l['kind']==pymupdf.LINK_GOTO]
        assert len(links)==4
        for span in red:
            assert abs(span['size']-5.6)<.001
            matched=[l for l in links if covers(l['from'],span['bbox'])];assert len(matched)==1
            assert matched[0]['page']==(1 if span['text']=='-2' else 0)
        body=[s for p in doc for s in spans(p) if 'NOTE_BODY' in s['text']]
        assert len(body)==3
    for i,p in enumerate(doc):p.get_pixmap(matrix=pymupdf.Matrix(1.4,1.4)).save(root/f'example/{"native" if filename=="notes.pdf" else "word"}-{i+1}.png')
print('Sectioned/repeated/list citations: four visible multi-style marks, unique complete native link boxes and correct foot/endnote pages pass')

mixed=json.loads(Path('output/audit/mixed-citations.sf').read_text());mixed_word=json.loads((root/'mixed-word.sf').read_text());sf_equal(mixed,mixed_word);word_fragments(root/'mixed.docx',2)
doc=pymupdf.open(root/'mixed.pdf');assert len(doc)==1
red=[s for s in spans(doc[0]) if s['text']=='A' and s['color']==0xFF0000];assert len(red)==2
links=[l for l in doc[0].get_links() if l['kind']==pymupdf.LINK_GOTO];assert len(links)==4
for span in red:
    assert abs(span['size']-7)<.001 and sum(covers(l['from'],span['bbox']) for l in links)==1
arabic=[s for s in spans(doc[0]) if s['text']=='بب'];assert len(arabic)==2 and all(abs(s['size']-8.4)<.001 for s in arabic)
for span in spans(doc[0]):assert span['bbox'][0]>=23.8 and span['bbox'][2]<=276.2
assert all(l['page']==0 and l['to'].y>150 for l in links)
doc[0].get_pixmap(matrix=pymupdf.Matrix(2,2)).save(root/'mixed-native.png')
lo=pymupdf.open(root/'lo/mixed.pdf')
for p in lo:
    visible=spans(p)
    for left in [s for s in visible if s['text'] in ('بب','ب')]:
        right=[s for s in visible if s['text']=='A' and abs(s['bbox'][0]-left['bbox'][2])<.5 and min(s['bbox'][3],left['bbox'][3])-max(s['bbox'][1],left['bbox'][1])>3]
        assert len(right)==1, (left, right)
for i,p in enumerate(lo):p.get_pixmap(matrix=pymupdf.Matrix(1.6,1.6)).save(root/f'mixed-word-{i+1}.png')
print('Mixed Arabic/Latin rich and simple labels: LTR/RTL surrounding flow, supplied font, logical fragments, complete link rectangles and safe bounds pass')
print('Word superscript ratio/font/pagination choices are independent producer behavior, not native geometry or source-fidelity claims')
