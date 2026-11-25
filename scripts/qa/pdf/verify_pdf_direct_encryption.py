"""Optional independent encrypted-source, asset, structure and password QA."""
import argparse
import json
import io
import os
from pathlib import Path
import subprocess
from pypdf import PdfReader
from pypdf.generic import IndirectObject
import pymupdf

parser = argparse.ArgumentParser()
parser.add_argument('directory', type=Path)
parser.add_argument('--binary', type=Path, default=Path('bin/manifold'))
parser.add_argument('--generate-mupdf', action='store_true')
parser.add_argument('--require-pypdf-prefix', action='store_true')
parser.add_argument('--require-mupdf-repairs', action='store_true')
args = parser.parse_args()
root = args.directory
if args.generate_mupdf:
    for method, label in [(pymupdf.PDF_ENCRYPT_RC4_40, 'rc4-40'),
                          (pymupdf.PDF_ENCRYPT_RC4_128, 'rc4-128'),
                          (pymupdf.PDF_ENCRYPT_AES_128, 'aes128'),
                          (pymupdf.PDF_ENCRYPT_AES_256, 'aes256')]:
        target = root / ('mupdf-' + label + '-encrypted.pdf')
        if not target.exists():
            with pymupdf.open(root / 'rich-true-clear.pdf') as doc:
                doc.save(target, encryption=method, user_pw='reader', owner_pw='owner',
                         permissions=0, use_objstms=1, deflate=1)

def extract(path, password=None):
    destination = path.with_suffix('.sf')
    command = [str(args.binary), 'extract', '-pdf-skip-inference', '-format', 'sf',
               '-o', str(destination), str(path)]
    if password is not None:
        command[2:2] = ['-password-env', 'MANYDOC_QA_PASSWORD']
    result = subprocess.run(command, capture_output=True, text=True,
                            env=dict(os.environ, MANYDOC_QA_PASSWORD=password or ''))
    assert result.returncode == 0, (path, result.stderr)
    return json.loads(destination.read_text())

controls = {mode: extract(root / ('rich-' + mode + '-clear.pdf')) for mode in ['false', 'true']}
assert controls['false'] == controls['true'], 'xref encoding changed control SF'
for path in sorted(root.glob('*-encrypted.pdf')):
    original = path.read_bytes()
    header = original.find(b'%PDF-')
    if header > 0:
        try:
            strict_reader = PdfReader(path, password='reader', strict=True)
            assert len(strict_reader.pages) == 1
            assert strict_reader.pages[0].extract_text().strip() == 'COMPRESSEDBOLD'
        except Exception:
            if args.require_pypdf_prefix:
                raise
            print(path.stem + ': pypdf original-prefix oracle fails; MuPDF checks original below, pypdf checks canonical view')
        reader = PdfReader(io.BytesIO(original[header:]), password='reader', strict=True)
    else:
        reader = PdfReader(path, password='reader', strict=True)
    assert reader.is_encrypted and len(reader.pages) == 1
    assert reader.metadata.title == 'OWNED METADATA'
    page = reader.pages[0]
    assert list(page.mediabox) == [0, 0, 300, 400]
    assert page.extract_text().strip() == 'COMPRESSEDBOLD'
    assert str(page['/Annots'][0].get_object()['/A']['/URI']) == 'https://example.test/owned'
    image = reader.get_object(IndirectObject(10, 0, reader))
    assert image['/Width'] == 2 and image['/Height'] == 1
    assert image.get_data() == bytes.fromhex('ff000000ff00')
    fonts = {str(v.get_object()['/BaseFont']) for v in page['/Resources']['/Font'].values()}
    assert fonts == {'/Courier', '/Courier-Bold'}, (path, fonts)
    def check_mupdf(view):
        with pymupdf.open(view) as doc:
            assert doc.authenticate('reader')
            assert not (doc.permissions & pymupdf.PDF_PERM_COPY)
            assert doc[0].get_links()[0]['uri'] == 'https://example.test/owned'
            rects = doc[0].get_image_rects(10)
            assert [tuple(r) for r in rects] == [(20, 149, 22, 150)], (path, rects)
            chars = [c for block in doc[0].get_text('rawdict')['blocks'] if 'lines' in block
                     for line in block['lines'] for span in line['spans'] for c in span['chars']]
            text = ''.join(c['c'] for c in chars)
            assert text.replace(' ', '') == 'COMPRESSEDBOLD', (path, text)
            assert abs(chars[0]['origin'][0]-20) < .001 and abs(chars[0]['origin'][1]-100) < .001
    try:
        check_mupdf(path)
    except (AssertionError, RuntimeError) as error:
        if args.require_mupdf_repairs or not ('-inherited-' in path.stem or '-prefix-' in path.stem):
            raise
        print(path.stem + ': MuPDF original repair-shape oracle fails: ' + str(error))
        view = path.with_name(path.stem.replace('-encrypted', '-adapted') + '.pdf')
        assert view.exists(), view
        check_mupdf(view)
    for password in ['reader', 'owner']:
        assert extract(path, password) == controls['true'], (path, password, 'full SF/asset/source graph changed')
    for password in ['', 'wrong']:
        destination = path.with_suffix('.rejected.sf')
        destination.write_text('unchanged destination')
        result = subprocess.run([str(args.binary), 'extract', '-password-env', 'MANYDOC_QA_PASSWORD',
                                 '-format', 'sf', '-o', str(destination), str(path)],
                                capture_output=True, text=True,
                                env=dict(os.environ, MANYDOC_QA_PASSWORD=password))
        assert result.returncode and destination.read_text() == 'unchanged destination', (path, password)
    assert path.read_bytes() == original, 'input source mutated'
    print(path.stem + ': independent metadata/fonts/links/pixels/positions, exact full SF and password/atomic checks pass')

for mode in ['false', 'true']:
    original = root / ('rich-' + mode + '-encrypted.pdf')
    adapted = root / ('rich-' + mode + '-adapted.pdf')
    assert adapted.read_bytes().startswith(original.read_bytes())
    before = PdfReader(original, password='reader', strict=True)
    after = PdfReader(adapted, password='reader', strict=True)
    assert isinstance(before.trailer.raw_get('/Encrypt'), dict)
    assert isinstance(after.trailer.raw_get('/Encrypt'), IndirectObject)
    assert before.trailer['/Encrypt'] == after.trailer['/Encrypt']
    assert before.trailer['/ID'] == after.trailer['/ID']
    assert before.trailer.raw_get('/Root') == after.trailer.raw_get('/Root') or (
        before.trailer.raw_get('/Root').idnum == after.trailer.raw_get('/Root').idnum)
    for number in [5, 6, 10]:
        assert before.get_object(IndirectObject(number, 0, before)).get_data() == after.get_object(IndirectObject(number, 0, after)).get_data()
    assert extract(adapted, 'reader') == controls['true']
    print(mode + ': independently checked adapted trailer, unchanged source prefix/file IDs/encryption values and object payloads')
