"""Independent raster/source/placement and atomic-rejection QA; optional tools only."""
import argparse
import json
import io
import zlib
from pathlib import Path
import subprocess
from PIL import Image
from pypdf import PdfReader
import pymupdf

parser = argparse.ArgumentParser()
parser.add_argument('directory', type=Path)
parser.add_argument('--binary', type=Path, default=Path('bin/manydoc'))
parser.add_argument('--require-cmyk-agreement', action='store_true')
parser.add_argument('--require-mupdf-jpeg', action='store_true')
args = parser.parse_args()
expected = {
    'gray1': bytes([0xa0]), 'gray2': bytes([0x1c]),
    'rgb4': bytes([255 & 0xf0, 0, 0xf0]),
    'rgb8decode': bytes([255, 0, 255]),
    'rgb16': bytes.fromhex('123456789abc'),
    'gray16decode': bytes.fromhex('1234'), 'cmyk4': bytes.fromhex('0ff0'),
}
for path in sorted(args.directory.glob('*.pdf')):
    name = path.stem
    if name.startswith('reject-'):
        if name == 'reject-oversized-image':
            image = PdfReader(path, strict=True).pages[0]['/Resources']['/XObject']['/X'].get_object()
            decoder = zlib.decompressobj()
            encoded = image._data
            count = sum(len(decoder.decompress(encoded[i:i+256])) for i in range(0, len(encoded), 256))
            count += len(decoder.flush())
            assert count == 200_000_001 and decoder.eof, count
        dest = path.with_suffix('.rejected.sf')
        dest.write_text('unchanged destination')
        result = subprocess.run([str(args.binary), 'extract', '-format', 'sf', '-o',
                                 str(dest), str(path)], capture_output=True, text=True)
        assert result.returncode and dest.read_text() == 'unchanged destination', name
        print(name + ': CLI rejected without changing destination')
        continue
    source = PdfReader(path, strict=True).pages[0]['/Resources']['/XObject']['/X'].get_object()
    base = name.rsplit('-', 1)[0]
    if base in expected:
        assert source.get_data() == expected[base], name
    if name == 'rgb16-mask':
        assert source.get_data() == bytes.fromhex('123456789abc')
        assert source['/SMask'].get_object().get_data() == bytes.fromhex('8012')
    if name == 'indexed':
        assert source.get_data() == bytes([0x40])
        assert source['/ColorSpace'][3].get_object().get_data() == bytes.fromhex('ff000000ff00')
    asset = Image.open(path.with_suffix('.png')).convert('RGBA')
    if name == 'jpeg-chain':
        jpeg = Image.open(io.BytesIO(source.get_data())).convert('RGB')
        for x in range(jpeg.width):
            assert max(abs(a-b) for a,b in zip(jpeg.getpixel((x,0)), asset.getpixel((x,0))[:3])) <= 1

    with pymupdf.open(path) as doc:
        rects = doc[0].get_image_rects(6)
        want = [(10, 350, 50, 370)]
        if name == 'indexed':
            want.append((100, 300, 120, 310))
        assert [tuple(r) for r in rects] == want, (name, rects)
        pix = doc[0].get_pixmap(alpha=False)
        for x in range(asset.width):
            # Compare every source sample at its displayed pixel's center.
            px = int(10 + (x + .5) * 40 / asset.width)
            offset = (360 * pix.width + px) * pix.n
            got = tuple(pix.samples[offset:offset+3])
            rgba = asset.getpixel((x, 0))
            composite = tuple(round(c * rgba[3] / 255 + 255 - rgba[3]) for c in rgba[:3])
            if name.startswith('cmyk4') and not args.require_cmyk_agreement:
                assert got == (237, 28, 36) and composite == (255, 0, 0), (name, got, composite)
            elif name == 'jpeg-chain' and not args.require_mupdf_jpeg:
                assert got != composite, 'expected preserved chroma interpolation discrepancy disappeared'
            else:
                assert max(abs(a-b) for a, b in zip(got, composite)) <= 1, (name, x, got, composite)
        if name == 'rgb16-mask':
            pix.save(str(path.with_suffix('.source-render.png')))
    dest = path.with_suffix('.sf')
    subprocess.run([str(args.binary), 'extract', '-pdf-skip-inference', '-format', 'sf',
                    '-o', str(dest), str(path)], capture_output=True, check=True)
    sf = json.loads(dest.read_text())
    if name.startswith('cmyk4'):
        assert 'pdf_image_color' in dest.read_text(), name
        print(name + ': original CMYK samples/placements pass; explicit device-color approximation differs from MuPDF')
    elif name == 'jpeg-chain':
        print(name + ': Pillow JPEG pixels within one level; MuPDF chroma interpolation differs, original strict oracle retained')
    else:
        print(name + ': independent source samples/pixels/placements and CLI SF pass')
