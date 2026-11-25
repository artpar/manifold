"""Independent editable-style, standard Word package and PDF paint/link audit."""
from pathlib import Path
from zipfile import ZipFile
import json
from lxml import etree
from docx import Document
from pypdf import PdfReader
import pymupdf

root=Path('output/note-styles')
sf=json.loads((root/'notes.sf').read_text())
got=json.loads((root/'notes-docx.sf').read_text())
assert sf['attrs']['notes']==got['attrs']['notes']
assert sf['content']==got['content']
Document(root/'notes.docx')
ns={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main','md':'urn:manydoc:metadata:1'}
with ZipFile(root/'notes.docx') as z:
    main=etree.fromstring(z.read('word/document.xml'))
    target=main.xpath('//w:bookmarkStart[@md:anchor="citation-return"]/@w:name',namespaces=ns)
    assert len(target)==1
    for part in ('footnotes','endnotes'):
        tree=etree.fromstring(z.read(f'word/{part}.xml'))
        labels=tree.xpath('//w:hyperlink[@md:noteBodyStyle="1"]',namespaces=ns)
        assert len(labels)==(2 if part=='footnotes' else 1)
        for label in labels:
            assert label.get('{'+ns['w']+'}anchor')==target[0]
            props=label.find('w:r/w:rPr',ns)
            assert props.find('w:sz',ns).get('{'+ns['w']+'}val')=='19'
            assert props.find('w:color',ns).get('{'+ns['w']+'}val')=='203040'
            assert all(props.find('w:'+p,ns) is not None for p in ('b','i','u'))
            assert props.find('w:rFonts',ns).get('{'+ns['w']+'}ascii')=='Go'
            assert props.find('w:lang',ns).get('{'+ns['w']+'}val')=='en'
reader=PdfReader(root/'notes.pdf')
dest=reader.named_destinations['citation-return']
assert reader.get_destination_page_number(dest)==0
assert abs(float(reader.pages[0].mediabox.height)-float(dest.top)-36)<.02
for label,page_number in [('N-1',0),('§E',1)]:
    for filename in ('notes.pdf','lo/notes.pdf'):
        doc=pymupdf.open(root/filename);page=doc[page_number]
        spans=[s for b in page.get_text('dict')['blocks'] for l in b.get('lines',[]) for s in l['spans'] if s['text'].strip()==label]
        assert len(spans)==1,(filename,label,spans)
        span=spans[0]
        if label=='N-1':
            body=[s for b in page.get_text('dict')['blocks'] for l in b.get('lines',[]) for s in l['spans'] if 'FIRST_NOTE_BODY:' in s['text']]
            assert len(body)==1 and body[0]['bbox'][0]-span['bbox'][2]>=4, 'label/body gutter is cramped'
        assert abs(span['size']-9.5)<.001 and span['color']==0x203040
        assert span['flags']&2 and span['flags']&16 # italic/bold independently read from emitted font
        assert 0<=span['bbox'][1]<span['bbox'][3]<=page.rect.height
        if filename=='notes.pdf':
            candidates=[link for link in page.get_links() if link.get('nameddest')=='citation-return' and link['from'].intersects(pymupdf.Rect(span['bbox']))]
            assert len(candidates)==1 and candidates[0]['page']==0
        page.get_pixmap(matrix=pymupdf.Matrix(1.4,1.4)).save(root/f'{"native" if filename=="notes.pdf" else "word"}-{page_number+1}.png')
print('SF/Word editable mark equality, standard bookmark/link/rPr, native named backlink views and native/Word 9.5pt bold italic colored paint pass')
print('LibreOffice substitutes unavailable Go with Liberation Serif and omits custom note-label hyperlinks in its PDF; package links and native PDF links verified separately')
