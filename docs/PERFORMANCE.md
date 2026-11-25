# Load and stress testing

## Plan and scope

Baseline: the user's MacBook Pro (MacBookPro18,2), Apple M1 Max, 10 CPU cores,
32 GiB RAM, Darwin arm64, Go 1.25.0. This is an active desktop, not an isolated
production server. Record background CPU contention and run-to-run variation.
The test process uses GOMAXPROCS=10 and a 4096 MiB Go soft memory limit; that
limit is not an RSS cap. Race-instrumented executions are correctness checks,
never throughput results.

Use `tools/loadtest`, an independent Go development command calling public APIs:

1. Scout small/large real DOCX/PDF inputs, mixed-content SF conversions, and
   10/100-slide synthetic PPTX inspection, edits and PNG rendering at 72/144 DPI.
2. Run repeated concurrency sweeps through saturation; report successful jobs/s,
   p50/p95/p99 latency, completed/error/partial counts, allocations, GC and heap.
3. Hold selected workloads under sustained load and overdrive worker counts.
4. Apply open-loop arrival rates below/near/above measured capacity with a bounded
   queue; report rejects and end-to-end latency including queue/scheduler delay.
5. Collect separate CPU and heap/allocation profiles for identified expensive paths.
6. Exercise concurrent operations with the race detector, independently of timing.

A job includes input parsing and the named operation/output serialization.
Document input bytes are cached in RAM; PPTX APIs include file reads/writes on a
warm local filesystem. PPTX inspect returns its native object result without JSON
encoding. Conversion output is materialized, not sent to io.Discard. Each request
owns its mutable document/output. Input fixture hashes/sizes appear in raw output.
Original source documents and expectations remain unchanged. Synthetic fixtures
are reproducible and temporary; they do not establish real-deck visual fidelity.

One untimed warmup per phase populates process caches. Closed-loop workers submit
as soon as their previous job completes; this measures saturated API capacity,
not HTTP service capacity. Open-loop arrivals use scheduled absolute timestamps,
a queue of twice the worker count and immediate rejection when full. Queue delay
and scheduler lateness are included; no hidden unbounded backlog is allowed.
In-flight operations drain before throughput is calculated, and drain time is
reported. Memory is sampled every 20ms; short spikes can be missed. Output latency
percentiles use fixed logarithmic buckets with at most 2.19% upper-bound error.
Fast/full-GC instrumentation and the harness have some overhead; no correction is
subtracted. Forced GCs occur outside each timed interval for comparable starting
state and retained-heap checks; ordinary allocation/GC inside the interval counts.

No HTTP server, network/storage upload, authentication, durable queue or CLI
process startup is included. Jobs/s can be read as embedded operation TPS; API
capacity is not a measured end-to-end HTTP RPS or production latency guarantee.
Partial renderer responses are counted and explicitly reported separately from
fully supported results. Conversion diagnostics are explicitly acknowledged with
AllowLossy; throughput does not imply lossless or visual-equivalent conversion.

## Reproduction

Run from the repository root. Build once, outside measurement:

```sh
GOWORK=off go test ./tools/loadtest
CGO_ENABLED=0 GOWORK=off go build -o bin/manifold-loadtest ./tools/loadtest
bin/manifold-loadtest -list
bin/manifold-loadtest -duration 3s -repeat 3 -concurrency 1,2,4,8,16 > output/loadtest-matrix.jsonl
```

`-workloads` selects comma-separated names; `-rate` enables open-loop jobs/s;
`-procs`/`-memory-mib` set the process limits. `-cpuprofile` and `-memprofile` collect
separate profiling runs. Use `go tool pprof -top` and `-alloc_space` to inspect them.
Generated raw JSONL, temporary documents, binaries and profiles stay untracked in
output/ or bin/. Curated measured results and limitations belong in this document.

## Pipeline completion and preliminary memory finding

Capacity qualification paused while PPTX was connected to the central SF API.
The final matrix includes PPTX→SF, slide-SF→PPTX/PDF and PDF→PPTX. Slide fixtures
now contain explicit visible frames rather than minimal native test skeletons.
A mixed workload specifies its weights in the raw metadata; a varied-title edit/
render workload probes cache growth under distinct document content.

The preliminary 4-worker PDF profiling run found process-global font retention in
the old textlayout shaping path. After migration to request-owned go-text buffers,
three 8-second phases retained about 3.0/3.2/3.4 MiB post-GC instead of about
146/292/447 MiB. Existing shaping expectations were preserved. These were separate
profiled diagnostic runs, not final uninstrumented capacity results. The native
renderer result cache is now bounded to 4096 entries, with oversized keys excluded.

## Results

Measurement is in progress. No capacity claim is established by this plan alone.
