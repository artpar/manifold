package manydoc

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"
)

type colleafTextContext struct {
	DeferNoteNumbering bool
	Citations          *colleafCitationState
	NoteLabels         map[string]string
	DefaultText        json.RawMessage
	DefaultParagraph   json.RawMessage
	Styles             map[string]colleafNamedStyle
	NoteIDs            map[string]noteKey
	Lists              map[string]json.RawMessage
	ListBindings       []colleafListBinding
	Drawings           map[string]json.RawMessage
	DrawingOrder       []string
	ImageBindings      []colleafImageBinding
	ImageCache         outputImageCache
}

func colleafRawInt(style colleafStyleMap, key string) (int, bool, error) {
	raw, ok := style[key]
	if !ok {
		return 0, false, nil
	}
	var n *int
	if err := json.Unmarshal(raw, &n); err != nil || n == nil {
		return 0, true, fmt.Errorf("native %s must be an integer", key)
	}
	return *n, true, nil
}
func colleafUTF16Len(text string) int {
	n := 0
	for _, r := range text {
		n += utf16.RuneLen(r)
	}
	return n
}

// These private text workers are shared by future main/story/note mappings.
// Unsupported rich containers reject before returning partial document content.
func encodeColleafTextBody(ctx context.Context, blocks []Block) (colleafBody, error) {
	return encodeColleafTextBodyWithNotes(ctx, blocks, nil)
}
func encodeColleafTextBodyWithNotes(ctx context.Context, blocks []Block, noteIDs map[noteKey]string, projection ...map[noteKey]string) (colleafBody, error) {
	body := colleafBody{TextRuns: []colleafTextRun{}}
	var stream strings.Builder
	offset := 0
	serial := 0
	appendText := func(text string) { stream.WriteString(text); offset += colleafUTF16Len(text) }
	for _, block := range blocks {
		if err := ctx.Err(); err != nil {
			return body, err
		}
		if (block.Kind != Paragraph && block.Kind != Heading) || block.Source != nil || len(block.Anchors) > 0 {
			return body, fmt.Errorf("%w: native text worker requires paragraph/heading content without unmapped provenance/anchors", ErrUnsupported)
		}
		p := colleafParagraphRecord{ID: fmt.Sprintf("p%d", serial)}
		serial++
		properties := map[string]any{}
		if block.Kind == Heading {
			if block.Level < 1 || block.Level > 6 {
				return body, fmt.Errorf("invalid heading rank")
			}
			properties["outlineLevel"] = block.Level - 1
			if block.Level <= 5 {
				properties["namedStyleType"] = block.Level + 3
			}
		}
		if block.Direction == "ltr" {
			properties["direction"] = 1
		} else if block.Direction == "rtl" {
			properties["direction"] = 2
		} else if block.Direction != "" {
			return body, fmt.Errorf("invalid paragraph direction")
		}
		if block.PageBreakBefore {
			properties["pageBreakBefore"] = 1
		}
		if len(properties) > 0 {
			p.Style = sfAttrs(properties)
		}
		for _, run := range block.Runs {
			if run.Anchor != "" || strings.ContainsAny(run.Text, "\r\f\v\b\x1a\x1b\x1c\x1d\x0e\x0f\x1e\x1f") {
				return body, fmt.Errorf("%w: native text atom/break needs its full structural mapping", ErrUnsupported)
			}
			if err := validateString(run.Text); err != nil {
				return body, err
			}
			if run.Note != nil && len(projection) > 0 {
				if shared, projected := projection[0][referenceKey(run.Note)]; projected {
					id, ok := noteIDs[referenceKey(run.Note)]
					if !ok {
						return body, fmt.Errorf("native literal citation lacks definition")
					}
					run = canonicalCitationRun(run)
					mode := "default"
					var label []Run
					if run.Note.LabelContent != nil {
						mode = "rich"
						label = run.Note.LabelContent
					} else {
						text := shared
						if run.Note.Label != nil {
							mode = "literal"
							text = *run.Note.Label
						}
						r := run
						r.Note = nil
						r.Text = text
						label = []Run{r}
					}
					fragment, err := writeColleafCitation(ctx, run, id, mode, label)
					if err != nil {
						return body, err
					}
					for _, r := range fragment.TextRuns {
						r.St += offset
						r.Ed += offset
						body.TextRuns = append(body.TextRuns, r)
					}
					for _, a := range fragment.CustomRanges {
						a.StartIndex += offset
						a.EndIndex += offset
						a.ID = fmt.Sprintf("citation%d", len(body.CustomRanges))
						nameColleafCitationRange(&a)
						body.CustomRanges = append(body.CustomRanges, a)
					}
					appendText(fragment.DataStream)
					continue
				}
			}
			if run.Note != nil {
				id, ok := noteIDs[referenceKey(run.Note)]
				if !ok || run.Text != "" || run.Link != "" || run.Destination != "" || run.Note.Label != nil || run.Note.LabelContent != nil || run.Note.Inferred {
					return body, fmt.Errorf("%w: native note citation requires a mapped definition and shared marker semantics", ErrUnsupported)
				}
				style, err := colleafRunStyle(run)
				if err != nil {
					return body, err
				}
				kind := 8
				if run.Note.Kind == Endnote {
					kind = 9
				}
				body.CustomRanges = append(body.CustomRanges, colleafRangeRecord{StartIndex: offset, EndIndex: offset, ID: fmt.Sprintf("note%d", len(body.CustomRanges)), Type: kind, Whole: true, Properties: sfAttrs(map[string]string{"noteId": id})})
				body.TextRuns = append(body.TextRuns, colleafTextRun{St: offset, Ed: offset + 1, TS: style})
				appendText("\ufffc")
				continue
			}
			if run.Text == "" {
				continue
			}
			style, err := colleafRunStyle(run)
			if err != nil {
				return body, err
			}
			start := offset
			appendText(strings.ReplaceAll(run.Text, "\n", "\v"))
			body.TextRuns = append(body.TextRuns, colleafTextRun{St: start, Ed: offset, TS: style})
			if run.Link != "" || run.Destination != "" {
				if run.Link != "" && run.Destination != "" && run.Link != "#"+run.Destination {
					return body, fmt.Errorf("conflicting native link targets")
				}
				properties := map[string]any{"textStyleMode": "text"}
				if run.Destination != "" {
					properties["bookmarkId"] = run.Destination
				} else {
					properties["url"] = run.Link
				}
				body.CustomRanges = append(body.CustomRanges, colleafRangeRecord{StartIndex: start, EndIndex: offset - 1, ID: fmt.Sprintf("link%d", len(body.CustomRanges)), Type: 0, Properties: sfAttrs(properties)})
			}
		}
		p.StartIndex = offset
		appendText("\r")
		body.Paragraphs = append(body.Paragraphs, p)
	}
	if len(blocks) == 0 {
		body.Paragraphs = append(body.Paragraphs, colleafParagraphRecord{StartIndex: 0, ID: "p0"})
		appendText("\r")
	}
	body.SectionBreaks = []colleafSectionRecord{{StartIndex: offset, ID: "s0"}}
	appendText("\n")
	body.DataStream = stream.String()
	if _, err := indexColleafBody(ctx, body); err != nil {
		return body, err
	}
	return body, nil
}

func decodeColleafTextBody(ctx context.Context, body colleafBody, settings colleafTextContext) (*Document, error) {
	index, err := indexColleafBody(ctx, body)
	if err != nil {
		return nil, err
	}
	if len(index.tables) > 0 || len(index.sections) != 1 {
		return nil, fmt.Errorf("%w: native containers/sections require the rich body mapping", ErrUnsupported)
	}
	var blockBounds []int
	for position, block := range index.blocks {
		key, ok := settings.NoteIDs[block.NoteID]
		if !ok || (block.Type != nil && *block.Type != 1) || (block.NoteType != "" && block.NoteType != key.kind) {
			return nil, fmt.Errorf("%w: unmapped/contradictory native note atom", ErrUnsupported)
		}
		blockBounds = append(blockBounds, position, position+1)
	}
	sort.Ints(blockBounds)
	doc := &Document{}
	resolver := colleafStyleResolver{named: settings.Styles}
	baseText, err := colleafStyleObject(settings.DefaultText)
	if err != nil {
		return nil, err
	}
	baseParagraph, err := colleafStyleObject(settings.DefaultParagraph)
	if err != nil {
		return nil, err
	}
	paragraphMetadata := map[int]colleafParagraphRecord{}
	for _, p := range body.Paragraphs {
		paragraphMetadata[p.StartIndex] = p
	}
	spans, err := colleafStyleSpans(ctx, index.text, body.TextRuns)
	if err != nil {
		return nil, err
	}
	var annotations []colleafRangeRecord
	for _, a := range body.CustomRanges {
		if a.Type != 8 && a.Type != 9 {
			annotations = append(annotations, a)
		}
	}
	sort.Slice(annotations, func(i, j int) bool { return annotations[i].StartIndex < annotations[j].StartIndex })
	var annotationBounds []int
	for i, annotation := range annotations {
		if annotation.Type != 0 {
			return nil, fmt.Errorf("%w: native annotation type %d requires its semantic mapping", ErrUnsupported, annotation.Type)
		}
		if i > 0 && annotations[i-1].EndIndex >= annotation.StartIndex {
			return nil, fmt.Errorf("conflicting native hyperlink extents")
		}
		annotationBounds = append(annotationBounds, annotation.StartIndex, annotation.EndIndex+1)
	}
	for _, node := range index.sections[0].children {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if node.kind != "paragraph" || len(node.children) > 0 {
			return nil, fmt.Errorf("%w: nontext native paragraph", ErrUnsupported)
		}
		p := paragraphMetadata[node.end-1]
		named, err := resolver.resolve(p.StyleID, 0)
		if err != nil {
			return nil, err
		}
		namedParagraph, err := colleafStyleObject(named.ParagraphStyle)
		if err != nil {
			return nil, err
		}
		directParagraph, err := colleafStyleObject(p.Style)
		if err != nil {
			return nil, err
		}
		properties := mergeColleafStyle(mergeColleafStyle(baseParagraph, namedParagraph), directParagraph)
		block := Block{Kind: Paragraph}
		level, hasLevel, err := colleafRawInt(properties, "outlineLevel")
		if err != nil {
			return nil, err
		}
		namedType, hasNamed, err := colleafRawInt(properties, "namedStyleType")
		if err != nil {
			return nil, err
		}
		if hasLevel && level >= 0 && level <= 5 {
			block.Kind = Heading
			block.Level = level + 1
		} else if !hasLevel && hasNamed && namedType >= 4 && namedType <= 8 {
			block.Kind = Heading
			block.Level = namedType - 3
		} else if hasLevel && level != 9 {
			return nil, fmt.Errorf("%w: native outline level %d", ErrUnsupported, level)
		}
		direction, _, err := colleafRawInt(properties, "direction")
		if err != nil {
			return nil, err
		}
		switch direction {
		case 0:
		case 1:
			block.Direction = "ltr"
		case 2:
			block.Direction = "rtl"
		default:
			return nil, fmt.Errorf("invalid native paragraph direction")
		}
		if raw, ok := properties["pageBreakBefore"]; ok {
			block.PageBreakBefore, err = colleafBool(raw)
			if err != nil {
				return nil, err
			}
		}
		pText, err := colleafStyleObject(properties["textStyle"])
		if err != nil {
			return nil, err
		}
		namedText, err := colleafStyleObject(named.TextStyle)
		if err != nil {
			return nil, err
		}
		defaults := mergeColleafStyle(mergeColleafStyle(baseText, namedText), pText)
		boundaries := []int{node.start, node.end - 1}
		firstSpan := sort.Search(len(spans), func(i int) bool { return spans[i].end > node.start })
		for _, span := range spans[firstSpan:] {
			if span.start >= node.end-1 {
				break
			}
			if span.start > node.start && span.start < node.end-1 {
				boundaries = append(boundaries, span.start)
			}
			if span.end > node.start && span.end < node.end-1 {
				boundaries = append(boundaries, span.end)
			}
		}
		firstAnnotation := sort.SearchInts(annotationBounds, node.start+1)
		for _, boundary := range annotationBounds[firstAnnotation:] {
			if boundary >= node.end-1 {
				break
			}
			boundaries = append(boundaries, boundary)
		}
		firstBlock := sort.SearchInts(blockBounds, node.start+1)
		for _, boundary := range blockBounds[firstBlock:] {
			if boundary >= node.end-1 {
				break
			}
			boundaries = append(boundaries, boundary)
		}
		sort.Ints(boundaries)
		previous := -1
		for i := 0; i < len(boundaries)-1; i++ {
			start, end := boundaries[i], boundaries[i+1]
			if start == end || start == previous {
				continue
			}
			previous = start
			text, err := index.text.exclusive(start, end)
			if err != nil {
				return nil, err
			}
			if strings.ContainsAny(text, "\f\x1e\x1f") {
				return nil, fmt.Errorf("%w: native text control requires semantic mapping", ErrUnsupported)
			}
			record := -1
			position := sort.Search(len(spans), func(i int) bool { return spans[i].end > start })
			if position < len(spans) {
				record = spans[position].record
			}
			style := defaults
			if record >= 0 {
				raw := body.TextRuns[record]
				character, err := resolver.resolve(raw.StyleID, 0)
				if err != nil {
					return nil, err
				}
				inherited, err := colleafStyleObject(character.TextStyle)
				if err != nil {
					return nil, err
				}
				direct, err := colleafStyleObject(raw.TS)
				if err != nil {
					return nil, err
				}
				style = mergeColleafStyle(mergeColleafStyle(defaults, inherited), direct)
			}
			run, err := colleafStyleRun(style, doc)
			if err != nil {
				return nil, err
			}
			run.Text = strings.ReplaceAll(text, "\v", "\n")
			if text == "\b" || (text == "\ufffc" && index.blocks[start].NoteID != "") {
				atom, ok := index.blocks[start]
				if !ok {
					return nil, fmt.Errorf("unowned native note atom")
				}
				key := settings.NoteIDs[atom.NoteID]
				run.Text = ""
				run.Note = &NoteReference{ID: key.id, Kind: key.kind}
				if settings.Citations != nil {
					if scope, ok := settings.Citations.Scopes[atom.ID]; ok {
						settings.Citations.Pending[run.Note] = colleafCitationPending{Scope: scope}
					}
				}

				if label, ok := settings.NoteLabels[atom.ID]; ok {
					copy := label
					run.Note.Label = &copy
				}
			} else if strings.ContainsRune(text, '\b') {
				return nil, fmt.Errorf("native note atom was not isolated")
			}
			var link *colleafRangeRecord
			position = sort.Search(len(annotations), func(i int) bool { return annotations[i].EndIndex >= start })
			if position < len(annotations) && annotations[position].StartIndex <= start {
				link = &annotations[position]
			}
			if link != nil {
				if run.Note != nil {
					return nil, fmt.Errorf("%w: hyperlink on native note citation requires navigation mapping", ErrUnsupported)
				}
				var target struct {
					URL      string `json:"url"`
					Bookmark string `json:"bookmarkId"`
					Heading  string `json:"headingId"`
					Mode     string `json:"textStyleMode"`
				}
				if err := json.Unmarshal(link.Properties, &target); err != nil {
					return nil, err
				}
				count := 0
				for _, value := range []string{target.URL, target.Bookmark, target.Heading} {
					if value != "" {
						count++
					}
				}
				if count != 1 {
					return nil, fmt.Errorf("native hyperlink requires one target")
				}
				run.Link = target.URL
				run.Destination = target.Bookmark
				if target.Heading != "" {
					run.Destination = target.Heading
				}
				if target.Mode != "text" {
					doc.warn("colleaf_link_presentation", "Native automatic hyperlink presentation may differ; target and authored text style are retained")
				}
			}
			block.Runs = append(block.Runs, run)
		}
		if len(p.Bullet) > 0 && string(p.Bullet) != "null" {
			return nil, fmt.Errorf("%w: native paragraph numbering needs list ownership mapping", ErrUnsupported)
		}
		keys := make([]string, 0, len(properties))
		for key := range properties {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			switch key {
			case "outlineLevel", "namedStyleType", "direction", "pageBreakBefore", "textStyle":
			default:
				doc.warn("colleaf_paragraph_style", "Native paragraph property "+key+" is not represented; text and supported paragraph semantics are retained")
			}
		}
		doc.Blocks = append(doc.Blocks, block)
	}
	return doc, nil
}
