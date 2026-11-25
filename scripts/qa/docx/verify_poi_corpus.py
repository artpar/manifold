"""Independent original/export reader, structure, native geometry/navigation QA."""
from pathlib import Path
import json,re,unicodedata,hashlib
from zipfile import ZipFile
from lxml import etree
from docx import Document
from PIL import Image,ImageDraw
import pymupdf
from pypdf import PdfReader

root=Path('output/real-docx-audit');generated=root/'generated'
oracles=json.loads(Path('testdata/real/poi-docx-oracle.json').read_text())
def walk(nodes):
    for n in nodes:
        yield n
        yield from walk(n.get('content',[]))
def normalized(s):return ''.join(unicodedata.normalize('NFC',s).split())
def model(v):
    if isinstance(v,dict):return {k:model(x) for k,x in v.items() if k!='diagnostics'}
    if isinstance(v,list):return [model(x) for x in v]
    return v
summary={};total_pages=0
for filename,oracle in oracles.items():
    stem=filename[4:-5];source=Path('testdata/real')/filename
    assert hashlib.sha256(source.read_bytes()).hexdigest()==oracle['sha256']
    Document(source);Document(generated/(stem+'.docx'))
    sf=json.loads((root/(stem+'.sf')).read_text());back=json.loads((generated/(stem+'.sf')).read_text());assert model(sf)==model(back)
    with ZipFile(source) as z:
        app=etree.fromstring(z.read('docProps/app.xml'))
        assert oracle['application']==app.find('{http://schemas.openxmlformats.org/officeDocument/2006/extended-properties}Application').text
    native=PdfReader(generated/(stem+'.pdf'));physical=json.loads((generated/(stem+'-pdf.sf')).read_text())
    assert len(native.pages)==len(physical['attrs']['pages'])
    for p,record in zip(native.pages,physical['attrs']['pages']):
        assert abs(float(p.mediabox.width)-record['width'])<.01 and abs(float(p.mediabox.height)-record['height'])<.01
    docs=[pymupdf.open(root/'original-render'/(stem+'.pdf')),pymupdf.open(generated/(stem+'.pdf')),pymupdf.open(root/'word-render'/(stem+'.pdf'))]
    total_pages+=sum(len(d) for d in docs)
    texts=[''.join(p.get_text() for p in d) for d in docs]
    for i,text in enumerate(texts):
        # Physical note numbers interrupt paragraph text; check each literal
        # segment around source reference sites independently, not invented labels.
        for para in oracle['body']:
            for part in re.split('\ufffc(?:footnote|endnote):[^\ufffc]+\ufffc',para):
                assert normalized(part) in normalized(text),(stem,i,part)
        for note in oracle['notes']:
            for para in note['paragraphs']:assert normalized(para) in normalized(text)
    # Source comments are annotations; final visible documents need not render them.
    for comment in oracle['comments']:assert comment not in texts[1] and comment not in texts[2]
    for target in oracle['hyperlinks']:
        links=[l.get('uri') for p in docs[1] for l in p.get_links() if l['kind']==pymupdf.LINK_URI]
        assert target in links,(stem,target)
        with ZipFile(generated/(stem+'.docx')) as z:
            rels=etree.fromstring(z.read('word/_rels/document.xml.rels'))
            assert target in [r.get('Target') for r in rels]
    if stem=='delins':
        # This independent producer emits no URI annotations from either the
        # original or generated Word file. Package/native targets are separate.
        assert all(not p.get_links() for doc in (docs[0],docs[2]) for p in doc)
    if oracle['notes']:
        links=[l for p in docs[1] for l in p.get_links() if l['kind']==pymupdf.LINK_GOTO]
        assert len(links)==1 and links[0]['page']==(1 if stem=='endnotes' else 0)
    if stem=='HeaderFooterUnicode':
        spans=[s for b in docs[1][0].get_text('dict')['blocks'] for line in b.get('lines',[]) for s in line['spans']]
        title=[s for s in spans if s['text']=='Molière'];assert len(title)==1 and abs(title[0]['size']-24)<.001 and title[0]['flags']&16
        for doc in docs:
            assert len(doc)==2
            for p in doc:
                assert '€ euro' in p.get_text() and 'The footer, with Molière' in p.get_text()
    if stem=='DiffFirstPageHeadFoot':
        for doc in docs:
            assert len(doc)==2
            assert 'I am the header on the first page' in doc[0].get_text() and 'The footer of the first page' in doc[0].get_text()
            assert all(value in doc[1].get_text() for value in ('First header column!','Mid header','Right header!','Footer Left','Footer Middle','Footer Right'))
        words=docs[1][1].get_text('words')
        assert words[2][4]=='column!' and words[3][4]=='Mid' and words[3][0]-words[2][2]>10
        assert any(d['code']=='position_tab' for d in sf['attrs']['diagnostics'])
    sheet=Image.new('RGB',(1200,30+max(len(d) for d in docs)*580),'#dddddd');draw=ImageDraw.Draw(sheet)
    for col,(label,doc) in enumerate(zip(('Original LibreOffice','Native supplied DejaVu','Generated Word LibreOffice'),docs)):
        draw.text((col*400+8,8),label,fill='black')
        for page,p in enumerate(doc):
            pix=p.get_pixmap(matrix=pymupdf.Matrix(.7,.7));im=Image.frombytes('RGB',(pix.width,pix.height),pix.samples)
            im.thumbnail((390,570));sheet.paste(im,(col*400+5,30+page*580))
    sheet.save(root/(stem+'-comparison.png'))
    summary[stem]={'original_pages':len(docs[0]),'native_pages':len(docs[1]),'word_pages':len(docs[2]),'exact_sf_roundtrip':True,'original_hyperlinks':len(oracle['hyperlinks']),'note_bodies':len(oracle['notes'])}
(root/'independent.json').write_text(json.dumps({'sources':summary,'rendered_pages':total_pages},indent=2)+'\n')
print('Eight original exports: source metadata, exact SF/Word, actual PDF geometry, literal visible segments, note targets, native/package URI relationships and conditional/Unicode stories pass; LibreOffice emits zero delins URI annotations from both original/generated Word;',total_pages,'rendered pages')
