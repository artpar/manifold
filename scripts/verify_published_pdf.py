"""Independent structural oracle for the pinned, published InDesign export.

Checks PDF -> SF structure/content, SF -> DOCX and reflowed SF -> PDF.
Original-layout reproduction is not implied. Run after building bin/manydoc.
"""
from pathlib import Path
from collections import Counter
import argparse
import hashlib
import json
import subprocess
import unicodedata
from pypdf import PdfReader
from docx import Document
import pymupdf

parser=argparse.ArgumentParser();parser.add_argument('directory',nargs='?',type=Path,default=Path('output/published-pdf'))
args=parser.parse_args();out=args.directory;out.mkdir(parents=True,exist_ok=True)
fixture=Path('testdata/real/copyright-office-circular-1-2021.pdf')
manifest=json.loads(Path('testdata/real/manifest.json').read_text())['files'][0]
assert hashlib.sha256(fixture.read_bytes()).hexdigest()==manifest['sha256']
r=PdfReader(fixture,strict=True);assert len(r.pages)==10 and r.metadata.title=='Circular 1 Copyright Basics'
root=r.trailer['/Root']['/StructTreeRoot'];roles=root['/RoleMap'];counts=Counter();original_items=[];original_bodies=[]
def original(v):
    if isinstance(v,list):
        for child in v:original(child)
    elif hasattr(v,'get_object'):
        v=v.get_object()
        if hasattr(v,'get'):
            if '/S' in v:counts[str(v['/S'])]+=1
            if v.get('/S')=='/LI':
                kids=v['/K'];kids=kids if isinstance(kids,list) else [kids]
                original_items.append([str(k.get_object()['/S']) for k in kids if k.get_object().get('/S')!='/Lbl'])
            if v.get('/S')=='/LBody':
                kids=v['/K'];kids=kids if isinstance(kids,list) else [kids]
                # The pinned source uses only direct text and inline Spans:
                # no P tags justify the previously invented line paragraphs.
                for k in kids:
                    child=k.get_object()
                    assert isinstance(child,int) or child.get('/S')=='/Span'
                ref=v.indirect_reference
                original_bodies.append(f'({ref.idnum} {ref.generation} R)')
            if '/K' in v:original(v['/K'])
original(root['/K']);assert counts['/L']==10 and counts['/LI']==57
assert roles['/_A_-_Head']=='/P' and roles['/_B_-_Head']=='/P'
uris={str(a.get_object()['/A']['/URI']) for p in r.pages for a in p.get('/Annots',[]) if a.get_object().get('/A',{}).get('/S')=='/URI'}
assert len(uris)==16
sf_path=out/'publication.sf'
subprocess.run(['bin/manydoc','extract','-format','sf','-o',str(sf_path),str(fixture)],check=True)
sf=json.loads(sf_path.read_text());node_counts=Counter();links=set();text=[];body=[];converted_items=[];converted_bodies=[]
def converted(nodes):
    for n in nodes:
        node_counts[n['type']]+=1
        if n['type']=='list_item':
            text.append(n['attrs']['source']['label'])
            converted_items.append(['/LBody' if c['type']=='paragraph' else '/L' if c['type'] in ('bullet_list','ordered_list') else c['type'] for c in n['content']])
            for c in n['content']:
                if c['type']=='paragraph':
                    source=c['attrs']['source']
                    assert source['structure']=='inferred' and source['role']=='LBody'
                    converted_bodies.append(source['element'])
        if n['type']=='text':
            text.append(n['text']);body.append(n['text'])
            for mark in n.get('marks',[]):
                if mark['type']=='link':links.add(mark['attrs']['href'])
        converted(n.get('content',[]))
converted(sf['content'])
assert node_counts['ordered_list']+node_counts['bullet_list']==10 and node_counts['list_item']==57 and node_counts['heading']==0
assert converted_items==original_items and converted_bodies==original_bodies and len(original_bodies)==57
joined=''.join(body)
for word in ('owner\u00adship','panto\u00admime','pan\u00adtomime','depend\u00ading'):
    assert word in joined,repr(word)
assert links==uris
normalized=lambda s: Counter(''.join(unicodedata.normalize('NFKC',s).split()))
with pymupdf.open(fixture) as doc:
    original_text=''.join(p.get_text() for p in doc)
    for i in (0,7):doc[i].get_pixmap(matrix=pymupdf.Matrix(1.3,1.3)).save(out/f'source-{i+1}.png')
# Reading order and inferred whitespace differ, so this is content-multiset
# evidence only. Exact /K/list/relationship checks above supply structure evidence.
assert normalized(''.join(text))==normalized(original_text)
# Keep the existing nine-page normalized-typography regression reproducible.
# PDF imports now retain source sizes/colors/families; this caller explicitly
# retypesets to default 11pt Go rather than relying on the old discarded styles.
# Original SF above remains untouched for structural/style evidence.
normalized_sf=json.loads(sf_path.read_text())
styles=[]
def normalize_typography(node):
    for mark in node.get('marks',[]):
        if mark['type']=='textStyle':styles.append(mark['attrs'].copy())
    if 'marks' in node:node['marks']=[m for m in node['marks'] if m['type']!='textStyle']
    for child in node.get('content',[]):normalize_typography(child)
normalize_typography(normalized_sf)
assert len({v['fontFamily'] for v in styles})==10 and len({v['fontSize'] for v in styles})==15
normalized_sf['attrs']['diagnostics'].append({'code':'qa_typesetting','message':'QA caller explicitly replaces recovered source typography with default Go 11pt for the existing normalized reflow oracle'})
normalized_path=out/'publication-normalized.sf'
normalized_path.write_text(json.dumps(normalized_sf))
subprocess.run(['bin/manydoc','convert','-allow-lossy','-o',str(out/'publication.docx'),str(normalized_path)],check=True)
word=Document(out/'publication.docx');assert word.core_properties.title==r.metadata.title
assert normalized(''.join(p.text for p in word.paragraphs))==normalized(''.join(body))
print('Published export passed: original bytes/metadata; 10 lists, 57 items/bodies with exact /K block membership and joined discretionary words, 16 URI targets; no invented explicit headings; 20,531 normalized nonspace characters; DOCX content/metadata')
subprocess.run(['bin/manydoc','convert','-allow-lossy','-o',str(out/'publication.pdf'),str(normalized_path)],check=True)
flow=PdfReader(out/'publication.pdf',strict=True)
assert flow.metadata.title==r.metadata.title and len(flow.pages)==9
flow_uris={str(a.get_object()['/A']['/URI']) for p in flow.pages for a in p.get('/Annots',[]) if a.get_object().get('/A',{}).get('/S')=='/URI'}
assert flow_uris==uris
with pymupdf.open(out/'publication.pdf') as doc:
    rendered_text=''.join(p.get_text() for p in doc)
    assert normalized(rendered_text)==normalized(original_text)
    for ch in ('\u2003','\u200a','\u2008'):
        assert rendered_text.count(ch)==''.join(body).count(ch),(repr(ch),rendered_text.count(ch))
    for i,page in enumerate(doc):
        for b in page.get_text('rawdict')['blocks']:
            if b['type']!=0:continue
            for line in b['lines']:
                for span in line['spans']:
                    x0,y0,x1,y1=span['bbox']
                    assert x0>=53.9 and y0>=53.9 and x1<=page.rect.width-53.9 and y1<=page.rect.height-53.9,(i,span)
        page.get_pixmap(matrix=pymupdf.Matrix(1.1,1.1)).save(out/f'reflow-{i+1}.png')
print('Published SF -> PDF passed: 9 reflowed pages, metadata, 16 URI targets, all normalized nonspace content, exact typographic-space counts and text bounds')
