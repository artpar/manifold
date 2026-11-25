"""Optional independent SF QA. Build bin/manifold and install prosemirror-model first."""
import json
import os
from pathlib import Path
import subprocess

root = Path(__file__).resolve().parent.parent
os.chdir(root)
folder = Path("output/sf-corpus")
folder.mkdir(parents=True, exist_ok=True)
paths = []
for case in json.loads(Path("testdata/corpus/manifest.json").read_text()):
    if case["expect"] == "reject":
        continue
    output = folder / (case["name"] + ".sf")
    command = ["bin/manifold", "extract", "-format", "sf", "-o", str(output)]
    if case["expect"] == "password":
        command += ["-password-env", "MANYDOC_QA_PASSWORD"]
    command += ["testdata/corpus/" + case["name"]]
    # This password belongs only to the public synthetic fixtures.
    subprocess.run(command, env={**os.environ, "MANYDOC_QA_PASSWORD": "secret"},
                   check=True, capture_output=True)
    paths.append(str(output))
subprocess.run(["node", "scripts/verify_structured.cjs", *paths], check=True,
               env={**os.environ, "NODE_PATH": str(root / "output/sf-qa/node_modules")})
print("Corpus files checked:", len(paths))
