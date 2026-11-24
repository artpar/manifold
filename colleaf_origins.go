package manydoc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

const colleafOriginProfile = "manydoc.paragraph-origin"

type colleafOrigin struct {
	Profile     string      `json:"profile"`
	Version     int         `json:"version"`
	Ref         string      `json:"ref"`
	Fingerprint string      `json:"fingerprint,omitempty"`
	Source      *SourceInfo `json:"source,omitempty"`
}

func paragraphFingerprint(b Block) (string, error) {
	b.Source = nil
	b.Runs = append([]Run(nil), b.Runs...)
	for i := range b.Runs {
		b.Runs[i] = colleafSourceFreeRun(b.Runs[i])
	}
	nodes, err := encodeSFBlocks([]Block{b})
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(nodes)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func newColleafOrigin(b Block) (*colleafOrigin, error) {
	if b.Source == nil {
		return nil, nil
	}
	fingerprint, err := paragraphFingerprint(b)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(struct {
		Source      *SourceInfo
		Fingerprint string
	}{b.Source, fingerprint})
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	return &colleafOrigin{Profile: colleafOriginProfile, Version: 1, Ref: hex.EncodeToString(sum[:]), Fingerprint: fingerprint, Source: cloneSourceInfo(b.Source)}, nil
}
func readColleafOrigin(raw json.RawMessage) (*colleafOrigin, error) {
	return readColleafOriginProfile(raw, colleafOriginProfile)
}
func readColleafOriginProfile(raw json.RawMessage, profile string) (*colleafOrigin, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var o colleafOrigin
	if err := validateTypedJSON(context.Background(), raw, reflect.TypeFor[colleafOrigin]()); err != nil {
		return nil, err
	}
	if err := strictSFAttrs(raw, &o); err != nil {
		return nil, err
	}
	if o.Profile != profile || o.Version != 1 || len(o.Ref) != 64 {
		return nil, fmt.Errorf("%w: native paragraph origin", ErrUnsupported)
	}
	if _, err := hex.DecodeString(o.Ref); err != nil {
		return nil, err
	}
	return &o, nil
}

// Collect text-free source records once per unique source/fingerprint. Native
// paragraph properties carry only references; no body/style cache is stored.
func collectColleafOrigins(out *colleafNoteSnapshot, e **colleafDocumentEvidence) error {
	pool := map[string]colleafOrigin{}
	process := func(body *colleafBody) error {
		for i := range body.Paragraphs {
			p := &body.Paragraphs[i]
			props, err := colleafStyleObject(p.Style)
			if err != nil {
				return err
			}
			o, err := readColleafOrigin(props["manydocOrigin"])
			if err != nil {
				return err
			}
			if o == nil {
				continue
			}
			if o.Source == nil {
				return fmt.Errorf("native output origin lacks source")
			}
			if old, ok := pool[o.Ref]; ok && !reflect.DeepEqual(old, *o) {
				return fmt.Errorf("conflicting origin reference")
			}
			pool[o.Ref] = *o
			o.Source = nil
			o.Fingerprint = ""
			props["manydocOrigin"] = sfAttrs(o)
			p.Style = sfAttrs(props)
		}
		return nil
	}
	if err := process(&out.Body); err != nil {
		return err
	}
	for _, header := range []bool{true, false} {
		stories := out.Footers
		if header {
			stories = out.Headers
		}
		for id, story := range stories {
			if err := process(&story.Body); err != nil {
				return err
			}
			stories[id] = story
		}
	}
	for id, note := range out.Notes {
		if err := process(&note.Body); err != nil {
			return err
		}
		out.Notes[id] = note
	}
	if len(pool) == 0 {
		return nil
	}
	if *e == nil {
		*e = &colleafDocumentEvidence{Version: 1}
	}
	refs := make([]string, 0, len(pool))
	for ref := range pool {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	for _, ref := range refs {
		(*e).Origins = append((*e).Origins, pool[ref])
	}
	return nil
}
func validateColleafOrigin(o colleafOrigin, pages []PDFPage) error {
	return validateColleafOriginProfile(o, pages, colleafOriginProfile)
}
func validateColleafOriginProfile(o colleafOrigin, pages []PDFPage, profile string) error {
	checked, err := readColleafOriginProfile(sfAttrs(o), profile)
	if err != nil {
		return err
	}
	if checked == nil || checked.Source == nil || len(checked.Fingerprint) != 64 {
		return fmt.Errorf("incomplete native source record")
	}
	if _, err = hex.DecodeString(checked.Fingerprint); err != nil {
		return err
	}
	if err = checked.Source.validate(pages); err != nil {
		return err
	}
	raw, err := json.Marshal(struct {
		Source      *SourceInfo
		Fingerprint string
	}{checked.Source, checked.Fingerprint})
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	if checked.Ref != hex.EncodeToString(sum[:]) {
		return fmt.Errorf("native source reference digest mismatch")
	}
	return nil
}
func restoreColleafOrigins(d *Document, e *colleafDocumentEvidence) error {
	pool := map[string]colleafOrigin{}
	if e != nil {
		for _, o := range e.Origins {
			if err := validateColleafOrigin(o, d.Pages); err != nil {
				return err
			}
			if _, exists := pool[o.Ref]; exists {
				return fmt.Errorf("duplicate native origin reference")
			}
			pool[o.Ref] = o
		}
	}
	used := map[string]bool{}
	var walk func([]Block) error
	walk = func(bs []Block) error {
		for i := range bs {
			b := &bs[i]
			if o := b.colleafOrigin; o != nil {
				record, ok := pool[o.Ref]
				if !ok {
					return fmt.Errorf("native paragraph references missing source record")
				}
				if (o.Source != nil || o.Fingerprint != "") && !reflect.DeepEqual(*o, record) {
					return fmt.Errorf("contradictory inline/native source record")
				}
				hash, err := paragraphFingerprint(*b)
				if err != nil {
					return err
				}
				b.Source = cloneSourceInfo(record.Source)
				b.colleafOrigin = nil
				used[o.Ref] = true
				if hash != record.Fingerprint {
					b.Source.Historical = true
					d.warn("colleaf_source_historical", "Original PDF source facts are retained as historical after current paragraph content or presentation changed")
				}
			}
			if b.Section != nil {
				if err := walk(b.Section.Blocks); err != nil {
					return err
				}
				for _, story := range sectionStories(b.Section) {
					if story != nil {
						if err := walk(story.Blocks); err != nil {
							return err
						}
					}
				}
			}
			for j := range b.Items {
				if b.Items[j].Marker != nil {
					if err := walk(b.Items[j].Marker.Blocks); err != nil {
						return err
					}
				}
				if err := walk(b.Items[j].Blocks); err != nil {
					return err
				}
			}
			for j := range b.Rows {
				for k := range b.Rows[j].Cells {
					if err := walk(b.Rows[j].Cells[k].Blocks); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	for _, bs := range [][]Block{d.Blocks, d.Headers, d.Footers} {
		if err := walk(bs); err != nil {
			return err
		}
	}
	for i := range d.Notes {
		if err := walk(d.Notes[i].Blocks); err != nil {
			return err
		}
	}
	seen := map[string]bool{}
	for _, s := range d.UnboundSources {
		seen[string(sfAttrs(s))] = true
	}
	refs := make([]string, 0, len(pool))
	for ref := range pool {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	for _, ref := range refs {
		if !used[ref] {
			source := pool[ref].Source
			key := string(sfAttrs(source))
			if !seen[key] {
				d.UnboundSources = append(d.UnboundSources, *cloneSourceInfo(source))
				seen[key] = true
			}
			d.warn("colleaf_source_unbound", "Original PDF source facts are retained unbound after their native paragraph binding disappeared")
		}
	}
	return nil
}

func newColleafSourceOrigin(source *SourceInfo, hash, profile string) (*colleafOrigin, error) {
	if source == nil {
		return nil, nil
	}
	raw, err := json.Marshal(struct {
		Source      *SourceInfo
		Fingerprint string
	}{source, hash})
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	return &colleafOrigin{Profile: profile, Version: 1, Ref: hex.EncodeToString(sum[:]), Fingerprint: hash, Source: cloneSourceInfo(source)}, nil
}
