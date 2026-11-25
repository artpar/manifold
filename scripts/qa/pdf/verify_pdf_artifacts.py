"""Audit artifact properties in original PDF operators against recovered SF."""
import argparse
import gzip
import json
import hashlib
from pathlib import Path
from pypdf import PdfReader
from pypdf.generic import ContentStream, DictionaryObject

p=argparse.ArgumentParser();p.add_argument('pdf',type=Path);p.add_argument('sf',type=Path);p.add_argument('--report',type=Path);p.add_argument('--projected-sha256',help='require the prior normalized SF hash after removing only additive artifacts keys');a=p.parse_args()
with (gzip.open(a.sf,'rt') if str(a.sf).endswith('.gz') else open(a.sf)) as f:s=json.load(f)
r=PdfReader(a.pdf,strict=True);expected=[]
def obj(o):return o.get_object() if hasattr(o,'get_object') else o
for pi,page in enumerate(r.pages,1):
    records=[]
    def content(stream,resources,scope,depth=0):
        assert depth<64
        data=ContentStream(stream,r)
        for args,op in data.operations:
            if op in (b'BMC',b'BDC') and str(args[0])=='/Artifact':
                props={}
                if op==b'BDC':
                    props=obj(args[1])
                    if not isinstance(props,DictionaryObject):props=obj(obj(resources.get('/Properties',{}))[str(props)])
                v=dict(id=f'page:{pi}:artifact:{len(records)+1}',stream=scope,type=str(props.get('/Type',''))[1:],subtype=str(props.get('/Subtype',''))[1:])
                if '/BBox' in props:v['bbox']=list(map(float,obj(props['/BBox'])))
                if '/Attached' in props:v['attached']=[str(e)[1:] for e in obj(props['/Attached'])]
                records.append(v)
            elif op==b'Do':
                ref=obj(resources.get('/XObject',{})).get(str(args[0]));x=obj(ref)
                if x.get('/Subtype')=='/Form':
                    subresources=obj(x.get('/Resources',resources));name=f'({ref.idnum} {ref.generation} R)'
                    content(x,subresources,name,depth+1)
    if page.get('/Contents'):content(page['/Contents'],obj(page['/Resources']),f'page:{pi}')
    # Current corpus has no artifact appearance streams; fail rather than silently
    # overlook a new source case that requires the annotation appearance traversal.
    for ref in page.get('/Annots',[]):
        app=obj(obj(ref).get('/AP',{})).get('/N')
        if app is not None:
            value=obj(app)
            if hasattr(value,'get_data'):
                assert b'/Artifact' not in value.get_data(),'extend independent appearance traversal for this source'
    got=s['attrs']['pages'][pi-1].get('artifacts',[])
    assert len(got)==len(records),(pi,len(got),len(records))
    transform=s['attrs']['pages'][pi-1]['transform']
    def point(x,y):
        aa,b,c,d,e,f=transform;return {'x':aa*x+c*y+e,'y':b*x+d*y+f}
    for source,recovered in zip(records,got):
        for key in ('id','stream','type','subtype','bbox','attached'):
            assert recovered.get(key,[] if key=='attached' else '')==source.get(key,[] if key=='attached' else ''),(pi,key,source,recovered)
        assert recovered['matrix']==transform
        if 'bbox' in source:
            x0,y0,x1,y1=source['bbox'];assert recovered['bounds']==[point(x0,y0),point(x1,y0),point(x1,y1),point(x0,y1)]
        else:assert 'bounds' not in recovered
    expected.extend((pi,v) for v in records)
known={v['id']:page for page,v in expected};members={k:0 for k in known}
def walk(value):
    if isinstance(value,dict):
        # Source node attrs and source marks both have explicit source format.
        if value.get('format')=='pdf':
            for ref in value.get('artifacts',[]):assert ref in known and value.get('page',0) in (0,known[ref]);members[ref]+=1
        for v in value.values():walk(v)
    elif isinstance(value,list):
        for v in value:walk(v)
walk(s)
report=dict(pdf=str(a.pdf),sf=str(a.sf),pages=len(r.pages),artifacts=len(expected),types={name:sum(v.get('subtype')==name for _,v in expected) for name in sorted({v.get('subtype','') for _,v in expected})},memberships=sum(members.values()),unpainted_or_empty_groups=sum(n==0 for n in members.values()),verification='exact original Type/Subtype/BBox/Attached, stream/page IDs, default-space transform and valid source memberships; no inferred reusable template claim')
if a.projected_sha256:
    def project(value):
        if isinstance(value,dict):
            value.pop('artifacts',None)
            for child in value.values():project(child)
        elif isinstance(value,list):
            for child in value:project(child)
    project(s);digest=hashlib.sha256()
    for chunk in json.JSONEncoder(sort_keys=True,separators=(',',':'),ensure_ascii=False,allow_nan=False).iterencode(s):digest.update(chunk.encode())
    assert digest.hexdigest()==a.projected_sha256,digest.hexdigest()
    report['preexisting_values_sha256']=digest.hexdigest()
if a.report:a.report.write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report,indent=2))
