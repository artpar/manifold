package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"
)

type profileConfig struct {
	Directory string
	Mode      string
	MemRate   int
	LegacyCPU string
}
type diagnosticProfiler struct {
	config   profileConfig
	wholeCPU *os.File
}

func newDiagnosticProfiler(c profileConfig) (*diagnosticProfiler, error) {
	p := &diagnosticProfiler{config: c}
	if c.Directory != "" {
		if c.Mode != "cpu" && c.Mode != "memory" {
			return nil, fmt.Errorf("profile-mode must be cpu or memory")
		}
		if c.MemRate <= 0 {
			return nil, fmt.Errorf("memory profile rate must be positive")
		}
		if err := os.MkdirAll(c.Directory, 0700); err != nil {
			return nil, err
		}
		if c.Mode == "memory" {
			runtime.MemProfileRate = c.MemRate
		}
	}
	if c.LegacyCPU != "" {
		if c.Directory != "" {
			return nil, fmt.Errorf("cpuprofile and profile-dir are mutually exclusive")
		}
		f, err := os.Create(c.LegacyCPU)
		if err != nil {
			return nil, err
		}
		if err = pprof.StartCPUProfile(f); err != nil {
			f.Close()
			return nil, err
		}
		p.wholeCPU = f
	}
	return p, nil
}
func (p *diagnosticProfiler) close() {
	if p.wholeCPU != nil {
		pprof.StopCPUProfile()
		p.wholeCPU.Close()
	}
}
func (p *diagnosticProfiler) phase(ctx context.Context, name string, fn func(context.Context) error) error {
	var cpu *os.File
	if p.config.Directory != "" && p.config.Mode == "cpu" {
		var err error
		cpu, err = os.Create(filepath.Join(p.config.Directory, name+".cpu.pprof"))
		if err != nil {
			return err
		}
		if err = pprof.StartCPUProfile(cpu); err != nil {
			cpu.Close()
			return err
		}
		defer func() { pprof.StopCPUProfile(); cpu.Close() }()
	}
	var before, after runtime.MemStats
	if p.config.Directory != "" {
		runtime.ReadMemStats(&before)
	}
	start := time.Now()
	var err error
	pprof.Do(ctx, pprof.Labels("stage", name), func(labelled context.Context) { err = fn(labelled) })
	elapsed := time.Since(start)
	if p.config.Directory != "" {
		runtime.ReadMemStats(&after)
		emit(map[string]any{"stage": "profile_phase", "phase": name, "wall_seconds": elapsed.Seconds(), "allocated_bytes": after.TotalAlloc - before.TotalAlloc, "allocated_objects": after.Mallocs - before.Mallocs, "gc_cycles": after.NumGC - before.NumGC, "gc_pause_ns": after.PauseTotalNs - before.PauseTotalNs, "heap_alloc_bytes": after.HeapAlloc, "heap_inuse_bytes": after.HeapInuse, "heap_sys_bytes": after.HeapSys, "heap_released_bytes": after.HeapReleased, "gc_cpu_fraction_lifetime": after.GCCPUFraction, "diagnostic_only": true})
	}
	return err
}
func (p *diagnosticProfiler) snapshot(name string, collect bool) error {
	if p.config.Directory == "" || p.config.Mode != "memory" {
		return nil
	}
	start := time.Now()
	// MemProfile can lag by two GC cycles. Forced snapshots deliberately differ
	// from natural end-of-stage snapshots and are never throughput measurements.
	if collect {
		runtime.GC()
		runtime.GC()
	}
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	path := filepath.Join(p.config.Directory, name+".heap.pprof")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = pprof.WriteHeapProfile(f)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	emit(map[string]any{"stage": "profile_snapshot", "name": name, "forced_two_gc": collect, "path": path, "heap_alloc_bytes": stats.HeapAlloc, "heap_inuse_bytes": stats.HeapInuse, "heap_sys_bytes": stats.HeapSys, "heap_released_bytes": stats.HeapReleased, "total_alloc_bytes": stats.TotalAlloc, "mallocs": stats.Mallocs, "heap_objects": stats.HeapObjects, "gc_cycles": stats.NumGC, "mem_profile_rate": runtime.MemProfileRate, "snapshot_seconds": time.Since(start).Seconds(), "diagnostic_only": true})
	return nil
}
