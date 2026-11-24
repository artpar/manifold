"""Independent rich-label layout/structure checks; optional pypdf/Pillow QA.
Generate using MANYDOC_LITERAL_OUTPUT_QA=output/literal-output go test -run TestPDFLiteralMarker.
This verifies physical PDF operators and actual standard structure, not embedded SF.
"""
from collections import Counter
from pathlib import Path
from pypdf import PdfReader
from pypdf.generic import ContentStream, IndirectObject
root = Path('output/literal-output')

def walk(ref):
    if isinstance(ref, list):
        for child in ref: yield from walk(child)
    elif isinstance(ref, IndirectObject):
        node = ref.get_object()
        if node.get('/Type') == '/StructElem':
            yield node
            yield from walk(node.get('/K', []))

def descendants(node):
    return list(walk(node.get('/K', [])))

for name in ['rich', 'continued', 'nested', 'running', 'narrow-rtl']:
    reader = PdfReader(root/(name+'.pdf'), strict=True)
    nodes = list(walk(reader.trailer['/Root']['/StructTreeRoot'].raw_get('/K')))
    roles = Counter(n['/S'] for n in nodes)
    labels = [n for n in nodes if n['/S'] == '/Lbl' and n['/P']['/S'] == '/LI']
    if name in ['rich', 'continued']:
        assert roles['/L'] == 1 and roles['/LI'] == roles['/LBody'] == 3
        assert len(labels) == 3 and roles['/Table'] == 0, 'layout table leaked into logical hierarchy'
        assert roles['/FENote'] == roles['/Reference'] == 1
        reference = next(n for n in nodes if n['/S'] == '/Reference')
        assert any(n is reference for n in descendants(labels[1])), 'citation not owned by literal label'
        definition = next(n for n in nodes if n['/S'] == '/FENote')
        assert reference['/Ref'][0].get_object() is definition
        assert definition['/Ref'][0].get_object() is reference
        # Empty marker has a semantic paragraph but owns no phantom paint.
        assert all(not n.get('/K') for n in descendants(labels[2]))
        text = '\n'.join(p.extract_text() for p in reader.pages)
        for token in ['α)', 'β)', 'FIRST', 'SECOND', 'EMPTY', 'Marker note']:
            assert text.count(token) == 1, (name, token, text)
        # Borderless list flow must emit no rectangle or stroked grid paths.
        for page in reader.pages:
            ops = ContentStream(page.get_contents(), reader).operations
            assert not any(op in [b're', b's'] for _, op in ops), 'list layout rectangle painted'
            for index, (_, op) in enumerate(ops):
                if op == b'S':
                    # The existing footnote separator is the only legal stroke.
                    (start, move), (end, line) = ops[index-2:index]
                    assert move == b'm' and line == b'l'
                    assert start[1] == end[1] and abs(float(end[0]-start[0])-90) < .001
            assert sum(op == b'S' for _, op in ops) <= 1
        links = [a.get_object() for p in reader.pages for a in p.get('/Annots', [])]
        assert any(a.get('/A', {}).get('/URI') == 'https://example.org/alpha' for a in links)
        internal = next(a for a in links if a.get('/A', {}).get('/D') == 'marker-point')
        target = reader.named_destinations['marker-point']
        page_no = reader.get_destination_page_number(target)
        assert page_no >= 0 and 0 < target.top < float(reader.pages[page_no].mediabox.height)
        if name == 'rich':
            assert len(reader.pages) == 1
            # Destination is the actual second label position, left of body link.
            assert target.left < internal['/Rect'][0]
        else:
            assert len(reader.pages) == 4
            for prefix in ['LBL_', 'BODY_']:
                for index in range(28):
                    assert text.count(f'{prefix}{index:02}') == 1
    elif name == 'nested':
        # Two actual nested label tables (body + footnote), plus one outer table.
        assert roles['/Table'] == 3 and roles['/LI'] == 2 and len(labels) == 2
        figures = [n for n in nodes if n['/S'] == '/Figure']
        assert len(figures) == 2 and all(n['/Alt'] == 'RED_MARKER' for n in figures)
        assert all(any(c['/S'] == '/Table' for c in descendants(label)) for label in labels)
        images = [im.image.convert('RGB') for p in reader.pages for im in p.images]
        assert images and all(im.size == (1, 1) and im.getpixel((0, 0)) == (255, 0, 0) for im in images)
    elif name == 'narrow-rtl':
        assert len(reader.pages) == 1 and roles['/L'] == 2
        # Correlate exact per-run structure replacement with its real per-paint
        # marked spans, preserving CHILD across a mid-word baseline wrap.
        stack, pieces = [], {}
        for args, op in ContentStream(reader.pages[0].get_contents(), reader).operations:
            if op == b'BDC':
                props = args[1]
                mcid = props.get('/MCID')
                if '/ActualText' in props and args[0] != '/Artifact':
                    owner = next((x for x in reversed(stack) if x is not None), None)
                    if owner is not None:
                        pieces.setdefault(owner, []).append(str(props['/ActualText']))
                stack.append(mcid)
            elif op == b'BMC':
                stack.append(None)
            elif op == b'EMC':
                stack.pop()
        exact = {}
        for node in nodes:
            if '/ActualText' not in node: continue
            painted = ''.join(''.join(pieces.get(k['/MCID'], [])) for k in node.get('/K', []) if isinstance(k, dict) and '/MCID' in k)
            assert painted == node['/ActualText'], (painted, node['/ActualText'])
            exact[str(node['/ActualText'])] = node
        assert set(exact) == {'BODY', 'LABEL', 'CHILD'}
        child = exact['CHILD']
        assert len(child['/K']) == 2
        assert [pieces[k['/MCID']] for k in child['/K']] == [['CHIL'], ['D']]
        visible = reader.pages[0].extract_text()
        assert 'CHIL' in visible and 'D' in visible
    else:
        assert len(reader.pages) == 2 and roles['/L'] == 0, 'running artifact acquired body structure'
        for page in reader.pages:
            text = page.extract_text()
            for token in ['RICH_LABEL', 'RICH_BODY', 'INNER_LABEL']:
                assert text.count(token) == 1
    print(name, len(reader.pages), 'pages: logical ownership, physical content and scoped layout checks pass')
print('No DOCX/native support or general Word-layout claim. See LITERAL_LIST_MARKERS.md for scope.')
