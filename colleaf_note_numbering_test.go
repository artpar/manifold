package manydoc

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func colleafForeignNumberingExample() colleafNoteSnapshot {
	w := colleafNoteSnapshot{Body: colleafBody{TextRuns: []colleafTextRun{}}, ID: "foreign-numbering", Title: "Native automatic marks", DocumentStyle: json.RawMessage(`{"documentFlavor":1,"pageSize":{"width":800,"height":1000},"marginTop":40,"marginBottom":40,"marginLeft":40,"marginRight":40,"autoHyphenation":0}`), NoteSettings: json.RawMessage(`{"footnote":{"numberFormat":"upperRoman","startNumber":3,"restart":"eachSect","position":"beneathText"},"endnote":{"numberFormat":"upperLetter","startNumber":27,"restart":"continuous","position":"sectEnd"}}`), Notes: map[string]colleafNativeNote{}}
	var stream strings.Builder
	serial := 0
	offset := 0
	add := func(text string) { stream.WriteString(text); offset += colleafUTF16Len(text) }
	section := func(id string, refs []string, properties string) {
		add("A😀 ")
		for _, id := range refs {
			kind := Footnote
			typ := 8
			if strings.HasPrefix(id, "e") {
				kind = Endnote
				typ = 9
			}
			serial++
			w.Body.CustomRanges = append(w.Body.CustomRanges, colleafRangeRecord{ID: "citation-" + string(rune('a'+serial)), StartIndex: offset, EndIndex: offset, Type: typ, Whole: true, Properties: sfAttrs(map[string]string{"noteId": id})})
			add("\ufffc ")
			body := "Body " + id
			w.Notes[id] = colleafNativeNote{ID: id, Kind: kind, Body: colleafBody{TextRuns: []colleafTextRun{}, DataStream: body + "\r\n", Paragraphs: []colleafParagraphRecord{{ID: "body-" + id, StartIndex: len(body)}}}}
		}
		w.Body.Paragraphs = append(w.Body.Paragraphs, colleafParagraphRecord{ID: "p-" + id, StartIndex: offset})
		add("\r")
		record := colleafSectionRecord{ID: id, StartIndex: offset}
		if properties != "" {
			record.Extra = map[string]json.RawMessage{"noteProperties": json.RawMessage(properties)}
		}
		w.Body.SectionBreaks = append(w.Body.SectionBreaks, record)
		add("\n")
	}
	section("one", []string{"f1", "e1", "shared", "shared", "custom", "f2"}, "")
	section("two", []string{"f3", "e2", "shared"}, `{"footnote":{"numberFormat":"decimalZero","startNumber":0,"restart":"eachSect"}}`)
	section("three", []string{"silent"}, `{"footnote":{"numberFormat":"none","startNumber":1,"restart":"eachSect"}}`)
	n := w.Notes["custom"]
	mark := "§"
	n.Mark = &mark
	w.Notes["custom"] = n
	w.Body.DataStream = stream.String()
	return w
}
func TestColleafForeignAutomaticNoteNumbers(t *testing.T) {
	w := colleafForeignNumberingExample()
	doc, err := decodeColleafDocument(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Layout.HeaderDistance == nil || *doc.Layout.HeaderDistance != 22.5 || doc.Layout.FooterDistance == nil || *doc.Layout.FooterDistance != 22.5 {
		t.Fatal("native omitted-distance default changed", doc.Layout)
	}
	c, err := catalogNotes(doc, false)
	if err != nil {
		t.Fatal(err)
	}
	var labels []string
	if err = walkNoteRuns(doc.Blocks, func(r Run) error {
		if r.Note != nil {
			labels = append(labels, noteReferenceLabel(c, r.Note))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{"III", "AA", "IV", "V", "§", "VI", "00", "BB", "01", ""}
	if !reflect.DeepEqual(labels, want) {
		t.Fatalf("visible native labels changed: got%q want%q", labels, want)
	}
	foundShared := false
	foundSilent := false
	for _, n := range doc.Notes {
		if n.ID == "shared" {
			foundShared = true
			if n.Label != "IV" || n.BodyLabel == nil || *n.BodyLabel != "01" {
				t.Fatal("repeated native body mark lost", n)
			}
		}
		if n.ID == "silent" {
			foundSilent = true
			if n.BodyLabel == nil || *n.BodyLabel != "" {
				t.Fatal("markerless note became automatic", n)
			}
		}
	}
	if !foundShared || !foundSilent || len(doc.Diagnostics) != 2 {
		t.Fatal("native settings/numbering losses not disclosed", doc.Diagnostics)
	}
	sf, err := ToStructured(doc)
	if err != nil {
		t.Fatal(err)
	}
	got, err := FromStructured(sf)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, doc) {
		t.Fatal("SF changed actual note marks")
	}
	if file := os.Getenv("MANYDOC_COLLEAF_NUMBER_QA"); file != "" {
		data, err := json.MarshalIndent(w, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if file := os.Getenv("MANYDOC_COLLEAF_NUMBER_SAVED"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var saved colleafNoteSnapshot
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		got, err = decodeColleafDocument(context.Background(), saved)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, doc) {
			a, _ := json.Marshal(got)
			b, _ := json.Marshal(doc)
			t.Fatalf("actual facade changed imported note semantics\ngot%s\nwant%s", a, b)
		}
	}
}
func TestColleafDefaultNativeEndnoteRomanAndEmptyMark(t *testing.T) {
	w := colleafForeignNumberingExample()
	w.NoteSettings = nil
	w.Body.SectionBreaks = w.Body.SectionBreaks[:1]
	w.Body.SectionBreaks[0].Extra = nil
	// Keep just the first authored section; defaults are independently significant.
	w.Body.DataStream = strings.SplitN(w.Body.DataStream, "\n", 2)[0] + "\n"
	w.Body.Paragraphs = w.Body.Paragraphs[:1]
	w.Body.CustomRanges = w.Body.CustomRanges[:6]
	for _, id := range []string{"f3", "e2", "silent"} {
		delete(w.Notes, id)
	}
	doc, err := decodeColleafDocument(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range doc.Notes {
		if n.ID == "e1" && n.Label != "i" {
			t.Fatal("native endnote Roman default changed", n.Label)
		}
	}
	n := w.Notes["custom"]
	empty := ""
	n.Mark = &empty
	w.Notes["custom"] = n
	doc, err = decodeColleafDocument(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range doc.Notes {
		if n.ID == "custom" && (n.BodyLabel == nil || *n.BodyLabel != "") {
			t.Fatal("empty custom mark lost")
		}
	}
}
func TestColleafNativeNumberingRejectsUnprovenOrMalformedSettings(t *testing.T) {
	for _, raw := range []string{`{"footnote":{"restart":"eachPage"}}`, `{"footnote":{"startNumber":-1}}`, `{"footnote":{"startNumber":null}}`, `{"endnote":{"columnCount":2}}`, `{"footnote":{"position":"docEnd"}}`, `{"footnote":{"separator":{"dataStream":"Visible separator\r\n"}}}`, `{"footnote":{"numberFormat":"not-a-format"}}`} {
		w := colleafForeignNumberingExample()
		w.NoteSettings = json.RawMessage(raw)
		if _, err := decodeColleafDocument(context.Background(), w); err == nil {
			t.Fatal("unproven setting accepted", raw)
		}
	}
	w := colleafForeignNumberingExample()
	w.Body.CustomRanges[0], w.Body.CustomRanges[1] = w.Body.CustomRanges[1], w.Body.CustomRanges[0]
	if _, err := decodeColleafDocument(context.Background(), w); err == nil {
		t.Fatal("counter order silently changed")
	}
}
func TestColleafNativeNoteFormatZeroAndSymbols(t *testing.T) {
	for _, tc := range []struct {
		format string
		value  int
		want   string
	}{{"decimalZero", 0, "00"}, {"lowerRoman", 0, ""}, {"upperLetter", 0, ""}, {"chicago", 7, "**"}, {"decimalFullWidth", 12, "１２"}, {"ideographDigital", 10, "一〇"}, {"decimalEnclosedCircle", 36, "㊱"}, {"ideographZodiac", 12, "亥"}, {"upperRoman", 14, "XIV"}, {"lowerLetter", 27, "aa"}, {"decimalFullWidth2", 0, "０"}, {"taiwaneseDigital", 0, "〇"}, {"decimalEnclosedCircle", 51, "51"}, {"ideographEnclosedCircle", 10, "㈩"}, {"ideographTraditional", 10, "癸"}} {
		got, err := colleafNoteNumber(tc.format, tc.value)
		if err != nil || got != tc.want {
			t.Fatal(tc, got, err)
		}
	}
}

func TestColleafNativeLiteralMarksDoNotRequirePageNumberInference(t *testing.T) {
	w := colleafForeignNumberingExample()
	w.NoteSettings = json.RawMessage(`{"footnote":{"restart":"eachPage","columnCount":2,"position":"beneathText"}}`)
	for id, n := range w.Notes {
		label := "fixed-" + id
		n.Mark = &label
		w.Notes[id] = n
	}
	for i := range w.Body.SectionBreaks {
		w.Body.SectionBreaks[i].Extra = nil
	}
	doc, err := decodeColleafDocument(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Diagnostics) != 1 || doc.Diagnostics[0].Code != "colleaf_note_settings" {
		t.Fatal(doc.Diagnostics)
	}
}
func TestColleafImportedNativeMarksUseSharedOutputPaths(t *testing.T) {
	ctx := context.Background()
	doc, err := decodeColleafDocument(ctx, colleafForeignNumberingExample())
	if err != nil {
		t.Fatal(err)
	}
	font, err := os.ReadFile("testdata/assets/DejaVuSans.ttf")
	if err != nil {
		t.Fatal(err)
	}
	bold, err := os.ReadFile("testdata/assets/DejaVuSans-Bold.ttf")
	if err != nil {
		t.Fatal(err)
	}
	options := WriteOptions{AllowLossy: true, PDFFonts: []FontFace{{Family: "DejaVu", Data: font}, {Family: "DejaVu", Data: bold, Bold: true}}, PDFFontMap: map[string]string{"Go": "DejaVu"}}
	for _, format := range []Format{DOCX, PDF} {
		t.Run(string(format), func(t *testing.T) {
			var out bytes.Buffer
			if err := Write(ctx, &out, doc, format, options); err != nil {
				t.Fatal(err)
			}
			if format == DOCX {
				back, err := Read(ctx, bytes.NewReader(out.Bytes()), DOCX, ReadOptions{})
				if err != nil {
					t.Fatal(err)
				}
				c, err := catalogNotes(back, false)
				if err != nil {
					t.Fatal(err)
				}
				var labels []string
				if err = walkNoteRuns(back.Blocks, func(r Run) error {
					if r.Note != nil {
						labels = append(labels, noteReferenceLabel(c, r.Note))
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(labels, []string{"III", "AA", "IV", "V", "§", "VI", "00", "BB", "01", ""}) {
					t.Fatal("Word conversion changed native marks", labels)
				}
			}
			if file := os.Getenv("MANYDOC_COLLEAF_NUMBER_OUTPUT"); file != "" {
				if err = os.WriteFile(file+"."+string(format), out.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
