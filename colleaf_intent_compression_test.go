package manydoc

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func compressedIntentForTest(t *testing.T, raw string) colleafIntentCarrier {
	t.Helper()
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	if _, err := z.Write([]byte(raw)); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return colleafIntentCarrier{Profile: colleafIntentProfile, Version: 2, GzipRecords: buf.Bytes(), RecordsBytes: len(raw)}
}
func TestColleafIntentCompressionAndValidation(t *testing.T) {
	ctx := context.Background()
	records := []colleafResource{{Name: colleafDocumentResource, Data: strings.Repeat("source 😀", 10000)}}
	carrier, err := makeColleafIntentCarrier(records)
	if err != nil {
		t.Fatal(err)
	}
	if carrier.Version != 2 || carrier.Records != nil {
		t.Fatal("large metadata not compressed")
	}
	got, err := carrier.records(ctx)
	if err != nil || !reflect.DeepEqual(records, got) {
		t.Fatal("metadata changed", err)
	}
	again, err := makeColleafIntentCarrier(records)
	if err != nil || !reflect.DeepEqual(carrier, again) {
		t.Fatal("nondeterministic compression", err)
	}
	for name, mutate := range map[string]func(*colleafIntentCarrier){
		"checksum":    func(c *colleafIntentCarrier) { c.GzipRecords[len(c.GzipRecords)-8] ^= 1 },
		"truncated":   func(c *colleafIntentCarrier) { c.GzipRecords = c.GzipRecords[:len(c.GzipRecords)-1] },
		"trailing":    func(c *colleafIntentCarrier) { c.GzipRecords = append(c.GzipRecords, 0) },
		"multistream": func(c *colleafIntentCarrier) { c.GzipRecords = append(c.GzipRecords, c.GzipRecords...) },
		"short":       func(c *colleafIntentCarrier) { c.RecordsBytes-- },
		"long":        func(c *colleafIntentCarrier) { c.RecordsBytes++ },
		"bound":       func(c *colleafIntentCarrier) { c.RecordsBytes = maxColleafIntentBytes + 1 },
		"mixed":       func(c *colleafIntentCarrier) { c.Records = records },
		"profile":     func(c *colleafIntentCarrier) { c.Profile = "other" },
		"version":     func(c *colleafIntentCarrier) { c.Version = 3 },
	} {
		t.Run(name, func(t *testing.T) {
			bad := carrier
			bad.GzipRecords = bytes.Clone(carrier.GzipRecords)
			mutate(&bad)
			if _, err := bad.records(ctx); err == nil {
				t.Fatal("malformed compression accepted")
			}
		})
	}
	for _, raw := range []string{`null`, `{}`, `[{"name":"x","name":"y","data":"{}"}]`, `[{"name":"x","data":"{}","unknown":0}]`, `[`, `[null]`} {
		if _, err := compressedIntentForTest(t, raw).records(ctx); err == nil {
			t.Fatalf("invalid records accepted: %s", raw)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := carrier.records(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored", err)
	}
	legacy := colleafIntentCarrier{Profile: colleafIntentProfile, Version: 1, Records: records}
	legacy.GzipRecords = []byte{1}
	if _, err := legacy.records(ctx); err == nil {
		t.Fatal("legacy mixed encoding accepted")
	}
	// Strict resource checks must see JSON inside compression, before typed resource
	// parsing could discard duplicate keys. This uses the public-envelope preflight.
	c := compressedIntentForTest(t, `[{"name":"`+colleafDocumentResource+`","data":"{\"version\":1,\"version\":1}"}]`)
	native := colleafNoteSnapshot{Body: colleafBody{CustomRanges: []colleafRangeRecord{{Properties: sfAttrs(c)}}}}
	if err := checkColleafResourceJSON(ctx, native); err == nil {
		t.Fatal("duplicate resource JSON hidden by compression")
	}
}

func FuzzColleafCompressedIntent(f *testing.F) {
	f.Add([]byte("not gzip"), 100)
	c := compressedIntentForTestForFuzz()
	f.Add(c.GzipRecords, c.RecordsBytes)
	f.Fuzz(func(t *testing.T, data []byte, n int) {
		// Bound this fuzz target's allocations independently of production limits.
		if len(data) > 65536 || n > 65536 {
			return
		}
		c := colleafIntentCarrier{Profile: colleafIntentProfile, Version: 2, GzipRecords: data, RecordsBytes: n}
		records, err := c.records(context.Background())
		if err == nil {
			if _, err := json.Marshal(records); err != nil {
				t.Fatal(err)
			}
		}
	})
}
func compressedIntentForTestForFuzz() colleafIntentCarrier {
	raw := []byte(`[{"name":"manydoc.document-properties.v1","data":"{}"}]`)
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	_, _ = z.Write(raw)
	_ = z.Close()
	return colleafIntentCarrier{Profile: colleafIntentProfile, Version: 2, GzipRecords: buf.Bytes(), RecordsBytes: len(raw)}
}
