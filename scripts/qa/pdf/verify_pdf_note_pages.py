"""Optional independent cross-page note QA; never invoked by the Go runtime."""
import argparse
import base64
import hashlib
import io
import json
from pathlib import Path
import subprocess
import sys
import zipfile
import xml.etree.ElementTree as ET
import pymupdf
from pypdf import PdfReader
from PIL import Image

parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('root',nargs='?',type=Path,default=Path('output/pdf-note-pages'))
parser.add_argument('--cli',default='bin/manifold')
parser.add_argument('--probe',type=Path,default=Path('output/pdf-note-continuation-crosspage'))
parser.add_argument('--word-render',type=Path)
args=parser.parse_args()
W='http://schemas.openxmlformats.org/wordprocessingml/2006/main'


def command(*words):
    p=subprocess.run([args.cli,*map(str,words)],capture_output=True,text=True)
    assert p.returncode==0,(words,p.stderr)


def runs(blocks):
    for b in blocks:
        yield from b.get('runs',[])
        for item in b.get('items',[]):yield from runs(item['blocks'])
        for row in b.get('rows',[]):
            for cell in row['cells']:yield from runs(cell['blocks'])


def body_text(note):return [''.join(r.get('text','') for r in runs([b])) for b in note['blocks']]


def extract(path,target,flags=()):
    command('extract',*flags,'-format','json','-o',target,path)
    return json.loads(target.read_text())


def check_semantics(expected,actual):
    assert len(actual['notes'])==len(expected['notes'])==1
    a,b=actual['notes'][0],expected['notes'][0]
    assert a['kind']==b['kind'] and a.get('label')==b.get('label')
    assert [x['kind'] for x in a['blocks']]==[x['kind'] for x in b['blocks']]
    assert body_text(a)==body_text(b), 'note paragraphs/text changed'
    refs=[r for r in runs(actual['blocks']) if r.get('note')]
    assert len(refs)==1 and refs[0]['note']['id']==a['id']
    for x,y in zip(a['blocks'],b['blocks']):
        if x['kind']=='table':assert [[c.get('row_span',0) for c in row['cells']] for row in x['rows']]==[[c.get('row_span',0) for c in row['cells']] for row in y['rows']]
        if x['kind']=='image':
            left=Image.open(io.BytesIO(base64.b64decode(x['image']['data']))).convert('RGB')
            right=Image.open(io.BytesIO(base64.b64decode(y['image']['data']))).convert('RGB')
            assert left.size==right.size and left.tobytes()==right.tobytes()


def paragraphs(blocks):
    for block in blocks:
        if block['kind'] in ('paragraph','heading'):
            yield ''.join(r.get('text','') for r in block.get('runs',[]))
        for item in block.get('items',[]):yield from paragraphs(item['blocks'])
        for row in block.get('rows',[]):
            for cell in row['cells']:yield from paragraphs(cell['blocks'])


def independent_note_text(reader,page_index):
    page=reader.pages[page_index]
    page_ids={p.indirect_reference.idnum:i for i,p in enumerate(reader.pages)}
    members=set()
    def walk(value,parent_page=None,inside=False):
        if isinstance(value,list):
            for child in value:walk(child,parent_page,inside)
            return
        if isinstance(value,int):
            if inside and parent_page is not None:members.add((parent_page,value))
            return
        if hasattr(value,'get_object'):value=value.get_object()
        if not isinstance(value,dict):return
        inside=inside or value.get('/S') in ('/Note','/FENote','/Footnote','/Endnote')
        pg=value.raw_get('/Pg') if '/Pg' in value else None
        if pg is not None:parent_page=page_ids[pg.idnum]
        if inside and '/MCID' in value and parent_page is not None:members.add((parent_page,value['/MCID']))
        walk(value.get('/K'),parent_page,inside)
    walk(reader.trailer['/Root']['/StructTreeRoot'].get('/K'))
    stack=[];parts=[];baselines=[];artifacts=[]
    def operator(op,args,cm,tm):
        if op in (b'BMC',b'BDC'):
            props=args[1].get_object() if op==b'BDC' and isinstance(args[1],dict) else {}
            stack.append((str(args[0]),props.get('/MCID')))
        elif op==b'EMC':stack.pop()
    def visitor(text,cm,tm,font,size):
        if text=='':return
        if any(role=='/Artifact' for role,mcid in stack):artifacts.append(text);return
        if any((page_index,mcid) in members for role,mcid in stack if mcid is not None):
            parts.append(text)
            y=cm[1]*tm[4]+cm[3]*tm[5]+cm[5]
            baselines.append(float(page.cropbox.top)-y)
    page.extract_text(visitor_operand_before=operator,visitor_text=visitor)
    return ''.join(parts),baselines,artifacts


def verify_outputs(root,model,name,flags=()):
    sf=root/'model.sf'
    source=root/'source.pdf'
    command('extract',*flags,'-format','sf','-o',sf,source)
    word=root/'model.docx';command('convert','-allow-lossy','-o',word,sf)
    reread=extract(word,root/'docx.json');check_semantics(model,reread)
    command('extract','-format','sf','-o',root/'docx.sf',word)
    part='word/'+('footnotes.xml' if model['notes'][0]['kind']=='footnote' else 'endnotes.xml')
    with zipfile.ZipFile(word) as z:
        tree=ET.fromstring(z.read(part));definitions=[x for x in tree if x.get('{'+W+'}type','normal')=='normal']
        assert len(definitions)==1
        text=''.join(t.text or '' for t in definitions[0].iter('{'+W+'}t'))
        for paragraph in paragraphs(model['notes'][0]['blocks']):
            if paragraph:assert paragraph in text
    maps=[]
    allruns=list(runs(model['blocks']))+list(runs(model['notes'][0]['blocks']))
    for family in sorted({r['font_family'] for r in allruns if r.get('font_family')}):maps += ['-pdf-font-map',family+'=Go Mono']
    native=root/'native.pdf';command('convert','-allow-lossy',*maps,'-o',native,sf)
    recovered=extract(native,root/'native.json');check_semantics(model,recovered)
    command('extract','-format','sf','-o',root/'native.sf',native)
    for path,kind in [(native,'native')]:
        reader=PdfReader(path,strict=True)
        independent=[independent_note_text(reader,i) for i in range(len(reader.pages))]
        for page,item in zip(reader.pages,independent):
            assert page.extract_text().count('Footnotes continued')==''.join(item[2]).count('Footnotes continued'), (path,'generated caption lacks Artifact membership')
        text=' '.join(item[0] for item in independent)
        normalized=lambda s:' '.join(s.split())
        for paragraph in paragraphs(model['notes'][0]['blocks']):
            if paragraph:assert normalized(paragraph) in normalized(text),(path,'independent note MCIDs omit paragraph')
        with pymupdf.open(path) as pdf:
            retained=[]
            for i,page in enumerate(pdf):
                ys=independent[i][1]
                for block in page.get_text('rawdict')['blocks']:
                    if 'lines' not in block:continue
                    for line in block['lines']:
                        chars=[c['c'] for span in line['spans'] for c in span['chars'] if any(abs(c['origin'][1]-y)<.03 for y in ys)]
                        if chars:retained.append(''.join(chars))
            text=' '.join(retained)
            for paragraph in paragraphs(model['notes'][0]['blocks']):
                if paragraph:assert normalized(paragraph) in normalized(text),(path,'independent positioned note glyphs omit paragraph')
            links=[link for page in pdf for link in page.get_links()]
            assert any(link.get('page',-1)>=0 for link in links),'actual note navigation missing'
            for i,page in enumerate(pdf):page.get_pixmap().save(root/(kind+'-page-'+str(i+1)+'.png'))
    return {'native_pages':len(PdfReader(native).pages),'word_checked':bool(args.word_render)}


report={}
for name,count in [('mixed',2),('new-paragraph',3),('wide-new-paragraph',3),('endnote',2),('rich-page',5)]:
    root=args.root/name
    model=json.loads((root/'model.json').read_text());note=model['notes'][0]
    assert len(note['blocks'])==count and len(model['pages'])==3 and len(note['source']['regions']) in (2,3)
    assert note['kind_inferred'] and note['source']['structure']=='inferred' and 'cross_page_note_zone' in note['source']['evidence']
    source=root/'source.pdf';original=source.read_bytes();reader=PdfReader(source,strict=True)
    assert len(reader.pages)==3 and '/StructTreeRoot' not in reader.trailer['/Root']
    evidence=[(r.get('text',''),r.get('source')) for r in runs(model['blocks']) if not r.get('note')]
    evidence += [(r.get('text',''),r.get('source')) for r in runs(note['blocks'])]
    evidence += [(note['label'],note['label_source'])]+[(note['label'],r['source']) for r in runs(model['blocks']) if r.get('note')]
    with pymupdf.open(source) as pdf:
        chars=[(i,c['c'],c['origin']) for i,p in enumerate(pdf) for b in p.get_text('rawdict')['blocks'] if 'lines' in b for line in b['lines'] for s in line['spans'] for c in s['chars']]
        seen=[]
        for text,s in evidence:
            if not text or not s or s.get('precision')=='synthetic_separator':continue
            start,end=s['baseline'];step=(end['x']-start['x'])/len(text)
            for i,c in enumerate(text):
                expected=(start['x']+i*step,start['y'])
                found=[j for j,(page,char,origin) in enumerate(chars) if page==s['page']-1 and char==c and abs(origin[0]-expected[0])<.025 and abs(origin[1]-expected[1])<.025]
                assert len(found)==1,(name,c,expected,found)
                seen.append(found[0])
        assert len(seen)==len(set(seen))==len(chars),(name,'physical glyph omission/duplication')
    report[name]={'glyphs':len(seen),'body_blocks':count,'physical_pages':3,**verify_outputs(root,model,name)}
    assert source.read_bytes()==original

if args.probe:
    root=args.root/'authored-long';root.mkdir(exist_ok=True,parents=True)
    source=args.probe/'continuation-source.pdf'
    (root/'source.pdf').write_bytes(source.read_bytes())
    model=extract(source,root/'model.json',('-pdf-skip-structure',))
    checked=subprocess.run([sys.executable,'scripts/qa/pdf/generate_pdf_note_continuation_probe.py','--cross-page','--check',str(root/'model.json')],capture_output=True,text=True)
    assert checked.returncode==0,checked.stdout+checked.stderr
    note=model['notes'][0]
    assert len(note['blocks'])==2 and len(note['blocks'][1]['source']['regions'])==3
    assert len(model['pages'])==3 and len(model['blocks'])==1
    expected=json.loads((args.probe/'expected.json').read_text())
    normalized=lambda s:' '.join(s.split())
    for method in ['pypdf','mupdf']:
        if method=='pypdf':text=' '.join(p.extract_text() for p in PdfReader(source,strict=True).pages)
        else:
            with pymupdf.open(source) as pdf:text=' '.join(p.get_text() for p in pdf)
        assert normalized(expected['body_paragraphs'][1]) in normalized(text)
    report['authored-long']={'source_sha256':hashlib.sha256(source.read_bytes()).hexdigest(),'paragraphs':2,'physical_pages':3,'strict_original_oracle':'passed unchanged',**verify_outputs(root,model,'authored-long',('-pdf-skip-structure',))}
(args.root/'report.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report,indent=2))
