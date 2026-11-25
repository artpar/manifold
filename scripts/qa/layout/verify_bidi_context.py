"""Native item-context and actual two-font PDF glyph/placement QA, read-only."""
import hashlib,json,re,unicodedata,sys
from pathlib import Path
import uharfbuzz as hb
from pypdf import PdfReader
from pypdf.generic import ContentStream

fixture=json.loads(Path('testdata/rtl-item-context.json').read_text())
faces={};hashes={}
for style,f in fixture['fonts'].items():
 data=Path(f['path']).read_bytes();assert hashlib.sha256(data).hexdigest()==f['sha256']
 faces[style]=hb.Font(hb.Face(data));hashes[f['sha256']]=style
reader=PdfReader(sys.argv[1] if len(sys.argv)>1 else 'output/bidi/context.pdf',strict=True);assert len(reader.pages)==1
records=[];logical=[];used=set()
for page in reader.pages:
 fonts={}
 for name,ref in page['/Resources']['/Font'].items():
  f=ref.get_object()
  if '/DescendantFonts' not in f:continue
  d=f['/DescendantFonts'][0].get_object();descriptor=d['/FontDescriptor']
  sha=hashlib.sha256(descriptor['/FontFile2'].get_data()).hexdigest()
  if sha not in hashes:continue
  fonts[str(name)]=(hashes[sha],d['/CIDToGIDMap'].get_data());used.add(sha)
 state={'font':'','size':0,'mode':0,'x':0,'y':0};saved=[];frames=[];pending=None
 for args,op in ContentStream(page.get_contents(),reader).operations:
  if op==b'q':saved.append(state.copy())
  elif op==b'Q':state=saved.pop()
  elif op==b'Tf':state['font'],state['size']=str(args[0]),float(args[1])
  elif op==b'Tr':state['mode']=int(args[0])
  elif op==b'Tm':
   assert list(map(float,args[:4]))==[1,0,0,1]
   state['x'],state['y']=map(float,args[4:])
  elif op==b'BDC':frames.append({'tag':str(args[0]),'text':str(args[1].get('/ActualText','')),'glyphs':[]})
  elif op==b'Tj' and frames and state['font'] in fonts:
   raw=args[0].original_bytes if hasattr(args[0],'original_bytes') else bytes(args[0]);assert len(raw)==2
   style,mapping=fonts[state['font']];cid=int.from_bytes(raw,'big')
   frames[-1]['glyphs'].append(dict(state,style=style,gid=int.from_bytes(mapping[2*cid:2*cid+2],'big')))
  elif op==b'EMC':
   frame=frames.pop();text=frame['text'];glyphs=frame['glyphs']
   if frame['tag']=='/Artifact':assert text=='';pending=glyphs
   elif text:
    logical.append(text)
    if glyphs:
     assert len(glyphs)==1 and glyphs[0]['mode']==3 and pending is not None
     records.append({'text':text,'carrier':glyphs[0],'glyphs':pending});pending=None
 assert not frames and not saved
assert len(used)==2 and len(records)==len(fixture['items'])==14
for item,record in zip(fixture['items'],records):
 style=item['font'];font=faces[style];upem=font.face.upem
 text=item['context'][item['start']:item['end']];assert record['text']==text
 b=hb.Buffer();b.add_codepoints(list(map(ord,unicodedata.normalize('NFC',item['context']))),item['start'],item['end']-item['start'])
 b.direction='rtl';b.script='Arab';b.language=item['language'];b.flags=hb.BufferFlags.DO_NOT_INSERT_DOTTED_CIRCLE;hb.shape(font,b)
 assert len(record['glyphs'])==len(b.glyph_infos)==len(item['glyphs'])
 origin=record['carrier'];assert origin['style']==style and origin['size']==item['size']
 pen=0
 for native,p,g,want in zip(b.glyph_infos,b.glyph_positions,record['glyphs'],item['glyphs']):
  assert native.codepoint==g['gid']==want['gid']
  assert native.cluster-item['start']==want['cluster']
  assert pen+p.x_offset==want['x'] and p.y_offset==want['y'] and p.x_advance==want['advance']
  assert g['style']==style and g['size']==item['size']
  assert abs(g['x']-(origin['x']+(pen+p.x_offset)*item['size']/upem))<1e-6
  assert abs(g['y']-(origin['y']+p.y_offset*item['size']/upem))<1e-6
  pen+=p.x_advance
 assert pen==item['advance']
for i in range(0,len(records),2):
 a,z=records[i:i+2];ia,iz=fixture['items'][i:i+2]
 assert a['carrier']['x']>z['carrier']['x']
 advance=ia['advance']*ia['size']/faces[ia['font']].face.upem
 prior=iz['advance']*iz['size']/faces[iz['font']].face.upem
 assert abs(a['carrier']['x']+advance-315)<.011
 assert abs(z['carrier']['x']+prior-a['carrier']['x'])<.011
assert all(ch in ''.join(logical) for ch in ['\u200f','\u200b','\u2060','\ufeff'])
print('Bidi context passed: 14 native contextual items / 7 PDF rows, two original font programs, font/size/language boundaries and transparent controls, exact glyphs/positions and RTL continuity')
