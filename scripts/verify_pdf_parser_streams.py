"""Optional independent xref/member/source checks; never a runtime dependency."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import zlib
from pypdf import PdfReader
from pypdf.generic import IndirectObject
import pymupdf

parser = argparse.ArgumentParser()
parser.add_argument('directory', type=Path)
parser.add_argument('--binary', type=Path, default=Path('bin/manydoc'))
parser.add_argument('--generate-encrypted', action='store_true')
parser.add_argument('--require-encrypted', action='store_true', help='Compatibility alias: encrypted sources are always required when present')
parser.add_argument('--large', action='store_true')
args = parser.parse_args()
root = args.directory
catalog = b'<< /Type /Catalog /Pages 2 0 R >>'
font = b'<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>'

# A large, valid but mostly unused compressed member. The root stays direct, so
# its body can remain lazy until manydoc verifies the complete original stream.
if args.large:
    padding = 64 << 20
    body_offset = len(font) + padding + 3
    header = f'4 0 8 {body_offset} '.encode()
    compressor = zlib.compressobj()
    chunks = [compressor.compress(header + font + b'\n%')]
    block = b'A' * (1 << 20)
    for _ in range(64):
        chunks.append(compressor.compress(block))
    chunks.append(compressor.compress(b'\n42') + compressor.flush())
    encoded = b''.join(chunks)
    output = bytearray(b'%PDF-1.7\n')
    offsets = {}
    def add(number, body):
        offsets[number] = len(output)
        output.extend(f'{number} 0 obj\n'.encode() + body + b'\nendobj\n')
    def stream(dictionary, data):
        return b'<< ' + dictionary + f' /Length {len(data)} >>\nstream\n'.encode() + data + b'\nendstream'
    add(1, catalog)
    add(2, b'<< /Type /Pages /Kids [3 0 R] /Count 1 >>')
    add(3, b'<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> >> /Contents 5 0 R >>')
    add(5, stream(b'', b'BT /F 10 Tf 20 300 Td (COMPRESSED) Tj ET'))
    add(6, stream(f'/Type /ObjStm /N 2 /First {len(header)} /Filter /FlateDecode'.encode(), encoded))
    offsets[7] = len(output)
    entries = bytearray()
    for number in range(9):
        kind, field, generation = 1, offsets.get(number, 0), 0
        if number == 0:
            kind, field, generation = 0, 0, 65535
        elif number in (4, 8):
            kind, field, generation = 2, 6, int(number == 8)
        entries.extend(bytes([kind]) + field.to_bytes(4, 'big') + generation.to_bytes(2, 'big'))
    add(7, stream(b'/Type /XRef /Size 9 /Root 1 0 R /W [1 4 2]', bytes(entries)))
    output.extend(f'startxref\n{offsets[7]}\n%%EOF\n'.encode())
    (root / 'reject-oversized-lazy.pdf').write_bytes(output)
    # Verify actual expansion without retaining the whole decoded result.
    decoder = zlib.decompressobj()
    count = sum(len(decoder.decompress(encoded[i:i+256])) for i in range(0, len(encoded), 256))
    count += len(decoder.flush())
    assert count == len(header) + len(font) + padding + 5 and decoder.eof
    assert count > (64 << 20)
    print('large lazy source: independently counted', count, 'decoded bytes')

if args.generate_encrypted:
    for method, name in [(pymupdf.PDF_ENCRYPT_RC4_40, 'rc4-40'),
                         (pymupdf.PDF_ENCRYPT_RC4_128, 'rc4-128'),
                         (pymupdf.PDF_ENCRYPT_AES_128, 'aes128'),
                         (pymupdf.PDF_ENCRYPT_AES_256, 'aes256')]:
        target = root / ('encrypted-' + name + '.pdf')
        # Preserve exact failing ciphertext/file IDs across implementation changes.
        if target.exists():
            continue
        with pymupdf.open(root / 'raw.pdf') as doc:
            doc.save(target, encryption=method,
                     user_pw='reader', owner_pw='owner', permissions=0,
                     use_objstms=1, deflate=1)

reference = None
for path in sorted(root.glob('*.pdf')):
    name = path.stem
    if name.startswith('reject-'):
        if name.endswith('checksum'):
            reader = PdfReader(path, strict=True)
            number = 6 if name == 'reject-object-checksum' else 7
            encoded = reader.get_object(IndirectObject(number, 0, reader))._data
            try:
                zlib.decompress(encoded)
            except zlib.error:
                pass
            else:
                raise AssertionError('source checksum is not corrupt')
        elif name in ('reject-wrong-count', 'reject-wrong-header-owner', 'reject-zero-member', 'reject-vertical-tab', 'reject-count-too-large', 'reject-duplicate-member'):
            reader = PdfReader(path, strict=True)
            obj = reader.get_object(IndirectObject(6, 0, reader))
            header = obj.get_data()[:obj['/First']]
            if name == 'reject-wrong-count':
                assert obj['/N'] == 1 and len(header.split()) == 4
            elif name == 'reject-wrong-header-owner':
                assert int(header.split()[0]) == 9 and reader.xref_objStm[1] == (6, 0)
            elif name == 'reject-zero-member':
                assert header.startswith(b'0 0 ')
            elif name == 'reject-count-too-large':
                assert reader.get_object(IndirectObject(6, 0, reader))['/N'] == 3
            elif name == 'reject-duplicate-member':
                data = reader.get_object(IndirectObject(6, 0, reader)).get_data()
                assert data.startswith(b'1 0 1 34 ')
            else:
                assert b'\x0b' in header  # Not PDF whitespace.
        destination = path.with_suffix('.rejected.sf')
        destination.write_text('original destination')
        result = subprocess.run([str(args.binary), 'extract', '-format', 'sf', '-o',
                                 str(destination), str(path)], capture_output=True, text=True)
        assert result.returncode and destination.read_text() == 'original destination', (name, result.stderr)
        if name == 'reject-oversized-lazy':
            assert 'decoded stream limit exceeded' in result.stderr, result.stderr
        print(name + ': original corruption/header/size oracle and atomic CLI rejection pass')
        continue
    encrypted = name.startswith('encrypted-')
    reader = PdfReader(path, password='reader' if encrypted else None, strict=True)
    assert len(reader.pages) == 1 and reader.pages[0].extract_text() == 'COMPRESSED'
    if not encrypted:
        obj = reader.get_object(IndirectObject(6, 0, reader))
        data = obj.get_data()
        prolog = re.sub(rb'%[^\r\n]*', b'', data[:obj['/First']]).replace(b'\x00', b' ')
        assert list(map(int, prolog.split())) == [1, 0, 4, 34]
        assert obj['/N'] == 2 and data[obj['/First']:] == catalog + b' ' + font
        assert reader.xref_objStm[1] == (6, 0)
        if name != 'incremental':
            assert reader.xref_objStm[4] == (6, 1)
    with pymupdf.open(path) as doc:
        if encrypted:
            assert doc.authenticate('reader')
        chars = [c for block in doc[0].get_text('rawdict')['blocks'] if 'lines' in block
                 for line in block['lines'] for span in line['spans'] for c in span['chars']]
        assert ''.join(c['c'] for c in chars) == 'COMPRESSED'
        for i, c in enumerate(chars):
            assert abs(c['origin'][0] - (20+6*i)) < .001 and abs(c['origin'][1]-100) < .001
    destination = path.with_suffix('.sf')
    command = [str(args.binary), 'extract', '-pdf-skip-inference', '-format', 'sf', '-o',
               str(destination), str(path)]
    if encrypted:
        command[2:2] = ['-password-env', 'MANYDOC_QA_PASSWORD']
    result = subprocess.run(command, capture_output=True, text=True,
                            env=dict(os.environ, MANYDOC_QA_PASSWORD='reader'))
    assert result.returncode == 0, (name, result.stderr)
    sf = json.loads(destination.read_text())
    assert len(sf['content']) == 1 and sf['content'][0]['type'] == 'paragraph'
    run = sf['content'][0]['content'][0]
    assert run['text'] == 'COMPRESSED'
    style = next(m['attrs'] for m in run['marks'] if m['type'] == 'textStyle')
    source = next(m['attrs'] for m in run['marks'] if m['type'] == 'source')
    assert style['fontFamily'] == 'Courier' and style['fontSize'] == 10
    assert source['font'] == ('Courier-Bold' if name == 'incremental' else 'Courier')
    assert source['baseline'] == [{'x': 20, 'y': 100}, {'x': 80, 'y': 100}]
    assert sf['attrs']['pages'][0]['media_box'] == [0, 0, 300, 400]
    if name == 'incremental':
        assert any(mark['type'] == 'strong' for mark in sf['content'][0]['content'][0]['marks'])
    elif encrypted:
        assert reference is not None and sf == reference, (name, 'complete decrypted SF graph changed')
    elif encrypted:
        assert reference is not None and sf == reference, (name, 'encrypted full SF source graph changed')
    else:
        if reference is None:
            reference = sf
        else:
            assert sf == reference, (name, 'complete SF source graph changed')
    print(name + ': source identities, original members, independent glyph geometry and CLI SF pass')
