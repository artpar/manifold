#!/usr/bin/env python3
"""Optional independent audits of original inputs vs streamed SF (not production).
Requires ijson, PyMuPDF, Pillow for QA only; Node prosemirror-model for schema checks.
Character inventory ignores layout/reading order and is not a visual fidelity test.
"""
import argparse, base64, collections, hashlib, json, os, subprocess, unicodedata
from pathlib import Path
import xml.etree.ElementTree as ET
import zipfile
import ijson
import pymupdf


def characters(text):
    return collections.Counter(c for c in unicodedata.normalize('NFKC', text) if not c.isspace())


def native_tree(e):
    return (e.tag, tuple(sorted(e.attrib.items())), (e.text or "").strip(), tuple(native_tree(c) for c in e))


def walk(node):
    yield node
    for child in node.get('content', []):
        yield from walk(child)


def audit(source, sf):
    pages = collections.defaultdict(collections.Counter)
    all_text = collections.Counter()
    images = collections.Counter()
    provenance_label_characters = 0
    math_count = 0
    math_trees = collections.Counter()
    nodes = 0
    env = dict(os.environ)
    env.setdefault('NODE_PATH', str(Path('output/sf-qa/node_modules').resolve()))
    consumer = subprocess.Popen(['node', 'scripts/qa/structured/verify_structured_stream.cjs'], stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True, env=env)
    with open(sf, 'rb') as f:
        for body in ijson.items(f, 'content.item', use_float=True):
            consumer.stdin.write(json.dumps(body, ensure_ascii=False) + '\n')
            nodes += 1
            for n in walk(body):
                typ = n.get('type')
                attrs = n.get('attrs') or {}
                if typ == 'list_item' and source.suffix == '.pdf':
                    original = attrs.get('source') or {}
                    if original.get('label') and original.get('page'):
                        chars = characters(original['label'])
                        pages[original['page']].update(chars)
                        all_text.update(chars)
                        provenance_label_characters += chars.total()
                if typ == 'text':
                    chars = characters(n['text'])
                    all_text.update(chars)
                    for mark in n.get('marks', []):
                        if mark['type'] == 'source':
                            page = mark.get('attrs', {}).get('page')
                            if page:
                                pages[page].update(chars)
                elif typ == 'office_math':
                    math_count += 1
                    native = ET.fromstring(attrs['omml'])
                    equations = [native] if native.tag.endswith("}oMath") else list(native.iter("{http://schemas.openxmlformats.org/officeDocument/2006/math}oMath"))
                    math_trees.update(native_tree(e) for e in equations)
                    all_text.update(characters(''.join(e.text or '' for e in native.iter() if e.tag.endswith('}t'))))
                elif typ == 'image':
                    raw = base64.b64decode(attrs['src'].split(',', 1)[1], validate=True)
                    images[hashlib.sha256(raw).hexdigest()] += 1
    consumer.stdin.close()
    checked = consumer.stdout.read()
    if consumer.wait():
        raise RuntimeError('independent ProseMirror rejection')
    result = {'source': str(source), 'sf': str(sf), 'body_nodes': nodes, 'prosemirror': json.loads(checked), 'images': sum(images.values()), 'distinct_images': len(images), 'native_equations': math_count, 'original_list_label_characters_in_provenance': provenance_label_characters}
    if source.suffix == '.docx':
        with zipfile.ZipFile(source) as z:
            root = ET.fromstring(z.read('word/document.xml'))
            expected = collections.Counter()
            W = '{http://schemas.openxmlformats.org/wordprocessingml/2006/main}'
            M = '{http://schemas.openxmlformats.org/officeDocument/2006/math}'
            def visit(e, deleted=False):
                deleted = deleted or e.tag in {W+'del', W+'moveFrom'}
                if not deleted:
                    if e.tag in {W+'t', M+'t'}:
                        expected.update(characters(e.text or ''))
                    elif e.tag == W+'sym':
                        known = {'F057':'Ω', 'F077':'ω', 'F061':'α'}
                        code = e.attrib[W+'char'].upper()
                        if code not in known:
                            raise ValueError('unmapped independent symbol '+code)
                        expected.update(characters(known[code]))
                for c in e:
                    visit(c, deleted)
            visit(root)
            original_assets = {hashlib.sha256(z.read(n)).hexdigest() for n in z.namelist() if n.startswith('word/media/')}
            result.update(original_distinct_assets=len(original_assets), original_assets_missing=sorted(original_assets-set(images)), original_preview_placements=len(list(root.iter('{urn:schemas-microsoft-com:vml}imagedata'))), original_equations=len(list(root.iter(M+'oMath'))))
            original_math = collections.Counter(native_tree(e) for e in root.iter(M+"oMath"))
            result["original_equation_structures_missing"] = sum((original_math-math_trees).values())
            missing = expected-all_text
            result.update(source_characters=expected.total(), recovered_characters=all_text.total(), missing_character_count=missing.total(), missing_characters=missing.most_common(20), extra_characters=(all_text-expected).most_common(20))
    else:
        d = pymupdf.open(source)
        missing_total = 0
        source_total = 0
        worst = []
        original_image_placements = 0
        replacement = 0
        expected_all = collections.Counter()
        control_characters = collections.Counter()
        for i, page in enumerate(d, 1):
            original_image_placements += len(page.get_image_info())
            expected = characters(page.get_text())
            replacement += expected.pop('\ufffd', 0)
            expected_all.update(expected)
            control_characters.update({c:n for c,n in expected.items() if ord(c)<32})
            missing = expected-pages[i]
            source_total += expected.total()
            missing_total += missing.total()
            if missing:
                worst.append({'page':i, 'missing':missing.total(), 'source_chars':expected.total(), 'characters':missing.most_common(8)})
        result.update(source_pages=len(d), source_image_placements=original_image_placements, recovered_text_pages=len(pages), source_characters=source_total, missing_character_count=missing_total, missing_fraction=missing_total/max(1,source_total), source_unmapped_replacement_characters=replacement, source_control_characters=control_characters.most_common(), global_missing_character_count=(expected_all-all_text).total(), global_missing_characters=(expected_all-all_text).most_common(30), worst_pages=sorted(worst,key=lambda x:x['missing'],reverse=True)[:20])
    return result

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path)
    parser.add_argument('sf', type=Path)
    args = parser.parse_args()
    print(json.dumps(audit(args.source,args.sf),indent=2))
