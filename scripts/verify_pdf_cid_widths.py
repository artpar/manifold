"""Independent source width/position/SF oracle; optional pypdf and PyMuPDF QA."""
import argparse
import json
from pathlib import Path
import subprocess
from pypdf import PdfReader
import pymupdf

parser = argparse.ArgumentParser()
parser.add_argument('directory', type=Path)
parser.add_argument('--binary', type=Path, default=Path('bin/manifold'))
parser.add_argument('--require-mupdf-agreement', action='store_true')
args = parser.parse_args()
for name in ['case-0', 'case-1', 'case-2', 'case-3', 'shared-array']:
    source = args.directory / (name + '.pdf')
    reader = PdfReader(source, strict=True)
    font = reader.pages[0]['/Resources']['/Font']['/F']
    assert font['/Encoding'] == '/Identity-H'
    descendant = font['/DescendantFonts'][0].get_object()
    entries = descendant['/W']
    widths, i, expanded = {}, 0, 0
    while i < len(entries):
        first = int(entries[i]); value = entries[i + 1].get_object(); i += 2
        if isinstance(value, list):
            assigned = [float(w.get_object()) for w in value]
            for j, w in enumerate(assigned):
                widths[first + j] = w
            expanded += len(assigned)
        else:
            last, width = int(value), float(entries[i]); i += 1
            # Independent oracle intentionally performs the source's assignments
            # directly; the Go parser uses a different bounded skipping algorithm.
            for code in range(first, last + 1):
                widths[code] = width
            expanded += last - first + 1
    assert len(widths) == 65536
    special = {1: 400, 2: 600, 3: 800, 65535: 1000}
    assert all(widths[c] == special.get(c, 500) for c in range(65536))
    with pymupdf.open(source) as doc:
        chars = [c for block in doc[0].get_text('rawdict')['blocks'] if 'lines' in block
                 for line in block['lines'] for span in line['spans'] for c in span['chars']]
    assert ''.join(c['c'] for c in chars) == 'ABCZA'
    expected_origins = [(x, 100) for x in [10, 14, 20, 28, 38]]
    observed_origins = [tuple(c['origin']) for c in chars]
    agrees = all(abs(a - b) < 0.001 for point, expected in zip(observed_origins, expected_origins, strict=True)
                 for a, b in zip(point, expected, strict=True))
    if name == 'case-3' or args.require_mupdf_agreement:
        assert agrees, (name, 'retained consumer geometry oracle', observed_origins, expected_origins)
    sf = args.directory / (name + '.sf')
    subprocess.run([str(args.binary), 'extract', '-pdf-skip-inference', '-format', 'sf',
                    '-o', str(sf), str(source)], check=True, capture_output=True)
    root = json.loads(sf.read_text())
    text_nodes = []
    def walk(node):
        if node['type'] == 'text':
            text_nodes.append(node)
        for child in node.get('content', []):
            walk(child)
    walk(root)
    assert [n['text'] for n in text_nodes] == ['ABCZ', 'A']
    sources = [next(m['attrs'] for m in n['marks'] if m['type'] == 'source') for n in text_nodes]
    precision = 'font_metrics' if name == 'case-3' else 'estimated_advance'
    assert all(s['page'] == 1 and s['stream'] == 'page:1' and s['precision'] == precision for s in sources)
    assert all(s['confidence'] == (1 if name == 'case-3' else 0.5) for s in sources)
    diagnostics = root['attrs'].get('diagnostics', [])
    assert any(d['code'] == 'pdf_cid_width_overlap' for d in diagnostics) == (name != 'case-3')
    assert sources[0]['baseline'] == [{'x': 10, 'y': 100}, {'x': 38, 'y': 100}]
    assert sources[1]['baseline'] == [{'x': 38, 'y': 100}, {'x': 42, 'y': 100}]
    print(f'{name}: {expanded} source assignments / 65536 exact legacy CIDs; SF source/uncertainty pass; MuPDF agreement={agrees}, origins={observed_origins}')
