"""Compare numbered SF headings with the unchanged original publisher PDF.

This oracle uses published page text, not the native renderer or newly generated
Word/PDF output. Whitespace is normalized; numbering and wording remain exact.
"""
import argparse
import hashlib
import json
from pathlib import Path
import pymupdf

parser=argparse.ArgumentParser()
parser.add_argument('--native',action='store_true')
args=parser.parse_args()
out=Path('output/numbering')
sf=json.loads((out/'mqtt.sf').read_text())
original=Path('testdata/real/mqtt-v5.0-os.pdf')
assert hashlib.sha256(original.read_bytes()).hexdigest()=='e8f8e9d2467618d5c5a6398bdf971cb90cdbe2c8e33242e028f442bee8e5de20'
pdf=pymupdf.open(original)
pages=[' '+ ' '.join(p.get_text().split())+' ' for p in pdf]
def number(fmt,n):
 if n==0:return '0'
 if fmt=='decimal':return str(n)
 if fmt=='decimalZero':return str(n).zfill(2)
 if fmt in ('upperLetter','lowerLetter'):
  return chr(ord('A' if fmt=='upperLetter' else 'a')+(n-1)%26)*((n-1)//26+1)
 if fmt in ('upperRoman','lowerRoman'):
  result=''
  for value,letters in ((1000,'M'),(900,'CM'),(500,'D'),(400,'CD'),(100,'C'),(90,'XC'),(50,'L'),(40,'XL'),(10,'X'),(9,'IX'),(5,'V'),(4,'IV'),(1,'I')):
   count,n=divmod(n,value);result+=letters*count
  return result.lower() if fmt=='lowerRoman' else result
 raise AssertionError(fmt)
def label(node,index):
 a=node['attrs'];n=a.get('numbering');value=a.get('order',1)+index
 if not n:return str(value)+'.'
 result='';template=n['template'];i=0
 while i<len(template):
  if template[i]=='%' and i+1<len(template) and template[i+1] in '123456789':
   level=int(template[i+1])-1;i+=2
   if level>n['level']:continue
   v=value if level==n['level'] else n['ancestors'][level]
   result+=number('decimal' if n.get('legal') else n['formats'][level],v)
  else:result+=template[i];i+=1
 return result
evidence=[]
def walk(node):
 if node['type']=='ordered_list':
  for index,item in enumerate(node['content']):
   first=item['content'][0]
   if first['type']!='heading':continue
   title=''.join(c.get('text','') for c in first.get('content',[]))
   text=' '.join((label(node,index)+' '+title).split())
   found=[i+1 for i,page in enumerate(pages) if ' '+text+' ' in page]
   variant=None
   if not found and label(node,index)=='4.8.1' and title=='Non‑shared Subscriptions':
    variant='4.8.1 Non-shared Subscriptions'
    found=[i+1 for i,page in enumerate(pages) if ' '+variant+' ' in page]
   evidence.append(dict(label=label(node,index),title=title,heading_level=first['attrs']['level'],pages=found,publisher_spelling=variant))
 for c in node.get('content',[]):walk(c)
walk(sf)
missing=[e for e in evidence if not e['pages']]
report=dict(numbered_headings=len(evidence),publisher_pages=len(pdf),matched=len(evidence)-len(missing),missing=missing,evidence=evidence,
 scope='label plus complete heading text present in unchanged original publisher PDF; not an all-paragraph/layout oracle')
report['exact_original_heading_text_matches']=sum(bool(e['pages']) and e['publisher_spelling'] is None for e in evidence)
report['publisher_title_spelling_differences']=sum(e['publisher_spelling'] is not None for e in evidence)
if args.native:
 native=pymupdf.open(out/'mqtt.pdf')
 native_pages=[' '+ ' '.join(p.get_text().split())+' ' for p in native]
 native_missing=[e for e in evidence if not any(' '+' '.join((e['label']+' '+e['title']).split())+' ' in page for page in native_pages)]
 report['native_pages']=len(native)
 report['native_heading_matches']=len(evidence)-len(native_missing)
 report['native_missing']=native_missing
 assert not native_missing,repr(native_missing)
 # Cached TOC text could satisfy a plain-text search. Require an actually
 # painted Go-family marker at the heading size and a title on its baseline.
 native_spans=[]
 for page in native:
  native_spans.append([s for b in page.get_text('dict')['blocks'] for line in b.get('lines',[]) for s in line['spans']])
 painted=[]
 for e in evidence:
  found=[]
  prefix=' '.join(e['title'].split()[:3])
  for pi,spans in enumerate(native_spans):
   for marker in spans:
    if marker['font']!='GoRegular' or marker['text'].strip()!=e['label'] or abs(marker['size']-(26-2*e['heading_level']))>.01:continue
    near=sorted([v for v in spans if abs(v['origin'][1]-marker['origin'][1])<.2 and v['bbox'][0]>=marker['bbox'][2]+5.8],key=lambda v:v['bbox'][0])
    text=' '.join(''.join(v['text'] for v in near).split())
    if text.startswith(prefix):found.append(dict(page=pi+1,marker_bounds=marker['bbox'],baseline=marker['origin'][1]))
  assert found,('missing painted body heading marker/title',e)
  painted.append(dict(label=e['label'],title=e['title'],painted=found))
 report['native_painted_body_headings']=len(painted)
 report['native_body_heading_evidence']=painted
(out/'source-numbering.json').write_text(json.dumps(report,indent=2,ensure_ascii=False)+'\n')
print(json.dumps({k:v for k,v in report.items() if k not in ('evidence','native_body_heading_evidence')},indent=2,ensure_ascii=False))
assert evidence and not missing,'original numbering/content discrepancy needs investigation'
