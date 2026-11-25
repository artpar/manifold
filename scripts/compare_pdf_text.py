"""Compare manydoc CLI extraction with Poppler across non-malformed PDF fixtures.

Usage: python scripts/compare_pdf_text.py [bin/manifold] [output/pdf-comparison.json]
"""
import json
import os
from pathlib import Path
import subprocess
import sys

root = Path(__file__).resolve().parents[1]
binary = (root / (sys.argv[1] if len(sys.argv) > 1 else "bin/manifold")).resolve()
output = root / (sys.argv[2] if len(sys.argv) > 2 else "output/pdf-comparison.json")
rows = []
for case in json.loads((root / "testdata/corpus/manifest.json").read_text()):
    if case["kind"] != "pdf" or case["expect"] == "reject":
        continue
    source = root / "testdata/corpus" / case["name"]
    cmd = [str(binary), "extract"]
    oracle = ["pdftotext"]
    env = dict(os.environ)
    if case["expect"] == "password":
        env["MANYDOC_TEST_PASSWORD"] = "secret"
        cmd += ["-password-env", "MANYDOC_TEST_PASSWORD"]
        oracle += ["-upw", "secret"]
    cmd += [str(source)]
    oracle += [str(source), "-"]
    actual = subprocess.run(cmd, text=True, capture_output=True, env=env, timeout=15)
    expected = subprocess.run(oracle, text=True, capture_output=True, timeout=15)
    normalized = lambda text: " ".join(text.split())
    passed = actual.returncode == expected.returncode == 0 and normalized(actual.stdout) == normalized(expected.stdout)
    rows.append({"fixture": case["name"], "passed": passed, "manydoc_exit": actual.returncode,
                 "poppler_exit": expected.returncode, "manydoc_text": actual.stdout, "poppler_text": expected.stdout})
output.parent.mkdir(parents=True, exist_ok=True)
output.write_text(json.dumps(rows, ensure_ascii=False, indent=2) + "\n")
failed = [r["fixture"] for r in rows if not r["passed"]]
print(f"{len(rows)-len(failed)}/{len(rows)} text comparisons passed")
if failed:
    print("Failed:", ", ".join(failed))
    sys.exit(1)
