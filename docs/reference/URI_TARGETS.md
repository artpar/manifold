# Unicode URI targets in PDF

Updated 2026-11-24. The full goal remains incomplete; no external blocker prevents
continued development. This fixes target serialization and preserves existing
geometry, story ownership and source data.

## Contract and standards

`Run.Link` and SF `link.href` can hold Unicode IRI references. SF/legacy JSON and
DOCX retain the source string. The PDF writer maps UTF-8 octets to percent-encoded
ASCII, preserving existing percent escapes and reserved delimiters. It also
encodes excluded ASCII characters and controls. No normalization, host lookup,
network request, scheme-specific transformation or font choice is involved.
Valid ASCII URIs remain byte-identical; applying the mapping again is idempotent.
NFD and NFC resource names remain distinct. The reader keeps the serialized URI
reference rather than unescaping reserved bytes or inventing an original spelling.
This changes representation while preserving the resource identity of valid IRIs.

[ISO 32000-1:2008, Table 206](https://opensource.adobe.com/dc-acrobat-sdk-docs/pdfstandards/PDF32000_2008.pdf)
requires an ASCII URI string in a URI action. It is not a Unicode text-string
field. [RFC 3987 section 3.1](https://www.rfc-editor.org/rfc/rfc3987.html#section-3.1)
defines the equivalent UTF-8/percent mapping, including preservation of existing
escapes and delimiters and no normalization of already-Unicode input. Its optional
handling of excluded ASCII is also used. Scheme-specific resolution belongs to
the consuming application. The existing model's string validation policy remains.

`pdfASCIIURI` runs at the existing LinkString emission point in `internal/engine/pdf_text.go`.
Input models, text, font/glyph selections, rectangles, notes and source SF are
untouched. Relative references remain URI references; internal bookmark/named
PDF destination interpretation is still separate required recovery work. Typed
native note destinations keep their existing implementation. No private PDF
extension, schema change, public API change or new runtime dependency is added.

## Evidence and oracle policy

The real Word source contains three Unicode fragment targets that the earlier
writer stored as raw UTF-8 in an ASCII field. Independent readers interpreted
the bytes using PDFDocEncoding, turning U+2013 into `â•ﬁ`. The correct stored form
is `%E2%80%93`. The affected targets are CONNECT, CONNACK and Figure 4.3 fragments.
The old file has three unique wrong targets and four invalid non-ASCII URI
annotations, even though the target-set count is 434.

Original source fixtures and committed Word/PDF oracles are unchanged. The output
verifier now compares to Python urllib's independently calculated RFC mapping of
those source IRIs and also asserts actual URI bytes are ASCII. This is a format
serialization oracle, not an acceptance of the old corrupted values. The old
file still fails that same oracle; the corrected file matches all 434 references.
All page commands and annotation rectangles on all 207 pages are exact. The file
changes from 4,990,468 to 4,990,492 bytes. Existing font-subset, original JPEG,
metadata and U+2A6D4/U+2012 evidence remains valid. This does not establish complete
source text/structure/layout equivalence or Word-compatible rendering.

`examples/uri` exercises all four paths through SF with eight targets: RFC example,
IDN/path/query/fragment, mailto, decomposed Unicode, supplementary characters and
relative fragments. They occur in body, table cells, a list, a note, and repeated
headers/footers. Tests add existing escapes, IPv6/reserved punctuation, excluded
ASCII/controls, distinct NFC/NFD, exact expected URI octets, idempotence and source
ownership. A 1,000-execution byte-string fuzzer verifies ASCII output/idempotence.

Independent pypdf checks /S /URI and the raw ASCII annotation bytes, plus the one
existing native note /Dest. MuPDF reports the exact URI/file-reference targets;
it classifies relative fragments as file references, not external `uri` entries.
That classification is not a new launch action: pypdf verifies the stored action
remains /URI. Both reader paths agree on eight equivalent targets. DOCX import
retains all eight original Unicode strings; PDF→SF retains their URI encodings.
Repeated running labels have visible pixels on both pages, and both pages were
visually inspected. Three SF files pass actual ProseMirror with 84 nodes and all
marks/attributes. Full Go tests/race/vet, URI fuzz, module and CGO-disabled builds
are recorded in verification.json; Linux was cross-built, not executed.

## Reproduce and next work

```sh
GOWORK=off CGO_ENABLED=0 go run ./examples/uri
output/qa-venv/bin/python scripts/qa/layout/verify_uri.py
NODE_PATH=output/sf-qa/node_modules node scripts/qa/structured/verify_structured.cjs output/uri/source.sf output/uri/from-docx.sf output/uri/from-pdf.sf
GOWORK=off go test -run 'TestPDFIRI|TestPDFUnicodeURI' -v ./internal/engine
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFASCIIURI$' -fuzztime=1000x -parallel=2 -timeout=90s
```

Use FONT_SUBSETTING.md's explicit font mappings to regenerate the complete Word
render, then run `scripts/qa/layout/verify_mqtt_render.py`. It now passes the URI comparison;
its source data and other structural/font/asset checks remain independent. Ignored
output/uri/mqtt-before.pdf and mqtt-before-after.json retain the old corruption
and all-page invariance proof. Development downloads/PNG/JSON/PDF artifacts are
not committed or needed by the runtime.

Next implement source table width preferences through the shared SF and both
writers, using the visible narrow page-130 description as real evidence. Continue
source numbering patterns, internal destinations, broader rich-source/recovery
qualification and PDF→SF provenance/codec size under the original acceptance goal.
Keep all original fixtures/oracles; this milestone does not complete that goal.


## Subsequent Word split-attribute qualification

The 434-target evidence above belongs to its verified milestone. New source work
preserves Word's relationship + anchor composition, independently confirmed by the
original publisher PDF. The current Word-behavior oracle has 435 targets; the old
434-target raw attribute oracle remains unchanged and independently checked. See
INTERNAL_DESTINATIONS.md for the exact difference and remaining bookmark work.
