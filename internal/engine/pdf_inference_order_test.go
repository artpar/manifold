package engine

import (
	"context"
	"math"
	"math/rand/v2"
	"reflect"
	"sort"
	"testing"
)

// Frozen pre-optimization ordering oracle; includes stable ties and column labels.
func TestPDFLayoutIndexOrderingMatchesOriginal(t *testing.T) {
	random := rand.New(rand.NewPCG(17, 29))
	for trial := 0; trial < 100; trial++ {
		boxes := make([]pdfLayoutBox, 1+random.IntN(300))
		for i := range boxes {
			x, y := float64(random.IntN(12))*20, float64(random.IntN(60))*12
			boxes[i] = pdfLayoutBox{block: Block{Kind: Paragraph, Level: i}, x0: x, y0: y, x1: x + 10 + float64(random.IntN(20)), y1: y + 10, size: 10, base: y + 9}
		}
		want := append([]pdfLayoutBox(nil), boxes...)
		old := &pdfReader{ctx: context.Background(), doc: &Document{}}
		want = old.layoutOrderReference(want, 10, "", 0)
		current := &pdfReader{ctx: context.Background(), doc: &Document{}}
		got := current.layoutOrder(boxes, 10, "", 0)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("ordering/columns changed trial %d", trial)
		}
	}
}
func (r *pdfReader) layoutOrderReference(boxes []pdfLayoutBox, em float64, column string, depth int) []pdfLayoutBox {
	if r.ctx.Err() != nil {
		return boxes
	}
	if len(boxes) < 2 {
		for i := range boxes {
			boxes[i].column = column
		}
		return boxes
	}
	if depth >= 64 {
		r.warn("pdf_inference_uncertain", "Geometric subdivision reached its depth limit; remaining content uses top-to-bottom order")
	}
	if depth < 64 {
		// Peel isolated wide edge bands before looking for a persistent gutter.
		// Otherwise a title/footer masks the column whitespace, and selecting a
		// larger gap halfway through the body interleaves the two columns.
		left, right, top, bottom := boxes[0].x0, boxes[0].x1, 0, 0
		for i, b := range boxes {
			left = min(left, b.x0)
			right = max(right, b.x1)
			if b.y0 < boxes[top].y0 {
				top = i
			}
			if b.y1 > boxes[bottom].y1 {
				bottom = i
			}
		}
		for _, edge := range []int{top, bottom} {
			b := boxes[edge]
			if b.x1-b.x0 < (right-left)*.65 {
				continue
			}
			separate := true
			for i, other := range boxes {
				if i != edge && ((edge == top && other.y0-b.y1 <= max(.5, em*.12)) || (edge != top && b.y0-other.y1 <= max(.5, em*.12))) {
					separate = false
					break
				}
			}
			if !separate {
				continue
			}
			rest := append(append([]pdfLayoutBox{}, boxes[:edge]...), boxes[edge+1:]...)
			b.column = column
			if edge == top {
				return append([]pdfLayoutBox{b}, r.layoutOrderReference(rest, em, column, depth+1)...)
			}
			return append(r.layoutOrderReference(rest, em, column, depth+1), b)
		}
		xs := append([]pdfLayoutBox(nil), boxes...)
		sort.SliceStable(xs, func(i, j int) bool { return xs[i].x0 < xs[j].x0 })
		right = xs[0].x1
		for i := 1; i < len(xs); i++ {
			if xs[i].x0-right >= max(2*em, 12) && pdfColumnExtent(xs[:i], em) && pdfColumnExtent(xs[i:], em) {
				return append(r.layoutOrderReference(xs[:i], em, column+"L", depth+1), r.layoutOrderReference(xs[i:], em, column+"R", depth+1)...)
			}
			right = max(right, xs[i].x1)
		}
		ys := append([]pdfLayoutBox(nil), boxes...)
		sort.SliceStable(ys, func(i, j int) bool { return ys[i].y0 < ys[j].y0 })
		yBottom, cut := ys[0].y1, 0
		for i := 1; i < len(ys); i++ {
			if ys[i].y0-yBottom > max(.5, em*.12) {
				cut = i
				break
			}
			yBottom = max(yBottom, ys[i].y1)
		}
		if cut > 0 {
			return append(r.layoutOrderReference(ys[:cut], em, column, depth+1), r.layoutOrderReference(ys[cut:], em, column, depth+1)...)
		}
	}
	for i := range boxes {
		boxes[i].column = column
	}
	sort.SliceStable(boxes, func(i, j int) bool {
		if math.Round(boxes[i].base*1000) == math.Round(boxes[j].base*1000) {
			return boxes[i].x0 < boxes[j].x0
		}
		return boxes[i].base < boxes[j].base
	})
	return boxes
}
