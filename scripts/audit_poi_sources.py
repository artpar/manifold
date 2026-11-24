"""Derive immutable expectations from original pinned POI DOCX ZIP/XML only.

Never reads manydoc output. Regeneration is for inspection, not accepting a failed
conversion. See docs/REAL_DOCX_CORPUS.md for source/rights/interpretation policy.
"""
from pathlib import Path
from zipfile import ZipFile
from lxml import etree
import hashlib,json,posixpath

root=Path('testdata/real');W='http://schemas.openxmlformats.org/wordprocessingml/2006/main';R='http://schemas.openxmlformats.org/officeDocument/2006/relationships'
ns={'w':W};key=lambda n:'{'+W+'}'+n

def visible(n):
    tag=etree.QName(n).localname
    if tag in ('del','moveFrom','instrText','delText','pPr','rPr'):return ''
    if tag=='t':return n.text or ''
    if tag in ('tab','ptab'):return '\t'
    if tag in ('br','cr'):return '' if n.get(key('type'))=='page' else '\n'
    if tag in ('footnoteReference','endnoteReference'):return '\ufffc'+tag.replace('Reference','')+':'+n.get(key('id'))+'\ufffc'
    return ''.join(visible(c) for c in n)

def paragraphs(r):return [s for p in r.findall('.//w:p',ns) if (s:=visible(p))]
def styles(r):
    out=[];index=-1
    for p in r.findall('.//w:p',ns):
        if not visible(p):continue
        index+=1;offset=0
        for run in p.iter(key('r')):
            if any(etree.QName(a).localname in ('del','moveFrom') for a in run.iterancestors()):continue
            text=visible(run);props=run.find('w:rPr',ns);marks={}
            if props is not None:
                for xml,model in [('b','bold'),('i','italic'),('u','underline'),('strike','strike')]:
                    value=props.find('w:'+xml,ns)
                    if value is not None and value.get(key('val'),'1') not in ('0','false','off','none'):marks[model]=True
                value=props.find('w:sz',ns)
                if value is not None:marks['size']=float(value.get(key('val')))/2
                value=props.find('w:color',ns)
                if value is not None and value.get(key('val')) not in ('auto',None) and value.get(key('themeColor')) is None:marks['color']='#'+value.get(key('val')).upper()
            length=len(text.encode())
            if text and '\ufffc' not in text and marks:out.append({'paragraph':index,'start':offset,'end':offset+length,'marks':marks})
            offset+=length
    return out

result={}
for path in sorted(root.glob('poi-*.docx')):
    raw=path.read_bytes()
    with ZipFile(path) as z:
        document=etree.fromstring(z.read('word/document.xml'));app=etree.fromstring(z.read('docProps/app.xml'))
        props={etree.QName(c).localname:c.text for c in app}
        entry={'sha256':hashlib.sha256(raw).hexdigest(),'bytes':len(raw),'application':props.get('Application'),'app_version':props.get('AppVersion'),'body':paragraphs(document),'body_styles':styles(document),'notes':[],'sections':[]}
        for kind in ['footnote','endnote']:
            name='word/'+kind+'s.xml'
            if name not in z.namelist():continue
            for note in etree.fromstring(z.read(name)):
                if note.get(key('type'),'normal')!='normal':continue
                entry['notes'].append({'id':note.get(key('id')),'kind':kind,'paragraphs':paragraphs(note),'styles':styles(note)})
        rels={n.get('Id'):n.get('Target') for n in etree.fromstring(z.read('word/_rels/document.xml.rels'))}
        entry['hyperlinks']={}
        for link in document.findall('.//w:hyperlink',ns):
            target=rels.get(link.get('{'+R+'}id'))
            if target and visible(link):entry['hyperlinks'][target]=entry['hyperlinks'].get(target,'')+visible(link)
        for section in document.findall('.//w:sectPr',ns):
            if any(etree.QName(a).localname.endswith('Change') for a in section.iterancestors()):continue
            stories={};page=section.find('w:pgSz',ns);margins=section.find('w:pgMar',ns)
            for kind in ['header','footer']:
                for ref in section.findall('w:'+kind+'Reference',ns):
                    target=posixpath.normpath(posixpath.join('word',rels[ref.get('{'+R+'}id')]))
                    stories[kind+'_'+ref.get(key('type'))]=paragraphs(etree.fromstring(z.read(target)))
            entry['sections'].append({'stories':stories,'page':{etree.QName(k).localname:float(v)/20 for k,v in page.attrib.items()} if page is not None else {},'margins':{etree.QName(k).localname:float(v)/20 for k,v in margins.attrib.items()} if margins is not None else {},'first_page':section.find('w:titlePg',ns) is not None})
        core=etree.fromstring(z.read('docProps/core.xml'));dc='{http://purl.org/dc/elements/1.1/}'
        entry['metadata']={k:core.findtext(dc+xml,'') for k,xml in [('author','creator'),('title','title'),('subject','subject')]}
        entry['review']={tag:len(document.findall('.//w:'+tag,ns)) for tag in ['ins','del','fldChar','fldSimple','commentReference']}
        entry['deleted_text']=''.join(document.xpath('//w:delText/text()',namespaces=ns))
        entry['field_instructions']=document.xpath('//w:instrText/text()',namespaces=ns)
        entry['comments']=[]
        if 'word/comments.xml' in z.namelist():entry['comments']=paragraphs(etree.fromstring(z.read('word/comments.xml')))
        result[path.name]=entry
(root/'poi-docx-oracle.json').write_text(json.dumps(result,indent=2,ensure_ascii=False)+'\n')
print('Audited original ZIP/XML for',len(result),'POI exports')
