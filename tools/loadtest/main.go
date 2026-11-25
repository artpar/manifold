// Command loadtest measures embedded Manifold operations; it is not a server.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type histogram [2048]uint64

func (h *histogram) add(d time.Duration) {
	i := 0
	if d > 0 {
		i = int(math.Ceil(math.Log2(float64(d)) * 32))
	}
	if i >= len(h) {
		i = len(h) - 1
	}
	h[i]++
}
func (h *histogram) quantile(q float64) float64 {
	var n uint64
	for _, v := range h {
		n += v
	}
	if n == 0 {
		return 0
	}
	target := uint64(math.Ceil(float64(n) * q))
	var sum uint64
	for i, v := range h {
		sum += v
		if sum >= target {
			return math.Exp2(float64(i)/32) / 1e6
		}
	}
	return 0
}
func (h *histogram) merge(other *histogram) {
	for i, v := range other {
		h[i] += v
	}
}

type observation struct {
	diagnostic string
	bytes      int64
	partial    bool
}
type operation func(context.Context) (observation, error)
type workload struct {
	Name        string
	InputBytes  int
	InputSHA256 string
	Description string
	prepare     func(int) operation
}
type workerStats struct {
	success, failed, partial uint64
	bytes                    int64
	latency, service         histogram
	max                      time.Duration
	errors                   map[string]uint64
}
type result struct {
	Kind                                        string `json:"kind"`
	WarmupDiagnostic                            string `json:"warmup_diagnostic,omitempty"`
	Workload                                    string `json:"workload"`
	Workers, Repeat                             int
	TargetRPS                                   float64 `json:"target_rps"`
	ArrivalSeconds                              float64 `json:"arrival_seconds"`
	MaxGeneratorLatenessMS                      float64 `json:"max_generator_lateness_ms"`
	RequestedSeconds                            float64 `json:"requested_seconds"`
	WallSeconds                                 float64 `json:"wall_seconds"`
	DrainSeconds                                float64 `json:"drain_seconds"`
	Offered, Rejected, Success, Errors, Partial uint64
	SuccessfulJobsPerSecond                     float64           `json:"successful_jobs_per_second"`
	AchievedArrivalsPerSecond                   float64           `json:"achieved_arrivals_per_second"`
	P50ms                                       float64           `json:"p50_ms"`
	P95ms                                       float64           `json:"p95_ms"`
	P99ms                                       float64           `json:"p99_ms"`
	MaxMS                                       float64           `json:"max_ms"`
	ServiceP95MS                                float64           `json:"service_p95_ms"`
	AllocBytesPerJob                            float64           `json:"alloc_bytes_per_attempt"`
	AllocsPerJob                                float64           `json:"allocs_per_attempt"`
	PeakHeapMiB                                 float64           `json:"sampled_peak_heap_mib"`
	EndHeapMiB                                  float64           `json:"end_heap_mib"`
	RetainedHeapMiB                             float64           `json:"post_gc_heap_mib"`
	NumGC                                       uint32            `json:"gc_cycles"`
	GCPauseMS                                   float64           `json:"gc_pause_ms"`
	OutputBytes                                 int64             `json:"output_bytes"`
	ErrorMessages                               map[string]uint64 `json:"error_messages,omitempty"`
}

func runPhase(ctx context.Context, w workload, workers, repeat int, duration time.Duration, rate float64) (result, error) {
	ops := make([]operation, workers)
	for i := range ops {
		ops[i] = w.prepare(i)
	}
	warmup, err := ops[0](ctx)
	if err != nil {
		return result{}, fmt.Errorf("warmup %s: %w", w.Name, err)
	}
	runtime.GC()
	var before, after, retained runtime.MemStats
	runtime.ReadMemStats(&before)
	stats := make([]workerStats, workers)
	for i := range stats {
		stats[i].errors = map[string]uint64{}
	}
	var peak uint64 = before.HeapAlloc
	sampleStop, sampleDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(sampleDone)
		t := time.NewTicker(20 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-sampleStop:
				return
			case <-t.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				if m.HeapAlloc > peak {
					peak = m.HeapAlloc
				}
			}
		}
	}()
	startGate := make(chan struct{})
	var wg sync.WaitGroup
	start := time.Now()
	end := start.Add(duration)
	execute := func(i int, arrived time.Time) {
		began := time.Now()
		o, err := ops[i](ctx)
		finished := time.Now()
		s := &stats[i]
		s.service.add(finished.Sub(began))
		s.latency.add(finished.Sub(arrived))
		if d := finished.Sub(arrived); d > s.max {
			s.max = d
		}
		if err != nil {
			s.failed++
			if len(s.errors) < 8 {
				s.errors[err.Error()]++
			}
		} else {
			s.success++
			s.bytes += o.bytes
			if o.partial {
				s.partial++
			}
		}
	}
	var offered, rejected uint64
	arrivalSeconds := duration.Seconds()
	var generatorLateness time.Duration
	if rate == 0 {
		for i := range workers {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-startGate
				for time.Now().Before(end) && ctx.Err() == nil {
					execute(i, time.Now())
				}
			}(i)
		}
		runtime.ReadMemStats(&before)
		start = time.Now()
		end = start.Add(duration)
		close(startGate)
		wg.Wait()
	} else {
		queue := make(chan time.Time, workers*2)
		for i := range workers {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-startGate
				for at := range queue {
					execute(i, at)
				}
			}(i)
		}
		runtime.ReadMemStats(&before)
		start = time.Now()
		end = start.Add(duration)
		close(startGate)
		// Absolute arrival times retain queueing delay and scheduler lateness. The
		// bounded queue rejects overload instead of hiding it behind blocked senders.
		timer := time.NewTimer(time.Hour)
		if !timer.Stop() {
			<-timer.C
		}
		count := uint64(math.Ceil(duration.Seconds() * rate))
	arrivalLoop:
		for n := uint64(0); n < count; n++ {
			at := start.Add(time.Duration(float64(n) * float64(time.Second) / rate))
			if delay := time.Until(at); delay > 0 {
				timer.Reset(delay)
				select {
				case <-timer.C:
				case <-ctx.Done():
					break arrivalLoop
				}
			}
			if lag := time.Since(at); lag > generatorLateness {
				generatorLateness = lag
			}
			offered++
			select {
			case queue <- at:
			default:
				rejected++
			}
		}
		arrivalSeconds = math.Max(arrivalSeconds, time.Since(start).Seconds())
		timer.Stop()
		close(queue)
		wg.Wait()
	}
	elapsed := time.Since(start)
	close(sampleStop)
	<-sampleDone
	runtime.ReadMemStats(&after)
	if after.HeapAlloc > peak {
		peak = after.HeapAlloc
	}
	runtime.GC()
	runtime.ReadMemStats(&retained)
	r := result{Kind: "phase", WarmupDiagnostic: warmup.diagnostic, Workload: w.Name, Workers: workers, Repeat: repeat, TargetRPS: rate, RequestedSeconds: duration.Seconds(), WallSeconds: elapsed.Seconds(), DrainSeconds: math.Max(0, elapsed.Seconds()-duration.Seconds()), PeakHeapMiB: float64(peak) / 1048576, EndHeapMiB: float64(after.HeapAlloc) / 1048576, RetainedHeapMiB: float64(retained.HeapAlloc) / 1048576, NumGC: after.NumGC - before.NumGC, GCPauseMS: float64(after.PauseTotalNs-before.PauseTotalNs) / 1e6, ErrorMessages: map[string]uint64{}}
	var latency, service histogram
	for _, s := range stats {
		r.Success += s.success
		r.Errors += s.failed
		r.Partial += s.partial
		r.OutputBytes += s.bytes
		latency.merge(&s.latency)
		service.merge(&s.service)
		if d := float64(s.max) / 1e6; d > r.MaxMS {
			r.MaxMS = d
		}
		for e, n := range s.errors {
			r.ErrorMessages[e] += n
		}
	}
	attempts := r.Success + r.Errors
	if rate == 0 {
		offered = attempts
	}
	r.Offered = offered
	r.Rejected = rejected
	r.SuccessfulJobsPerSecond = float64(r.Success) / r.WallSeconds
	r.ArrivalSeconds = arrivalSeconds
	r.MaxGeneratorLatenessMS = float64(generatorLateness) / 1e6
	r.AchievedArrivalsPerSecond = float64(offered) / arrivalSeconds
	r.P50ms = latency.quantile(.5)
	r.P95ms = latency.quantile(.95)
	r.P99ms = latency.quantile(.99)
	r.ServiceP95MS = service.quantile(.95)
	if attempts > 0 {
		r.AllocBytesPerJob = float64(after.TotalAlloc-before.TotalAlloc) / float64(attempts)
		r.AllocsPerJob = float64(after.Mallocs-before.Mallocs) / float64(attempts)
	}
	return r, ctx.Err()
}
func digest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }
func parseWorkers(s string) ([]int, error) {
	var out []int
	for _, x := range strings.Split(s, ",") {
		n, err := strconv.Atoi(x)
		if err != nil || n < 1 || n > 128 {
			return nil, errors.New("workers must be integers from 1 to 128")
		}
		out = append(out, n)
	}
	return out, nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	selected := flag.String("workloads", "all", "comma separated workload names or all")
	concurrency := flag.String("concurrency", "1,2,4,8,16", "worker counts")
	duration := flag.Duration("duration", 3*time.Second, "measurement duration per phase; in-flight jobs are drained")
	repeat := flag.Int("repeat", 3, "repeats per worker count")
	rate := flag.Float64("rate", 0, "open-loop jobs/sec; zero uses closed-loop workers")
	procs := flag.Int("procs", runtime.NumCPU(), "GOMAXPROCS")
	memory := flag.Int64("memory-mib", 4096, "Go runtime soft memory limit, MiB; 0 disables")
	cpu := flag.String("cpuprofile", "", "optional CPU profile destination")
	heap := flag.String("memprofile", "", "optional heap/allocation profile destination")
	list := flag.Bool("list", false, "only list fixtures/workloads")
	flag.Parse()
	if *duration <= 0 || *repeat < 1 || *rate < 0 || *rate > 100000 || *procs < 1 || *memory < 0 {
		return errors.New("invalid duration/repeat/rate/procs/memory")
	}
	workers, err := parseWorkers(*concurrency)
	if err != nil {
		return err
	}
	runtime.GOMAXPROCS(*procs)
	if *memory > 0 {
		debug.SetMemoryLimit(*memory * 1048576)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	dir, err := os.MkdirTemp("", "manifold-loadtest-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	workloads, err := makeWorkloads(ctx, dir)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	revision := ""
	dirty := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				revision = s.Value
			}
			if s.Key == "vcs.modified" {
				dirty = s.Value
			}
		}
	}
	if err := enc.Encode(map[string]any{"kind": "environment", "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "cpus": runtime.NumCPU(), "gomaxprocs": runtime.GOMAXPROCS(0), "soft_memory_limit_mib": *memory, "revision": revision, "modified": dirty, "started_utc": time.Now().UTC(), "latency_histogram_relative_error_max": math.Exp2(1.0/32) - 1, "measurement": "warm embedded API; document input in RAM; PPTX uses warm filesystem; includes output serialization; no HTTP or process startup"}); err != nil {
		return err
	}
	names := strings.Split(*selected, ",")
	wanted := func(n string) bool {
		if *selected == "all" {
			return true
		}
		for _, x := range names {
			if x == n {
				return true
			}
		}
		return false
	}
	found := map[string]bool{}
	for _, w := range workloads {
		if wanted(w.Name) {
			found[w.Name] = true
			if err := enc.Encode(map[string]any{"kind": "fixture", "name": w.Name, "input_bytes": w.InputBytes, "sha256": w.InputSHA256, "description": w.Description}); err != nil {
				return err
			}
		}
	}
	for _, n := range names {
		if n != "all" && !found[n] {
			return fmt.Errorf("unknown workload %q", n)
		}
	}
	if *list {
		return nil
	}
	if *cpu != "" {
		f, err := os.Create(*cpu)
		if err != nil {
			return err
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			f.Close()
			return err
		}
		defer func() { pprof.StopCPUProfile(); f.Close() }()
	}
	// Rotate worker-count order between repeats to reduce order/thermal bias.
	for rep := 1; rep <= *repeat; rep++ {
		order := append([]int(nil), workers...)
		if rep%2 == 0 {
			sort.Sort(sort.Reverse(sort.IntSlice(order)))
		}
		for _, w := range workloads {
			if !wanted(w.Name) {
				continue
			}
			for _, n := range order {
				r, err := runPhase(ctx, w, n, rep, *duration, *rate)
				if err != nil {
					return err
				}
				if err := enc.Encode(r); err != nil {
					return err
				}
			}
		}
	}
	if *heap != "" {
		f, err := os.Create(filepath.Clean(*heap))
		if err != nil {
			return err
		}
		defer f.Close()
		runtime.GC()
		if err := pprof.WriteHeapProfile(f); err != nil {
			return err
		}
	}
	return nil
}
