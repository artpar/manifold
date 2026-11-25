// Command hugetest measures downloaded original documents in isolated child processes.
// It is development tooling, not a production service or a microbenchmark.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"crypto/sha256"
	"io"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/artpar/manifold"
)

type fixture struct {
	Name              string `json:"name"`
	Path              string `json:"path"`
	SourceURL         string `json:"source_url"`
	SHA256            string `json:"sha256"`
	Bytes             int64  `json:"bytes"`
	Pages             int    `json:"pdf_pages"`
	SourceParagraphs  int    `json:"source_docx_paragraphs,omitempty"`
	ProducerPageCount int    `json:"producer_page_count,omitempty"`
}

func emit(v any) { _ = json.NewEncoder(os.Stdout).Encode(v) }
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	selected := flag.String("cases", "all", "comma separated manifest case names or all")
	manifest := flag.String("manifest", "docs/verification/HUGE_CORPUS.json", "original-file manifest with source URLs and hashes")
	target := flag.String("to", "sf", "output format: sf, sf.gz, pdf, docx")
	storage := flag.String("storage", "memory", "memory or file: input/output materialization; file output is streamed to disk")
	outputDir := flag.String("output-dir", "", "optional directory retaining converted files (file storage only)")
	outputPath := flag.String("output", "", "internal output path")
	large := flag.Bool("large", false, "explicit large-document work policy: 4 GiB decoded bytes, 2B raster pixels, 1M navigation entries, 2M logical structure entries")
	memoryMiB := flag.Int64("memory-mib", 4096, "Go soft memory limit per child, MiB; not an RSS cap")
	workers := flag.Int("workers", 1, "isolated concurrent conversions per batch")
	repeat := flag.Int("repeat", 2, "batches per case")
	timeout := flag.Duration("timeout", 120*time.Second, "hard child process deadline")
	guard := flag.Int64("rss-mib", 8192, "sampled aggregate child RSS guard; requires ps on Unix")
	profile := flag.String("cpuprofile", "", "child-only diagnostic CPU profile")
	operationTimeout := flag.Duration("operation-timeout", 0, "child-only graceful conversion deadline; zero disables")
	worker := flag.Bool("worker", false, "internal child mode")
	input := flag.String("input", "", "internal input path")
	pages := flag.Int("pages", 0, "internal expected markers")

	flag.Parse()
	if *worker {
		return convert(*input, manifold.Format(*target), *pages, *profile, *operationTimeout, *large, *storage, *outputPath, *memoryMiB)
	}
	if *storage != "memory" && *storage != "file" {
		return fmt.Errorf("invalid storage mode")
	}
	if *outputDir != "" {
		if *storage != "file" {
			return fmt.Errorf("output-dir requires file storage")
		}
		if e := os.MkdirAll(*outputDir, 0700); e != nil {
			return e
		}
	}
	if *memoryMiB < 1 {
		return fmt.Errorf("invalid memory limit")
	}
	if *workers < 1 || *repeat < 1 || *timeout <= 0 || *guard < 1 {
		return fmt.Errorf("invalid batch bounds")
	}
	if *target != "sf" && *target != "sf.gz" && *target != "pdf" && *target != "docx" {
		return fmt.Errorf("invalid output format")
	}
	if _, e := exec.LookPath("ps"); e != nil {
		return fmt.Errorf("RSS guard requires Unix ps: %w", e)
	}
	manifestBytes, e := os.ReadFile(*manifest)
	if e != nil {
		return e
	}
	var fixtures []fixture
	if e = json.Unmarshal(manifestBytes, &fixtures); e != nil {
		return e
	}
	if len(fixtures) == 0 {
		return fmt.Errorf("empty source manifest")
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	emit(map[string]any{"kind": "environment", "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "cpus": runtime.NumCPU(), "soft_memory_mib_per_child": *memoryMiB, "rss_guard_mib_batch": *guard, "timeout": timeout.String(), "started_utc": time.Now().UTC(), "large_document_work_policy": *large, "storage": *storage, "measurement": "cold process; storage mode recorded; file read + parse + content checks + fully materialized SHA-256 output; buffered file writes without fsync; separate timed parse/write; no warmup; unmodified downloaded originals"})
	baseCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	wanted := map[string]bool{}
	for _, n := range strings.Split(*selected, ",") {
		wanted[n] = false
	}
	for _, f := range fixtures {
		if _, ok := wanted[f.Name]; !ok && *selected != "all" {
			continue
		}
		wanted[f.Name] = true
		path := f.Path
		source, e := os.Open(path)
		if e != nil {
			return e
		}
		h := sha256.New()
		size, e := io.Copy(h, source)
		source.Close()
		if e != nil {
			return e
		}
		hash := fmt.Sprintf("%x", h.Sum(nil))
		if hash != f.SHA256 || size != f.Bytes {
			return fmt.Errorf("source integrity mismatch for %s", f.Name)
		}
		emit(map[string]any{"kind": "fixture", "name": f.Name, "bytes": size, "sha256": hash, "source_url": f.SourceURL, "pdf_pages": f.Pages, "source_docx_paragraphs": f.SourceParagraphs, "producer_page_count": f.ProducerPageCount})
		for rep := 1; rep <= *repeat; rep++ {
			if err := baseCtx.Err(); err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(baseCtx, *timeout)
			cmds := make([]*exec.Cmd, *workers)
			outs := make([]bytes.Buffer, *workers)
			errs := make([]bytes.Buffer, *workers)
			completed := make(chan int, *workers)
			var wg sync.WaitGroup
			start := time.Now()
			for i := range cmds {
				args := []string{"-worker", "-input", path, "-to", *target, "-pages", strconv.Itoa(f.Pages), "-storage", *storage, "-memory-mib", strconv.FormatInt(*memoryMiB, 10)}
				if *outputDir != "" {
					args = append(args, "-output", filepath.Join(*outputDir, fmt.Sprintf("%s-w%d.%s", f.Name, i, *target)))
				}
				if *large {
					args = append(args, "-large")
				}
				cmd := exec.CommandContext(ctx, exe, args...)
				cmd.Stdout = &outs[i]
				cmd.Stderr = &errs[i]
				cmds[i] = cmd
				if e := cmd.Start(); e != nil {
					cancel()
					return e
				}
				wg.Add(1)
				go func(i int) { defer wg.Done(); _ = cmds[i].Wait(); completed <- i }(i)
			}
			deadlineDone := ctx.Done()
			active := map[int]bool{}
			for i := range cmds {
				active[i] = true
			}
			tick := time.NewTicker(200 * time.Millisecond)
			peakRSS := int64(0)
			reason := ""
			for len(active) > 0 {
				select {
				case i := <-completed:
					delete(active, i)
				case <-tick.C:
					var ids []string
					for i := range active {
						ids = append(ids, strconv.Itoa(cmds[i].Process.Pid))
					}
					probe, stop := context.WithTimeout(context.Background(), time.Second)
					raw, err := exec.CommandContext(probe, "ps", "-o", "rss=", "-p", strings.Join(ids, ",")).Output()
					stop()
					if err == nil {
						var rss int64
						for _, n := range strings.Fields(string(raw)) {
							v, _ := strconv.ParseInt(n, 10, 64)
							rss += v
						}
						peakRSS = max(peakRSS, rss)
						if rss > *guard*1024 {
							reason = "rss_guard"
							cancel()
						}
					}
				case <-deadlineDone:
					deadlineDone = nil
					if reason == "" {
						reason = "deadline"
						if baseCtx.Err() != nil {
							reason = "cancelled"
						}
					}
					for i := range active {
						_ = cmds[i].Process.Kill()
					}
				}
			}
			tick.Stop()
			wg.Wait()
			cancel()
			success := 0
			for i, cmd := range cmds {
				var records []json.RawMessage
				for _, l := range bytes.Split(bytes.TrimSpace(outs[i].Bytes()), []byte("\n")) {
					if json.Valid(l) {
						records = append(records, bytes.Clone(l))
					}
				}
				ok := cmd.ProcessState.Success()
				if ok {
					success++
				}
				emit(map[string]any{"kind": "child", "case": f.Name, "to": *target, "repeat": rep, "workers": *workers, "worker": i, "success": ok, "exit_code": cmd.ProcessState.ExitCode(), "records": records, "stderr": errs[i].String()})
			}
			wall := time.Since(start).Seconds()
			emit(map[string]any{"kind": "batch", "case": f.Name, "to": *target, "repeat": rep, "workers": *workers, "success": success, "wall_seconds": wall, "successful_jobs_per_second": float64(success) / wall, "sampled_peak_rss_mib": float64(peakRSS) / 1024, "termination": reason})
		}
	}
	for n, found := range wanted {
		if n != "all" && !found {
			return fmt.Errorf("unknown case %q", n)
		}
	}
	return nil
}
func convert(path string, to manifold.Format, pages int, profile string, operationTimeout time.Duration, large bool, storage, outputPath string, memoryMiB int64) error {
	ctx := context.Background()
	if operationTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, operationTimeout)
		defer cancel()
	}
	if profile != "" {
		f, e := os.Create(profile)
		if e != nil {
			return e
		}
		if e = pprof.StartCPUProfile(f); e != nil {
			f.Close()
			return e
		}
		defer func() { pprof.StopCPUProfile(); f.Close() }()
	}

	runtime.GOMAXPROCS(runtime.NumCPU())
	debug.SetMemoryLimit(memoryMiB << 20)
	start := time.Now()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	var mu sync.Mutex
	var peak uint64
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(20 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				mu.Lock()
				peak = max(peak, m.HeapAlloc)
				mu.Unlock()
			}
		}
	}()
	defer func() { close(stop); <-done }()

	var source io.Reader
	var data []byte
	var inputBytes int64
	if storage == "file" {
		file, e := os.Open(path)
		if e != nil {
			return e
		}
		defer file.Close()
		st, e := file.Stat()
		if e != nil {
			return e
		}
		inputBytes = st.Size()
		source = file
	} else {
		var e error
		data, e = os.ReadFile(path)
		if e != nil {
			return e
		}
		inputBytes = int64(len(data))
		source = bytes.NewReader(data)
	}

	format, e := manifold.FormatFromName(path)
	if e != nil {
		return e
	}
	beginRead := time.Now()
	opts := manifold.ReadOptions{}
	if large {
		opts.PDFLimits = &manifold.PDFReadLimits{MaxDecodedBytes: 4 << 30, MaxRasterPixels: 2_000_000_000, MaxNavigationEntries: 1_000_000, MaxStructureEntries: 2_000_000}
	}
	d, e := manifold.Read(ctx, source, format, opts)
	if e != nil {
		return e
	}
	readSeconds := time.Since(beginRead).Seconds()
	textBytes, paragraphs := 0, 0
	pictures, breaks := 0, 0
	var walk func([]manifold.Block)
	walk = func(bs []manifold.Block) {
		for _, b := range bs {
			if b.Section != nil {
				walk(b.Section.Blocks)
			}
			if b.Kind == manifold.Picture {
				pictures++
			}
			if b.Kind == manifold.PageBreak {
				breaks++
			}
			if b.Kind == manifold.Paragraph || b.Kind == manifold.Heading {
				paragraphs++
			}
			for _, r := range b.Runs {
				textBytes += len(r.Text)
			}
			for _, row := range b.Rows {
				for _, c := range row.Cells {
					walk(c.Blocks)
				}
			}
			for _, it := range b.Items {
				walk(it.Blocks)
			}
		}
	}
	walk(d.Blocks)
	if format == manifold.PDF && len(d.Pages) != pages {
		return fmt.Errorf("PDF pages %d/%d", len(d.Pages), pages)
	}
	if textBytes == 0 && pictures == 0 {
		return fmt.Errorf("empty recovered document")
	}

	codes := map[string]int{}
	for _, x := range d.Diagnostics {
		codes[x.Code]++
	}
	emit(map[string]any{"stage": "read_complete", "read_seconds": readSeconds, "elapsed_seconds": time.Since(start).Seconds(), "input_bytes": inputBytes, "recovered_text_bytes": textBytes, "recovered_paragraphs": paragraphs, "images": pictures, "explicit_breaks": breaks, "physical_pages": len(d.Pages), "diagnostics": codes})

	var out bytes.Buffer
	var destination io.Writer = &out
	var outputFile *os.File
	var temporary string
	if storage == "file" {
		dir := ""
		if outputPath != "" {
			dir = filepath.Dir(outputPath)
		}
		outputFile, e = os.CreateTemp(dir, ".manifold-huge-*")
		if e != nil {
			return e
		}
		temporary = outputFile.Name()
		defer os.Remove(temporary)
		defer outputFile.Close()
		destination = outputFile
	}
	hash := sha256.New()
	destination = io.MultiWriter(destination, hash)
	beginWrite := time.Now()
	e = manifold.Write(ctx, destination, d, to, manifold.WriteOptions{AllowLossy: true})
	if e != nil {
		return e
	}
	outputBytes := int64(out.Len())
	if outputFile != nil {
		st, e := outputFile.Stat()
		if e != nil {
			return e
		}
		outputBytes = st.Size()
		if e = outputFile.Close(); e != nil {
			return e
		}
		if outputPath != "" {
			if e = os.Rename(temporary, outputPath); e != nil {
				return e
			}
		}
	}

	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	mu.Lock()
	peak = max(peak, after.HeapAlloc)
	finalPeak := peak
	mu.Unlock()
	emit(map[string]any{"stage": "complete", "read_seconds": readSeconds, "write_seconds": time.Since(beginWrite).Seconds(), "total_seconds": time.Since(start).Seconds(), "output_bytes": outputBytes, "output_sha256": fmt.Sprintf("%x", hash.Sum(nil)), "storage": storage, "sampled_peak_heap_mib": float64(finalPeak) / (1 << 20), "allocated_mib": float64(after.TotalAlloc-before.TotalAlloc) / (1 << 20), "gc_cycles": after.NumGC - before.NumGC, "gc_pause_ms": float64(after.PauseTotalNs-before.PauseTotalNs) / 1e6})
	runtime.KeepAlive(data)
	runtime.KeepAlive(d)
	runtime.KeepAlive(out)
	return nil
}
