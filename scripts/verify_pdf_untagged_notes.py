"""Optional original-export note QA; never used by the pure-Go runtime.

Requires original-render PDFs from docs/REAL_DOCX_CORPUS.md, pypdf,
PyMuPDF in the optional QA environment.
"""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import zipfile
import xml.etree.ElementTree as ET
from pypdf import PdfReader, PdfWriter
import pymupdf

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--cli', default='bin/manydoc')
parser.add_argument('--sources', type=Path, default=Path('output/real-docx-audit/original-render'))
parser.add_argument('--out', type=Path, default=Path('output/pdf-untagged-notes'))
parser.add_argument('--word-render', type=Path, help='optional independently LibreOffice-rendered DOCX PDFs')
args = parser.parse_args()
args.out.mkdir(parents=True, exist_ok=True)
ns = {'w': 'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}
report = {}

def command(*words):
    result = subprocess.run([args.cli, *map(str, words)], capture_output=True, text=True)
    assert result.returncode == 0, (words, result.stderr)

def extract(path, stem, flags=()):
    out = args.out / (stem+'.json')
    command('extract', *flags, '-format', 'json', '-o', out, path)
    return json.loads(out.read_text())

def runs(blocks):
    for b in blocks:
        yield from b.get('runs', [])
        for item in b.get('items', []): yield from runs(item['blocks'])
        for row in b.get('rows', []):
            for cell in row['cells']: yield from runs(cell['blocks'])

def physical(d):
    values = []
    for run in runs(d['blocks']):
        s = run.get('source', {})
        if 'quad' in s:
            values.append((s['page'], s['order'], run.get('text', ''), s['quad'], s.get('baseline'), s.get('matrix'), s.get('font'), s.get('font_size')))
    return sorted(values, key=lambda x:(x[0], x[1]))

def recovered_physical(d):
    result = physical(d)
    for note in d.get('notes', []):
        result += physical({'blocks': note['blocks']})
        s = note['label_source']
        result.append((s['page'],s['order'],note['label'],s['quad'],s.get('baseline'),s.get('matrix'),s.get('font'),s.get('font_size')))
    labels = {n['id']: n['label'] for n in d.get('notes', [])}
    note_orders = {r['source']['order']: labels[r['note']['id']] for r in runs(d['blocks']) if 'note' in r}
    return sorted([(p,o,note_orders.get(o,t),q,b,m,f,z) for p,o,t,q,b,m,f,z in result],key=lambda x:(x[0],x[1]))

def nav_source(d):
    n = json.loads(json.dumps(d['pdf_navigation']))
    for dest in n['destinations']:
        dest.pop('flow_binding',None); dest.pop('flow_order',None)
    return n

for name, kind, label in [('footnotes','footnote','1'),('endnotes','endnote','i')]:
    source = args.sources / (name+'.pdf')
    digest = hashlib.sha256(source.read_bytes()).hexdigest()
    original = PdfReader(source, strict=True)
    # Actual DOCX body text and reference ownership are independent XML facts.
    with zipfile.ZipFile(Path('testdata/real')/('poi-'+name+'.docx')) as archive:
        story = ET.fromstring(archive.read('word/'+name+'.xml'))
        definitions = [n for n in story if n.get('{'+ns['w']+'}type', 'normal') == 'normal']
        assert len(definitions)==1
        xmlbody = ''.join(t.text or '' for t in definitions[0].findall('.//w:t', ns)).strip()
        main = ET.fromstring(archive.read('word/document.xml'))
        assert len(main.findall('.//w:'+kind+'Reference',ns))==1
    # Preserve the original; remove only the logical-tree catalog entry in a QA derivative.
    writer = PdfWriter(clone_from=original)
    del writer.root_object['/StructTreeRoot']
    untagged = args.out/(name+'-untagged.pdf')
    writer.write(untagged)
    assert '/StructTreeRoot' not in PdfReader(untagged,strict=True).trailer['/Root']
    for path, suffix, flags in [(source,'original-geometric',('-pdf-skip-structure',)),(untagged,'untagged',())]:
        stem = name+'-'+suffix
        d = extract(path,stem,flags)
        physical_d = extract(path,stem+'-physical',('-pdf-skip-structure','-pdf-skip-inference'))
        assert len(d.get('notes',[]))==1, stem
        note=d['notes'][0]
        refs=[r for r in runs(d['blocks']) if 'note' in r]
        assert note['kind']==kind and note['kind_inferred'] and note['label']==label
        assert len(refs)==1 and refs[0]['note']['inferred'] and refs[0]['note']['id']==note['id']
        assert note['source']['structure']=='inferred' and refs[0]['source']['evidence']=='pdf_note_reciprocal_views'
        assert ''.join(r.get('text','') for r in runs(note['blocks'])).strip()==xmlbody, (stem,xmlbody)
        assert recovered_physical(d)==physical(physical_d), (stem,'glyph/source changed or duplicated')
        assert nav_source(d)==nav_source(physical_d), (stem,'physical navigation changed')
        # Independent readers retain identical visual characters and both reciprocal annotations.
        with pymupdf.open(path) as pdf:
            glyphs=[c for page in pdf for b in page.get_text('rawdict')['blocks'] if 'lines' in b for line in b['lines'] for span in line['spans'] for c in span['chars']]
            assert xmlbody in ''.join(c['c'] for c in glyphs)
            assert sum(len(page.get_links()) for page in pdf)==2
            for mark, marksource in [(label,note['label_source']),(label,refs[0]['source'])]:
                point=marksource['baseline'][0]
                matching=[c for c in glyphs if c['c']==mark and abs(c['origin'][0]-point['x'])<.02 and abs(c['origin'][1]-point['y'])<.02]
                assert len(matching)==1, (stem,'independent mark position',mark,point)
        parsed=PdfReader(path,strict=True)
        assert sum(len(page.get('/Annots',[])) for page in parsed.pages)==2
        assert xmlbody in ''.join(page.extract_text() for page in parsed.pages)
        # Reflow both formats from the recovered semantic graph; QA font choice is explicit.
        sf=args.out/(stem+'.sf')
        command('extract',*flags,'-format','sf','-o',sf,path)
        docx=args.out/(stem+'.docx')
        command('convert','-allow-lossy','-o',docx,sf)
        with zipfile.ZipFile(docx) as z:
            tree=ET.fromstring(z.read('word/'+name+'.xml'))
            assert xmlbody in ''.join(t.text or '' for t in tree.findall('.//w:t',ns))
            tree=ET.fromstring(z.read('word/document.xml'))
            assert len(tree.findall('.//w:'+kind+'Reference',ns))==1
        reread=extract(docx,stem+'-docx')
        assert len(reread['notes'])==1 and reread['notes'][0]['kind_inferred']
        rr=[r for r in runs(reread['blocks']) if 'note' in r]
        assert len(rr)==1 and rr[0]['note']['inferred'] and rr[0]['note']['id']==reread['notes'][0]['id']
        if args.word_render:
            wordpdf=args.word_render/(stem+'.pdf')
            wordreader=PdfReader(wordpdf,strict=True)
            expected_pages=1 if kind=='footnote' else 2
            assert len(wordreader.pages)==expected_pages
            assert xmlbody in ''.join(page.extract_text() for page in wordreader.pages)
            with pymupdf.open(wordpdf) as worddoc:
                assert len(worddoc)==expected_pages
                assert xmlbody in ''.join(page.get_text() for page in worddoc)
                for i,page in enumerate(worddoc): page.get_pixmap().save(args.out/(stem+'-word-page-'+str(i+1)+'.png'))
        outpdf=args.out/(stem+'-reflow.pdf')
        fontroot=Path('output/font-subset/dejavu')
        faces=[('', 'DejaVuSans.ttf'),('/B','DejaVuSans-Bold.ttf'),('/I','DejaVuSans-Oblique.ttf'),('/BI','DejaVuSans-BoldOblique.ttf')]
        fontargs=[]
        for face,filename in faces: fontargs += ['-pdf-font','DejaVu Sans'+face+'='+str(fontroot/filename)]
        families={r['font_family'] for r in runs(d['blocks']) if r.get('font_family')}
        families|={r['font_family'] for r in runs(note['blocks']) if r.get('font_family')}
        for family in sorted(families): fontargs += ['-pdf-font-map',family+'=DejaVu Sans']
        command('convert','-allow-lossy',*fontargs,'-o',outpdf,sf)
        with pymupdf.open(outpdf) as pdf:
            assert len(pdf)==(1 if kind=='footnote' else 2), (stem,'unexpected blank reflow page',len(pdf))
            assert any(link.get('page')==len(pdf)-1 for page in pdf for link in page.get_links()), (stem,'citation target does not own note body')
            assert xmlbody in ''.join(page.get_text() for page in pdf)
            assert sum(len(page.get_links()) for page in pdf)>=1
            page=pdf[0] if kind=='footnote' else pdf[-1]
            page.get_pixmap(matrix=pymupdf.Matrix(1,1)).save(args.out/(stem+'-reflow.png'))
            for i,p in enumerate(pdf): p.get_pixmap().save(args.out/(stem+'-page-'+str(i+1)+'.png'))
        assert xmlbody in ''.join(page.extract_text() for page in PdfReader(outpdf,strict=True).pages)
    assert hashlib.sha256(source.read_bytes()).hexdigest()==digest
    report[name]={'original_sha256':digest,'original_pages':len(original.pages),'original_docx_note':xmlbody,'word_reader_checked':bool(args.word_render),'profiles':['unchanged-original-skip-structure','untagged-catalog-derivative'],'checks':'exact physical glyph/font/geometry/navigation; inferred graph/label/kind; independent source text/annotations; DOCX XML and reread; PDF pypdf/MuPDF and render'}
(args.out/'report.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report,indent=2))
