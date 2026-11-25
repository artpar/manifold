# Testing and qualification

Production and ordinary tests require only Go. Python, Node, font tools and
LibreOffice are optional independent QA, never runtime requirements.

## Required checks

```sh
GOWORK=off go test ./...
GOWORK=off go test -race -timeout 20m ./...
GOWORK=off go vet ./...
GOWORK=off go mod verify
CGO_ENABLED=0 GOWORK=off go build -o bin/manifold ./cmd/manifold
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOWORK=off go build -o bin/manifold-linux-amd64 ./cmd/manifold
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GOWORK=off go build -o bin/manifold-linux-arm64 ./cmd/manifold
git diff --check
```

The custom-font race seed can exceed Go's default package timeout; 20 minutes
changes the time limit, not test selection. Linux cross-builds prove compilation,
not Linux execution. Do not repeat full suites without changed code or an unresolved
concern; run focused checks while implementing, then qualify the final snapshot.

## Test placement and fixtures

`public_api_test.go` tests the public facade from an external package. Engine unit,
structural, round-trip, malformed and fuzz tests live next to their implementation
under `internal/engine`. The engine TestMain sets the repository as the working
directory once, before tests/fuzz workers, so the existing corpus/QA output paths
remain stable. Test processes remain separate from CLI/public API tests.

`testdata/corpus` contains named synthetic/malformed cases; `testdata/real` contains
pinned original exports and independent source-only oracles. Preserve their bytes,
hashes and expected behavior. Do not change an oracle just to match output.

```sh
GOWORK=off go test -run 'TestPDFTaggedNotePlacementKind|TestPDFParentTree' ./internal/engine
GOWORK=off go test -run '^$' -fuzz '^FuzzReadPDF$' -fuzztime=15s -parallel=2 ./internal/engine
```

Fuzzing and resource guards are scoped evidence, not exhaustive safety claims.
Preserve discovered failures as permanent regression seeds.

## Independent QA

```sh
CGO_ENABLED=0 GOWORK=off go build -o bin/manifold ./cmd/manifold
GOWORK=off go run ./examples/sections
output/qa-venv/bin/python scripts/qa/structured/verify_structured_corpus.py
output/qa-venv/bin/python scripts/qa/layout/verify_sections.py
output/qa-venv/bin/python scripts/qa/docx/generate_poi_qa.py
output/qa-venv/bin/python scripts/qa/docx/verify_poi_corpus.py
```

Optional environments/dependencies are declared in `scripts/qa/requirements.txt`
and the relevant reference contracts. Producer renders and downloaded fonts stay
ignored. Reader structure/content/navigation/assets, physical geometry and visual
checks are distinct. Plain text agreement alone does not qualify structured
conversion. Do not claim Word-equivalent layout from synthetic outputs.

See [QA tools](../scripts/qa/README.md), [current evidence](verification.json),
[acceptance](verification/FINAL_AUDIT.md) and [historical qualification](history/README.md).


## Hosted automation

GitHub Actions runs the required Go matrix and publishes new VERSION values only
after it passes. See [CI/CD](CI.md) for triggers, platform coverage, artifact
contents and retry rules. Hosted run logs and release checksums are the source of
truth for new releases; workstation-built artifacts are not the release path.


## Integrated presentation checks

The ordinary Go/race/vet/build commands include internal/presentation and the
public presentation facade. Public API and CLI tests exercise the integration;
original package-preservation/edit/renderer tests retain their content oracles.
See [the integration record](verification/PRESENTATION_INTEGRATION.md) for actual
checks, hosted results, the Windows JSON fixture fix and skipped external visual
qualification. Rendering a valid PNG is not a PowerPoint visual-parity assertion.

## Huge original-file repair qualification

Publisher URLs, exact SHA-256 values, unmodified extracted members and download
commands are in `verification/HUGE_CORPUS.json` and `verification/HUGE_INPUTS.md`.
Publisher bytes, generated SF, binaries, environments and profiles remain ignored.
Do not replace those originals with generated or padded documents.

The pure-Go runner uses isolated cold child processes, explicit work policy,
Go soft memory targets, sampled external RSS guards, context/deadline controls and
fully materialized output SHA-256. OS file data is warmed by the integrity check;
file output is buffered without fsync. `-storage file` avoids the old caller-held
input/output buffers, and is recorded separately from that historical baseline.

```sh
CGO_ENABLED=0 GOWORK=off go build -o bin/manifold-hugetest ./tools/hugetest
bin/manifold-hugetest -large -memory-mib 8192 -rss-mib 12288 \
  -storage file -output-dir output/huge-final-output -repeat 2 -timeout 10m
bin/manifold-hugetest -large -memory-mib 8192 -rss-mib 20480 \
  -storage file -repeat 1 -workers 2 -timeout 15m
```

A runner exit status alone is not a conversion result. Inspect `child.success`,
its error/stages, and `batch.termination`; budget rejection, timeout and RSS kills
are never counted as successful capacity. Do not benchmark while running other QA.
The aggregate guard covers child RSS, not all operating-system memory.

Original-content and derived-persistence checks are separate:

```sh
MANIFOLD_HUGE_IMAGES=1 GOWORK=off go test ./internal/engine \
  -run 'TestHugeOriginal(ImagePixels|GlyphOutlines)' -v -count=1
MANIFOLD_HUGE_WORD=1 GOWORK=off go test ./internal/engine \
  -run TestHugeOriginalWordAssets -v -count=1
MANIFOLD_HUGE_SF=ecma376-part1 GOWORK=off go test ./internal/engine \
  -run TestHugeOriginalDerivedSFRead -v -count=1 -timeout 10m
```

The Word test checks all 73 preview placements / 71 distinct original media hashes
and 15/21 native equations through SF and DOCX. Derived SF tests require identical
SHA-256 after central Go Read/Write; they are not additional Internet originals.
Optional independent Python/Node readers use `scripts/qa/requirements.txt` plus
`prosemirror-model` installed in ignored `output/sf-qa/node_modules`:

```sh
output/qa-venv/bin/python scripts/qa/structured/huge_audit.py \
  output/hugetest-real/ipcc-ar6-wgi.pdf \
  output/huge-final-output/ipcc-ar6-wgi-w0.sf
```

The audit streams body nodes through actual ProseMirror, inventories normalized
nonwhitespace characters by source page and globally, separately counts original
list labels retained in provenance, and checks Word native math structures/assets.
Character inventories do not prove reading order, pagination or visual parity.
MuPDF `get_image_info()` also reports raster tiles it creates for vector shading:
on original IPCC page 62, 12 additional RGB occurrences correspond to 12 `sh`
operators with xref=0; the one embedded CMYK image is retained. Do not mislabel
those derived tiles as missing embedded image assets. Shading/vector reconstruction
remains explicitly diagnosed as unsupported appearance recovery.

## Huge-original CPU and heap diagnosis

See [the pprof investigation](verification/HUGE_PPROF.md) for source-backed
allocation sites, retained model sizes, the modest buffering A/B and caveats.
Use fresh profile roots; a repeated root/case/repeat/worker overwrites its files.
Never treat diagnostic runs as replacement throughput measurements.

```sh
CGO_ENABLED=0 GOWORK=off go build -o bin/manifold-hugeprof ./tools/hugetest
bin/manifold-hugeprof -large -memory-mib 8192 -rss-mib 12288 \
  -storage file -cases ipcc-ar6-wgi,intel-sdm-084,ecma376-part1 \
  -repeat 1 -timeout 15m -profile-mode cpu -profile-dir output/pprof-new/cpu
bin/manifold-hugeprof -large -memory-mib 8192 -rss-mib 12288 \
  -storage file -cases ipcc-ar6-wgi,intel-sdm-084,ecma376-part1 \
  -repeat 1 -timeout 15m -profile-mode memory -profile-dir output/pprof-new/memory
go tool pprof -top output/pprof-new/cpu/ecma376-part1-r1-w0/read.cpu.pprof
go tool pprof -top -sample_index=inuse_space \
  output/pprof-new/memory/ecma376-part1-r1-w0/read_live.heap.pprof
go tool pprof -top -sample_index=alloc_space \
  -base output/pprof-new/memory/ecma376-part1-r1-w0/read_live.heap.pprof \
  output/pprof-new/memory/ecma376-part1-r1-w0/write_live.heap.pprof
```

Memory mode records baseline, natural read/write ends, explicit two-GC read/write
live models, and the released two-GC heap. The document remains alive at live-model
snapshots. MemStats phase deltas are exact counters; pprof site attribution is
sampled. Inspect both alloc_space/alloc_objects and inuse_space/inuse_objects.
`-profile-output-buffer 1048576` adds a diagnostic caller-side file buffer; zero
preserves normal buffering. Underlying write counters and child OS user/system CPU
allow cross-checking syscall attribution. Flush errors propagate and flush time
is included. Neither this option nor profiling changes production codec defaults.
