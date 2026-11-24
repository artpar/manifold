"""Independent tagged-PDF -> SF literal-marker ownership/asset checks (QA only).
Export fixtures via MANYDOC_LITERAL_MARKER_QA=output/literal-markers go test ...
No DOCX/PDF generation capability is claimed for the new marker node yet.
"""
from pathlib import Path
import base64
import io
import json
from pypdf import PdfReader
from PIL import Image

root = Path('output/literal-markers')

def identity(ref):
    return f'({ref.idnum} {ref.generation} R)'

def refs(value):
    return value if isinstance(value, list) else [value]

def text(node):
    return node.get('text', '') + ''.join(text(c) for c in node.get('content', []))

for name in ['variable-None', 'variable-UpperAlpha', 'annotation', 'picture']:
    pdf = PdfReader(root/(name+'.pdf'), strict=True)
    original_ref = pdf.trailer['/Root']['/StructTreeRoot'].raw_get('/K')
    original_list = original_ref.get_object()
    assert original_list['/S'] == '/L'
    sf = json.loads((root/(name+'.sf')).read_text())
    group = sf['content'][0]
    assert group['type'] == ('bullet_list' if name == 'variable-None' else 'ordered_list')
    assert group['attrs']['source']['element'] == identity(original_ref)
    items = refs(original_list.raw_get('/K'))
    assert len(group['content']) == len(items)
    for original_item_ref, item in zip(items, group['content']):
        original_item = original_item_ref.get_object()
        assert original_item['/S'] == '/LI'
        label_ref, body_ref = refs(original_item.raw_get('/K'))
        assert label_ref.get_object()['/S'] == '/Lbl' and body_ref.get_object()['/S'] == '/LBody'
        assert item['type'] == 'list_item'
        source = item['attrs']['source']
        assert source['element'] == identity(original_item_ref) and source['parent_element'] == identity(original_ref)
        label = item['content'][0]
        assert label['type'] == 'list_marker'
        label_source = label['attrs']['source']
        assert label_source['element'] == identity(label_ref) and label_source['parent_element'] == identity(original_item_ref)
        assert label_source['role'] == 'Lbl' and label_source['structure'] == 'explicit'
        assert label_source['regions'] and label_source['page'] == 1
        assert label_source['quad'] and label_source['confidence'] == 1
        assert item['content'][1]['type'] == 'paragraph'
    if name.startswith('variable'):
        assert [text(i['content'][0]) for i in group['content']] == ['[done]', '[later]']
        assert [text(i['content'][1]) for i in group['content']] == ['FIRST', 'SECOND']
        if name.endswith('None'):
            assert any(d['code'] == 'pdf_list_kind_unobserved' for d in sf['attrs']['diagnostics'])
    elif name == 'annotation':
        label = group['content'][0]['content'][0]
        original_uri = str(pdf.pages[0]['/Annots'][0].get_object()['/A']['/URI'])
        assert original_uri == 'https://example.org/marker'
        run = label['content'][0]['content'][0]
        assert run['text'] == '3.' and text(group['content'][0]['content'][1]) == 'Body'
        assert any(m['type'] == 'link' and m['attrs']['href'] == original_uri for m in run['marks'])
    else:
        label = group['content'][0]['content'][0]
        marker_text, picture = label['content']
        assert text(marker_text) == '3.' and picture['type'] == 'image'
        attrs = picture['attrs']
        original_image = pdf.pages[0]['/Resources']['/XObject']['/X']
        assert original_image['/Width'] == original_image['/Height'] == 1
        assert original_image.get_data() == bytes([255, 0, 0])
        raster = Image.open(io.BytesIO(base64.b64decode(attrs['src'].split(',', 1)[1]))).convert('RGBA')
        assert raster.size == (1, 1) and raster.getpixel((0, 0)) == (255, 0, 0, 255)
        assert attrs['alt'] == 'MARKER_ICON' and attrs['width'] == attrs['height'] == 10
        assert attrs['source']['quad'] == [{'x':35,'y':95},{'x':45,'y':95},{'x':45,'y':105},{'x':35,'y':105}]
    print(name, 'actual L/LI/Lbl ownership, marker/body correspondence, source regions and relationships/assets pass')
print('Four original source fixtures / six explicit items pass; marker generation remains an open capability.')
