"""Independent standard-URI, ownership, all-story and relationship checks."""
import json
from pathlib import Path
from urllib.parse import quote
from pypdf import PdfReader
import pymupdf
from docx import Document

out=Path('output/uri')
def targets(sf):
 result=set()
 def walk(n):
  for m in n.get('marks',[]):
   if m['type']=='link':result.add(m['attrs']['href'])
  for child in n.get('content',[]):walk(child)
 walk(sf)
 for role in ('headers','footers'):
  for n in sf['attrs'].get(role,[]):walk(n)
 for note in sf['attrs'].get('notes',[]):
  for n in note['content']:walk(n)
 return result
source=json.loads((out/'source.sf').read_text());original=targets(source)
expected={quote(x,safe=":/?#[]@!$&'()*+,;=%") for x in original}
assert len(original)==len(expected)==8
assert targets(json.loads((out/'from-docx.sf').read_text()))==original
assert targets(json.loads((out/'from-pdf.sf').read_text()))==expected
reader=PdfReader(out/'uri.pdf',strict=True);pdf=pymupdf.open(out/'uri.pdf')
assert len(reader.pages)==len(pdf)==2
got=set();internal=0
for page in reader.pages:
 for ref in page['/Annots']:
  a=ref.get_object()
  if '/Dest' in a:internal+=1
  if a.get('/A',{}).get('/S')=='/URI':
   value=a['/A']['/URI'];assert str(value).isascii()
   assert all(b<128 for b in value.original_bytes)
   got.add(str(value))
assert got==expected and internal==1
# MuPDF classifies a relative fragment URI as a file-reference link. The raw
# action above remains /S /URI; this does not add a launch or named destination.
assert {link.get('uri',link.get('file')) for page in pdf for link in page.get_links() if 'uri' in link or 'file' in link}==expected
assert Document(out/'uri.docx').sections[0].header.paragraphs[0].text=='HEADER'
for page in pdf:
 assert page.get_text().count('HEADER')==page.get_text().count('FOOTER')==1
 for box in (pymupdf.Rect(38,18,78,32),pymupdf.Rect(38,378,78,392)):
  assert min(page.get_pixmap(clip=box).samples)<200
for label in ('HEADER','FOOTER','NOTE','RFC3987','TABLE','FRAGMENT','LIST','END','ANCHOR'):
 assert label in ''.join(p.get_text() for p in pdf),label
for i,p in enumerate(pdf):p.get_pixmap(matrix=pymupdf.Matrix(1.5,1.5)).save(out/f'page-{i+1}.png')
report=dict(pages=2,source_iri_targets=8,serialized_ascii_uri_targets=8,
            docx_targets='exact original Unicode IRIs',pdf_and_recovered_sf_targets='RFC 3987 equivalent ASCII URI references',
            sources='original source SF unchanged; reserved/percent delimiters preserved; no normalization',
            readers='pypdf raw URI annotation bytes and MuPDF URI/file-reference targets exact',internal_note_destinations=internal,
            scope='body, table cells, list, note and repeated headers/footers; internal bookmark resolution remains outside this URI-action change')
(out/'independent.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report,indent=2))
