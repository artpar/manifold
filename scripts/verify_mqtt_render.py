"""Independent full-source font/link/asset checks; not a layout-equivalence oracle."""
import argparse
import hashlib
import json
from pathlib import Path
from pypdf import PdfReader
import pymupdf
from font_oracles import original_font_name, verify_pdf_glyph_program

parser=argparse.ArgumentParser()
parser.add_argument('--out',type=Path,default=Path('output/font-subset'))
a=parser.parse_args();out=a.out
sf=json.loads((out/'mqtt.sf').read_text())
expected_links=set()
def walk(n):
 for mark in n.get('marks',[]):
  if mark['type']=='link':expected_links.add(mark['attrs']['href'])
 for child in n.get('content',[]):walk(child)
walk(sf)
r=PdfReader(out/'mqtt.pdf',strict=True);pdf=pymupdf.open(out/'mqtt.pdf')
assert len(r.pages)==len(pdf) and r.metadata.title=='MQTT Version 5.0'
sources={name:Path('output/font-subset/dejavu')/(name+'.ttf') for name in ('DejaVuSans','DejaVuSans-Bold','DejaVuSans-Oblique','DejaVuSans-BoldOblique')}
sources['BabelStoneHan']=Path('output/font-research/BabelStoneHan.ttf')
links=set();images={};programs={}
for page in r.pages:
 for ref in page.get('/Annots',[]):
  annot=ref.get_object()
  if annot.get('/A',{}).get('/URI'):links.add(str(annot['/A']['/URI']))
 for ref in page['/Resources'].get('/XObject',{}).values():
  obj=ref.get_object()
  if obj.get('/Subtype')=='/Image':images[ref.idnum]=hashlib.sha256(obj.get_data()).hexdigest()
 for key,ref in page['/Resources']['/Font'].items():
  if not str(key).startswith('/MDG') or ref.idnum in programs:continue
  f=ref.get_object();name=original_font_name(str(f['/BaseFont']))
  original=sources[name].read_bytes() if name in sources else None
  programs[ref.idnum]=dict(name=name,**verify_pdf_glyph_program(f,original))
word=json.loads(Path('testdata/real/mqtt-v5.0-oracle.json').read_text())
assert word['media']['word/media/image1.jpg']['sha256'] in images.values()
texts=[p.get_text() for p in pdf]
assert sum(s.count('𪛔') for s in texts)==1
han_page=next(i for i,s in enumerate(texts) if '𪛔' in s)
assert r.pages[han_page].extract_text().count('𪛔')==1
assert sum(s.count('‒') for s in texts)==4
selected={0,next(i for i,s in enumerate(texts) if '𪛔' in s),next(i for i,s in enumerate(texts) if 'SessionExpiryInterval(0)' in ''.join(s.split()))}
for i in sorted(selected):pdf[i].get_pixmap(matrix=pymupdf.Matrix(1,1)).save(out/f'mqtt-page-{i+1}.png')
report=dict(pages=len(pdf),bytes=(out/'mqtt.pdf').stat().st_size,title=r.metadata.title,
            uri_targets=len(links),expected_source_uri_targets=len(expected_links),
            missing_uri_targets=sorted(expected_links-links),unexpected_uri_targets=sorted(links-expected_links),
            image_programs=len(images),original_jpeg_sha_preserved=True,
            supplementary_character_count=1,figure_dash_count=4,
            unique_glyph_font_programs=list(programs.values()),
            selected_pages_for_visual_inspection=[i+1 for i in sorted(selected)],
            limitation='Explicit caller font normalization and acknowledged source diagnostics. This does not prove complete source text/structure/layout recovery or Word-equivalent reflow; widths/numbering/shading and further corpus work remain.')
(out/'mqtt-independent.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report,indent=2))
assert links==expected_links,(len(links),len(expected_links),expected_links-links)
