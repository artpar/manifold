"""Independent rich inferred-note qualification, optional QA dependencies only."""
import argparse
import base64
import io
import json
from pathlib import Path
import posixpath
import subprocess
import sys
import zipfile
import xml.etree.ElementTree as ET
from PIL import Image
from pypdf import PdfReader
from pypdf.generic import IndirectObject
import pymupdf

parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('root',type=Path,nargs='?',default=Path('output/pdf-note-extent'))
parser.add_argument('--cli',default='bin/manydoc')
parser.add_argument('--probe',type=Path,default=Path('output/pdf-note-continuation'))
parser.add_argument('--word-render',type=Path)
args=parser.parse_args()
ns={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main','a':'http://schemas.openxmlformats.org/drawingml/2006/main'}
R='http://schemas.openxmlformats.org/officeDocument/2006/relationships'


def command(*words):
    result=subprocess.run([args.cli,*map(str,words)],capture_output=True,text=True)
    assert result.returncode==0,(words,result.stderr)


def runs(blocks):
    for block in blocks:
        yield from block.get('runs',[])
        for item in block.get('items',[]):
            if item.get('marker'):yield from runs(item['marker']['blocks'])
            yield from runs(item['blocks'])
        for row in block.get('rows',[]):
            for cell in row['cells']:yield from runs(cell['blocks'])


def text(blocks):return ''.join(r.get('text','') for r in runs(blocks))


def extract(path,target):
    command('extract','-format','json','-o',target,path)
    return json.loads(target.read_text())


def bounds(quad):return min(p['x'] for p in quad),min(p['y'] for p in quad),max(p['x'] for p in quad),max(p['y'] for p in quad)


def close(actual,expected):assert all(abs(a-b)<.025 for a,b in zip(actual,expected)),(actual,expected)


def check_graph(doc,kind):
    assert len(doc['notes'])==1
    note=doc['notes'][0]
    refs=[r for r in runs(doc['blocks']) if r.get('note')]
    assert len(refs)==1 and refs[0]['note']['id']==note['id'] and note['kind']==kind
    blocks=note['blocks']
    assert [b['kind'] for b in blocks]==['paragraph','paragraph','table','image','paragraph'],[b['kind'] for b in blocks]
    assert text([blocks[0]]).strip()=='body note' and text([blocks[1]])=='CONTINUATION' and text([blocks[4]])=='TAIL'
    assert blocks[1]['runs'][0]['bold'] and blocks[1]['runs'][0]['link']=='https://example.test/note-continuation'
    cells=blocks[2]['rows']
    assert [[text(c['blocks']) for c in row['cells']] for row in cells]==[['A','B'],['C','D']]
    with Image.open(io.BytesIO(base64.b64decode(blocks[3]['image']['data']))) as image:
        assert image.size==(2,1) and list(image.convert('RGB').getdata())==[(255,0,0),(0,0,255)]
    return note,refs[0]


report={}
for kind in ['footnote','endnote']:
    root=args.root/kind
    original=root/'rich.pdf'
    original_bytes=original.read_bytes()
    doc=json.loads((root/'rich.json').read_text())
    note,ref=check_graph(doc,kind)
    assert note['kind_inferred'] and ref['note']['inferred']
    assert note['source']['structure']=='inferred' and note['source']['evidence']=='pdf_note_reciprocal_views;separated_note_zone_continuation'
    source=PdfReader(original,strict=True)
    body_page=0 if kind=='footnote' else 1
    assert len(source.pages)==body_page+1
    assert '/StructTreeRoot' not in source.trailer['/Root']
    image=source.get_object(IndirectObject(8,0,source))
    assert image['/Width']==2 and image['/Height']==1 and image.get_data()==bytes([255,0,0,0,0,255])
    assert sum(len(p.get('/Annots',[])) for p in source.pages)==3
    assert source.pages[body_page]['/Annots'][-1].get_object()['/A']['/URI']=='https://example.test/note-continuation'
    expected_rect=(70,515-300*body_page,100,535-300*body_page)
    close(bounds(note['blocks'][3]['source']['quad']),expected_rect)
    # Every painted character in the model has its original independent glyph origin.
    with pymupdf.open(original) as pdf:
        chars=[(page_index,c['c'],c['origin']) for page_index,page in enumerate(pdf) for b in page.get_text('rawdict')['blocks'] if 'lines' in b for line in b['lines'] for span in line['spans'] for c in span['chars']]
        evidence=[(r.get('text',''),r.get('source')) for r in runs(doc['blocks']) if not r.get('note')]
        evidence += [(r.get('text',''),r.get('source')) for r in runs(note['blocks'])]
        evidence += [(note['label'],note['label_source']),(note['label'],ref['source'])]
        matched=[]
        for literal,s in evidence:
            if not literal or not s or s.get('precision')=='synthetic_separator':continue
            baseline=s['baseline']
            width=(baseline[1]['x']-baseline[0]['x'])/len(literal)
            for i,char in enumerate(literal):
                position=(baseline[0]['x']+i*width,baseline[0]['y'])
                candidates=[j for j,(page,c,origin) in enumerate(chars) if page==s['page']-1 and c==char and abs(origin[0]-position[0])<.025 and abs(origin[1]-position[1])<.025]
                assert len(candidates)==1,(kind,literal,char,position,candidates)
                matched.append(candidates[0])
        assert len(matched)==len(set(matched))==len(chars),(kind,'painted character omission/duplication',len(matched),len(chars))
        close(tuple(pdf[body_page].get_image_rects(8)[0]),expected_rect)
        spans=[s for b in pdf[body_page].get_text('dict')['blocks'] if 'lines' in b for line in b['lines'] for s in line['spans'] if 'CONTINUATION' in s['text']]
        assert len(spans)==1 and 'Bold' in spans[0]['font'] and spans[0]['color']==255 and spans[0]['size']==10
    sf=root/'rich.sf'
    word=root/'rich.docx'
    command('convert','-allow-lossy','-o',word,sf)
    part='word/'+('footnotes.xml' if kind=='footnote' else 'endnotes.xml')
    relpart=posixpath.join('word/_rels',Path(part).name+'.rels')
    with zipfile.ZipFile(word) as archive:
        tree=ET.fromstring(archive.read(part))
        definitions=[e for e in tree if e.get('{'+ns['w']+'}type','normal')=='normal']
        assert len(definitions)==1
        body=definitions[0]
        assert len(body.findall('w:tbl',ns))==1 and len(body.findall('.//w:tr',ns))==2 and len(body.findall('.//w:tc',ns))==4
        relations={r.get('Id'):r for r in ET.fromstring(archive.read(relpart))}
        links=body.findall('.//w:hyperlink',ns)
        assert any(''.join(t.text or '' for t in link.findall('.//w:t',ns))=='CONTINUATION' and relations[link.get('{'+R+'}id')].get('Target')=='https://example.test/note-continuation' for link in links)
        blips=body.findall('.//a:blip',ns)
        assert len(blips)==1
        target=relations[blips[0].get('{'+R+'}embed')].get('Target')
        with Image.open(io.BytesIO(archive.read(posixpath.normpath(posixpath.join('word',target))))) as image:
            assert list(image.convert('RGB').getdata())==[(255,0,0),(0,0,255)]
    worddoc=extract(word,root/'rich-docx.json');check_graph(worddoc,kind)
    command('extract','-format','sf','-o',root/'rich-docx.sf',word)
    fontargs=[]
    for family in sorted({r['font_family'] for r in runs(doc['blocks']) if r.get('font_family')}|{r['font_family'] for r in runs(note['blocks']) if r.get('font_family')}):fontargs += ['-pdf-font-map',family+'=Go Mono']
    native=root/'rich-native.pdf'
    command('convert','-allow-lossy',*fontargs,'-o',native,sf)
    native_doc=extract(native,root/'rich-native.json');check_graph(native_doc,kind)
    command('extract','-format','sf','-o',root/'rich-native.sf',native)
    with pymupdf.open(native) as pdf:
        rendered_text=''.join(page.get_text() for page in pdf)
        assert 'CONTINUATION' in rendered_text and 'body note' in rendered_text and 'TAIL' in rendered_text
        assert any(link.get('uri')=='https://example.test/note-continuation' for page in pdf for link in page.get_links())
        assert any(link.get('page')==len(pdf)-1 for page in pdf for link in page.get_links())
        for i,page in enumerate(pdf):page.get_pixmap().save(root/('native-page-'+str(i+1)+'.png'))
    assert 'CONTINUATION' in ''.join(page.extract_text() for page in PdfReader(native,strict=True).pages)
    if args.word_render:
        path=args.word_render/kind/'rich.pdf'
        assert 'CONTINUATION' in ''.join(page.extract_text() for page in PdfReader(path,strict=True).pages)
        with pymupdf.open(path) as pdf:
            assert 'TAIL' in ''.join(page.get_text() for page in pdf)
            for i,page in enumerate(pdf):page.get_pixmap().save(root/('word-page-'+str(i+1)+'.png'))
    assert original.read_bytes()==original_bytes
    report[kind]={'physical_pages':len(source.pages),'glyphs':len(matched),'note_blocks':5,'table':'2x2 A/B/C/D','pixels':'red/blue exact','source':'fully untagged independent raw PDF objects','outputs':'DOCX ZIP/XML, SF reread, actual tagged native PDF reread, independent pypdf/MuPDF and renders','word_render_checked':bool(args.word_render)}

if args.probe:
    root=args.probe
    source=root/'continuation-source.pdf'
    target=root/'geometric.json'
    command('extract','-pdf-skip-structure','-format','json','-o',target,source)
    result=subprocess.run([sys.executable,'scripts/generate_pdf_note_continuation_probe.py','--check',str(target)],capture_output=True,text=True)
    assert result.returncode==0,result.stdout+result.stderr
    doc=json.loads(target.read_text());note=doc['notes'][0]
    expected=json.loads((root/'expected.json').read_text())
    parsed=PdfReader(source,strict=True)
    assert expected['body_paragraphs'][1] in ''.join(page.extract_text() for page in parsed.pages)
    with pymupdf.open(source) as pdf:
        assert expected['body_paragraphs'][1] in ''.join(page.get_text() for page in pdf)
        bottom=note['blocks'][1]['source']['quad'];box=bounds(bottom)
        spans=[s for b in pdf[0].get_text('dict')['blocks'] if 'lines' in b for line in b['lines'] for s in line['spans'] if s['bbox'][1]>=box[1]-.05 and s['bbox'][3]<=box[3]+.05]
        assert spans and all(span['flags']&2 for span in spans)
    report['authored_docx_derivative']={'note_paragraphs':2,'italic':True,'source_relationship':'authored OOXML; pinned original unchanged','strict_oracle':'passed unchanged'}
(args.root/'report.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report,indent=2))
