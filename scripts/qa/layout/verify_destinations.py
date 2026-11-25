"""Independent named-target checks; production needs none of these QA tools."""
import argparse
import json
import zipfile
from collections import Counter
from pathlib import Path
from urllib.parse import quote
import xml.etree.ElementTree as ET
from pypdf import PdfReader
import pymupdf

parser=argparse.ArgumentParser()
parser.add_argument('--office-odt',type=Path)
args=parser.parse_args()
out=Path('output/destinations')
native=PdfReader(out/'destinations.pdf',strict=True)
baseline=PdfReader(out/'without-points.pdf',strict=True)
assert len(native.pages)==len(baseline.pages)==2
for a,b in zip(native.pages,baseline.pages):
    assert a.get_contents().get_data()==b.get_contents().get_data(), 'navigation changed paint'
expected={'header':0,'footer':0,'start':0,'ligature':0,'table':0,'cell':0,
          'empty-cell':0,'empty-alias':0,'between':0,'between-alias':0,'list':0,
          'item':0,'note':0,'end':1,'章 space–%':1,'tail':1,'tail-alias':1}
assert set(native.named_destinations)==set(expected)
for name,page in expected.items():
    d=native.named_destinations[name]
    assert native.get_destination_page_number(d)==page
    assert d.typ=='/XYZ' and float(d.left)==0 and 0<=float(d.top)<=400
    assert d.zoom is None or str(d.zoom)=='NullObject'
for names in [('empty-cell','empty-alias'),('between','between-alias'),('tail','tail-alias')]:
    assert len({float(native.named_destinations[n].top) for n in names})==1
assert float(native.named_destinations['start'].top)==360
assert float(native.named_destinations['end'].top)==360
assert float(native.named_destinations['ligature'].top)==340.2
assert 'missing' not in native.named_destinations
f=pymupdf.open(out/'destinations.pdf')
for name in ('start','end','header','footer','cell','note','table','章 space–%'):
    page,x,y=f.resolve_link('#nameddest='+quote(name))
    d=native.named_destinations[name]
    assert page==expected[name] and abs(y-(400-float(d.top)))<.001,(name,page,y,d)
for i,p in enumerate(f):p.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/f'page-{i+1}.png')
uris=set();targets=Counter()
for p in native.pages:
    for ref in p.get('/Annots',[]):
        a=ref.get_object().get('/A',{})
        if a.get('/S')=='/URI':uris.add(str(a['/URI']))
        if a.get('/S')=='/GoTo':targets[str(a['/D'])]+=1
assert uris=={'#literal','urn:manydoc:internal:0:0'}
assert set(targets)=={'table','end','missing','note','cell','start'} and targets['missing']==1
w='{http://schemas.openxmlformats.org/wordprocessingml/2006/main}'
with zipfile.ZipFile(out/'destinations.docx') as z:
    starts=[];ends=[];public_names=set()
    for name in z.namelist():
        if name.startswith('word/') and name.endswith('.xml'):
            root=ET.fromstring(z.read(name))
            part_starts=[n.get(w+'id') for n in root.iter(w+'bookmarkStart')]
            part_ends=[n.get(w+'id') for n in root.iter(w+'bookmarkEnd')]
            assert len(part_starts)==len(set(part_starts)) and set(part_starts)==set(part_ends)
            starts.extend(n.get(w+'name') for n in root.iter(w+'bookmarkStart'))
            public_names.update(n.get(w+'name') for n in root.iter(w+'bookmarkStart') if n.get('{urn:manydoc:metadata:1}anchor'))
    assert len(starts)>=17
assert json.loads((out/'source.sf').read_text())==json.loads((out/'from-docx.sf').read_text())
office=None
if args.office_odt:
    t='{urn:oasis:names:tc:opendocument:xmlns:text:1.0}';x='{http://www.w3.org/1999/xlink}'
    imported_names=set();imported_links=[]
    with zipfile.ZipFile(args.office_odt) as z:
        for file in ('content.xml','styles.xml'):
            root=ET.fromstring(z.read(file))
            imported_names.update(n.get(t+'name') for n in root.iter() if n.tag in (t+'bookmark',t+'bookmark-start'))
            imported_links.extend((n.get(x+'href'),''.join(n.itertext())) for n in root.iter(t+'a'))
    assert public_names<=imported_names and len(public_names)==17
    assert Counter(imported_links)==Counter([('#end','FORWARD'),('#literal',' URI'),('urn:manydoc:internal:0:0',' COLLISION'),('#missing',' MISSING'),('#note','TABLE'),('#cell','LIST'),('#end','NOTE'),('#start','BACKWARD'),('#table','HEADER'),('#start','FOOTER')])
    office=dict(imported_names=17,imported_links=10,scope='LibreOffice ODT import preserves names/targets/text; its PDF export omitted most links, also on an independent minimal python-docx probe')
# The real-source oracle is independent XML; old fixtures/oracles stay unchanged.
with zipfile.ZipFile('testdata/real/mqtt-v5.0-os.docx') as z:
    root=ET.fromstring(z.read('word/document.xml'))
    names={n.get(w+'name') for n in root.iter(w+'bookmarkStart')}
    links={n.get(w+'anchor') for n in root.iter(w+'hyperlink')
           if n.get(w+'anchor') and not n.get('{http://schemas.openxmlformats.org/officeDocument/2006/relationships}id') and ''.join(n.itertext()).strip()}
r=PdfReader(out/'mqtt.pdf',strict=True);old=PdfReader('output/internal-destinations/mqtt.pdf',strict=True)
assert len(r.pages)==len(old.pages)==189
assert set(r.named_destinations)==names and len(names)==3106
counts=Counter();go=Counter();uris=set()
for index,(a,b) in enumerate(zip(r.pages,old.pages)):
    assert a.get_contents().get_data()==b.get_contents().get_data(),('page paint',index+1)
    aa=[n.get_object() for n in a.get('/Annots',[])];bb=[n.get_object() for n in b.get('/Annots',[])]
    assert len(aa)==len(bb)
    for n,o in zip(aa,bb):
        assert list(n['/Rect'])==list(o['/Rect'])
        action=n.get('/A',{});counts[str(action.get('/S'))]+=1
        if action.get('/S')=='/GoTo':go[str(action['/D'])]+=1
        if action.get('/S')=='/URI':
            assert action['/URI']==o['/A']['/URI'];uris.add(str(action['/URI']))
assert set(go)==links and len(links)==379
assert set(go)-names=={'_Introduction'} and go['_Introduction']==3
assert counts=={'/GoTo':1145,'/URI':81} and len(uris)==56
for name,d in r.named_destinations.items():
    page=r.get_destination_page_number(d)
    assert 0<=page<189 and d.typ=='/XYZ' and 0<=float(d.top)<=float(r.pages[page].mediabox.top)
report=dict(synthetic_pages=2,synthetic_names=17,synthetic_and_real_paint='exact content bytes; real annotation rectangles unchanged',
            office_import=office,real_pages=189,real_names=3106,real_link_ids=379,real_annotations=dict(counts),real_uri_targets=56,
            unresolved={'_Introduction':3},scope='SF/DOCX/native PDF navigation; PDF-to-SF view/annotation evidence is checked separately by verify_navigation_import.py')
(out/'independent.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report,indent=2))
