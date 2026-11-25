"""Independent forward-tagged-table audit of the original publisher PDF.

This records explicit memberships/default spans, not inferred merged geometry.
It never reads manydoc output or rewrites the source PDF.
"""
import argparse, hashlib
from pathlib import Path
import json
from collections import Counter
from pypdf import PdfReader
parser=argparse.ArgumentParser()
parser.add_argument('--source',type=Path,default=Path('testdata/real/mqtt-v5.0-os.pdf'))
parser.add_argument('--output',type=Path)
a=parser.parse_args()
r=PdfReader(a.source,strict=True)
root=r.trailer['/Root']['/StructTreeRoot'];roles=root.get('/RoleMap',{});classes=root.get('/ClassMap',{})
pgs={p.indirect_reference.idnum:i+1 for i,p in enumerate(r.pages)}
def obj(n):return n.get_object() if hasattr(n,'get_object') else n
def kids(n):
 k=obj(n.get('/K',[]));return k if isinstance(k,list) else [k]
def role(n):
 s=n.get('/S','');seen=set()
 while s in roles and s not in seen:seen.add(s);s=roles[s]
 return str(s)
def attrs(n):
 out={}
 def add(a):
  a=obj(a)
  if isinstance(a,list):
   for c in a:add(c)
  elif isinstance(a,dict):out.update({str(k):v for k,v in a.items()})
  elif a in classes:add(classes[a])
 add(n.get('/C',[]));add(n.get('/A',[]));return out
def rows(n):
 out=[]
 for k in kids(n):
  k=obj(k)
  if not isinstance(k,dict):continue
  if role(k)=='/TR':out.append(k)
  elif role(k) in ('/TBody','/THead','/TFoot'):out+=rows(k)
 return out
reports=[]
def walk(n):
 n=obj(n)
 if not isinstance(n,dict):return
 if role(n)=='/Table':
  rr=rows(n);grid=[[] for _ in rr];detail=[]
  for i,row in enumerate(rr):
   cc=[];col=0
   for raw in kids(row):
    c=obj(raw)
    if not isinstance(c,dict):continue
    at=attrs(c);cs=int(at.get('/ColSpan',1));rs=int(at.get('/RowSpan',1))
    while col<len(grid[i]) and grid[i][col]:col+=1
    for j in range(i,min(i+rs,len(rr))):
     grid[j]+=[False]*max(0,col+cs-len(grid[j]));grid[j][col:col+cs]=[True]*cs
    ref=c.raw_get('/Pg') if '/Pg'in c else None
    cc.append({'id':raw.idnum if hasattr(raw,'idnum') else None,'role':role(c),'col':col,'cs':cs,'rs':rs,'page':pgs.get(ref.idnum) if ref else None,'attrs':{k:str(v) for k,v in at.items()}});col+=cs
   detail.append(cc)
  widths=[len(g) for g in grid]
  reports.append({'id':n.indirect_reference.idnum if getattr(n,'indirect_reference',None) else None,'rows':len(rr),'widths':widths,'cells':detail})
 for k in kids(n):walk(k)
walk(root)
report={'source_sha256':hashlib.sha256(a.source.read_bytes()).hexdigest(),'source_bytes':a.source.stat().st_size,'pages':len(r.pages),'tables':reports,'summary':{'tables':len(reports),'rows':sum(t['rows'] for t in reports),'cells':sum(len(row) for t in reports for row in t['cells']),'ragged_unit_span_tables':sum(len(set(t['widths']))>1 for t in reports)}}
if a.output:a.output.write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps({k:v for k,v in report.items() if k!='tables'},indent=2))
