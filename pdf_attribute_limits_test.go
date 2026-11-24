package manydoc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// An acyclic shared-object graph amplifies visits without exceeding depth16.
// At fanout8/depth6 it expands262144 leaves from about1KiB of source arrays.
func pdfAttributeDAG(levels, fanout int, content string, owners int) []byte {
	leaf := 8 + owners
	refs := make([]string, owners)
	contentStream := ""
	for i := range refs {
		refs[i] = fmt.Sprintf("%d 0 R", 8+i)
		contentStream += markedText(i, 20, 300-i*20, fmt.Sprintf("BODY%d", i))
	}
	root := fmt.Sprintf("<< /Type /StructTreeRoot /K [%s] /ClassMap << /Shared %d 0 R >> >>", strings.Join(refs, " "), leaf+levels)
	elements := []string{}
	for i := 0; i < owners; i++ {
		elements = append(elements, fmt.Sprintf("<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /C /Shared /K %d >>", i))
	}
	elements = append(elements, content)
	for i := 0; i < levels; i++ {
		elements = append(elements, "["+strings.Repeat(fmt.Sprintf("%d 0 R ", leaf+i), fanout)+"]")
	}
	return taggedTestPDF(contentStream, root, elements...)
}

func TestPDFAttributeExpansionBudget(t *testing.T) {
	for _, tc := range []struct {
		name           string
		levels, owners int
	}{
		{"shared-array", 6, 1},
		{"shared-across-owners", 5, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := pdfAttributeDAG(tc.levels, 8, "<< /O /List /ListNumbering /Decimal >>", tc.owners)
			if len(data) > 5000 {
				t.Fatal("fixture unexpectedly materializes expanded graph")
			}
			d, err := ReadPDF(context.Background(), bytes.NewReader(data), ReadOptions{})
			if err == nil || d != nil || !strings.Contains(err.Error(), "structure attributes exceed") {
				t.Fatalf("small shared graph was not bounded: doc=%v err=%v", d != nil, err)
			}
		})
	}
}

func TestPDFAttributeSharedGraphPreservesOverrides(t *testing.T) {
	// The same class supplies repeated Decimal attributes. Local LowerRoman wins.
	objects := taggedTestObjects(markedText(0, 20, 300, "iv.")+markedText(1, 40, 300, "Body"),
		"<< /Type /StructTreeRoot /K 8 0 R /ClassMap << /Shared 15 0 R >> >>",
		"<< /Type /StructElem /S /L /P 7 0 R /Pg 3 0 R /C /Shared /A << /O /List /ListNumbering /LowerRoman >> /K 9 0 R >>",
		"<< /Type /StructElem /S /LI /P 8 0 R /K [10 0 R 11 0 R] >>",
		"<< /Type /StructElem /S /Lbl /P 9 0 R /K 0 >>",
		"<< /Type /StructElem /S /LBody /P 9 0 R /K 1 >>",
		"<< /O /List /ListNumbering /Decimal >>")
	for i := 12; i < 15; i++ {
		objects = append(objects, "["+strings.Repeat(fmt.Sprintf("%d 0 R ", i), 8)+"]")
	}
	d := readRecovery(t, recoveryPDF(objects...))
	assertPDFNumberedLabels(t, d, 4, []string{"iv."})
	if d.Blocks[0].Items[0].Source.Label != "iv." || d.Blocks[0].Items[0].Blocks[0].Runs[0].Text != "Body" {
		t.Fatal("shared class or local override changed content")
	}
	assertRecoverySF(t, d)
}

// The context transitions to cancelled during attribute visits, without a timer
// or scheduler-dependent race. It otherwise delegates the Context contract.
type pdfAttributeCancelContext struct {
	context.Context
	cancel context.CancelFunc
	calls  int
}

func (c *pdfAttributeCancelContext) Err() error {
	c.calls++
	if c.calls == 4 {
		c.cancel()
	}
	return c.Context.Err()
}
func TestPDFAttributeTraversalCancellation(t *testing.T) {
	data := pdfAttributeDAG(3, 8, "<< /O /List /ListNumbering /Decimal >>", 1)
	p, err := pdfcpu.ReadWithContext(context.Background(), bytes.NewReader(data), &model.Configuration{Reader15: true, ValidationMode: model.ValidationRelaxed, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := p.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	root, err := p.DereferenceDict(catalog["StructTreeRoot"])
	if err != nil {
		t.Fatal(err)
	}
	classes, err := p.DereferenceDict(root["ClassMap"])
	if err != nil {
		t.Fatal(err)
	}
	owner, err := p.DereferenceDict(*types.NewIndirectRef(8, 0))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &pdfAttributeCancelContext{Context: ctx, cancel: cancel}
	recovery := pdfStructureRecovery{reader: &pdfReader{ctx: c, pdf: p, doc: &Document{}}, classes: classes}
	n := pdfStructureNode{attrs: map[string]types.Object{}}
	if err := recovery.attributes(&n, owner); !errors.Is(err, context.Canceled) {
		t.Fatalf("attribute walk ignored cancellation: %v", err)
	}
}

func FuzzPDFAttributeGraph(f *testing.F) {
	f.Add(uint8(6), uint8(7), uint8(0))
	f.Add(uint8(5), uint8(7), uint8(1))
	f.Add(uint8(3), uint8(1), uint8(2))
	f.Fuzz(func(t *testing.T, depth, branches, count uint8) {
		levels, fanout, owners := int(depth)%13, int(branches)%16+1, int(count)%4+1
		data := pdfAttributeDAG(levels, fanout, "<< /O /List /ListNumbering /Decimal >>", owners)
		d, err := ReadPDF(context.Background(), bytes.NewReader(data), ReadOptions{})
		if err != nil {
			if !strings.Contains(err.Error(), "structure attributes exceed") {
				t.Fatal("unexpected rejection", err)
			}
			return
		}
		if len(d.Blocks) != owners {
			t.Fatal("attribute graph changed forward membership")
		}
		for i, b := range d.Blocks {
			if b.Kind != Paragraph || paragraphText(b) != fmt.Sprintf("BODY%d", i) || b.Source == nil || b.Source.Structure != "explicit" {
				t.Fatal("attribute graph changed content/order/source")
			}
		}
		assertRecoverySF(t, d)
	})
}
