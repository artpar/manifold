# Original huge-file qualification

**Historical baseline:** current repaired import/persistence results are in
[HUGE_QUALIFICATION.md](HUGE_QUALIFICATION.md). The original acquisition, hashes and
zero-success observations below remain unchanged.

Updated 2026-11-25. This follows the user's explicit request to use actual files
from the internet. No duplicated pages, generated document payloads, padding,
rewritten packages, stripped metadata or skipped inference/structure are used.
Earlier synthetic huge-file scouts were stopped and excluded; their generator
and generated huge inputs were removed. Production code remains v0.3.0 (`45bebca`); these changes are QA/KT.

## Source acquisition and independent checks

[HUGE_CORPUS.json](HUGE_CORPUS.json) records exact URLs, sizes, hashes, archive
members and independently observed counts. Downloaded files stay ignored under
output/hugetest-real/. Only an original archive's named member is extracted;
its bytes are not repacked or otherwise changed. Public download access is not
an assertion of redistribution rights; publisher rights remain in force.

The corpus covers:

- IPCC AR6 WGI full report: original scientific report, 423,349,528 bytes,
  2,409 PDF pages. [Publisher](https://www.ipcc.ch/report/ar6/wg1/).
- Intel June 2024 SDM, revision 084: original combined manual, 25,169,252 bytes,
  5,057 PDF pages. [Original PDF](https://cdrdv2-public.intel.com/825743/325462-sdm-vol-1-2abcd-3abcd-4.pdf).
- ECMA-376 Part 1, fifth edition: original reference PDF extracted from the
  publisher archive, 35,304,993 bytes, 5,039 pages.
  [Publisher/archive links](https://ecma-international.org/publications-and-standards/standards/ecma-376/).
- 3GPP TS 38.331 V18.0.0: original DOCX, 3,891,288 bytes, 66,185 body XML
  paragraphs. [Publisher archive](https://www.3gpp.org/ftp/Specs/archive/38_series/38.331/38331-i00.zip).
- 3GPP TS 38.331 V18.1.0: original DOCX, 4,002,052 bytes, 68,220 body XML
  paragraphs. [Publisher archive](https://www.3gpp.org/ftp/Specs/archive/38_series/38.331/38331-i10.zip).
- Corresponding ETSI TS 138 331 V18.1.0: original 7,235,315-byte, 1,624-page PDF.
  [Publisher PDF](https://www.etsi.org/deliver/etsi_ts/138300_138399/138331/18.01.00_60/ts_138331v180100p.pdf).
- WG21 C++ N4950 working draft: original 8,043,551-byte, 2,134-page PDF.
  [Original committee document](https://www.open-std.org/jtc1/sc22/wg21/docs/papers/2023/n4950.pdf).

PyMuPDF 1.28.2 independently opened each PDF and counted physical pages; sampled
first/last text was checked where relevant. Python's ZIP/XML readers independently
counted DOCX document.xml paragraphs. Both Word files cache `Pages=1` in app.xml;
that value is stale, not a page-count oracle. The 1,624-page corresponding ETSI PDF
establishes publication scale, not exact Word pagination or byte-identical layout.
PyMuPDF also independently counted 279,501 Intel named destinations (longest
name 100 UTF-8 bytes); the engine registry caps names at 100,000 entries.
Independent Python tools are optional local QA; production remains pure Go.

## Results

All seven originals rejected before SF output in every ordinary attempt: 14
single-child jobs, 28 jobs in two-child batches, and four simultaneous IPCC jobs
(**0/46 successful conversions**). Normal runs returned explicit errors; none hit
the 120s/8 GiB harness bounds. Two additional real-file controls confirmed an
intentional one-second deadline kill and a 1 GiB sampled-RSS guard kill. They do
not establish hard OS memory safety. Peak ordinary combined sampled RSS was
7,763 MiB. Exact errors/timings/memory and next work are in
[PERFORMANCE.md](../PERFORMANCE.md#huge-real-input-results--2026-11-25).

This establishes specific compatibility/resource failures, not supported huge-file
throughput. No production feature/work limit was weakened and no original source
was changed. Successful huge-file serialization and sustained load remain
unmeasured until these import failures are resolved.


## Reproduction

Build the Go harness outside measurements. Download originals separately; hashes
are verified by the harness before starting each case. Commands below preserve
original bytes and extract only the named document from each publisher ZIP:

```sh
mkdir -p output/hugetest-real
curl -fL --max-time 300 -o output/hugetest-real/ipcc-ar6-wgi.pdf https://www.ipcc.ch/report/ar6/wg1/downloads/report/IPCC_AR6_WGI_FullReport.pdf
curl -fL --max-time 300 -o output/hugetest-real/intel-sdm.pdf https://cdrdv2-public.intel.com/825743/325462-sdm-vol-1-2abcd-3abcd-4.pdf
curl -fL --max-time 300 -o output/hugetest-real/ecma376-part1.zip https://ecma-international.org/wp-content/uploads/ECMA-376-1_5th_edition_december_2016.zip
unzip -p output/hugetest-real/ecma376-part1.zip 'Ecma Office Open XML Part 1 - Fundamentals And Markup Language Reference.pdf' > output/hugetest-real/ecma376-part1.pdf
curl -fL --max-time 300 -o output/hugetest-real/38331-i00.zip https://www.3gpp.org/ftp/Specs/archive/38_series/38.331/38331-i00.zip
unzip -p output/hugetest-real/38331-i00.zip 38331-i00.docx > output/hugetest-real/38331-i00.docx
curl -fL --max-time 300 -o output/hugetest-real/38331-i10.zip https://www.3gpp.org/ftp/Specs/archive/38_series/38.331/38331-i10.zip
unzip -p output/hugetest-real/38331-i10.zip 38331-i10.docx > output/hugetest-real/38331-i10.docx
curl -fL --max-time 300 -o output/hugetest-real/etsi-38331-v180100.pdf https://www.etsi.org/deliver/etsi_ts/138300_138399/138331/18.01.00_60/ts_138331v180100p.pdf
curl -fL --max-time 300 -o output/hugetest-real/cpp-n4950.pdf https://www.open-std.org/jtc1/sc22/wg21/docs/papers/2023/n4950.pdf
CGO_ENABLED=0 GOWORK=off go build -o bin/manifold-hugetest ./tools/hugetest
bin/manifold-hugetest -repeat 2 > output/huge-real-single.jsonl
bin/manifold-hugetest -workers 2 -repeat 2 > output/huge-real-concurrent.jsonl
```

This is an explicit opt-in stress command, not a slow/default CI test. It requires
Unix ps for RSS observation; production APIs/CLI require neither ps nor curl/unzip.
Result semantics and guarded process limits are in [PERFORMANCE.md](../PERFORMANCE.md).

## Regression checks

On this final QA change: GOWORK=off go test ./... passed (engine 27.239s);
go vet ./... passed; CGO_ENABLED=0 CLI build passed; git diff --check passed.
Race tool tests passed for tools/loadtest; tools/hugetest compiles with the race
detector but has no unit-test files. A separately built race-instrumented huge
harness ran two concurrent real ETSI-file children: both returned the expected
page-41 mask error, with no race detector findings. Instrumented timing/memory
are excluded from capacity and ordinary-attempt totals. No full local race suite
was rerun for this tooling/docs-only change; full hosted race/OS checks run on push.

Next commands after resolving source-specific import limits: rebuild the harness,
run the seven originals alone with -repeat 2, then -workers 2 -repeat 2, and only
then qualify completed-output throughput/soaks. All current failures remain open;
this report does not claim their implementation is fixed.
