"""Lossless SF JSON/gzip comparison; uses only Python's standard library."""
import argparse
from collections import Counter
import gc
import gzip
import hashlib
import json
from pathlib import Path


def inspect(path):
    opener=gzip.open if str(path).lower().endswith('.sf.gz') else open
    with opener(path,'rt',encoding='utf8') as f:document=json.load(f)
    assert document['attrs']['schema']=='manydoc-prosemirror/1'
    counts=Counter()
    def walk(value):
        if isinstance(value,dict):
            if 'type' in value:counts[value['type']]+=1
            for child in value.values():walk(child)
        elif isinstance(value,list):
            for child in value:walk(child)
    walk(document)
    digest=hashlib.sha256()
    # Field ordering/indentation are not ProseMirror JSON semantics. Every value,
    # including source geometry, asset bytes, relationships and nullable parameters,
    # contributes to this independently normalized digest.
    for chunk in json.JSONEncoder(sort_keys=True,separators=(',',':'),ensure_ascii=False,allow_nan=False).iterencode(document):digest.update(chunk.encode('utf8'))
    result=dict(path=str(path),bytes=path.stat().st_size,semantic_sha256=digest.hexdigest(),types=dict(counts))
    del document;gc.collect();return result

p=argparse.ArgumentParser();p.add_argument('files',nargs='+',type=Path);p.add_argument('--report',type=Path);a=p.parse_args()
reports=[inspect(path) for path in a.files]
assert all(r['semantic_sha256']==reports[0]['semantic_sha256'] and r['types']==reports[0]['types'] for r in reports),reports
if a.report:a.report.write_text(json.dumps(reports,indent=2)+'\n')
print(json.dumps(reports,indent=2))
