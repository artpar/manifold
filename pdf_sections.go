package manydoc

import (
	"fmt"
	"math"
	"reflect"

	"github.com/phpdave11/gofpdf"
)

type pdfSectionVariant struct{ header, footer *pdfRunningStory }
type pdfSectionPlan struct {
	section  resolvedSection
	geometry pageGeometry
	bounds   pdfBodyBounds
	variants [3]pdfSectionVariant // default, first, even
}

func (r *pdfRenderer) addPDFPage() {
	if r.sectioned {
		orientation := "P"
		if r.width > r.height {
			orientation = "L"
		}
		r.pdf.AddPageFormat(orientation, gofpdf.SizeType{Wd: math.Min(r.width, r.height), Ht: math.Max(r.width, r.height)})
	} else {
		r.pdf.AddPage()
	}
}
func (r *pdfRenderer) planSection(d *Document, s resolvedSection) (*pdfSectionPlan, error) {
	oldPage, oldWidth, oldHeight, oldMargin := r.page, r.width, r.height, r.margin
	oldHeader, oldFooter, oldContent, oldMinimum := r.header, r.footer, r.content, r.minimumBody
	defer func() {
		r.page, r.width, r.height, r.margin = oldPage, oldWidth, oldHeight, oldMargin
		r.header, r.footer, r.content, r.minimumBody = oldHeader, oldFooter, oldContent, oldMinimum
	}()
	g := pageLayout(s.layout)
	if g.width > 14400 || g.height > 14400 || fmt.Sprintf("%.2f", g.width) == "0.00" || fmt.Sprintf("%.2f", g.height) == "0.00" {
		return nil, fmt.Errorf("%w: invalid PDF section page dimensions", ErrUnsupported)
	}
	r.page, r.width, r.height, r.margin = &g, g.width, g.height, g.margins.Left
	r.minimumBody = 0
	running := false
	for _, story := range []*RunningStory{s.headers.Default, s.headers.First, s.headers.Even, s.footers.Default, s.footers.First, s.footers.Even} {
		if story != nil && len(story.Blocks) > 0 {
			running = true
		}
	}
	if running {
		minimum, err := r.minimumRunningBody(s.blocks)
		if err != nil {
			return nil, err
		}
		r.minimumBody = minimum
	}
	plan := &pdfSectionPlan{section: s, geometry: g, bounds: pdfBodyBounds{top: r.baseTop(), bottom: r.baseBottom()}}
	for i := range plan.variants {
		if i == 1 && !s.firstPage || i == 2 && !d.EvenAndOddHeaders {
			continue
		}
		header := selectStory(s.headers, i == 1, i == 2, s.firstPage, d.EvenAndOddHeaders)
		footer := selectStory(s.footers, i == 1, i == 2, s.firstPage, d.EvenAndOddHeaders)
		if err := r.prepareRunning(header, footer, len(s.blocks) > 0); err != nil {
			return nil, err
		}
		plan.variants[i] = pdfSectionVariant{r.header, r.footer}
		plan.bounds.top = math.Max(plan.bounds.top, r.bodyTop())
		plan.bounds.bottom = math.Min(plan.bounds.bottom, r.bodyBottom())
	}
	if plan.bounds.bottom <= plan.bounds.top {
		return nil, fmt.Errorf("%w: running variants leave no common section body", ErrUnsupported)
	}
	return plan, nil
}
func (r *pdfRenderer) renderSections(d *Document) error {
	r.sectioned = true
	var active, pending *pdfSectionPlan
	localPage := 0
	blank, nextBlank := false, false
	r.pdf.SetHeaderFunc(func() {
		blank = nextBlank
		nextBlank = false
		if pending != nil {
			active = pending
			pending = nil
			localPage = 0
			r.page = &active.geometry
			r.width, r.height, r.margin = r.page.width, r.page.height, r.page.margins.Left
			r.content = &active.bounds
			r.pdf.SetMargins(r.bodyLeft(), r.bodyTop(), r.page.margins.Right)
			r.pdf.SetAutoPageBreak(false, r.height-r.bodyBottom())
		}
		localPage++
		variant := 0
		if localPage == 1 && active.section.firstPage {
			variant = 1
		} else if localPage%2 == 0 && d.EvenAndOddHeaders {
			variant = 2
		}
		r.header, r.footer = active.variants[variant].header, active.variants[variant].footer
		r.y = r.bodyTop()
		if !blank {
			r.paintRunning(r.header)
		}
	})
	r.pdf.SetFooterFunc(func() {
		if !blank {
			r.paintRunning(r.footer)
		}
	})
	sections := documentSections(d)
	for i, s := range sections {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		if i > 0 && s.start == SectionContinuous {
			previous := sections[i-1]
			if !reflect.DeepEqual(pageLayout(s.layout), pageLayout(previous.layout)) || !reflect.DeepEqual(s.headers, previous.headers) || !reflect.DeepEqual(s.footers, previous.footers) || s.firstPage != previous.firstPage {
				return fmt.Errorf("%w: continuous section changes page geometry or running stories; shared-page resolution is unfinished", ErrUnsupported)
			}
			if err := r.blocks(s.blocks, r.bodyWidth(), r.bodyLeft()); err != nil {
				return err
			}
			continue
		}
		if i > 0 {
			if err := r.nextPage(false); err != nil {
				return err
			}
		}
		if i > 0 && (s.start == SectionEvenPage && (r.pdf.PageNo()+1)%2 != 0 || s.start == SectionOddPage && (r.pdf.PageNo()+1)%2 != 1) {
			nextBlank = true
			r.addPDFPage()
		}
		plan, err := r.planSection(d, s)
		if err != nil {
			return err
		}
		pending = plan
		// gofpdf runs the old footer before invoking the new header. Only the
		// header installs the new geometry, so the old footer keeps its frame.
		orientation := "P"
		if plan.geometry.width > plan.geometry.height {
			orientation = "L"
		}
		r.pdf.AddPageFormat(orientation, gofpdf.SizeType{Wd: math.Min(plan.geometry.width, plan.geometry.height), Ht: math.Max(plan.geometry.width, plan.geometry.height)})
		if err := r.blocks(s.blocks, r.bodyWidth(), r.bodyLeft()); err != nil {
			return err
		}
	}
	return nil
}
