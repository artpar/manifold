package manydoc

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

const colleafDocumentResource = "manydoc.document-properties.v1"

type colleafPageProperties struct {
	PageSize       *colleafImageSize `json:"pageSize,omitempty"`
	Top            *float64          `json:"marginTop,omitempty"`
	Right          *float64          `json:"marginRight,omitempty"`
	Bottom         *float64          `json:"marginBottom,omitempty"`
	Left           *float64          `json:"marginLeft,omitempty"`
	HeaderDistance *float64          `json:"marginHeader,omitempty"`
	FooterDistance *float64          `json:"marginFooter,omitempty"`
	Orient         *int              `json:"pageOrient,omitempty"`
	Flavor         *int              `json:"documentFlavor,omitempty"`
	DefaultHeader  *string           `json:"defaultHeaderId,omitempty"`
	DefaultFooter  *string           `json:"defaultFooterId,omitempty"`
	FirstHeader    *string           `json:"firstPageHeaderId,omitempty"`
	FirstFooter    *string           `json:"firstPageFooterId,omitempty"`
	EvenHeader     *string           `json:"evenPageHeaderId,omitempty"`
	EvenFooter     *string           `json:"evenPageFooterId,omitempty"`
	FirstPage      *int              `json:"useFirstPageHeaderFooter,omitempty"`
	EvenOdd        *int              `json:"evenAndOddHeaders,omitempty"`
}
type colleafSectionProperties struct {
	colleafPageProperties
	Start *int `json:"sectionType,omitempty"`
	Next  *int `json:"sectionTypeNext,omitempty"`
}
type colleafRunningStory struct {
	HeaderID     string                     `json:"headerId,omitempty"`
	FooterID     string                     `json:"footerId,omitempty"`
	Body         colleafBody                `json:"body"`
	Tables       map[string]json.RawMessage `json:"tableSource,omitempty"`
	Lists        map[string]json.RawMessage `json:"lists,omitempty"`
	Drawings     map[string]json.RawMessage `json:"drawings,omitempty"`
	DrawingOrder []string                   `json:"drawingsOrder,omitempty"`
}
type colleafLayoutBinding struct {
	ID            string                `json:"id"`
	Original      *Layout               `json:"original"`
	Native        colleafPageProperties `json:"native"`
	ImplicitStart bool                  `json:"implicitStart,omitempty"`
	Empty         bool                  `json:"empty,omitempty"`
}
type colleafDocumentIntent struct {
	Author       string                 `json:"author,omitempty"`
	Subject      string                 `json:"subject,omitempty"`
	EmptyTitle   bool                   `json:"emptyTitle,omitempty"`
	Sectioned    bool                   `json:"sectioned,omitempty"`
	Layouts      []colleafLayoutBinding `json:"layouts"`
	EmptyStories map[string]bool        `json:"emptyStories,omitempty"`
}

func colleafFloat(v float64) *float64 { return &v }
func colleafInt(v int) *int           { return &v }
func colleafString(v string) *string  { return &v }
func colleafPageGeometry(p colleafPageProperties) colleafPageProperties {
	p.DefaultHeader = nil
	p.DefaultFooter = nil
	p.FirstHeader = nil
	p.FirstFooter = nil
	p.EvenHeader = nil
	p.EvenFooter = nil
	p.FirstPage = nil
	p.EvenOdd = nil
	p.Flavor = nil
	return p
}
func colleafLayoutProjection(l Layout) colleafPageProperties {
	g := pageLayout(l)
	return colleafPageProperties{PageSize: &colleafImageSize{g.width * 4 / 3, g.height * 4 / 3}, Top: colleafFloat(g.margins.Top * 4 / 3), Right: colleafFloat(g.margins.Right * 4 / 3), Bottom: colleafFloat(g.margins.Bottom * 4 / 3), Left: colleafFloat(g.margins.Left * 4 / 3), HeaderDistance: colleafFloat(g.headerDistance() * 4 / 3), FooterDistance: colleafFloat(g.footerDistance() * 4 / 3), Orient: colleafInt(0)}
}
func colleafLayoutFromProperties(p colleafPageProperties, previous Layout) (Layout, error) {
	g := pageLayout(previous)
	l := Layout{Width: g.width, Height: g.height, Margins: &PageMargins{g.margins.Top, g.margins.Right, g.margins.Bottom, g.margins.Left}, HeaderDistance: colleafFloat(g.headerDistance()), FooterDistance: colleafFloat(g.footerDistance())}
	if p.PageSize != nil {
		l.Width = p.PageSize.Width * .75
		l.Height = p.PageSize.Height * .75
		if l.Width <= 0 || l.Height <= 0 {
			return Layout{}, fmt.Errorf("native page dimensions must be positive")
		}
	}
	for _, pair := range []struct {
		source *float64
		target *float64
	}{{p.Top, &l.Margins.Top}, {p.Right, &l.Margins.Right}, {p.Bottom, &l.Margins.Bottom}, {p.Left, &l.Margins.Left}, {p.HeaderDistance, l.HeaderDistance}, {p.FooterDistance, l.FooterDistance}} {
		if pair.source != nil {
			*pair.target = *pair.source * .75
		}
	}
	if p.Orient != nil && *p.Orient != 0 {
		if *p.Orient != 1 {
			return Layout{}, fmt.Errorf("unsupported native page orientation")
		}
		l.Width, l.Height = l.Height, l.Width
	}
	if err := validateLayout(l); err != nil {
		return Layout{}, err
	}
	return l, nil
}
func colleafSectionStart(start SectionStart) (int, error) {
	switch start {
	case "", SectionNextPage:
		return 3, nil
	case SectionContinuous:
		return 1, nil
	case SectionEvenPage:
		return 4, nil
	case SectionOddPage:
		return 5, nil
	}
	return 0, fmt.Errorf("unsupported section start")
}
func colleafStartFromNative(value int) (SectionStart, error) {
	switch value {
	case 0, 3:
		return SectionNextPage, nil
	case 1:
		return SectionContinuous, nil
	case 4:
		return SectionEvenPage, nil
	case 5:
		return SectionOddPage, nil
	}
	return "", fmt.Errorf("%w: native section start%d", ErrUnsupported, value)
}
func colleafEmptyBlocks(blocks []Block) bool {
	return len(blocks) == 1 && reflect.DeepEqual(blocks[0], Block{Kind: Paragraph})
}

func colleafResourceMap(records []colleafResource, name string, target any) error {
	found := false
	for _, r := range records {
		if r.Name != name {
			continue
		}
		if found {
			return fmt.Errorf("duplicate native intent record %q", name)
		}
		found = true
		if err := strictSFAttrs(json.RawMessage(r.Data), target); err != nil {
			return err
		}
	}
	return nil
}

func colleafReplaceResource(records []colleafResource, name string, value any) []colleafResource {
	var out []colleafResource
	for _, r := range records {
		if r.Name != name {
			out = append(out, r)
		}
	}
	return append(out, colleafResource{Name: name, Data: string(sfAttrs(value))})
}

// Whole native document composition adds real page/section/story relationships
// around the already-qualified main/note text, table, list and image workers.
func encodeColleafDocument(ctx context.Context, d *Document) (colleafNoteSnapshot, error) {
	if err := d.Validate(); err != nil {
		return colleafNoteSnapshot{}, err
	}
	resolved := resolveNumberingDocument(d)
	plain := *resolved
	if hasSectionBlocks(d.Blocks) {
		plain.Blocks = resolved.Blocks[0].Section.Blocks
		plain.Headers = nil
		plain.Footers = nil
		plain.EvenAndOddHeaders = false
	}
	catalog, err := catalogNotes(resolved, false)
	if err != nil {
		return colleafNoteSnapshot{}, err
	}
	out, err := encodeColleafNotesWithCatalog(ctx, &plain, catalog)
	if err != nil {
		return out, err
	}
	out, err = openColleafIntent(ctx, out)
	if err != nil {
		return out, err
	}
	intent := colleafDocumentIntent{Author: d.Metadata.Author, Subject: d.Metadata.Subject, EmptyTitle: d.Metadata.Title == "", Sectioned: hasSectionBlocks(d.Blocks), EmptyStories: map[string]bool{}}
	out.Title = d.Metadata.Title
	if out.Title == "" {
		out.Title = "Untitled"
	}
	root := colleafLayoutProjection(d.Layout)
	root.Flavor = colleafInt(1)
	root.FirstPage = colleafInt(0)
	root.EvenOdd = colleafInt(0)
	if d.EvenAndOddHeaders {
		root.EvenOdd = colleafInt(1)
	}
	rootOriginal := cloneLayout(d.Layout)
	intent.Layouts = append(intent.Layouts, colleafLayoutBinding{ID: "document", Original: &rootOriginal, Native: colleafPageGeometry(root)})
	ownership := colleafListOwnership{}
	if err = colleafResourceMap(out.Resources, colleafListResource, &ownership); err != nil {
		return out, err
	}
	if ownership.Stories == nil {
		ownership.Stories = map[string][]colleafListBinding{}
	}
	geometry := colleafImageGeometry{}
	if err = colleafResourceMap(out.Resources, colleafImageResource, &geometry); err != nil {
		return out, err
	}
	if geometry.Stories == nil {
		geometry.Stories = map[string][]colleafImageBinding{}
	}
	out.Headers = map[string]colleafRunningStory{}
	out.Footers = map[string]colleafRunningStory{}
	cache := outputImageCache{}
	runningBlocks := map[string][]Block{}
	storySerial := 0
	encodeStory := func(story *RunningStory, header bool) (*string, error) {
		if story == nil {
			return nil, nil
		}
		storySerial++
		prefix := "footer"
		if header {
			prefix = "header"
		}
		id := fmt.Sprintf("manydoc-%s-%d", prefix, storySerial)
		runningBlocks[id] = story.Blocks
		e, err := encodeColleafAssetBody(ctx, story.Blocks, nil, out.ID, id, cache)
		if err != nil {
			return nil, err
		}
		n := colleafRunningStory{Body: e.Body, Tables: e.Tables, Lists: e.Lists, Drawings: e.Drawings, DrawingOrder: e.DrawingOrder}
		if header {
			n.HeaderID = id
			out.Headers[id] = n
		} else {
			n.FooterID = id
			out.Footers[id] = n
		}
		ownership.Stories[id] = e.ListsOwnership
		geometry.Stories[id] = e.ImageGeometry
		if len(story.Blocks) == 0 {
			intent.EmptyStories[id] = true
		}
		return colleafString(id), nil
	}
	encodeStories := func(headers, footers RunningStories, p *colleafPageProperties) error {
		targets := []**string{&p.DefaultHeader, &p.FirstHeader, &p.EvenHeader, &p.DefaultFooter, &p.FirstFooter, &p.EvenFooter}
		sources := []*RunningStory{headers.Default, headers.First, headers.Even, footers.Default, footers.First, footers.Even}
		for i, story := range sources {
			id, err := encodeStory(story, i < 3)
			if err != nil {
				return err
			}
			*targets[i] = id
		}
		return nil
	}
	if !intent.Sectioned {
		if len(d.Headers) > 0 {
			root.DefaultHeader, err = encodeStory(&RunningStory{Blocks: d.Headers}, true)
			if err != nil {
				return out, err
			}
		}
		if len(d.Footers) > 0 {
			root.DefaultFooter, err = encodeStory(&RunningStory{Blocks: d.Footers}, false)
			if err != nil {
				return out, err
			}
		}
	}
	ids := map[noteKey]string{}
	var identities colleafNoteIdentities
	if err = colleafResourceMap(out.Resources, colleafNoteIdentityResource, &identities); err != nil {
		return out, err
	}
	for _, b := range identities.Bindings {
		ids[noteKey{b.Kind, b.ID}] = b.NativeID
	}
	w := colleafBodyWriter{ctx: ctx, noteIDs: ids, documentID: out.ID, imageCache: cache, tables: map[string]json.RawMessage{}, lists: map[string]json.RawMessage{}, drawings: map[string]json.RawMessage{}, body: colleafBody{TextRuns: []colleafTextRun{}}}
	sectionBlocks := resolved.Blocks
	if !intent.Sectioned {
		sectionBlocks = []Block{{Kind: SectionBlock, Section: &Section{Blocks: resolved.Blocks}}}
	}
	for i, b := range sectionBlocks {
		if b.Source != nil || len(b.Anchors) > 0 {
			return out, fmt.Errorf("%w: native section source/anchors need mapping", ErrUnsupported)
		}
		section := b.Section
		if err = w.blocks(section.Blocks, 0); err != nil {
			return out, err
		}
		record := &w.body.SectionBreaks[len(w.body.SectionBreaks)-1]
		record.ID = fmt.Sprintf("manydoc-section-%d", i+1)
		start, err := colleafSectionStart(section.Start)
		if err != nil {
			return out, err
		}
		record.Start = &start
		record.FirstPage = colleafInt(0)
		if section.FirstPage {
			record.FirstPage = colleafInt(1)
		}
		layoutBinding := colleafLayoutBinding{ID: record.ID, ImplicitStart: section.Start == "", Empty: len(section.Blocks) == 0}
		if section.Layout != nil {
			l := cloneLayout(*section.Layout)
			record.colleafPageProperties = colleafLayoutProjection(l)
			layoutBinding.Original = &l
			layoutBinding.Native = colleafPageGeometry(record.colleafPageProperties)
			record.FirstPage = colleafInt(0)
			if section.FirstPage {
				record.FirstPage = colleafInt(1)
			}
		}
		if err = encodeStories(section.Headers, section.Footers, &record.colleafPageProperties); err != nil {
			return out, err
		}
		intent.Layouts = append(intent.Layouts, layoutBinding)
		w.listDepth = 0
		w.listGroup = ""
	}
	w.body.DataStream = w.stream.String()
	if _, err = indexColleafBody(ctx, w.body); err != nil {
		return out, err
	}
	out.Body = w.body
	out.Tables = w.tables
	out.Lists = w.lists
	out.Drawings = w.drawings
	out.DrawingOrder = w.drawingOrder
	ownership.Stories[""] = w.listBindings
	geometry.Stories[""] = w.imageGeometry
	// Executable SDK readers use the document catalogs for running stories;
	// local header/footer catalogs alone are not enough for rendering.
	storyIDs := make([]string, 0, len(out.Headers)+len(out.Footers))
	for id := range out.Headers {
		storyIDs = append(storyIDs, id)
	}
	for id := range out.Footers {
		storyIDs = append(storyIDs, id)
	}
	sort.Strings(storyIDs)
	for _, id := range storyIDs {
		story, header := out.Headers[id]
		if !header {
			story = out.Footers[id]
		}
		tableNames := map[string]string{}
		for old, raw := range story.Tables {
			scoped := id + "-" + old
			tableNames[old] = scoped
			var data map[string]json.RawMessage
			if err := json.Unmarshal(raw, &data); err != nil {
				return out, err
			}
			data["tableId"] = sfAttrs(scoped)
			out.Tables[scoped] = sfAttrs(data)
		}
		for i := range story.Body.Tables {
			story.Body.Tables[i].ID = tableNames[story.Body.Tables[i].ID]
		}
		listNames := map[string]string{}
		for old, raw := range story.Lists {
			scoped := id + "-" + old
			listNames[old] = scoped
			var data colleafNativeList
			if err := json.Unmarshal(raw, &data); err != nil {
				return out, err
			}
			data.Type = scoped
			out.Lists[scoped] = sfAttrs(data)
		}
		for i, p := range story.Body.Paragraphs {
			if len(p.Bullet) > 0 {
				var bullet colleafBullet
				if err := json.Unmarshal(p.Bullet, &bullet); err != nil {
					return out, err
				}
				bullet.Type = listNames[bullet.Type]
				bullet.ID = id + "-" + bullet.ID
				story.Body.Paragraphs[i].Bullet = sfAttrs(bullet)
			}
		}
		for i, b := range ownership.Stories[id] {
			b.Type = listNames[b.Type]
			b.Group = id + "-" + b.Group
			ownership.Stories[id][i] = b
		}
		for drawing, raw := range story.Drawings {
			if _, ok := out.Drawings[drawing]; ok {
				return out, fmt.Errorf("duplicate native global drawing identity")
			}
			out.Drawings[drawing] = raw
		}
		out.HeaderFooterDrawingOrder = append(out.HeaderFooterDrawingOrder, story.DrawingOrder...)
		story.Tables = nil
		story.Lists = nil
		story.Drawings = nil
		story.DrawingOrder = nil
		if header {
			out.Headers[id] = story
		} else {
			out.Footers[id] = story
		}
	}
	styleOutput, _ := colleafStyleObject(sfAttrs(root))
	styleOutput["autoHyphenation"] = json.RawMessage("0")
	styleOutput["consecutiveHyphenLimit"] = json.RawMessage("0")
	styleOutput["doNotHyphenateCaps"] = json.RawMessage("0")
	out.DocumentStyle = sfAttrs(styleOutput)
	out.Resources = colleafReplaceResource(out.Resources, colleafListResource, ownership)
	if err = fitColleafDocumentImages(ctx, resolved, &out, &geometry, runningBlocks, cache); err != nil {
		return out, err
	}
	out.Resources = colleafReplaceResource(out.Resources, colleafImageResource, geometry)
	out.Resources = colleafReplaceResource(out.Resources, colleafDocumentResource, intent)
	if err = sealColleafIntent(&out); err != nil {
		return out, err
	}
	return out, nil
}

func decodeColleafDocument(ctx context.Context, input colleafNoteSnapshot) (*Document, error) {
	opened, err := openColleafIntent(ctx, input)
	if err != nil {
		return nil, err
	}
	for _, r := range opened.Resources {
		if r.Name == "DOC_DRAWING_PLUGIN" {
			if err := checkColleafDrawingMirror(opened, r); err != nil {
				return nil, err
			}
		}
	}
	var root colleafPageProperties
	if len(opened.DocumentStyle) > 0 {
		if err = json.Unmarshal(opened.DocumentStyle, &root); err != nil {
			return nil, err
		}
	}
	if (root.Flavor == nil || *root.Flavor == 0) && root.PageSize == nil {
		return nil, fmt.Errorf("%w: native unpaginated canvas needs layout mapping", ErrUnsupported)
	}
	if root.Flavor != nil && *root.Flavor != 0 && *root.Flavor != 1 {
		return nil, fmt.Errorf("%w: native nontraditional document flavor", ErrUnsupported)
	}
	numberPlan, err := planColleafNoteNumbers(ctx, opened)
	if err != nil {
		return nil, err
	}
	var intent colleafDocumentIntent
	if err = colleafResourceMap(opened.Resources, colleafDocumentResource, &intent); err != nil {
		return nil, err
	}
	hints := map[string]colleafLayoutBinding{}
	for _, h := range intent.Layouts {
		if _, ok := hints[h.ID]; ok {
			return nil, fmt.Errorf("duplicate native layout intent")
		}
		hints[h.ID] = h
	}
	// Traditional native defaults are measured against the pinned executable reader.
	baseline := Layout{Width: 794 * .75, Height: 1124 * .75, Margin: 54, HeaderDistance: colleafFloat(22.5), FooterDistance: colleafFloat(22.5)}
	layout, err := colleafLayoutFromProperties(root, baseline)
	if err != nil {
		return nil, err
	}
	var diagnostics []Diagnostic
	restoreLayout := func(id string, p colleafPageProperties, actual Layout) *Layout {
		if h, ok := hints[id]; ok {
			if reflect.DeepEqual(h.Native, colleafPageGeometry(p)) {
				if h.Original == nil {
					return nil
				}
				l := cloneLayout(*h.Original)
				return &l
			}
			diagnostics = append(diagnostics, Diagnostic{Code: "colleaf_stale_layout", Message: "Stale page size intent ignored; actual native layout is retained"})
		}
		l := cloneLayout(actual)
		return &l
	}
	rootLayout := restoreLayout("document", root, layout)
	if rootLayout == nil {
		rootLayout = &layout
	}
	ownership := colleafListOwnership{}
	if err = colleafResourceMap(opened.Resources, colleafListResource, &ownership); err != nil {
		return nil, err
	}
	geometry := colleafImageGeometry{}
	if err = colleafResourceMap(opened.Resources, colleafImageResource, &geometry); err != nil {
		return nil, err
	}
	settings := colleafTextContext{ImageCache: outputImageCache{}, Styles: opened.Styles, NoteLabels: numberPlan.Overrides}
	styleFields, err := colleafStyleObject(opened.DocumentStyle)
	if err != nil {
		return nil, err
	}
	settings.DefaultText = styleFields["textStyle"]
	settings.DefaultParagraph = styleFields["defaultParagraphStyle"]
	// Notes are decoded once by the existing composition. Main root sections are
	// then visited against the same validated global stream/source indexes.
	stub := opened
	stub.Body = colleafBody{DataStream: "\r\n"}
	stub.Tables = nil
	stub.Lists = nil
	stub.Drawings = nil
	stub.DrawingOrder = nil
	var records []colleafResource
	for _, r := range opened.Resources {
		if r.Name != colleafDocumentResource && r.Name != "DOC_DRAWING_PLUGIN" {
			records = append(records, r)
		}
	}
	stub.Resources = records
	stubOwnership := colleafListOwnership{Stories: map[string][]colleafListBinding{}}
	stubGeometry := colleafImageGeometry{Stories: map[string][]colleafImageBinding{}}
	for id := range opened.Notes {
		stubOwnership.Stories[id] = ownership.Stories[id]
		stubGeometry.Stories[id] = geometry.Stories[id]
	}
	stub.Resources = colleafReplaceResource(stub.Resources, colleafListResource, stubOwnership)
	stub.Resources = colleafReplaceResource(stub.Resources, colleafImageResource, stubGeometry)
	noteSettings := settings
	noteSettings.DeferNoteNumbering = true
	doc, err := decodeColleafNotes(ctx, stub, noteSettings)
	if err != nil {
		return nil, err
	}
	styleKnown := map[string]bool{}
	for _, key := range []string{"pageSize", "pageOrient", "documentFlavor", "marginTop", "marginRight", "marginBottom", "marginLeft", "marginHeader", "marginFooter", "defaultHeaderId", "defaultFooterId", "firstPageHeaderId", "firstPageFooterId", "evenPageHeaderId", "evenPageFooterId", "useFirstPageHeaderFooter", "evenAndOddHeaders", "textStyle", "defaultParagraphStyle"} {
		styleKnown[key] = true
	}
	if raw, ok := styleFields["autoHyphenation"]; ok {
		enabled, err := colleafBool(raw)
		if err != nil {
			return nil, err
		}
		if !enabled {
			styleKnown["autoHyphenation"] = true
			styleKnown["consecutiveHyphenLimit"] = true
			styleKnown["doNotHyphenateCaps"] = true
		}
	}
	var styleKeys []string
	for key := range styleFields {
		if !styleKnown[key] {
			styleKeys = append(styleKeys, key)
		}
	}
	sort.Strings(styleKeys)
	for _, key := range styleKeys {
		doc.warn("colleaf_document_style", "Native document property "+key+" is not represented; supported page geometry and content are retained")
	}
	doc.Blocks = nil
	doc.Layout = *rootLayout
	doc.Metadata = Metadata{Title: input.Title, Author: intent.Author, Subject: intent.Subject}
	if intent.EmptyTitle && input.Title == "Untitled" {
		doc.Metadata.Title = ""
	}
	if root.EvenOdd != nil {
		if *root.EvenOdd < 0 || *root.EvenOdd > 1 {
			return nil, fmt.Errorf("invalid native even/odd flag")
		}
		doc.EvenAndOddHeaders = *root.EvenOdd == 1
	}
	noteIDs := map[string]noteKey{}
	var identities colleafNoteIdentities
	_ = colleafResourceMap(opened.Resources, colleafNoteIdentityResource, &identities)
	for id, n := range opened.Notes {
		noteIDs[id] = noteKey{n.Kind, id}
	}
	for _, binding := range identities.Bindings {
		if n, ok := opened.Notes[binding.NativeID]; ok && n.Kind == binding.Kind {
			noteIDs[binding.NativeID] = noteKey{binding.Kind, binding.ID}
		}
	}
	usedTables := map[string]bool{}
	usedLists := map[string]bool{}
	usedDrawings := map[string]bool{}
	selectSources := func(body colleafBody, localTables, localLists, localDrawings map[string]json.RawMessage) (map[string]json.RawMessage, map[string]json.RawMessage, map[string]json.RawMessage, error) {
		tables, lists, drawings := map[string]json.RawMessage{}, map[string]json.RawMessage{}, map[string]json.RawMessage{}
		for _, table := range body.Tables {
			raw, ok := localTables[table.ID]
			if !ok {
				raw, ok = opened.Tables[table.ID]
				usedTables[table.ID] = true
			}
			if !ok {
				return nil, nil, nil, fmt.Errorf("native story table lacks source")
			}
			tables[table.ID] = raw
		}
		for _, p := range body.Paragraphs {
			if len(p.Bullet) > 0 && string(p.Bullet) != "null" {
				var bullet colleafBullet
				if err := json.Unmarshal(p.Bullet, &bullet); err != nil {
					return nil, nil, nil, err
				}
				raw, ok := localLists[bullet.Type]
				if !ok {
					raw, ok = opened.Lists[bullet.Type]
					usedLists[bullet.Type] = true
				}
				if !ok {
					return nil, nil, nil, fmt.Errorf("native story list lacks source")
				}
				lists[bullet.Type] = raw
			}
		}
		for _, atom := range body.CustomBlocks {
			if atom.Type != nil && *atom.Type == 0 {
				raw, ok := localDrawings[atom.ID]
				if !ok {
					raw, ok = opened.Drawings[atom.ID]
					usedDrawings[atom.ID] = true
				}
				if !ok {
					return nil, nil, nil, fmt.Errorf("native story drawing lacks source")
				}
				drawings[atom.ID] = raw
			}
		}
		// Catalog entries without a stream relationship may contain visible content.
		// Do not hide them by passing only the selected subset to the body reader.
		for id := range localTables {
			if _, ok := tables[id]; !ok {
				return nil, nil, nil, fmt.Errorf("native local table source is orphaned")
			}
		}
		for id := range localDrawings {
			if _, ok := drawings[id]; !ok {
				return nil, nil, nil, fmt.Errorf("native local drawing source is orphaned")
			}
		}
		return tables, lists, drawings, nil
	}
	mainTables, mainLists, mainDrawings, err := selectSources(opened.Body, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	settings.NoteIDs = noteIDs
	settings.Lists = mainLists
	settings.ListBindings = ownership.Stories[""]
	settings.Drawings = mainDrawings
	settings.DrawingOrder = opened.DrawingOrder
	settings.ImageBindings = geometry.Stories[""]
	index, err := indexColleafBody(ctx, opened.Body)
	if err != nil {
		return nil, err
	}
	reader, err := newColleafBodyReader(ctx, opened.Body, settings, mainTables)
	if err != nil {
		return nil, err
	}
	usedStories := map[string]bool{}
	stories := map[string]*RunningStory{}
	readStory := func(id *string, header bool) (*RunningStory, error) {
		if id == nil || *id == "" {
			return nil, nil
		}
		if story, ok := stories[*id]; ok {
			return story, nil
		}
		native, ok := opened.Footers[*id]
		if header {
			native, ok = opened.Headers[*id]
		}
		if !ok {
			return nil, fmt.Errorf("native running story reference lacks body %q", *id)
		}
		if (header && native.HeaderID != *id) || (!header && native.FooterID != *id) {
			return nil, fmt.Errorf("native running story identity mismatch")
		}
		tables, lists, drawings, err := selectSources(native.Body, native.Tables, native.Lists, native.Drawings)
		if err != nil {
			return nil, err
		}
		child := settings
		child.NoteIDs = nil
		child.Lists = lists
		child.ListBindings = ownership.Stories[*id]
		child.Drawings = drawings
		child.DrawingOrder = native.DrawingOrder
		if len(child.DrawingOrder) == 0 {
			for _, id := range opened.HeaderFooterDrawingOrder {
				if _, ok := drawings[id]; ok {
					child.DrawingOrder = append(child.DrawingOrder, id)
				}
			}
		}
		child.ImageBindings = geometry.Stories[*id]
		parsed, err := decodeColleafBody(ctx, native.Body, child, tables)
		if err != nil {
			return nil, err
		}
		doc.Diagnostics = append(doc.Diagnostics, parsed.Diagnostics...)
		blocks := parsed.Blocks
		if intent.EmptyStories[*id] && colleafEmptyBlocks(blocks) {
			blocks = nil
		}
		story := &RunningStory{Blocks: blocks}
		stories[*id] = story
		usedStories[*id] = true
		return story, nil
	}
	readStories := func(p colleafPageProperties) (RunningStories, RunningStories, error) {
		var h, f RunningStories
		targets := []**RunningStory{&h.Default, &h.First, &h.Even, &f.Default, &f.First, &f.Even}
		ids := []*string{p.DefaultHeader, p.FirstHeader, p.EvenHeader, p.DefaultFooter, p.FirstFooter, p.EvenFooter}
		for i, id := range ids {
			story, err := readStory(id, i < 3)
			if err != nil {
				return h, f, err
			}
			*targets[i] = story
		}
		return h, f, nil
	}
	rootHeaders, rootFooters, err := readStories(root)
	if err != nil {
		return nil, err
	}
	sectionRecords := map[int]colleafSectionRecord{}
	for _, record := range opened.Body.SectionBreaks {
		sectionRecords[record.StartIndex] = record
	}
	previous := layout
	for i, node := range index.sections {
		record := sectionRecords[node.end-1]
		var extras []string
		for key := range record.Extra {
			if key == "noteProperties" {
				continue
			}
			extras = append(extras, key)
		}
		sort.Strings(extras)
		for _, key := range extras {
			doc.warn("colleaf_section_style", "Native section property "+key+" is not represented; supported body order and page geometry are retained")
		}
		if record.EvenOdd != nil && ((*record.EvenOdd != 0 && *record.EvenOdd != 1) || (*record.EvenOdd == 1) != doc.EvenAndOddHeaders) {
			return nil, fmt.Errorf("%w: per-section even/odd mode cannot use a single document-wide flag", ErrUnsupported)
		}
		actual, err := colleafLayoutFromProperties(record.colleafPageProperties, previous)
		if err != nil {
			return nil, err
		}
		start := 3
		if record.Start != nil {
			start = *record.Start
		}
		if i > 0 {
			before := sectionRecords[index.sections[i-1].end-1]
			if before.Next != nil {
				start = *before.Next
			}
		}
		mode, err := colleafStartFromNative(start)
		if err != nil {
			return nil, err
		}
		blocks, err := reader.sections([]colleafStreamNode{node}, 0)
		if err != nil {
			return nil, err
		}
		if hint, ok := hints[record.ID]; ok {
			if hint.Empty && colleafEmptyBlocks(blocks) {
				blocks = nil
			}
			if hint.ImplicitStart && start == 3 {
				mode = ""
			}
		}
		h, f, err := readStories(record.colleafPageProperties)
		if err != nil {
			return nil, err
		}
		if i == 0 {
			h = inheritStories(h, rootHeaders)
			f = inheritStories(f, rootFooters)
		}
		section := &Section{Layout: restoreLayout(record.ID, record.colleafPageProperties, actual), Start: mode, Headers: h, Footers: f, Blocks: blocks}
		if record.FirstPage != nil {
			if *record.FirstPage < 0 || *record.FirstPage > 1 {
				return nil, fmt.Errorf("invalid native first-page flag")
			}
			section.FirstPage = *record.FirstPage == 1
		}
		doc.Blocks = append(doc.Blocks, Block{Kind: SectionBlock, Section: section})
		previous = actual
	}
	if len(intent.Layouts) > 0 && !intent.Sectioned && len(doc.Blocks) == 1 && !doc.EvenAndOddHeaders && !doc.Blocks[0].Section.FirstPage && doc.Blocks[0].Section.Headers.First == nil && doc.Blocks[0].Section.Headers.Even == nil && doc.Blocks[0].Section.Footers.First == nil && doc.Blocks[0].Section.Footers.Even == nil {
		section := doc.Blocks[0].Section
		doc.Blocks = section.Blocks
		if section.Layout != nil {
			doc.Layout = *section.Layout
		}
		if section.Headers.Default != nil {
			doc.Headers = section.Headers.Default.Blocks
		}
		if section.Footers.Default != nil {
			doc.Footers = section.Footers.Default.Blocks
		}
		doc.EvenAndOddHeaders = false
	}
	if len(usedStories) != len(opened.Headers)+len(opened.Footers) {
		return nil, fmt.Errorf("native running source is orphaned")
	}
	if len(usedTables) != len(opened.Tables) || len(usedDrawings) != len(opened.Drawings) {
		return nil, fmt.Errorf("native main source is orphaned")
	}
	autoLabels := map[noteKey]string{}
	for _, binding := range identities.Bindings {
		if n, ok := opened.Notes[binding.NativeID]; ok && n.Kind == binding.Kind && binding.AutoLabel != nil && n.Mark != nil && *n.Mark == *binding.AutoLabel {
			autoLabels[noteKey{binding.Kind, binding.ID}] = *binding.AutoLabel
		}
	}
	applyColleafNoteNumbers(doc, numberPlan, noteIDs)
	if err = finishColleafNoteNumbering(doc, autoLabels); err != nil {
		return nil, err
	}
	doc.Diagnostics = append(doc.Diagnostics, reader.doc.Diagnostics...)
	doc.Diagnostics = append(doc.Diagnostics, diagnostics...)
	if err = doc.Validate(); err != nil {
		return nil, err
	}
	return doc, nil
}

func (r *colleafBodyReader) breakParagraph(start, end int, p colleafParagraphRecord) ([]Block, error) {
	text, err := r.index.text.exclusive(start, end)
	if err != nil {
		return nil, err
	}
	var breaks []int
	offset := start
	for _, ch := range text {
		if ch == '\f' {
			breaks = append(breaks, offset)
		}
		offset++
		if ch > 0xffff {
			offset++
		}
	}
	if len(breaks) == 0 {
		return r.imageParagraph(start, end, p)
	}
	soft := map[int]bool{}
	for _, position := range r.body.RenderedPageBreaks {
		soft[position] = true
	}
	var blocks []Block
	cursor := start
	join := false
	appendPart := func(part []Block) {
		for _, block := range part {
			if join && len(blocks) > 0 && (block.Kind == Paragraph || block.Kind == Heading) && blocks[len(blocks)-1].Kind == block.Kind && blocks[len(blocks)-1].Level == block.Level && blocks[len(blocks)-1].Direction == block.Direction {
				blocks[len(blocks)-1].Runs = append(blocks[len(blocks)-1].Runs, block.Runs...)
			} else {
				blocks = append(blocks, block)
			}
			join = false
		}
	}
	for _, position := range breaks {
		if cursor < position {
			part, err := r.imageParagraph(cursor, position, p)
			if err != nil {
				return nil, err
			}
			appendPart(part)
		}
		if soft[position] {
			r.doc.warn("colleaf_soft_page_break", "Native rendered soft page boundary omitted from logical flow; literal content is retained")
			join = true
		} else {
			blocks = append(blocks, Block{Kind: PageBreak})
			join = false
		}
		cursor = position + 1
		props, err := colleafStyleObject(p.Style)
		if err != nil {
			return nil, err
		}
		props["pageBreakBefore"] = json.RawMessage("0")
		p.Style = sfAttrs(props)
	}
	if cursor < end {
		part, err := r.imageParagraph(cursor, end, p)
		if err != nil {
			return nil, err
		}
		appendPart(part)
	}
	return blocks, nil
}
