"""Create an explicitly synthetic continuation derivative of the pinned POI source.

Does not modify the real fixture, invoke a converter, or provide production code.
Render the resulting DOCX with an independent editor as documented in
PDF_UNTAGGED_NOTES.md. Actual expected note ownership is authored in OOXML.
"""
from pathlib import Path
import argparse
import copy
import hashlib
import json
import zipfile
import xml.etree.ElementTree as ET

parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('--out',type=Path,default=Path('output/pdf-note-continuation'))
parser.add_argument('--check',type=Path,help='strictly require the authored two-paragraph note graph in CLI model JSON')
args=parser.parse_args()
args.out.mkdir(parents=True,exist_ok=True)
source=Path('testdata/real/poi-footnotes.docx')
original=source.read_bytes()
W='http://schemas.openxmlformats.org/wordprocessingml/2006/main'
ET.register_namespace('w',W)
q=lambda n:'{'+W+'}'+n
continuation='Continuation paragraph belongs to this footnote, with italic text.'
with zipfile.ZipFile(source) as archive:
    tree=ET.fromstring(archive.read('word/footnotes.xml'))
    notes=[n for n in tree if n.get(q('type'),'normal')=='normal']
    assert len(notes)==1 and notes[0].get(q('id'))=='1'
    paragraphs=notes[0].findall(q('p'))
    assert len(paragraphs)==1
    source_text=''.join(t.text or '' for t in paragraphs[0].iter(q('t')))
    assert source_text.strip()=='snoska'
    p=ET.SubElement(notes[0],q('p'))
    properties=paragraphs[0].find(q('pPr'))
    if properties is not None:p.append(copy.deepcopy(properties))
    paragraph_properties=p.find(q('pPr'))
    if paragraph_properties is None:paragraph_properties=ET.SubElement(p,q('pPr'))
    old_spacing=paragraph_properties.find(q('spacing'))
    if old_spacing is not None:paragraph_properties.remove(old_spacing)
    ET.SubElement(paragraph_properties,q('spacing'),{q('before'):'480'})
    run=ET.SubElement(p,q('r'))
    pr=ET.SubElement(run,q('rPr'))
    ET.SubElement(pr,q('i'))
    ET.SubElement(pr,q('sz'),{q('val'):'20'})
    ET.SubElement(run,q('t')).text=continuation
    with zipfile.ZipFile(args.out/'continuation-source.docx','w',zipfile.ZIP_DEFLATED) as target:
        for item in archive.infolist():
            target.writestr(item,ET.tostring(tree,encoding='utf-8',xml_declaration=True) if item.filename=='word/footnotes.xml' else archive.read(item.filename))
assert source.read_bytes()==original
expected={'kind':'footnote','note_count':1,'body_paragraphs':[source_text,continuation],'second_paragraph_italic':True,'original_fixture_sha256':hashlib.sha256(original).hexdigest(),'profile':'synthetic extension of an unchanged pinned real fixture; not an original exported document'}
(args.out/'expected.json').write_text(json.dumps(expected,indent=2)+'\n')
print(json.dumps(expected,indent=2))

if args.check:
    doc=json.loads(args.check.read_text())
    assert len(doc.get('notes',[]))==1, 'expected one independently owned note'
    note=doc['notes'][0]
    assert note['kind']==expected['kind']
    actual=[''.join(r.get('text','') for r in b.get('runs',[])) for b in note['blocks']]
    assert actual==expected['body_paragraphs'], ('authored continuation is not in its two-paragraph note',actual,expected['body_paragraphs'])
    assert all(r.get('italic') for r in note['blocks'][1]['runs'] if r.get('text','').strip()), 'continuation style lost'
    print('strict two-paragraph note source oracle passes')
