"""Original Go-font program hashes from pinned x/image v0.32.0, QA only.

Source: https://cs.opensource.google/go/x/image/+/refs/tags/v0.32.0:font/gofont/
Values were derived from the original module byte constants, not PDF output.
The package's Go font license is preserved in third_party/licenses/Go-fonts.txt.
"""
import hashlib
import io
import os
import re
import subprocess
from functools import lru_cache
from pathlib import Path
from fontTools.ttLib import TTFont
from fontTools.pens.recordingPen import RecordingPen

GO_FONT_SHA256 = {
 'GoRegular': '197d9f3703b4c00af609178876a8d73e396f64fe438b2c871778566632374be3',
 'Go-Bold': 'c18494baa7ea35b8dbfac3861787d360b9c0ab91232562371a09e6fbf09aaa40',
 'Go-Italic': '6efef00d080c8fe68bbe2bf08526294c83d52d7b90b16f938953025c0868cc57',
 'Go-BoldItalic': 'd6c33857c7ea7d740a8e1d81d37261782f698e9c17a2c9bf3bb0abbaf99e3c90',
 'GoMono': '8bc66a0154bbf69cd24e5bde41a12ec9495c8a242c5def2255e4a164900f1ed7',
 'GoMono-Bold': 'b165f1a142dc88d3c388de2c89d3ab62dd04620ef21ad07f43fb51f77857edc3',
 'GoMono-Italic': '10d5f73519ea32b4882897a9bec6c42ba2ca27d6c06ba9a0e55ae38a383cf7e6',
 'GoMono-BoldItalic': 'cb43aca5ab36631c0b4679831017cf249c33db4f721daad7379e04bba383ef1f',
}

def verify_go_program(name, data):
 assert name in GO_FONT_SHA256, name
 assert hashlib.sha256(data).hexdigest() == GO_FONT_SHA256[name], name

def original_font_name(name):
 return re.sub(r'^[A-Z]{6}\+', '', name.lstrip('/'))

@lru_cache(maxsize=8)
def original_go_program(name):
 name=original_font_name(name)
 packages={'GoRegular':'goregular','Go-Bold':'gobold','Go-Italic':'goitalic','Go-BoldItalic':'gobolditalic','GoMono':'gomono','GoMono-Bold':'gomonobold','GoMono-Italic':'gomonoitalic','GoMono-BoldItalic':'gomonobolditalic'}
 cache=Path(subprocess.check_output(['go','env','GOMODCACHE'],env={**os.environ,'GOWORK':'off'},text=True).strip())
 source=(cache/'golang.org/x/image@v0.32.0/font/gofont'/packages[name]/'data.go').read_text()
 literal=source.split('var TTF = []byte{',1)[1].split('}',1)[0]
 data=bytes(int(v,16) for v in re.findall(r'0x([0-9a-fA-F]{2})',literal))
 verify_go_program(name,data)
 return data

def verify_pdf_glyph_program(pdf_font, original=None):
 """Check outlines/components/metrics/notices independently, not a subset hash.

 Original programs and their pinned hashes remain the reference. Used glyphs
 come from the PDF CIDToGIDMap; fontTools reads actual sfnt tables and composite
 references. This deliberately supports the new subset behavior without
 changing any source-font, text, position or raster oracle.
 """
 cid=pdf_font['/DescendantFonts'][0].get_object();desc=cid['/FontDescriptor']
 data=desc['/FontFile2'].get_data();assert desc['/FontFile2']['/Length1']==len(data)
 name=str(pdf_font['/BaseFont'])[1:]
 assert re.match(r'^[A-Z]{6}\+',name),name
 if original is None: original=original_go_program(name)
 mapping=cid['/CIDToGIDMap'].get_data();assert len(mapping)%2==0
 used=tuple(sorted({int.from_bytes(mapping[i:i+2],'big') for i in range(0,len(mapping),2)}))
 return verify_subset_program(data,original,used)

@lru_cache(maxsize=64)
def verify_subset_program(data, original, used):
 src=TTFont(io.BytesIO(original),lazy=True)
 sub=TTFont(io.BytesIO(data),checkChecksums=2)
 assert src['maxp'].numGlyphs==sub['maxp'].numGlyphs
 names=src.getGlyphOrder();assert names==sub.getGlyphOrder()
 closure=set(used)|{0};pending=list(closure)
 while pending:
  gid=pending.pop();glyph=src['glyf'][names[gid]]
  if glyph.isComposite():
   for component in glyph.components:
    child=src.getGlyphID(component.glyphName)
    if child not in closure:closure.add(child);pending.append(child)
 src_set,sub_set=src.getGlyphSet(),sub.getGlyphSet()
 for gid in closure:
  a,b=RecordingPen(),RecordingPen();src_set[names[gid]].draw(a);sub_set[names[gid]].draw(b)
  assert a.value==b.value,(gid,names[gid])
  assert src['hmtx'][names[gid]]==sub['hmtx'][names[gid]]
  assert src['glyf'][names[gid]].program.getBytecode()==sub['glyf'][names[gid]].program.getBytecode() if hasattr(src['glyf'][names[gid]],'program') else True
 offsets=sub['loca'].locations
 assert all(gid in closure or offsets[gid]==offsets[gid+1] for gid in range(len(names)))
 for tag in ('name','cmap','hmtx','hhea','maxp','OS/2','post','cvt ','fpgm','prep','gasp'):
  if tag in src:assert tag in sub and src.reader[tag]==sub.reader[tag],tag
 assert not any(tag in sub for tag in ('DSIG','GSUB','GPOS','GDEF'))
 result={'source_bytes':len(original),'subset_bytes':len(data),'glyph_ids':len(names),'used_and_component_glyphs':len(closure)}
 src.close();sub.close()
 return result
