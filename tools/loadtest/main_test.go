package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestHistogramUpperBoundAndMerge(t *testing.T) {
	var a, b histogram
	for i := 1; i <= 1000; i++ {
		if i%2 == 0 {
			a.add(time.Duration(i) * time.Microsecond)
		} else {
			b.add(time.Duration(i) * time.Microsecond)
		}
	}
	a.merge(&b)
	for _, q := range []float64{.5, .95, .99} {
		want := q
		got := a.quantile(q)
		if got < want || got > want*1.022 {
			t.Fatalf("q=%v: %v outside upper-bound accuracy for %v", q, got, want)
		}
	}
}
func TestPhaseAccountsCompletedAndFailedOperations(t *testing.T) {
	var calls atomic.Int64
	w := workload{Name: "test", prepare: func(int) operation {
		return func(context.Context) (observation, error) { calls.Add(1); return observation{bytes: 7}, nil }
	}}
	r, err := runPhase(context.Background(), w, 2, 1, 10*time.Millisecond, 0)
	if err != nil {
		t.Fatal(err)
	}
	if r.Success == 0 || r.Errors != 0 || r.Success != r.Offered || r.OutputBytes != int64(r.Success)*7 || r.SuccessfulJobsPerSecond <= 0 {
		t.Fatalf("bad accounting: %+v", r)
	}
	if calls.Load() != int64(r.Success)+1 {
		t.Fatalf("warmup included in requests: calls=%d success=%d", calls.Load(), r.Success)
	}
}
func TestOpenLoopPreservesArrivalAccounting(t *testing.T) {
	w := workload{Name: "overload", prepare: func(int) operation {
		return func(context.Context) (observation, error) { time.Sleep(time.Millisecond); return observation{}, nil }
	}}
	r, err := runPhase(context.Background(), w, 1, 1, 20*time.Millisecond, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if r.Offered != 200 || r.Success+r.Errors+r.Rejected != r.Offered || r.Rejected == 0 {
		t.Fatalf("bad overload accounting: %+v", r)
	}
}
