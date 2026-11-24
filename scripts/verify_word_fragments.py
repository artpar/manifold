"""Check Word fragment targets against source-derived facts and unchanged paint."""
import json
from pathlib import Path
from urllib.parse import quote
from pypdf import PdfReader

out=Path('output/internal-destinations')
oracle=json.loads(Path('testdata/real/mqtt-v5.0-word-links.json').read_text())
before=PdfReader('output/numbering/mqtt.pdf',strict=True)
after=PdfReader(out/'mqtt.pdf',strict=True)
assert len(before.pages)==len(after.pages)==189
expected={quote(target,safe=":/?#[]@!$&'()*+,;=%") for target in oracle['hyperlink_text_sha256']}
got=set();changes=[];annotation_count=0
for index,(a,b) in enumerate(zip(before.pages,after.pages)):
 assert a.get_contents().get_data()==b.get_contents().get_data(),('page commands changed',index+1)
 aa=[n.get_object() for n in a.get('/Annots',[])];bb=[n.get_object() for n in b.get('/Annots',[])]
 assert len(aa)==len(bb)
 for old,new in zip(aa,bb):
  assert list(old['/Rect'])==list(new['/Rect'])
  if new.get('/A',{}).get('/S')!='/URI':continue
  value=new['/A']['/URI'];assert all(c<128 for c in value.original_bytes)
  target=str(value);got.add(target);annotation_count+=1
  previous=str(old['/A']['/URI'])
  if previous!=target:changes.append(dict(page=index+1,before=previous,after=target))
assert got==expected and len(got)==435
pairs={(c['relationship'],c['word_target']) for c in oracle['split_attribute_links']}
assert {(c['before'],c['after']) for c in changes}==pairs
assert all(c['publisher_exact'] for c in oracle['split_attribute_links'])
report=dict(pages=189,all_page_commands='exact baseline bytes',all_annotation_rectangles='exact baseline coordinates',
 uri_targets=435,uri_annotations=annotation_count,changed_uri_annotations=len(changes),changes=changes,
 source_oracle='independent original Word ZIP/XML and original publisher PDF; old normative attribute oracle remains unchanged',
 scope='relationship-plus-anchor URI identity, text ownership, ASCII URI actions and unchanged paint; bookmark navigation not yet implemented')
(out/'fragment-independent.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report,indent=2))
