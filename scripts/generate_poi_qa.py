"""Run current Go CLI paths over pinned original exports; optional QA driver."""
from pathlib import Path
import subprocess,shutil
root=Path('output/real-docx-audit');out=root/'generated';out.mkdir(parents=True,exist_ok=True)
fonts=Path('output/font-subset/dejavu')
fontargs=[]
for style,suffix in [('', ''),('/B','-Bold'),('/I','-Oblique'),('/BI','-BoldOblique')]:fontargs+=['-pdf-font','DejaVu'+style+'='+str(fonts/('DejaVuSans'+suffix+'.ttf'))]
for family in ['Times New Roman','Calibri','Cambria','Tahoma','Liberation Serif','Liberation Sans','Arial']:fontargs+=['-pdf-font-map',family+'=DejaVu']
fontargs+=['-pdf-font-map','Courier New=Go Mono']
def run(args):
 result=subprocess.run(['bin/manydoc']+args,capture_output=True,text=True)
 if result.returncode:raise RuntimeError(result.stderr)
 return result
for f in sorted(Path('testdata/real').glob('poi-*.docx')):
 stem=f.stem[4:];sf=root/(stem+'.sf')
 run(['extract','-format','sf','-o',str(sf),str(f)])
 for ext in ['docx','pdf']:
  result=run(['convert','-allow-lossy','-o',str(out/(stem+'.'+ext))]+(fontargs if ext=='pdf' else [])+[str(sf)])
  (out/(stem+'.'+ext+'.log')).write_text(result.stderr)
 run(['extract','-format','sf','-o',str(out/(stem+'.sf')),str(out/(stem+'.docx'))])
 run(['extract','-format','sf','-o',str(out/(stem+'-pdf.sf')),str(out/(stem+'.pdf'))])
 print(stem,': DOCX→SF→DOCX/SF→PDF→SF')
