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
