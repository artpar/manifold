package manydoc

import (
	"bytes"
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestPDFSignedRunningBounds(t *testing.T) {
	for _, top := range []float64{-24, 24} {
		for _, bottom := range []float64{-24, 24} {
			r := runningRenderer()
			g := pageLayout(Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: top, Right: 30, Bottom: bottom, Left: 30}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(12)})
			r.page = &g
			story := []Block{sectionParagraph("ONE"), sectionParagraph("TWO"), sectionParagraph("THREE"), sectionParagraph("FOUR")}
			if err := r.prepareRunning(story, story, true); err != nil {
				t.Fatal(err)
			}
			wantTop, wantBottom := 52.0, 348.0
			if top < 0 {
				wantTop = 24
			}
			if bottom < 0 {
				wantBottom = 376
			}
			if math.Abs(r.bodyTop()-wantTop) > .001 || math.Abs(r.bodyBottom()-wantBottom) > .001 {
				t.Fatal("fixed and minimum edges conflated", top, bottom, r.content)
			}
		}
	}
}
func TestPDFSignedRunningImagesDoNotCompeteWithFixedBody(t *testing.T) {
	image := Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: runningPNG(t, false), Width: 100, Height: 370}}
	d := &Document{Layout: Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: -24, Right: 30, Bottom: -24, Left: 30}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(12)}, Headers: []Block{image}, Footers: []Block{image}, Blocks: []Block{sectionParagraph("FIXED BODY")}}
	before := canonicalSF(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	images := 0
	for _, b := range back.Blocks {
		if b.Image != nil {
			images++
			if math.Abs(b.Image.Height-370) > .01 {
				t.Fatal("fixed running image shrank against body/other story", b.Image.Height)
			}
		}
	}
	if images != 2 || !strings.Contains(back.Text(), "FIXED BODY") {
		t.Fatal("overlapping content discarded", back.Text(), images)
	}
	if !bytes.Equal(before, canonicalSF(t, d)) {
		t.Fatal("source preferences changed")
	}
	checkParagraphRoundtrip(t, d)
}
func TestPDFSignedRunningStillRejectsOutOfCanvasContent(t *testing.T) {
	for _, header := range []bool{true, false} {
		d := &Document{Layout: Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: -24, Right: 30, Bottom: -24, Left: 30}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(12)}, Blocks: []Block{sectionParagraph("BODY")}}
		story := []Block{sectionParagraph(strings.Repeat("LINE\n", 100))}
		if header {
			d.Headers = story
		} else {
			d.Footers = story
		}
		for _, format := range []Format{DOCX, PDF} {
			var out bytes.Buffer
			if err := Write(context.Background(), &out, d, format, WriteOptions{}); !errors.Is(err, ErrUnsupported) || out.Len() != 0 {
				t.Fatal(format, "out-of-page text must reject atomically", header, err)
			}
		}
	}
}

func TestPDFSignedContinuousOriginsAndPositiveReservations(t *testing.T) {
	story := &RunningStory{Blocks: []Block{sectionParagraph("RUN ONE"), sectionParagraph("RUN TWO"), sectionParagraph("RUN THREE"), sectionParagraph("RUN FOUR")}}
	a := &Section{Headers: RunningStories{Default: story}, Footers: RunningStories{Default: story}, Blocks: []Block{sectionParagraph("FIXED FIRST"), {Kind: PageBreak}, sectionParagraph("FIXED SECOND")}}
	positive := Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: 60, Right: 30, Bottom: 60, Left: 30}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(12)}
	b := &Section{Start: SectionContinuous, Layout: &positive, Headers: RunningStories{Default: sectionStory("NEW HEADER")}, Footers: RunningStories{Default: sectionStory("NEW FOOTER")}, Blocks: []Block{sectionParagraph("POSITIVE SHARED"), {Kind: PageBreak}, sectionParagraph("POSITIVE SECOND")}}
	d := &Document{Layout: Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: -24, Right: 30, Bottom: -24, Left: 30}, HeaderDistance: pagePoint(12), FooterDistance: pagePoint(12)}, Blocks: []Block{{Kind: SectionBlock, Section: a}, {Kind: SectionBlock, Section: b}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Pages) != 3 {
		t.Fatal("shared positive/fixed frames added pages", len(back.Pages))
	}
	found := 0
	for _, block := range back.Blocks {
		for _, run := range block.Runs {
			want := 0.0
			switch run.Text {
			case "FIXED FIRST", "FIXED SECOND":
				want = 35
			case "POSITIVE SHARED", "POSITIVE SECOND":
				want = 71
			}
			if want > 0 {
				found++
				if run.Source == nil || run.Source.Baseline == nil || math.Abs(run.Source.Baseline[0].Y-want) > .01 {
					t.Fatal("shared reservation overrode fixed origin or permitted positive overlap", run.Text, run.Source)
				}
			}
		}
	}
	if found != 4 {
		t.Fatal("body lost", back.Text())
	}
	checkParagraphRoundtrip(t, d)
}
func TestPDFSignedContinuousDrainsOldNoteInterval(t *testing.T) {
	anchor := sectionParagraph("OLD ANCHOR")
	anchor.Runs = append(anchor.Runs, Run{Note: &NoteReference{ID: "old", Kind: Footnote}})
	var note []Block
	for i := 0; i < 32; i++ {
		note = append(note, sectionParagraph("OLD NOTE LINE"))
	}
	a := &Section{Blocks: []Block{anchor}}
	narrower := Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: 120, Right: 30, Bottom: 120, Left: 30}}
	b := &Section{Start: SectionContinuous, Layout: &narrower, FirstPage: true, Headers: RunningStories{First: sectionStory("NEW FIRST")}, Blocks: []Block{sectionParagraph("NEW BODY")}}
	d := &Document{Layout: Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: -24, Right: 30, Bottom: -24, Left: 30}}, Blocks: []Block{{Kind: SectionBlock, Section: a}, {Kind: SectionBlock, Section: b}}, Notes: []Note{{Kind: Footnote, ID: "old", Blocks: note}}}
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Pages) < 3 || strings.Count(back.Text(), "OLD NOTE LINE") != 32 || strings.Count(back.Text(), "NEW FIRST") != 1 {
		t.Fatal("old interval was re-used or note content lost", back.Text())
	}
	for _, block := range back.Blocks {
		for _, run := range block.Runs {
			if run.Text == "NEW BODY" {
				if run.Source.Page < 3 || run.Source.Baseline == nil || math.Abs(run.Source.Baseline[0].Y-131) > .01 {
					t.Fatal("new frame did not own fresh page", run.Source)
				}
			}
		}
	}
}

func TestFixedBlankStoryKeepsUnusedOutsideDistance(t *testing.T) {
	l := Layout{Width: 300, Height: 400, Margins: &PageMargins{Top: -24, Right: 30, Bottom: -24, Left: 30}, HeaderDistance: pagePoint(900)}
	d := &Document{Blocks: []Block{{Kind: SectionBlock, Section: &Section{Layout: &l, FirstPage: true, Headers: RunningStories{First: &RunningStory{}}, Blocks: []Block{sectionParagraph("BODY")}}}}}
	checkParagraphRoundtrip(t, d)
	var out bytes.Buffer
	if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
}

func FuzzSignedRunning(f *testing.F) {
	f.Add(byte(3), uint16(300), uint16(400), uint16(220), uint16(180))
	f.Add(byte(0), uint16(360), uint16(440), uint16(290), uint16(280))
	blob := runningPNG(f, false)
	f.Fuzz(func(t *testing.T, flags byte, width, height, header, footer uint16) {
		w, h := float64(width%201+300), float64(height%201+350)
		top, bottom := float64(flags%41+20), float64((flags/2)%41+20)
		if flags&1 != 0 {
			top = -top
		}
		if flags&2 != 0 {
			bottom = -bottom
		}
		dist := float64(flags%13 + 8)
		head := float64(header%uint16(h-dist-64) + 40)
		foot := float64(footer%uint16(h-dist-64) + 40)
		picture := func(height float64) Block {
			return Block{Kind: Picture, Image: &Image{MediaType: "image/png", Data: blob, Width: 80, Height: height}}
		}
		d := &Document{Layout: Layout{Width: w, Height: h, Margins: &PageMargins{Top: top, Right: 30, Bottom: bottom, Left: 30}, HeaderDistance: pagePoint(dist), FooterDistance: pagePoint(dist)}, Headers: []Block{picture(head)}, Footers: []Block{picture(foot)}, Blocks: []Block{sectionParagraph("BODY")}}
		before := canonicalSF(t, d)
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, PDF, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		back, err := ReadPDF(context.Background(), &out, ReadOptions{PDFSkipInference: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(back.Pages) != 1 || !strings.Contains(back.Text(), "BODY") {
			t.Fatal("body missing or paginated unnecessarily", back.Text())
		}
		var images []*Image
		for _, b := range back.Blocks {
			if b.Image != nil {
				images = append(images, b.Image)
			}
		}
		if len(images) != 2 {
			t.Fatal("running content lost", len(images))
		}
		if top < 0 && math.Abs(images[0].Height-head) > .01 || bottom < 0 && math.Abs(images[1].Height-foot) > .01 {
			t.Fatal("fixed story shrank to avoid body overlap", images[0].Height, images[1].Height, head, foot)
		}
		out.Reset()
		if err := Write(context.Background(), &out, d, DOCX, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		back, err = ReadDOCX(context.Background(), &out)
		if err != nil {
			t.Fatal(err)
		}
		if top < 0 && math.Abs(back.Headers[0].Image.Height-head) > .001 || bottom < 0 && math.Abs(back.Footers[0].Image.Height-foot) > .001 {
			t.Fatal("DOCX fixed story shrank against other anchors")
		}
		if !bytes.Equal(before, canonicalSF(t, d)) {
			t.Fatal("writer mutated source")
		}
	})
}
