"""Compare PDF→SF views/actions/regions against original PDF objects in pypdf."""
import argparse
import base64
from collections import Counter
import json
from pathlib import Path
from pypdf import PdfReader
from pypdf.generic import NameObject, TextStringObject, ByteStringObject, ArrayObject, DictionaryObject, NullObject

def raw(s):
    return bytes(s) if isinstance(s,ByteStringObject) else s.original_bytes

def verify(pdf,sf):
    r=PdfReader(pdf,strict=True);s=json.loads(Path(sf).read_text());nav=s['attrs']['pdfNavigation'];pages=s['attrs']['pages']
    page_refs={(p.indirect_reference.idnum,p.indirect_reference.generation):i+1 for i,p in enumerate(r.pages)}
    def refpage(o):return page_refs[(o.idnum,o.generation)]
    def deref(o):return o.get_object() if hasattr(o,"get_object") else o
    def view(o):
        o=deref(o)
        if isinstance(o,DictionaryObject):o=deref(o['/D'])
        return (refpage(o[0]),str(o[1])[1:],tuple(None if x is None or isinstance(deref(x),NullObject) else round(float(deref(x)),8) for x in o[2:]))
    named={}
    legacy=deref(r.trailer['/Root'].get('/Dests',{}))
    for key,v in legacy.items():named[('name',str(key)[1:].encode('utf8'))]=view(v)
    def tree(o):
        n=deref(o)
        pairs=deref(n.get('/Names',[]))
        for i in range(0,len(pairs),2):
            k=('string',raw(deref(pairs[i])));assert k not in named;named[k]=view(pairs[i+1])
        for child in deref(n.get('/Kids',[])):tree(child)
    names=deref(r.trailer['/Root'].get('/Names',{}))
    if '/Dests' in names:tree(names['/Dests'])
    def target(o):
        v=deref(o)
        if isinstance(v,NameObject):return ('named','name',str(v)[1:].encode('utf8'))
        if isinstance(v,(TextStringObject,ByteStringObject)):return ('named','string',raw(v))
        return ('array',view(v))
    expected=Counter();anonymous=set();missing=set()
    def transform(page,x,y):
        a,b,c,d,e,f=pages[page-1]['transform'];return (round(a*x+c*y+e,8),round(b*x+d*y+f,8))
    for i,p in enumerate(r.pages,1):
        for ref in p.get('/Annots',[]):
            a=deref(ref)
            if a.get('/Subtype')!='/Link':continue
            action=deref(a.get('/A',{}));kind=str(action.get('/S',''))
            if kind=='/URI':key=('URI',str(action['/URI']))
            elif kind=='/GoTo' or '/Dest' in a:
                key=('GoTo',target(action['/D'] if kind=='/GoTo' else a['/Dest']))
                t=key[1]
                if t[0]=='array':anonymous.add(t[1])
                elif (t[1],t[2]) not in named:missing.add((t[1],t[2]))
            else:continue
            if a.get('/QuadPoints'):
                q=a['/QuadPoints'];regions=tuple(tuple(transform(i,float(q[j+k]),float(q[j+k+1])) for k in range(0,8,2)) for j in range(0,len(q),8))
            else:
                x0,y0,x1,y1=map(float,a['/Rect']);regions=((transform(i,x0,y0),transform(i,x1,y0),transform(i,x1,y1),transform(i,x0,y1)),)
            expected[(i,key,regions)]+=1
    actual_named={};actual_anonymous=set();actual_missing=set();ids={};definitions=Counter();owners={}
    def anchors(n):
        definitions.update(n.get('attrs',{}).get('anchors',[]))
        children=n.get('content',[])
        candidates=[m['attrs'] for child in children if child.get('text') for m in child.get('marks',[]) if m['type']=='source' and m.get('attrs',{}).get('quad')]
        source=candidates[0] if candidates else n.get('attrs',{}).get('source',{})
        for name in n.get('attrs',{}).get('anchors',[]):owners[name]=(source.get('order',0),source.get('page',0))
        for index,child in enumerate(children):
            if child['type']=='anchor':
                following=[m['attrs'] for x in children[index+1:] if x.get('text') for m in x.get('marks',[]) if m['type']=='source']
                if following:owners[child['attrs']['name']]=(following[0].get('order',0),following[0].get('page',0))
        if n['type']=='anchor':definitions[n['attrs']['name']]+=1
        for child in n.get('content',[]):anchors(child)
    anchors(s)
    for v in nav['destinations']:
        name_kind=v.get('name_kind','');name_bytes=base64.b64decode(v.get('name_bytes',''))
        if name_kind:
            k=(name_kind,name_bytes);identity=('named',*k)
            if v.get('unresolved'):
                actual_missing.add(k);assert v['id'] in s['attrs']['unresolvedDestinations'] and not definitions[v['id']]
            else:actual_named[k]=(v['page'],v['view'],tuple(None if x is None else round(float(x),8) for x in v.get('parameters',[])))
        else:
            k=(v['page'],v['view'],tuple(None if x is None else round(float(x),8) for x in v.get('parameters',[])));identity=('array',k);actual_anonymous.add(k)
        assert v['id'] not in ids;ids[v['id']]=identity
        if not v.get('unresolved'):
            assert definitions[v['id']]==1 and v['flow_binding'] in ('nearest_content','page_boundary')
            if v['flow_binding']=='nearest_content':assert owners[v['id']]==(v.get('flow_order',0),v['page']),('flow anchor detached from selected source',v['id'],owners.get(v['id']),v)
            p=v.get('parameters',[])
            if v['view']=='XYZ' and p[0] is not None and p[1] is not None:
                assert (round(v['point']['x'],8),round(v['point']['y'],8))==transform(v['page'],*p[:2])
            else:assert 'point' not in v
    assert actual_named==named, ('named views',actual_named.keys()^named.keys())
    assert actual_anonymous==anonymous and actual_missing==missing
    actual=Counter()
    for l in nav['links']:
        key=('URI',l.get('uri','')) if l['kind']=='URI' else ('GoTo',ids[l['destination']])
        regions=tuple(tuple((round(p['x'],8),round(p['y'],8)) for p in q) for q in l['regions'])
        actual[(l['page'],key,regions)]+=1
    assert actual==expected,('annotation target/geometry mismatch',actual-expected,expected-actual)
    report=dict(pdf=str(pdf),sf=str(sf),pages=len(r.pages),named_views=len(named),anonymous_views=len(anonymous),missing_names=len(missing),annotation_links=sum(expected.values()),actions=dict(Counter(l['kind'] for l in nav['links'])),physical_views='exact raw namespace/name bytes, page, view and nullable parameters',regions='exact transformed annotation corners',flow='every resolved view has one separately identified inferred anchor; no authorial ownership claim')
    print(json.dumps(report,indent=2));return report

p=argparse.ArgumentParser();p.add_argument('pairs',nargs='+',help='PDF SF pairs');p.add_argument('--report',type=Path);a=p.parse_args();assert len(a.pairs)%2==0
reports=[verify(a.pairs[i],a.pairs[i+1]) for i in range(0,len(a.pairs),2)]
if a.report:a.report.write_text(json.dumps(reports,indent=2)+'\n')
