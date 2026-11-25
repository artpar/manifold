"""Optional independent readers for native numbering -> SF -> DOCX/PDF.

Run the artifact commands in docs/VDOCS_FORMAT.md; QA dependencies are ignored.
This verifies structure, displayed marks and link geometry, not Word fidelity.
"""
from pathlib import Path
from zipfile import ZipFile
from lxml import etree
from docx import Document
from pypdf import PdfReader
import pymupdf

root=Path('output/vdocs-qa/note-numbering')
Document(root.with_suffix('.docx'))
ns={'w':'http://schemas.openxmlformats.org/wordprocessingml/2006/main','md':'urn:manydoc:metadata:1'}
md='{'+ns['md']+'}'
with ZipFile(root.with_suffix('.docx')) as z:
    main=etree.fromstring(z.read('word/document.xml'))
    foot=etree.fromstring(z.read('word/footnotes.xml'))
    end=etree.fromstring(z.read('word/endnotes.xml'))
    notes={n.get(md+'originalId'):n for tree in (foot,end) for n in tree if n.get(md+'originalId')}
    assert set(notes)=={'custom','f1','f2','f3','shared','silent','e1','e2'}
    assert ''.join(notes['shared'].itertext())=='01 Body shared'
    assert ''.join(notes['silent'].itertext())=='Body silent'
    assert notes['silent'].xpath('.//w:footnoteRef',namespaces=ns)
    anchor=main.xpath('//w:footnoteReference[@w:id=$id]',namespaces=ns,id=notes['silent'].get('{'+ns['w']+'}id'))[0]
    assert anchor.get('{'+ns['w']+'}customMarkFollows')=='1'
    assert ''.join(anchor.getparent().itertext())=='\u200b'  # native body reference resolves the markerless anchor
    refs=main.xpath('//w:hyperlink[@md:noteCitation]',namespaces=ns)
    assert [''.join(n.itertext()) for n in refs]==['IV','V','01','']
    assert len({n.get(md+'noteCitation') for n in refs[:3]})==1
    target=refs[0].get(md+'noteCitation')
    assert notes['shared'].xpath('.//w:bookmarkStart[@w:name=$name]',namespaces=ns,name=target)
    assert len(main.xpath('//w:footnoteReference',namespaces=ns))==6
    assert len(main.xpath('//w:endnoteReference',namespaces=ns))==2

reader=PdfReader(root.with_suffix('.pdf'))
pdf=pymupdf.open(root.with_suffix('.pdf'))
assert len(reader.pages)==len(pdf)==4
labels=[];links=[]
for p in pdf:
    for link in p.get_links():
        labels.append(p.get_textbox(link['from']).strip());links.append(link)
assert labels==['III','AA','IV','V','§','VI','00','BB','01']
assert len([a for p in reader.pages for a in p.get('/Annots',[])])==9
for i in (3,8):
    assert links[i]['page']==links[2]['page']
    assert links[i]['to']==links[2]['to']
assert links[1]['page']==links[7]['page']==3
assert links[1]['to']!=links[7]['to']
text='\n'.join(p.get_text() for p in pdf)
assert '01 Body shared' in text and 'Body silent' in text and 'A😀' in text
assert not pdf[2].get_links()  # markerless citation still owns an editable note
assert 'Body silent' in pdf[2].get_text()
print('Independent python-docx/XML and pypdf/MuPDF: eight note bodies, variant/markerless Word relationships, nine displayed citation/link boxes, shared destinations and Unicode pass; no full visual-fidelity claim')
