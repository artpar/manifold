"""Optional independent codec/source/geometry and atomic-rejection QA."""
import argparse
import json
from pathlib import Path
import subprocess
import zlib
from pypdf import PdfReader
import pymupdf

parser = argparse.ArgumentParser()
parser.add_argument('directory', type=Path)
parser.add_argument('--binary', type=Path, default=Path('bin/manifold'))
parser.add_argument('--require-pypdf-all', action='store_true')
args = parser.parse_args()
body = b'BT /F 10 Tf 20 300 Td (FILTERED) Tj ET'
positive = ['raw', 'flate', 'asciihex', 'ascii85', 'runlength', 'lzw0', 'lzw1',
            'chain', 'png', 'tiff', 'long-lzw0', 'long-lzw1']
reference = None
for name in positive:
    path = args.directory / (name + '.pdf')
    reader = PdfReader(path, strict=True)
    expected = body
    if name.startswith('long-'):
        expected = b'%' + bytes(33 + (i * i + i // 7) % 90 for i in range(30000)) + b'\n' + body
    pypdf_status = 'pass'
    try:
        decoded = reader.pages[0]['/Contents'].get_data()
    except Exception as error:
        if name != 'long-lzw0' or args.require_pypdf_all:
            raise
        # This pinned pypdf ignores EarlyChange. Keep its original strict oracle
        # executable; MuPDF below must independently decode every original byte.
        assert type(error).__name__ == 'PdfStreamError'
        pypdf_status = 'known EarlyChange0 transition failure: ' + str(error)
    else:
        assert decoded == expected, (name, decoded)
    with pymupdf.open(path) as doc:
        assert doc.xref_stream(doc[0].get_contents()[0]) == expected
        chars = [c for block in doc[0].get_text('rawdict')['blocks'] if 'lines' in block
                 for line in block['lines'] for span in line['spans'] for c in span['chars']]
    assert ''.join(c['c'] for c in chars) == 'FILTERED', name
    for index, c in enumerate(chars):
        assert abs(c['origin'][0] - (20 + index * 6)) < .001 and abs(c['origin'][1] - 100) < .001
    destination = args.directory / (name + '.sf')
    subprocess.run([str(args.binary), 'extract', '-pdf-skip-inference', '-format', 'sf',
                    '-o', str(destination), str(path)], capture_output=True, check=True)
    sf = json.loads(destination.read_text())
    if reference is None:
        reference = sf
    else:
        assert sf == reference, (name, 'complete SF source/geometry graph changed')
    print(name + ': MuPDF decoded bytes/origins/exact full SF pass; pypdf=' + pypdf_status)

for name in ['checksum', 'truncated-flate', 'truncated-runlength', 'runlength-no-eod',
             'ascii85-no-eod', 'asciihex-invalid', 'oversized', 'oversized-intermediate']:
    path = args.directory / (name + '.pdf')
    reader = PdfReader(path, strict=True)
    encoded = reader.pages[0]['/Contents']._data
    if name in ('checksum', 'truncated-flate'):
        try:
            zlib.decompress(encoded)
        except zlib.error:
            pass
        else:
            raise AssertionError('independent zlib corruption oracle did not fail')
    if name.startswith('oversized'):
        # Bound the independent verifier too: do not materialize the full bomb.
        limited = zlib.decompressobj().decompress(encoded, (64 << 20) + 1)
        assert len(limited) == (64 << 20) + 1
    destination = args.directory / (name + '-rejected.sf')
    assert not destination.exists(), 'use a fresh QA directory for rejection probes'
    result = subprocess.run([str(args.binary), 'extract', '-format', 'sf', '-o',
                             str(destination), str(path)], capture_output=True, text=True)
    assert result.returncode != 0 and not destination.exists(), (name, result.stderr)
    if name.startswith('oversized'):
        assert 'decoded stream limit exceeded' in result.stderr
    print(name + ': source corruption/size oracle and CLI atomic rejection pass')
