"""Independent bidi PDF font/GID/GPOS, native UBA placement and logical-text QA."""
import ctypes as c
import ctypes.util
import hashlib
from font_oracles import verify_pdf_glyph_program
import re
import sys
import unicodedata
from pathlib import Path
from zipfile import ZipFile
from lxml import etree
from collections import defaultdict
import uharfbuzz as hb
import pymupdf
from pypdf import PdfReader
from pypdf.generic import ContentStream
from docx import Document

out = Path('output/bidi')
program = Path('testdata/assets/DejaVuSans.ttf').read_bytes()
sha = hashlib.sha256(program).hexdigest()
font = hb.Font(hb.Face(program)); upem = font.face.upem
pdf_path=Path(sys.argv[1]) if len(sys.argv)>1 else out/'bidi.pdf'
reader = PdfReader(pdf_path, strict=True)
assert len(reader.pages) == 1
records = []
for page in reader.pages:
    fonts = {}
    for name, ref in page['/Resources']['/Font'].items():
        f = ref.get_object()
        if '/DescendantFonts' not in f: continue
        d = f['/DescendantFonts'][0].get_object(); descriptor = d['/FontDescriptor']
        if str(descriptor.get('/FontFamily','')) != 'DejaVu Sans': continue
        verify_pdf_glyph_program(f,program)
        mapping = d['/CIDToGIDMap'].get_data()
        cmap = {int(a,16): bytes.fromhex(b.decode()).decode('utf-16-be') for a,b in re.findall(rb'<([0-9A-F]{4})>\s*<([0-9A-F]*)>', f['/ToUnicode'].get_data())}
        widths = {}; w = d['/W']; i = 0
        while i < len(w):
            cid = int(w[i]); values = w[i+1]; assert isinstance(values,list)
            for j,value in enumerate(values): widths[cid+j]=float(value)
            i += 2
        fonts[str(name)] = mapping,cmap,widths
    state = {'font':'','size':0,'x':0,'y':0,'mode':0,'rgb':(0,0,0),'clip':None}
    saved=[]; frames=[]; pending=None; rect=None; clipping=False
    for args, op in ContentStream(page.get_contents(),reader).operations:
        if op==b'q': saved.append(state.copy())
        elif op==b'Q': state=saved.pop()
        elif op==b'rg': state['rgb']=tuple(map(float,args))
        elif op==b'Tf': state['font'],state['size']=str(args[0]),float(args[1])
        elif op==b'Tr': state['mode']=int(args[0])
        elif op==b'Tm':
            assert list(map(float,args[:4]))==[1,0,0,1]
            state['x'],state['y']=map(float,args[4:])
        elif op==b're': rect=tuple(map(float,args))
        elif op in (b'W',b'W*'): clipping=True
        elif op in (b'n',b'S',b'f',b'f*',b'B'):
            if clipping: state['clip']=rect
            rect=None;clipping=False
        elif op==b'BDC': frames.append({'text':str(args[1].get('/ActualText','')),'tag':str(args[0]),'shows':[]})
        elif op==b'BMC': frames.append({'text':'','tag':str(args[0]),'shows':[]})
        elif op==b'Tj' and frames and state['font'] in fonts:
            raw=args[0].original_bytes if hasattr(args[0],'original_bytes') else bytes(args[0])
            assert len(raw)==2,(state["font"],raw,args)
            cid=int.from_bytes(raw,'big');mapping,cmap,widths=fonts[state['font']]
            frames[-1]['shows'].append(dict(state,gid=int.from_bytes(mapping[cid*2:cid*2+2],'big'),logical=cmap[cid],advance=widths[cid]*state['size']/1000))
        elif op==b'EMC':
            frame=frames.pop()
            if frame['tag']=='/Artifact':
                assert frame['text']==''
                pending=frame['shows']
            elif frame['text']:
                shows=frame['shows'];carrier=[g for g in shows if g['mode']==3]
                if carrier:
                    assert len(carrier)==1 and pending is not None
                    assert carrier[0]['logical']==frame['text'] and carrier[0]['gid']==3
                    assert all(g['logical']=='' for g in pending)
                    records.append(dict(text=frame['text'],glyphs=pending,carrier=carrier[0]));pending=None
                elif shows: records.append(dict(text=frame['text'],glyphs=shows,carrier=None))
                elif any(ord(x) in range(0x2066,0x206a) for x in frame['text']):
                    records.append(dict(text=frame['text'],glyphs=[],carrier=None))
    assert not frames and not saved
# Logical operator sequence retains all authored words; native extraction's
# formatting controls are checked in ActualText and Go SF rather than assuming
# every third-party text API returns invisible Unicode controls.
logical=''.join(r['text'] for r in records)
assert 'שלום עולם (123) testسلام بالعالم ١٢٣ABC שלום DEF 123first secondلاA\u2067אבג (12)\u2069 Z' in logical
assert 'שלום הערה1שלום רשימהאבְג سلامسلام הערהשלום' in logical
# Native C FriBidi provides levels from original logical row strings. The full
# original Unicode corpus and separate wrapped-line native fixture qualify context.
lib=c.CDLL(ctypes.util.find_library('fribidi')); U,I,L,P=c.c_uint32,c.c_int32,c.c_int8,c.POINTER
lib.fribidi_get_bidi_type.argtypes=[U];lib.fribidi_get_bidi_type.restype=U
lib.fribidi_get_bidi_types.argtypes=[P(U),I,P(U)]
lib.fribidi_get_bracket_types.argtypes=[P(U),I,P(U),P(U)]
lib.fribidi_get_par_embedding_levels_ex.argtypes=[P(U),P(U),I,P(U),P(L)];lib.fribidi_get_par_embedding_levels_ex.restype=L
lib.fribidi_reorder_line.argtypes=[U,P(U),I,I,U,P(L),P(U),P(I)];lib.fribidi_reorder_line.restype=L
rows=defaultdict(list)
for r in records:
    if not r['glyphs']:continue
    first=r['carrier'] or r['glyphs'][0]
    rows[round(first['y'],2)].append(r)
checked=0;clips=[]
for y,recs in rows.items():
    text=''.join(r['text'] for r in recs); n=len(text)
    base=0 if text.startswith('ABC ') or text.startswith('A') else 1
    chars=(U*n)(*map(ord,text));types=(U*n)();brackets=(U*n)();levels=(L*n)()
    lib.fribidi_get_bidi_types(chars,n,types);lib.fribidi_get_bracket_types(chars,n,types,brackets)
    if text=='first second':types[5]=lib.fribidi_get_bidi_type(ord('א'))
    direction=U(lib.fribidi_get_bidi_type(ord('א' if base else 'a')))
    assert lib.fribidi_get_par_embedding_levels_ex(types,brackets,n,c.byref(direction),levels)>0
    offset=0;groups=[]
    for r in recs:
        b=hb.Buffer();b.add_str(r['text']);b.guess_segment_properties()
        r=dict(r,level=int(levels[offset]),script=b.script or 'Zyyy',size=(r['carrier'] or r['glyphs'][0])['size']);offset+=len(r['text'])
        key=r['level'],r['script'],r['size']
        if groups and groups[-1]['key']==key:groups[-1]['records'].append(r)
        else:groups.append({'key':key,'records':[r]})
    for group in groups:
        level,script,size=group['key'];parts=group['records'];s=''.join(r['text'] for r in parts)
        b=hb.Buffer();b.add_codepoints(list(map(ord,unicodedata.normalize('NFC',s))));b.direction='rtl' if level%2 else 'ltr';b.script=script;b.language='und';b.flags=hb.BufferFlags.DO_NOT_INSERT_DOTTED_CIRCLE;hb.shape(font,b)
        actual=[];seen=set()
        for r in parts:
            for g in r['glyphs']:
                key=g['gid'],round(g['x'],7),round(g['y'],7)
                if g['clip'] is not None:
                    clips.append(g)
                    if key in seen:continue
                seen.add(key);actual.append(g)
        carrier=[r['carrier'] for r in parts if r['carrier']]
        x=min(g['x'] for g in carrier) if carrier else actual[0]['x']-b.glyph_positions[0].x_offset*size/upem
        baseline=(carrier[0]['y'] if carrier else actual[0]['y']-b.glyph_positions[0].y_offset*size/upem)
        expected=[];advance=0
        for info,p in zip(b.glyph_infos,b.glyph_positions):
            expected.append((info.codepoint,x+(advance+p.x_offset)*size/upem,baseline+p.y_offset*size/upem));advance+=p.x_advance
        assert len(actual)==len(expected),(s,actual,expected)
        for gid,gx,gy in expected:
            match=next((i for i,g in enumerate(actual) if g['gid']==gid and abs(g['x']-gx)<1e-6 and abs(g['y']-gy)<1e-6),None)
            assert match is not None,(s,gid,gx,gy,actual)
            actual.pop(match)
        if carrier:assert abs(sum(g['advance'] for g in carrier)-advance*size/upem)<.0002,(s,'carrier advance')
        group['left'],group['width']=x,advance*size/upem
        checked+=1
    # Collapse the native scalar L2 permutation into its shaped placement units.
    visual=(I*n)(*range(n));line_levels=(L*n)(*levels)
    assert lib.fribidi_reorder_line(0,types,n,0,direction,line_levels,None,visual)>0
    owners=[]
    for i,g in enumerate(groups):owners.extend([i]*sum(len(r['text']) for r in g['records']))
    want=[]
    for i in visual:
        owner=owners[i]
        if not want or want[-1]!=owner:want.append(owner)
    assert len(want)==len(groups),(text,'noncontiguous native placement units')
    got=sorted(range(len(groups)),key=lambda i:groups[i]['left'])
    assert got==want,(text,'native L2 placement',got,want)
    for i,g in enumerate(groups):
        assert g['left']>=44.99 and g['left']+g['width']<=315.011,(text,'page containment')
        if i in want[1:]:
            prior=groups[want[want.index(i)-1]]
            assert abs(g['left']-(prior['left']+prior['width']))<.011,(text,'physical span continuity')
assert len(clips)==2 and {g['gid'] for g in clips}=={5365}
red=next(g for g in clips if g['rgb'][0]>.7);blue=next(g for g in clips if g['rgb'][2]>.7)
assert red['clip'][0]>blue['clip'][0] and red['clip'][0]==blue['clip'][0]+blue['clip'][2]
assert checked==29
links=reader.pages[0]['/Annots'];assert any(a.get_object().get('/A',{}).get('/URI')=='https://example.invalid/rtl' for a in links)
assert any(a.get_object().get('/Dest') for a in links)
uri=next(a.get_object() for a in links if a.get_object().get('/A',{}).get('/URI')=='https://example.invalid/rtl')
carrier=next(r['carrier'] for r in records if r['text']=='ا')
assert abs(float(uri['/Rect'][0])-carrier['x'])<.011
assert abs(float(uri['/Rect'][2])-float(uri['/Rect'][0])-carrier['advance'])<.011
anchor=next(r for r in records if r['text']=='שלום הערה')
internal=next(a.get_object() for a in links if a.get_object().get('/Dest'))
assert float(internal['/Rect'][2])<=anchor['carrier']['x']+.011

word=Document(out/'bidi.docx');assert word.paragraphs[0].text=='שלום עולם (123) test'
assert word.tables[0].cell(0,0).text=='אבְג سلام'
with ZipFile(out/'bidi.docx') as package:
    ns={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}
    xml=etree.fromstring(package.read('word/document.xml'))
    assert xml.xpath('count(//w:pPr/w:bidi)',namespaces=ns)>=10
    assert xml.xpath('//w:r[w:rPr/w:rtl]/w:t[text()=" "]',namespaces=ns)
    for part in ['word/header1.xml','word/footer1.xml','word/footnotes.xml']:
        assert etree.fromstring(package.read(part)).xpath('//w:pPr/w:bidi',namespaces=ns)

for text in ['שלום עולם','سلام بالعالم','١٢٣','لا','אבְג','שלום רשימה']:
    assert text in ''.join(p.extract_text() for p in reader.pages),text
    assert text in ''.join(p.get_text().replace('\n','') for p in pymupdf.open(pdf_path)),text
print(f'Bidi output passed: {checked} native-HarfBuzz groups, original font/GIDs/positions, C FriBidi levels/L2 order/continuity, RTL ligature clips, carriers, both-reader words and relationships')
