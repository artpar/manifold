package manydoc

import (
	"fmt"
	"math"
	"reflect"

	"github.com/phpdave11/gofpdf"
)

type pdfSectionVariant struct{ header, footer *pdfRunningStory }
type pdfSectionPlan struct {
	section     resolvedSection
	geometry    pageGeometry
	bounds      pdfBodyBounds
	minimumBody float64
	variants    [3]pdfSectionVariant // default, first, even
}

type pdfSectionScope struct {
	index, total   int
	firstFootnotes map[noteKey]int
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
func (r *pdfRenderer) planSection(d *Document, s resolvedSection, minimumFloor float64, scope pdfSectionScope) (*pdfSectionPlan, error) {
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
	r.content = nil
	minimum, err := r.minimumSectionBody(s.blocks, scope)
	if err != nil {
		return nil, err
	}
	r.minimumBody = math.Max(minimum, minimumFloor)
	bodyNeeded := len(s.blocks) > 0
	if scope.index == scope.total-1 {
		for _, key := range r.notes.order {
			if key.kind == Endnote {
				bodyNeeded = true
				break
			}
		}
	}
	plan := &pdfSectionPlan{section: s, geometry: g, minimumBody: r.minimumBody, bounds: pdfBodyBounds{top: r.baseTop(), bottom: r.baseBottom()}}
	for i := range plan.variants {
		if i == 1 && !s.firstPage || i == 2 && !d.EvenAndOddHeaders {
			continue
		}
		header := selectStory(s.headers, i == 1, i == 2, s.firstPage, d.EvenAndOddHeaders)
		footer := selectStory(s.footers, i == 1, i == 2, s.firstPage, d.EvenAndOddHeaders)
		if err := r.prepareRunning(header, footer, bodyNeeded); err != nil {
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

// Definitions remain globally addressable, but unrelated notes must not impose
// their dimensions on a different section. Endnotes render in the final frame.
func (r *pdfRenderer) minimumSectionBody(blocks []Block, scope pdfSectionScope) (float64, error) {
	references := map[noteKey]bool{}
	var order []noteKey
	if err := walkNoteRuns(blocks, func(run Run) error {
		if run.Note != nil {
			key := referenceKey(run.Note)
			if !references[key] {
				references[key] = true
				order = append(order, key)
			}
		}
		return r.ctx.Err()
	}); err != nil {
		return 0, err
	}
	original, originalSeen := r.notes, r.seenFootnotes
	seen := make(map[noteKey]bool, len(originalSeen)+len(references))
	for key, value := range originalSeen {
		seen[key] = value
	}
	for _, key := range order {
		if first, ok := scope.firstFootnotes[key]; ok && first < scope.index {
			seen[key] = true
		}
	}
	local := *original
	local.order = nil
	for _, key := range order {
		if key.kind == Footnote && !seen[key] {
			local.order = append(local.order, key)
		}
	}
	if scope.index == scope.total-1 {
		for _, key := range original.order {
			if key.kind == Endnote {
				local.order = append(local.order, key)
			}
		}
	}
	r.notes, r.seenFootnotes = &local, seen
	defer func() { r.notes, r.seenFootnotes = original, originalSeen }()
	return r.minimumRunningBody(blocks)
}

// Plan a common body for each chain of continuous sections before painting any
// of it. Already-painted content cannot be moved when a later story is taller.
func (r *pdfRenderer) planSections(d *Document, sections []resolvedSection) ([]*pdfSectionPlan, error) {
	firstFootnotes := map[noteKey]int{}
	for i, s := range sections {
		if err := walkNoteRuns(s.blocks, func(run Run) error {
			if run.Note != nil && run.Note.Kind == Footnote {
				key := referenceKey(run.Note)
				if _, ok := firstFootnotes[key]; !ok {
					firstFootnotes[key] = i
				}
			}
			return r.ctx.Err()
		}); err != nil {
			return nil, err
		}
	}
	scope := func(i int) pdfSectionScope {
		return pdfSectionScope{index: i, total: len(sections), firstFootnotes: firstFootnotes}
	}
	plans := make([]*pdfSectionPlan, len(sections))
	for i, s := range sections {
		if err := r.ctx.Err(); err != nil {
			return nil, err
		}
		if i > 0 && s.start == SectionContinuous && !reflect.DeepEqual(pageLayout(s.layout), pageLayout(sections[i-1].layout)) {
			return nil, fmt.Errorf("%w: continuous section changes page geometry; shared-page resolution is unfinished", ErrUnsupported)
		}
		plan, err := r.planSection(d, s, 0, scope(i))
		if err != nil {
			return nil, err
		}
		plans[i] = plan
	}
	for start := 0; start < len(plans); {
		end := start + 1
		minimum := plans[start].minimumBody
		for end < len(plans) && sections[end].start == SectionContinuous {
			minimum = math.Max(minimum, plans[end].minimumBody)
			end++
		}
		for i := start; i < end; i++ {
			if plans[i].minimumBody < minimum {
				plan, err := r.planSection(d, sections[i], minimum, scope(i))
				if err != nil {
					return nil, err
				}
				plans[i] = plan
			}
		}
		bounds := plans[start].bounds
		for _, plan := range plans[start+1 : end] {
			bounds.top = math.Max(bounds.top, plan.bounds.top)
			bounds.bottom = math.Min(bounds.bottom, plan.bounds.bottom)
		}
		if bounds.bottom <= bounds.top {
			return nil, fmt.Errorf("%w: continuous running stories leave no common body", ErrUnsupported)
		}
		for _, plan := range plans[start:end] {
			plan.bounds = bounds
		}
		start = end
	}
	return plans, nil
}
func (r *pdfRenderer) renderSections(d *Document) error {
	sections := documentSections(d)
	plans, err := r.planSections(d, sections)
	if err != nil {
		return err
	}
	r.sectioned = true
	var active, pending *pdfSectionPlan
	localPage, pendingPage := 0, 0
	blank, nextBlank := false, false
	r.pdf.SetHeaderFunc(func() {
		blank = nextBlank
		nextBlank = false
		if pending != nil {
			active = pending
			pending = nil
			localPage = pendingPage
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
	for i, s := range sections {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		if i > 0 && s.start == SectionContinuous {
			// The opened page keeps its painted header and queued footer. It is
			// page one of the new section, so its next page selects variant two.
			pending, pendingPage = plans[i], 1
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
		plan := plans[i]
		pending, pendingPage = plan, 0
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
