"""Independent format/start import and external-reader literal label checks."""
import json
from pathlib import Path
import pymupdf

out=Path('output/numbering')
sf=json.loads((out/'format-probe.sf').read_text())
lists=[n for n in sf['content'] if n['type']=='ordered_list']
pdf=pymupdf.open(out/'format-probe.pdf')
text=' '.join(' '.join(p.get_text().split()) for p in pdf)
cases=[(f,n) for f in ['upperLetter','lowerLetter','upperRoman','lowerRoman','decimalZero'] for n in [0,1,26,27,28,52,53,3999,4000]]
assert len(lists)==len(cases)==45
for node,(fmt,value) in zip(lists,cases):
 assert node['attrs']['order']==value
 assert node['attrs']['numbering']['formats']==[fmt]
 assert node['attrs']['numbering']['template']=='[%1]'
 if value==0:label='0'
 elif 'Letter' in fmt:label=chr(ord('A' if fmt=='upperLetter' else 'a')+(value-1)%26)*((value-1)//26+1)
 elif fmt=='decimalZero':label=str(value).zfill(2)
 else:
  rest=value;parts=[]
  for count,symbol in [(1000,'M'),(900,'CM'),(500,'D'),(400,'CD'),(100,'C'),(90,'XC'),(50,'L'),(40,'XL'),(10,'X'),(9,'IX'),(5,'V'),(4,'IV'),(1,'I')]:
   q,rest=divmod(rest,count);parts.append(symbol*q)
  label=''.join(parts)
  if fmt=='lowerRoman':label=label.lower()
 assert '['+label+'] '+fmt+' '+str(value) in text,(fmt,value,label)
report=dict(cases=45,go_import='exact authored format, label template and counter start',reader='LibreOffice original DOCX labels, including 27/28/53 repeats, zero and Roman 4000',pages=len(pdf))
(out/'format-probe.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report,indent=2))
