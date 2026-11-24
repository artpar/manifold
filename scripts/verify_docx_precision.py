"""Independent Word size projection, relationship/asset and SF intent audit.

Optional original guide inputs remain ignored and are never redistributed.
"""
from pathlib import Path
from zipfile import ZipFile
from collections import Counter
import argparse,base64,gzip,hashlib,json,math,posixpath
from lxml import etree
from docx import Document
import pymupdf

parser=argparse.ArgumentParser()
parser.add_argument('--guide-sf',type=Path)
parser.add_argument('--guide-docx',type=Path)
parser.add_argument('--guide-roundtrip',type=Path)
a=parser.parse_args()
root=Path('output/note-precision')
ns={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main','md':'urn:manydoc:metadata:1','r':'http://schemas.openxmlformats.org/officeDocument/2006/relationships','a':'http://schemas.openxmlformats.org/drawingml/2006/main'}

def load(path):
    with (gzip.open(path,'rt') if str(path).endswith('.gz') else open(path)) as f:return json.load(f)
def editable(v):
    if isinstance(v,list):return [editable(x) for x in v if not(isinstance(x,dict) and x.get('type')=='source')]
    if isinstance(v,dict):
        out={k:editable(x) for k,x in v.items() if k not in ('source','label_source')}
        return {k:x for k,x in out.items() if not(k in ('marks','attrs') and not x)}
    return v

def compare(x,y,path='',image_attrs=False):
    global image_quantizations
    assert type(x)==type(y),(path,'type')
    if isinstance(x,list):
        assert len(x)==len(y),(path,'length',len(x),len(y))
        for i,(left,right) in enumerate(zip(x,y)):compare(left,right,path+'/'+str(i))
    elif isinstance(x,dict):
        assert x.keys()==y.keys(),(path,'keys',x.keys()^y.keys())
        for k in x:
            if image_attrs and k in ('width','height') and x[k]!=y[k]:
                assert abs(x[k]-y[k])<=1/12700,(path,k,'exceeds one EMU')
                image_quantizations+=1
            else:compare(x[k],y[k],path+'/'+k,x.get('type')=='image' and k=='attrs')
    else:assert x==y,(path,repr(x)[:100],repr(y)[:100])

def objects(v):
    if isinstance(v,dict):
        yield v
        for x in v.values():yield from objects(x)
    elif isinstance(v,list):
        for x in v:yield from objects(x)

def verify(sf_path,docx_path,roundtrip_path):
    global image_quantizations
    before=load(sf_path);after=load(roundtrip_path);image_quantizations=0
    compare(editable(before['content']),editable(after['content']))
    for k in ('notes','headers','footers'):
        compare(editable(before['attrs'].get(k)),editable(after['attrs'].get(k)),k)
    assert any(d['code']=='docx_font_size_precision' for d in after['attrs']['diagnostics'])
    Document(docx_path)
    expected_assets=Counter(hashlib.sha256(base64.b64decode(n['attrs']['src'].split(',',1)[1])).hexdigest() for n in objects(before) if n.get('type')=='image')
    actual_assets=Counter();count=0
    with ZipFile(docx_path) as z:
        for part in z.namelist():
            if not part.startswith('word/') or not part.endswith('.xml'):continue
            tree=etree.fromstring(z.read(part))
            for run in tree.xpath('//w:r[@md:fontSize]',namespaces=ns):
                original=float(run.get('{'+ns['md']+'}fontSize'));h=math.floor(original*2+.5)
                assert math.isfinite(original) and original*2!=h and abs(h/2-original)<=.25
                for key in ('sz','szCs'):
                    props=run.xpath('w:rPr/w:'+key,namespaces=ns)
                    assert len(props)==1 and props[0].get('{'+ns['w']+'}val')==str(h)
                count+=1
            relpart=posixpath.join(posixpath.dirname(part),'_rels',posixpath.basename(part)+'.rels')
            rels={}
            if relpart in z.namelist():
                rels={rel.get('Id'):rel.get('Target') for rel in etree.fromstring(z.read(relpart))}
            for blip in tree.xpath('//a:blip',namespaces=ns):
                target=rels[blip.get('{'+ns['r']+'}embed')]
                target=posixpath.normpath(posixpath.join(posixpath.dirname(part),target))
                actual_assets[hashlib.sha256(z.read(target)).hexdigest()]+=1
    assert count>0 and actual_assets==expected_assets
    print(docx_path,':',count,'explicit standard half-point projections; exact editable structure/fonts/notes/links and',sum(actual_assets.values()),'relationship-resolved asset instances;',image_quantizations,'image dimensions differ within one EMU')

verify(root/'sectioned/notes.sf',root/'sectioned/notes.docx',root/'sectioned/notes-docx.sf')
word=pymupdf.open(root/'sectioned/lo/notes.pdf')
assert len(word)==2
labels=[]
for p,page in enumerate(word):
    for b in page.get_text('dict')['blocks']:
        for line in b.get('lines',[]):
            for span in line['spans']:
                if span['text'].strip() in ('N-1','§E'):
                    assert abs(span['size']-7)<.001
                    labels.append(span['text'].strip())
    page.get_pixmap(matrix=pymupdf.Matrix(1.4,1.4)).save(root/f'sectioned/word-{p+1}.png')
assert sorted(labels)==['N-1','§E']
print('Independent Word renderer: both original 6.959646pt body marks paint at the declared portable 7pt size')
if a.guide_sf:
    assert a.guide_docx and a.guide_roundtrip
    verify(a.guide_sf,a.guide_docx,a.guide_roundtrip)
