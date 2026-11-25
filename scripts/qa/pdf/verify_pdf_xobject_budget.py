"""Independent shared-Form graph counting and public CLI atomic-rejection QA.
Requires optional pypdf; generated fixtures are synthetic and stay in output/.
"""
import argparse
from pathlib import Path
import subprocess
from pypdf import PdfReader
from pypdf.generic import ContentStream

parser = argparse.ArgumentParser()
parser.add_argument('directory', type=Path)
parser.add_argument('--binary', type=Path, default=Path('bin/manifold'))
args = parser.parse_args()
expected = {'acyclic-shared-form': 299593, 'shared-across-pages': 224694,
            'shared-appearances': 224694}
for name, wanted in expected.items():
    source = args.directory / (name + '.pdf')
    assert source.stat().st_size < 6000
    reader = PdfReader(source, strict=True)
    memo = {}
    active = set()
    def form(ref, parent):
        key = (ref.idnum, ref.generation)
        assert key not in active, 'oracle fixture must be acyclic'
        if key in memo:
            return memo[key]
        active.add(key)
        obj = ref.get_object()
        resources = obj.get('/Resources', parent)
        children, depth = 0, 1
        for operands, operator in ContentStream(obj, reader).operations:
            if operator == b'Do':
                target = resources['/XObject'].raw_get(operands[0])
                count, child_depth = form(target, resources)
                children += count
                depth = max(depth, child_depth + 1)
        active.remove(key)
        memo[key] = (children + 1, depth)
        return memo[key]
    total, depth = 0, 0
    for page in reader.pages:
        resources = page['/Resources']
        operations = ContentStream(page['/Contents'], reader).operations
        assert any(op == b'Tj' and str(operands[0]) == 'KEEP' for operands, op in operations)
        for operands, op in operations:
            if op == b'Do':
                count, height = form(resources['/XObject'].raw_get(operands[0]), resources)
                total += count
                depth = max(depth, height)
        for ref in page.get('/Annots', []):
            count, height = form(ref.get_object()['/AP'].raw_get('/N'), resources)
            total += count
            depth = max(depth, height)
    assert total == wanted, (name, total, wanted)
    assert 2 * depth < 32, 'must test expansion below the existing nesting guard'
    destination = args.directory / (name + '-rejected.sf')
    assert not destination.exists(), 'use a fresh QA output directory'
    result = subprocess.run([str(args.binary), 'extract', '-format', 'sf', '-o',
                             str(destination), str(source)], capture_output=True, text=True)
    assert result.returncode != 0 and 'XObject expansion exceeds 200000 visits' in result.stderr
    assert not destination.exists(), 'partial extraction was committed'
    print(f'{name}: {source.stat().st_size} bytes / {len(memo)} unique Forms / '
          f'{total} invocations / depth{depth}; CLI rejects atomically')
