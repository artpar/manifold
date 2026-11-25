"""Author independent literal OOXML numbering probes for optional reader QA."""
from pathlib import Path
from docx import Document
from docx.shared import Inches
from docx.oxml import OxmlElement
from docx.oxml.ns import qn

out=Path('output/numbering');out.mkdir(parents=True,exist_ok=True)
def element(name,value=None):
 n=OxmlElement('w:'+name)
 if value is not None:n.set(qn('w:val'),str(value))
 return n
def paragraph(doc,num,level,text):
 p=doc.add_paragraph(text);np=element('numPr');np.extend([element('ilvl',level),element('numId',num)]);p._p.get_or_add_pPr().append(np)
doc=Document();doc.sections[0].page_width=Inches(22);doc.sections[0].page_height=Inches(22);nr=doc.part.numbering_part.element
for i,(fmt,start) in enumerate([(f,n) for f in ['upperLetter','lowerLetter','upperRoman','lowerRoman','decimalZero'] for n in [0,1,26,27,28,52,53,3999,4000]],100):
 a=element('abstractNum');a.set(qn('w:abstractNumId'),str(i));level=element('lvl');level.set(qn('w:ilvl'),'0')
 level.extend([element('start',start),element('numFmt',fmt),element('lvlText','[%1]')]);a.append(level);nr.append(a)
 num=element('num');num.set(qn('w:numId'),str(i));num.append(element('abstractNumId',i));nr.append(num)
 paragraph(doc,i,0,fmt+' '+str(start))
doc.save(out/'format-probe.docx')

doc=Document();nr=doc.part.numbering_part.element
for numid,empty,start in [(100,True,0),(101,False,3)]:
 a=element('abstractNum');a.set(qn('w:abstractNumId'),str(numid))
 for i in (0,1):
  level=element('lvl');level.set(qn('w:ilvl'),str(i));level.extend([element('start',3 if i==0 else 1),element('numFmt','decimal'),element('lvlText','%1' if i==0 else '%1.%2')]);a.append(level)
 nr.append(a);num=element('num');num.set(qn('w:numId'),str(numid));num.append(element('abstractNumId',numid))
 if empty:
  override=element('lvlOverride');override.set(qn('w:ilvl'),'0');num.append(override)
 nr.append(num)
 paragraph(doc,numid,1,'IMPLICIT ZERO' if empty else 'IMPLICIT THREE')
 paragraph(doc,numid,0,'FOLLOWING ONE' if empty else 'FOLLOWING FOUR')
doc.save(out/'implicit-probe.docx')
print('Authored 45 literal format cases and two implicit-parent sequences')
