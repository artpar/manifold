"""Read-only native FriBidi/ICU verification of the wrapped-line reference fixture.

Optional development check; no native library is linked to or called by manydoc.
Run: python3 scripts/qa/layout/verify_bidi_levels.py
"""
import ctypes as c
import ctypes.util
import json
import os
from pathlib import Path

fixture = json.loads(Path('internal/bidilevels/testdata/lines.json').read_text())
U, I, L, B, H, P = c.c_uint32, c.c_int32, c.c_int8, c.c_uint8, c.c_uint16, c.POINTER
fribidi = c.CDLL(ctypes.util.find_library('fribidi'))
def bind(lib, name, args, result):
    fn = getattr(lib, name)
    fn.argtypes, fn.restype = args, result
    return fn
get_type = bind(fribidi, 'fribidi_get_bidi_type', [U], U)
get_types = bind(fribidi, 'fribidi_get_bidi_types', [P(U), I, P(U)], None)
get_brackets = bind(fribidi, 'fribidi_get_bracket_types', [P(U), I, P(U), P(U)], None)
resolve = bind(fribidi, 'fribidi_get_par_embedding_levels_ex', [P(U), P(U), I, P(U), P(L)], L)
reorder = bind(fribidi, 'fribidi_reorder_line', [U, P(U), I, I, U, P(L), P(U), P(I)], L)
icu_path = os.environ.get('MANYDOC_QA_ICU_LIBRARY') or ctypes.util.find_library('icuuc') or ctypes.util.find_library('icucore')
if not icu_path:
    raise RuntimeError('Install ICU or set MANYDOC_QA_ICU_LIBRARY to its shared library')
icu = c.CDLL(icu_path)
# ICU exports version-suffixed C symbols on common Linux/macOS distributions.
suffix = ''
if not hasattr(icu, 'ubidi_open'):
    suffix = next('_' + str(v) for v in range(100, 60, -1) if hasattr(icu, 'ubidi_open_' + str(v)))
def icu_fn(name, args, result):
    return bind(icu, name + suffix, args, result)
open_bidi = icu_fn('ubidi_open', [], c.c_void_p)
close_bidi = icu_fn('ubidi_close', [c.c_void_p], None)
set_para = icu_fn('ubidi_setPara', [c.c_void_p, P(H), I, B, P(B), P(I)], None)
set_line = icu_fn('ubidi_setLine', [c.c_void_p, I, I, c.c_void_p, P(I)], None)
get_levels = icu_fn('ubidi_getLevels', [c.c_void_p, P(I)], P(B))
get_map = icu_fn('ubidi_getVisualMap', [c.c_void_p, P(I), P(I)], None)
get_base = icu_fn('ubidi_getParaLevel', [c.c_void_p], B)
version = (B * 4)()
icu_fn('u_getVersion', [P(B)], None)(version)
print('ICU ' + '.'.join(str(v) for v in version) + ' from ' + icu._name)
count = 0
for row in fixture['cases']:
    text, base = row['text'], row['base']
    # This line corpus deliberately uses BMP scalars so the two native index
    # conventions coincide. The full Unicode character corpus covers astral text.
    assert all(ord(ch) < 0x10000 for ch in text)
    n = len(text)
    chars, types, brackets, levels = (U * n)(*map(ord, text)), (U * n)(), (U * n)(), (L * n)()
    get_types(chars, n, types)
    get_brackets(chars, n, types, brackets)
    direction = U(get_type(ord('א' if base == 1 else 'a' if base == 0 else '.')))
    assert resolve(types, brackets, n, c.byref(direction), levels) > 0
    assert direction.value & 1 == row['resolved_base']
    paragraph, status = open_bidi(), I()
    utf16 = (H * n)(*map(ord, text))
    set_para(paragraph, utf16, n, base if base >= 0 else 0xfe, None, c.byref(status))
    assert status.value <= 0 and get_base(paragraph) == row['resolved_base']
    try:
        for line in row['lines']:
            start, end = line['start'], line['end']
            size = end - start
            ls, order = (L * n)(*levels), (I * n)(*range(n))
            assert reorder(0, types, size, start, direction, ls, None, order) > 0
            x9 = {i for i in range(start, end) if line['levels'][i - start] < 0}
            assert [-1 if i in x9 else ls[i] for i in range(start, end)] == line['levels'], row['name']
            assert [i for i in list(order)[start:end] if i not in x9] == line['order'], row['name']
            child, status = open_bidi(), I()
            try:
                set_line(paragraph, start, end, child, c.byref(status))
                assert status.value <= 0
                il = get_levels(child, c.byref(status))
                assert status.value <= 0
                im = (I * size)()
                get_map(child, im, c.byref(status))
                assert status.value <= 0
                assert [-1 if i + start in x9 else il[i] for i in range(size)] == line['icu']['levels'], row['name']
                assert [i + start for i in im if i + start not in x9] == line['icu']['order'], row['name']
            finally:
                close_bidi(child)
            count += 1
    finally:
        close_bidi(paragraph)
print(c.c_char_p.in_dll(fribidi, 'fribidi_version_info').value.decode().splitlines()[0])
print(f'Preserved native FriBidi and ICU{suffix} observations: {len(fixture["cases"])} paragraphs / {count} wrapped lines')
print('Trailing-PDI FriBidi L1 difference and ICU even-level optimization remain recorded; Go tests check Unicode parity/order.')
