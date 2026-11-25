package render

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestShapingResultCacheBoundsDistinctTextAndRetainsNewest(t *testing.T) {
	c := boundedShapingCache{limit: 4}
	for i := range 100 {
		key := textShapingInput{Text: fmt.Sprint(i)}
		c.LoadOrStore(key, textShapingOutput{Advance: i})
	}
	if len(c.values) != 4 || len(c.order) != 4 {
		t.Fatalf("unbounded cache: %d/%d", len(c.values), len(c.order))
	}
	if _, ok := c.Load(textShapingInput{Text: "0"}); ok {
		t.Fatal("old entry not evicted")
	}
	if got, ok := c.Load(textShapingInput{Text: "99"}); !ok || got.Advance != 99 {
		t.Fatal("newest result missing")
	}
	c.LoadOrStore(textShapingInput{Text: strings.Repeat("x", 4097)}, textShapingOutput{})
	if len(c.values) != 4 {
		t.Fatal("oversized text retained")
	}
}
func TestShapingResultCacheConcurrentAccess(t *testing.T) {
	c := boundedShapingCache{limit: 16}
	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := range 100 {
				key := textShapingInput{Text: fmt.Sprintf("%d/%d", worker, i)}
				c.LoadOrStore(key, textShapingOutput{Advance: i})
				c.Load(key)
			}
		}(worker)
	}
	wg.Wait()
	if len(c.values) > 16 {
		t.Fatal("cache exceeded capacity")
	}
}
