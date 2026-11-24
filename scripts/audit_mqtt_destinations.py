"""Read-only source bookmark/URI research; never reads manydoc output."""
import argparse
from collections import Counter
import hashlib
import json
from pathlib import Path
from zipfile import ZipFile
from lxml import etree
from pypdf import PdfReader

parser=argparse.ArgumentParser()
parser.add_argument('--word-links-output',type=Path)
args=parser.parse_args()

word=Path('testdata/real/mqtt-v5.0-os.docx');pdf_path=Path('testdata/real/mqtt-v5.0-os.pdf')
assert hashlib.sha256(word.read_bytes()).hexdigest()=='a567582f279f53b300956282b760db95f66e5096d3ca8412321cf1e8403ac58b'
assert hashlib.sha256(pdf_path.read_bytes()).hexdigest()=='e8f8e9d2467618d5c5a6398bdf971cb90cdbe2c8e33242e028f442bee8e5de20'
w='{http://schemas.openxmlformats.org/wordprocessingml/2006/main}';rn='{http://schemas.openxmlformats.org/officeDocument/2006/relationships}'
with ZipFile(word) as z:
 root=etree.fromstring(z.read('word/document.xml'))
 rels={n.get('Id'):n.get('Target') for n in etree.fromstring(z.read('word/_rels/document.xml.rels'))}
links=list(root.iter(w+'hyperlink'))
local=[n for n in links if not n.get(rn+'id') and n.get(w+'anchor')]
both=[n for n in links if n.get(rn+'id') and n.get(w+'anchor')]
visible=[n for n in links if ''.join(x.text or '' for x in n.iter(w+'t')).strip()]
projection={rels[n.get(rn+'id')] if n.get(rn+'id') else '#'+n.get(w+'anchor','') for n in visible}
legacy=json.loads(Path('testdata/real/mqtt-v5.0-oracle.json').read_text())
assert projection==set(legacy['hyperlink_text_sha256'])
starts=list(root.iter(w+'bookmarkStart'));ends=list(root.iter(w+'bookmarkEnd'))
names={n.get(w+'name') for n in starts}
start_ids=Counter(n.get(w+'id') for n in starts);end_ids=Counter(n.get(w+'id') for n in ends)
assert start_ids==end_ids and len(start_ids)==len(starts)==len(names)
pdf=PdfReader(pdf_path);uris=set()
for page in pdf.pages:
 for ref in page.get('/Annots',[]):
  action=ref.get_object().get('/A',{})
  if action.get('/S')=='/URI':uris.add(str(action['/URI']))
external=[]
for n in both:
 uri=rels[n.get(rn+'id')];anchor=n.get(w+'anchor');target=uri+'#'+anchor
 external.append(dict(relationship=uri,anchor=anchor,word_target=target,publisher_exact=target in uris,publisher_base=uri in uris))
assert all(n['publisher_exact'] for n in external)
raw={rels[n.get(rn+'id')] if n.get(rn+'id') else '#'+n.get(w+'anchor','') for n in links}
report=dict(source_sha256=hashlib.sha256(word.read_bytes()).hexdigest(),hyperlink_elements=len(links),raw_attribute_target_projection=len(raw),
 visible_hyperlink_elements=len(visible),visible_attribute_target_projection=len(projection),empty_hyperlink_elements=len(links)-len(visible),
 relationship_elements=sum(bool(n.get(rn+'id')) for n in links),relationship_targets=len({rels[n.get(rn+'id')] for n in links if n.get(rn+'id')}),
 anchor_only_elements=len(local),anchor_only_targets=len({n.get(w+'anchor') for n in local}),
 bookmark_starts=len(starts),bookmark_end_pairs=len(ends),unique_names=len(names),
 inline_bookmark_starts=sum(any(p.tag==w+'p' for p in n.iterancestors()) for n in starts),
 block_bookmark_starts=sum(not any(p.tag==w+'p' for p in n.iterancestors()) for n in starts),
 unresolved_anchor_only=sorted({n.get(w+'anchor') for n in local}-names),
 unresolved_visible_anchor_only=sorted({n.get(w+'anchor') for n in visible if not n.get(rn+'id') and n.get(w+'anchor')}-names),
 unresolved_visible_occurrences=sum(not n.get(rn+'id') and n.get(w+'anchor') not in names for n in visible),relationship_plus_anchor=external,publisher_uri_targets=len(uris),
 scope='original package facts and publisher URI evidence; not an implementation or resolved-navigation qualification')
out=Path('output/internal-destinations');out.mkdir(parents=True,exist_ok=True)
(out/'source-audit.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report,indent=2))

# Keep the original normative attribute projection unchanged. This independent
# second oracle models Word's relationship-plus-anchor behavior, which is also
# present in the publisher PDF. No manydoc output is read.
legacy_text={};word_text={}
for node in links:
 pieces=[]
 for el in node.iter():
  if el.tag==w+'t':pieces.append(el.text or '')
  elif el.tag in (w+'tab',w+'br',w+'cr'):pieces.append(' ')
  elif el.tag==w+'noBreakHyphen':pieces.append('\u2011')
  elif el.tag==w+'softHyphen':pieces.append('\u00ad')
 visible_text=''.join(''.join(pieces).split())
 if not visible_text:continue
 rid=node.get(rn+'id');anchor=node.get(w+'anchor','')
 raw_target=rels[rid] if rid else '#'+anchor
 word_target=raw_target+('#'+anchor if rid and anchor else '')
 legacy_text[raw_target]=legacy_text.get(raw_target,'')+visible_text
 word_text[word_target]=word_text.get(word_target,'')+visible_text
word_external={rels[n.get(rn+'id')]+('#'+n.get(w+'anchor') if n.get(w+'anchor') else '') for n in links if n.get(rn+'id')}
authority_spellings={'http://www.ibm.com','http://www.microsoft.com','https://www.privacyshield.gov'}
assert word_external-uris==authority_spellings
assert uris-word_external=={x+'/' for x in authority_spellings}
external_spelling=[dict(word=x,pdf=x+'/') for x in sorted(authority_spellings)]
hashes=lambda values:{target:hashlib.sha256(text.encode()).hexdigest() for target,text in sorted(values.items())}
assert hashes(legacy_text)==legacy['hyperlink_text_sha256'],'original normative source oracle changed'
assert all(e['publisher_exact'] for e in external),'publisher does not confirm Word target composition'
if args.word_links_output:
 secondary=dict(source_sha256=report['source_sha256'],oracle='Original Word ZIP/XML plus original publisher PDF, independent of manydoc; Word relationship-plus-anchor rule from MS-OI29500 2.1.521',
  original_attribute_projection_targets=len(legacy_text),word_targets=len(word_text),word_external_targets=len(word_external),publisher_external_spelling_differences=external_spelling,publisher_external_targets=len(uris),split_attribute_links=external,hyperlink_text_sha256=hashes(word_text))
 args.word_links_output.write_text(json.dumps(secondary,indent=2)+'\n')
 print('Word-compatible target/text oracle:',len(word_text),'targets')
