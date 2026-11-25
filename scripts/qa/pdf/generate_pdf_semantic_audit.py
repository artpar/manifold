"""Reproducible independent PDF note/artifact semantic probes."""
from pathlib import Path

out=Path('output/audit');out.mkdir(parents=True,exist_ok=True)
def pdf(objects):
    data=bytearray(b'%PDF-1.7\n');offsets=[0]
    for i,obj in enumerate(objects,1):
        offsets.append(len(data));data.extend(f'{i} 0 obj\n{obj}\nendobj\n'.encode())
    start=len(data);data.extend(f'xref\n0 {len(offsets)}\n0000000000 65535 f \n'.encode())
    for offset in offsets[1:]:data.extend(f'{offset:010d} 00000 n \n'.encode())
    data.extend(f'trailer\n<< /Size {len(offsets)} /Root 1 0 R >>\nstartxref\n{start}\n%%EOF\n'.encode());return data

def stream(content):return f'<< /Length {len(content.encode())} >>\nstream\n{content}\nendstream'
def text(mcid,x,y,value):return f'BT /F 10 Tf /P << /MCID {mcid} >> BDC {x} {y} Td ({value}) Tj EMC ET\n'
base=['<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 6 0 R /MarkInfo << /Marked true >> >>',
      '<< /Type /Pages /Kids [3 0 R] /Count 1 >>',
      '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 400] /Resources << /Font << /F 4 0 R >> >> /Contents 5 0 R >>',
      '<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>']
objects=base+[stream(text(0,20,300,'BODY')+text(1,60,300,'1')+text(2,20,50,'NOTE BODY')),
 '<< /Type /StructTreeRoot /K [7 0 R 9 0 R] >>',
 '<< /Type /StructElem /S /P /P 6 0 R /Pg 3 0 R /K [0 8 0 R] >>',
 '<< /Type /StructElem /S /Reference /P 7 0 R /Pg 3 0 R /Ref [9 0 R] /K 1 >>',
 '<< /Type /StructElem /S /Note /P 6 0 R /Pg 3 0 R /ID (note-1) /K 10 0 R >>',
 '<< /Type /StructElem /S /P /P 9 0 R /Pg 3 0 R /K 2 >>']
(out/'explicit-note.pdf').write_bytes(pdf(objects))
objects=base.copy();objects[0]='<< /Type /Catalog /Pages 2 0 R >>'
objects.append(stream('/Artifact << /Type /Pagination /Subtype /Header /BBox [10 350 220 390] /Attached [/Top] >> BDC BT /F 10 Tf 20 370 Td (HEADER) Tj ET EMC BT /F 10 Tf 20 300 Td (BODY) Tj ET'))
(out/'pagination-artifact.pdf').write_bytes(pdf(objects))
print('Wrote explicit Note/Reference/Ref and Header artifact probes under',out)

# A second independent hand-authored source exercises a custom repeated mark,
# explicit subtype, a list, a table, exact raster pixels and a note-body view.
content=(text(0,20,300,'BODY')+text(1,60,300,'*1')+text(2,20,80,'*1')+
         text(3,40,80,'NOTE BODY')+text(4,40,65,'LIST ITEM')+text(5,40,50,'TABLE CELL')+
         text(6,80,300,'MIDDLE')+text(7,125,300,'*1')+text(8,145,300,'AFTER')+text(9,20,65,'1.')+
         '/Figure << /MCID 10 >> BDC q 20 0 0 10 40 30 cm BI /W 2 /H 1 /BPC 8 /CS /RGB /F /AHx ID FF00000000FF> EI Q EMC\n')
objects=base.copy()
objects[0]='<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 6 0 R /Dests << /note [3 0 R /XYZ 40 80 null] >> /MarkInfo << /Marked true >> >>'
objects[3]='<< /Type /Font /Subtype /Type1 /BaseFont /Courier-Bold >>'
objects += [stream(content), '<< /Type /StructTreeRoot /K [7 0 R 9 0 R] >>',
 '<< /Type /StructElem /S /P /P 6 0 R /Pg 3 0 R /K [0 8 0 R 6 18 0 R 8] >>',
 '<< /Type /StructElem /S /Reference /P 7 0 R /Pg 3 0 R /Ref [9 0 R] /K 1 >>',
 '<< /Type /StructElem /S /FENote /A << /O /FENote /NoteType /Footnote >> /P 6 0 R /Pg 3 0 R /ID (note-1) /Ref [8 0 R 18 0 R] /K [10 0 R 11 0 R 12 0 R 15 0 R 20 0 R] >>',
 '<< /Type /StructElem /S /Lbl /P 9 0 R /K 2 >>',
 '<< /Type /StructElem /S /P /P 9 0 R /K 3 >>',
 '<< /Type /StructElem /S /L /A << /O /List /ListNumbering /Decimal >> /P 9 0 R /K 13 0 R >>',
 '<< /Type /StructElem /S /LI /P 12 0 R /K [19 0 R 14 0 R] >>',
 '<< /Type /StructElem /S /LBody /P 13 0 R /K 4 >>',
 '<< /Type /StructElem /S /Table /P 9 0 R /K 16 0 R >>',
 '<< /Type /StructElem /S /TR /P 15 0 R /K 17 0 R >>',
 '<< /Type /StructElem /S /TD /P 16 0 R /K 5 >>',
 '<< /Type /StructElem /S /Span /P 7 0 R /Pg 3 0 R /K 7 >>',
 '<< /Type /StructElem /S /Lbl /P 13 0 R /K 9 >>',
 '<< /Type /StructElem /S /Figure /P 9 0 R /Alt (RED BLUE) /K 10 >>']
(out/'rich-explicit-note.pdf').write_bytes(pdf(objects))
print('Wrote rich explicit Footnote/custom marks/reverse Ref/list/table/raster/navigation probe')

# Distinct citation/body marks use the same explicit graph, with new paint facts.
variant=objects.copy()
variant[4]=stream(content.replace('125 300 Td (*1)','125 300 Td (OTHER)').replace('20 80 Td (*1)','20 80 Td (N-1)'))
(out/'variant-explicit-note.pdf').write_bytes(pdf(variant))
markerless=objects.copy()
empty=content
for value in (text(1,60,300,'*1'),text(7,125,300,'*1'),text(2,20,80,'*1')):empty=empty.replace(value,'')
markerless[4]=stream(empty)
markerless[7]=markerless[7].replace('/K 1 >>','/K [] >>')
markerless[17]=markerless[17].replace('/K 7 >>','/K [] >>')
markerless[9]=markerless[9].replace('/K 2 >>','/K [] >>')
(out/'markerless-explicit-note.pdf').write_bytes(pdf(markerless))
print('Wrote differing-mark and markerless explicit relationship probes')

# Published inline Reference/Note sibling pattern, deliberately without Ref.
adjacent=objects.copy()
adjacent[5]=adjacent[5].replace('/K [7 0 R 9 0 R]','/K 7 0 R')
adjacent[6]=adjacent[6].replace('/K [0 8 0 R 6 18 0 R 8]','/K [0 8 0 R 9 0 R 6 18 0 R 8]')
adjacent[7]=adjacent[7].replace('/Ref [9 0 R] ','')
adjacent[8]=adjacent[8].replace('/Ref [8 0 R 18 0 R] ','').replace('/P 6 0 R','/P 7 0 R').replace('/S /FENote /A << /O /FENote /NoteType /Footnote >>','/S /Note')
(out/'adjacent-note.pdf').write_bytes(pdf(adjacent))
print('Wrote published adjacent Reference/Note pattern without explicit Ref links')


# Individual styles inside one leading Lbl are independent paint facts, not a
# common-style template. Graphics-state isolation keeps the other oracles intact.
rich_label=objects.copy()
mixed='q BT /F 10 Tf /P << /MCID 2 >> BDC 20 80 Td (*) Tj /F 8 Tf 1 0 0 rg (1) Tj EMC ET Q\n'
rich_label[4]=stream(content.replace(text(2,20,80,'*1'),mixed))
(out/'rich-label-note.pdf').write_bytes(pdf(rich_label))
print('Wrote one Lbl with individually sized/colored literal runs')


# An authored SF probe exercises one rich mixed-direction mark on either gutter.
import json
mixed_notes=[]
for kind in ('footnote','endnote'):
    mixed_notes.append({'id':kind,'kind':kind,'label':'F' if kind=='footnote' else 'E',
        'body_label_content':[{'type':'text','text':'بب','marks':[{'type':'textStyle','attrs':{'fontFamily':'DejaVu','fontSize':10}}]},
                              {'type':'text','text':'A','marks':[{'type':'textStyle','attrs':{'fontFamily':'DejaVu','fontSize':9,'color':'#FF0000'}}]}],
        'content':[{'type':'paragraph','attrs':{'dir':'rtl'} if kind=='endnote' else {},'content':[{'type':'text','text':'BODY_'+kind.upper()}]}]})
mixed_sf={'type':'doc','attrs':{'schema':'manydoc-prosemirror/1','layout':{'width':420,'height':420,'margin':36},'notes':mixed_notes},
          'content':[{'type':'paragraph','content':[{'type':'text','text':'Mixed body labels '},{'type':'note_reference','attrs':{'kind':'footnote','id':'footnote'}},{'type':'text','text':' and '},{'type':'note_reference','attrs':{'kind':'endnote','id':'endnote'}}]}]}
(out/'mixed-note-labels.sf').write_text(json.dumps(mixed_sf,ensure_ascii=False))


# One explicit citation has individual glyph styles; the repeated site differs
# only in that it keeps the original uniform representation.
rich_citation=objects.copy()
fragment='q BT /F 10 Tf /P << /MCID 1 >> BDC 60 300 Td (*) Tj /F 8 Tf 1 0 0 rg (1) Tj EMC ET Q\n'
rich_citation[4]=stream(content.replace(text(1,60,300,'*1'),fragment))
(out/'rich-citation-note.pdf').write_bytes(pdf(rich_citation))

# Rich and uniform mixed-direction citation labels share the same note identity
# machinery, but retain their own independent style/text representation.
rich_mark=[{'type':'text','text':'بب','marks':[{'type':'textStyle','attrs':{'fontFamily':'DejaVu','fontSize':12}}]},
           {'type':'text','text':'A','marks':[{'type':'textStyle','attrs':{'fontFamily':'DejaVu','fontSize':10,'color':'#FF0000'}}]}]
mixed_citations={'type':'doc','attrs':{'schema':'manydoc-prosemirror/1','layout':{'width':300,'height':240,'margin':24},
    'notes':[{'id':n,'kind':'footnote','content':[{'type':'paragraph','content':[{'type':'text','text':'NOTE_'+n.upper()}]}]} for n in ('one','two')]},
    'content':[{'type':'paragraph','attrs':{'dir':direction},'content':[
        {'type':'text','text':'Before anchorONE'},
        {'type':'note_reference','attrs':{'id':'one','kind':'footnote','label_content':rich_mark}},
        {'type':'text','text':' and anchorTWO'},
        {'type':'note_reference','attrs':{'id':'two','kind':'footnote','label':'بA'},'marks':[{'type':'textStyle','attrs':{'fontFamily':'DejaVu','fontSize':14}}]},
        {'type':'text','text':' after the atomic citations.'}]} for direction in ('ltr','rtl')]}
(out/'mixed-citations.sf').write_text(json.dumps(mixed_citations,ensure_ascii=False))

# Exact enclosing structural ActualText retains two explicit citation sites.
# Paint operators and note/asset objects remain unchanged. One citation also has
# its own exact nested ActualText boundary and individually styled fragments.
aligned_note=rich_citation.copy()
aligned_note[6]=aligned_note[6].replace('/S /P','/S /P /ActualText (BODY*1MIDDLE*1AFTER)',1)
wrapper=len(aligned_note)+1
aligned_note[6]=aligned_note[6].replace('/K [0 8 0 R',f'/K [0 {wrapper} 0 R',1)
aligned_note[7]=aligned_note[7].replace('/P 7 0 R',f'/P {wrapper} 0 R',1)
aligned_note.append('<< /Type /StructElem /S /Span /P 7 0 R /ActualText (*1) /K 8 0 R >>')
(out/'aligned-actual-note.pdf').write_bytes(pdf(aligned_note))
opaque_note=aligned_note.copy()
opaque_note[6]=opaque_note[6].replace('BODY*1MIDDLE*1AFTER','DIFFERENT*1TEXT*1CONTENT')
(out/'opaque-actual-note.pdf').write_bytes(pdf(opaque_note))
print('Wrote exact nested/enclosing and opaque ActualText citation probes')

# A nested definition has its own main-flow citation. Source containment does
# not make its body part of the parent's editable text or a recursive citation.
nested_note=objects.copy();child=len(nested_note)+1
nested_note[8]=nested_note[8].replace('/Ref [8 0 R 18 0 R]','/Ref [8 0 R]').replace('20 0 R]',f'20 0 R {child} 0 R]')
nested_note[17]=nested_note[17].replace('/K 7',f'/Ref [{child} 0 R] /K 7')
nested_content=content.replace('125 300 Td (*1)','125 300 Td (*2)')+text(11,20,15,'*2')+text(12,40,15,'CHILD BODY')
nested_note[4]=stream(nested_content)
nested_note += [f'<< /Type /StructElem /S /FENote /A << /O /FENote /NoteType /Endnote >> /P 9 0 R /Pg 3 0 R /ID (note-2) /Ref [18 0 R] /K [{child+1} 0 R {child+2} 0 R] >>',
 f'<< /Type /StructElem /S /Lbl /P {child} 0 R /K 11 >>', f'<< /Type /StructElem /S /P /P {child} 0 R /K 12 >>']
(out/'nested-definition-note.pdf').write_bytes(pdf(nested_note))
wrapper_note=objects.copy();wrapper=len(wrapper_note)+1
wrapper_note[6]=wrapper_note[6].replace('/K [0 8 0 R',f'/K [0 {wrapper} 0 R')
wrapper_note[7]=wrapper_note[7].replace('/P 7 0 R',f'/P {wrapper} 0 R')
wrapper_note.append('<< /Type /StructElem /S /Span /P 7 0 R /Ref [9 0 R] /K 8 0 R >>')
(out/'redundant-wrapper-note.pdf').write_bytes(pdf(wrapper_note))
opaque_wrapper=wrapper_note.copy();opaque_wrapper[-1]=opaque_wrapper[-1].replace('/S /Span','/S /Span /ActualText (DIFFERENTMARK)')
(out/'opaque-wrapper-note.pdf').write_bytes(pdf(opaque_wrapper))
print('Wrote independently cited nested mixed-kind definition, redundant and opaque Ref wrapper probes')
