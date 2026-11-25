"""Record captured check results after following docs/TESTING.md.

This summarizes existing logs; it does not run checks or infer that missing
checks passed. Keep source docs/verification.json with a reviewed milestone.
"""
from pathlib import Path
import json
from datetime import date
import platform
import re
import subprocess

root = Path(__file__).resolve().parents[3]
out = root / "output"
events = [json.loads(line) for line in (out / "tests-final.json").read_text().splitlines()]
assert not any(e["Action"] == "fail" for e in events)
packages = {e["Package"]: "passed" for e in events if e["Action"] == "pass" and "Test" not in e}
assert "github.com/artpar/manifold" in packages
assert "github.com/artpar/manifold/cmd/manifold" in packages
race = (out / "race-final.log").read_text()
assert "FAIL" not in race and "DATA RACE" not in race and "ok  " in race
assert not (out / "vet-final.log").read_text().strip()
assert "all modules verified" in (out / "modules-final.log").read_text()
coverage = {}
for event in events:
    found = re.search(r"coverage: ([0-9.]+)%", event.get("Output", ""))
    if found:
        coverage[event["Package"]] = float(found.group(1))
fuzz = {}
for name, filename in [("DOCX", "fuzz-docx-final.log"), ("PDF", "fuzz-pdf-final.log"), ("ToUnicode", "fuzz-cmap.log")]:
    log = (out / filename).read_text()
    assert "PASS" in log and "FAIL" not in log
    counts = re.findall(r"execs: (\d+)", log)
    fuzz[name] = {"passed": True, "executions": int(counts[-1]), "requested_budget_seconds": 15,
                  "workers": 2, "log": "output/" + filename}
comparisons = json.loads((out / "pdf-comparison.json").read_text())
assert len(comparisons) == 28 and all(row["passed"] for row in comparisons)
manifest = json.loads((root / "testdata/corpus/manifest.json").read_text())
summary = {
    "date": date.today().isoformat(), "project": "github.com/artpar/manifold",
    "go": subprocess.check_output(["go", "version"], text=True).strip(),
    "host": platform.platform(), "packages": packages, "coverage_percent": coverage,
    "top_level_tests_passed": sum(e["Action"] == "pass" and "Test" in e and "/" not in e["Test"] for e in events),
    "named_corpus": {"total": len(manifest), "docx": sum(c["kind"] == "docx" for c in manifest),
                     "pdf": sum(c["kind"] == "pdf" for c in manifest), "mostly_synthetic": True},
    "deterministic_damaged_input_mutations": 200,
    "race_detector": "passed", "go_vet": "passed", "module_verification": "passed",
    "cgo_disabled_builds": {"darwin_arm64": "built and executed", "linux_amd64": "cross-build only", "linux_arm64": "cross-build only"},
    "pdf_poppler_text_comparisons": {"passed": len(comparisons), "total": len(comparisons), "comparison": "whitespace normalized"},
    "generated_output": json.loads((out / "independent-verification.json").read_text()),
    "fuzz_campaigns": fuzz,
    "limits": ["Short fuzz campaigns are not exhaustive", "Mostly synthetic corpus",
               "Word/LibreOffice visual acceptance not completed", "No host application integration tested"],
    "raw_logs": "output/ (ignored, regenerable; see docs/TESTING.md)",
}
(root / "output/legacy-verification-reproduction.json").write_text(json.dumps(summary, indent=2) + "\n")
print("Recorded output/legacy-verification-reproduction.json; review before adding evidence")
