"""Independent original Word package oracle; never reads manydoc output."""
import argparse
import hashlib
import json
import zipfile
from pathlib import Path
from lxml import etree

parser = argparse.ArgumentParser()
parser.add_argument('--source', type=Path, default=Path('testdata/real/mqtt-v5.0-os.docx'))
parser.add_argument('--output', type=Path)
parser.add_argument('--compare', type=Path, help='compare normalized structure/content/link/media facts with an original oracle')
a = parser.parse_args()
ns = {'w': 'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}
W = '{'+ns['w']+'}'


def normalize(text):
    return ' '.join(text.split())


def paragraph(p):
    parts = []
    for el in p.iter():
        if el.tag == W+'t':
            parts.append(el.text or '')
        elif el.tag in (W+'tab', W+'br', W+'cr'):
            parts.append(' ')
        elif el.tag == W+'noBreakHyphen':
            parts.append('\u2011')
        elif el.tag == W+'softHyphen':
            parts.append('\xad')
    return normalize(''.join(parts))


def digest(text):
    return hashlib.sha256(text.encode()).hexdigest()


raw = a.source.read_bytes()
with zipfile.ZipFile(a.source) as package:
    root = etree.fromstring(package.read('word/document.xml'))
    styles = etree.fromstring(package.read('word/styles.xml'))
    style_map = {s.get(W+'styleId'): s for s in styles.findall('w:style', ns)}
    def heading(pr, active=()):
        if pr is None:
            return False
        own = pr.find('w:outlineLvl', ns)
        if own is not None:
            return int(own.get(W+'val')) < 9
        pstyle = pr.find('w:pStyle', ns)
        if pstyle is not None:
            key = pstyle.get(W+'val')
            assert key not in active
            style = style_map.get(key)
            if style is None:
                return False
            spr = style.find('w:pPr', ns)
            if spr is not None and spr.find('w:outlineLvl', ns) is not None:
                return heading(spr, active+(key,))
        return False
    paragraphs = [paragraph(p) for p in root.findall('.//w:p', ns)]
    paragraphs = [p for p in paragraphs if p]
    tables = []
    for table in root.findall('.//w:tbl', ns):
        rows = table.findall('w:tr', ns)
        assert table.find('.//w:tbl', ns) is None
        active = {}
        cells = []
        header_rows = 0
        for ri, row in enumerate(rows):
            if row.find('w:trPr/w:tblHeader', ns) is not None:
                assert header_rows == ri
                header_rows += 1
            col = 0
            next_active = {}
            for cell in row.findall('w:tc', ns):
                span = cell.find('w:tcPr/w:gridSpan', ns)
                width = int(span.get(W+'val')) if span is not None else 1
                merge = cell.find('w:tcPr/w:vMerge', ns)
                if merge is not None and merge.get(W+'val', 'continue') == 'continue':
                    assert col in active
                    origin = active[col]
                    assert origin['colspan'] == width
                    assert not any(paragraph(p) for p in cell.findall('w:p', ns))
                    origin['rowspan'] += 1
                    next_active[col] = origin
                else:
                    text = '\n'.join(t for p in cell.findall('w:p', ns) if (t := paragraph(p)))
                    origin = {'row': ri, 'column': col, 'colspan': width, 'rowspan': 1, 'text_sha256': digest(text)}
                    cells.append(origin)
                    if merge is not None:
                        assert merge.get(W+'val') == 'restart'
                        next_active[col] = origin
                col += width
            active = next_active
        tables.append({'rows': len(rows), 'header_rows': header_rows, 'cells': cells})
    media = {name: {'bytes': len(package.read(name)), 'sha256': hashlib.sha256(package.read(name)).hexdigest()} for name in package.namelist() if name.startswith('word/media/')}
    relationships = etree.fromstring(package.read('word/_rels/document.xml.rels'))
    targets = {r.get('Id'): r.get('Target') for r in relationships}
    linked_text = {}
    for hyperlink in root.findall('.//w:hyperlink', ns):
        rid = hyperlink.get('{http://schemas.openxmlformats.org/officeDocument/2006/relationships}id')
        target = targets[rid] if rid else '#'+hyperlink.get(W+'anchor', '')
        visible = ''.join(paragraph(hyperlink).split())
        if visible:
            linked_text[target] = linked_text.get(target, '')+visible
    hyperlink_oracle = {target: digest(text) for target, text in sorted(linked_text.items())}
    result = {'source_sha256': hashlib.sha256(raw).hexdigest(), 'source_bytes': len(raw), 'oracle': 'Original ZIP/XML parsed with independent lxml, not manydoc; text compares normalized whitespace while retaining paragraph/cell membership', 'nonempty_body_paragraphs': len(paragraphs), 'body_paragraph_multiset_sha256': digest('\n'.join(sorted(paragraphs))), 'headings': sum(heading(p.find('w:pPr', ns)) for p in root.findall('.//w:p', ns)), 'sections': len(root.findall('.//w:sectPr', ns)), 'tables': tables, 'media': media, 'hyperlink_text_sha256': hyperlink_oracle}
if a.compare:
    expected = json.loads(a.compare.read_text())
    for key in ('nonempty_body_paragraphs', 'body_paragraph_multiset_sha256', 'headings', 'sections', 'tables', 'hyperlink_text_sha256'):
        assert result[key] == expected[key], key
    assert sorted((v['bytes'], v['sha256']) for v in result['media'].values()) == sorted((v['bytes'], v['sha256']) for v in expected['media'].values())
    print('Independent normalized structure/content/link/media comparison passed')
if a.output:
    a.output.write_text(json.dumps(result, indent=2)+'\n')
print(json.dumps({k: v for k, v in result.items() if k not in ('tables', 'media', 'hyperlink_text_sha256')}, indent=2))
print('tables', len(tables), 'rows', sum(t['rows'] for t in tables), 'origin cells', sum(len(t['cells']) for t in tables), 'media', media)
