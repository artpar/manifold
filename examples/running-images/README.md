# DOCX running-image budgets

```sh
GOWORK=off go run ./examples/running-images
mkdir -p output/running-images/lo
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file://"$PWD"/output/running-images/lo-profile --headless --convert-to pdf --outdir output/running-images/lo output/running-images/serial.docx output/running-images/nested.docx output/running-images/body.docx output/running-images/jpeg.docx
output/qa-venv/bin/python scripts/qa/docx/verify_docx_running_images.py
```

The example writes source SF, DOCX and reimported SF under ignored output/.
It uses original in-process PNG/JPEG assets, without external runtime tools.
Producer conversion and the Python readers are optional developer QA.

Cases cover six serial pictures; merged/nested running tables and links; a small
unchanged footer logo with a nested body figure; and JPEG. Source assets and
preferred sizes remain unchanged. DOCX contains fitted physical sizes. The CLI
second-round-trip checks in docs/TESTING.md verify stable physical SF afterward.

The image budget uses conservative text envelopes and a producer allowance
explained in docs/DECISIONS.md. It is not a Word-compatible layout engine. The
four tested LibreOffice pages preserve seventeen contained exact-pixel placements.
Source page sizes are emitted exactly in twips; the producer slightly normalizes
its paper width. Broader font/producer/section qualification remains unfinished.

Source hyperlinks pass independent DOCX reader/relationship checks. The tested
exporter omits URI annotations even in an independent python-docx control. To
reproduce that separate qualification gap:

```sh
output/qa-venv/bin/python scripts/qa/docx/verify_docx_running_images.py --write-uri-control
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file://"$PWD"/output/running-images/lo-profile --headless --convert-to pdf --outdir output/running-images/lo output/running-images/uri-control.docx
output/qa-venv/bin/python scripts/qa/docx/verify_docx_running_images.py
output/qa-venv/bin/python scripts/qa/docx/verify_docx_running_images.py --require-uri-annotations
```

The final strict URI gate currently fails (expected two, observed zero); the
independent control exports zero of one expected URI. Preserve those expectations
and observations. The broader rich-document goal remains incomplete.
