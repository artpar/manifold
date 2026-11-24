package manydoc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

const colleafRunOriginProfile = "manydoc.run-origin"

func colleafSourceFreeRun(r Run) Run {
	r.Source = nil
	if r.Note != nil && r.Note.LabelContent != nil {
		n := *r.Note
		n.LabelContent = append([]Run(nil), n.LabelContent...)
		for i := range n.LabelContent {
			n.LabelContent[i] = colleafSourceFreeRun(n.LabelContent[i])
		}
		r.Note = &n
	}
	return r
}
func colleafRunFingerprint(r Run) (string, error) {
	r = colleafSourceFreeRun(r)
	nodes, err := encodeSFBlocks([]Block{{Kind: Paragraph, Runs: []Run{r}}})
	if err != nil {
		return "", err
	}
	return colleafSourceFreeHash(nodes)
}
func newColleafRunOrigin(r Run) (*colleafOrigin, error) {
	if r.Source == nil {
		return nil, nil
	}
	hash, err := colleafRunFingerprint(r)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(struct {
		Source      *SourceInfo
		Fingerprint string
	}{r.Source, hash})
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	return &colleafOrigin{Profile: colleafRunOriginProfile, Version: 1, Ref: hex.EncodeToString(sum[:]), Fingerprint: hash, Source: cloneSourceInfo(r.Source)}, nil
}
func collectColleafRunOrigins(out *colleafNoteSnapshot, e **colleafDocumentEvidence) error {
	pool := map[string]colleafOrigin{}
	style := func(raw *json.RawMessage) error {
		props, err := colleafStyleObject(*raw)
		if err != nil {
			return err
		}
		o, err := readColleafOriginProfile(props["manydocOrigin"], colleafRunOriginProfile)
		if err != nil {
			return err
		}
		if o == nil {
			return nil
		}
		if o.Source == nil {
			return fmt.Errorf("native output run origin lacks source")
		}
		if old, ok := pool[o.Ref]; ok && !reflect.DeepEqual(old, *o) {
			return fmt.Errorf("conflicting run origin reference")
		}
		pool[o.Ref] = *o
		o.Source = nil
		o.Fingerprint = ""
		props["manydocOrigin"] = sfAttrs(o)
		*raw = sfAttrs(props)
		return nil
	}
	process := func(body *colleafBody) error {
		for i := range body.TextRuns {
			if err := style(&body.TextRuns[i].TS); err != nil {
				return err
			}
		}
		return nil
	}

	if err := process(&out.Body); err != nil {
		return err
	}
	for _, stories := range []map[string]colleafRunningStory{out.Headers, out.Footers} {
		for id, s := range stories {
			if err := process(&s.Body); err != nil {
				return err
			}
			stories[id] = s
		}
	}
	for id, n := range out.Notes {
		if err := process(&n.Body); err != nil {
			return err
		}
		out.Notes[id] = n
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
		(*e).RunOrigins = append((*e).RunOrigins, pool[ref])
	}
	return nil
}
func restoreColleafRunOrigins(d *Document, e *colleafDocumentEvidence) error {
	pool := map[string]colleafOrigin{}
	if e != nil {
		for _, o := range e.RunOrigins {
			if err := validateColleafOriginProfile(o, d.Pages, colleafRunOriginProfile); err != nil {
				return err
			}
			if _, ok := pool[o.Ref]; ok {
				return fmt.Errorf("duplicate native run origin reference")
			}
			pool[o.Ref] = o
		}
	}
	used := map[string]bool{}
	var runs func([]Run) error
	runs = func(rs []Run) error {
		for i := range rs {
			r := &rs[i]
			if r.Note != nil {
				if err := runs(r.Note.LabelContent); err != nil {
					return err
				}
			}
			if o := r.colleafRunOrigin; o != nil {
				record, ok := pool[o.Ref]
				if !ok {
					return fmt.Errorf("native run references missing source record")
				}
				if (o.Source != nil || o.Fingerprint != "") && !reflect.DeepEqual(*o, record) {
					return fmt.Errorf("contradictory inline/native run source record")
				}
				hash, err := colleafRunFingerprint(*r)
				if err != nil {
					return err
				}
				r.Source = cloneSourceInfo(record.Source)
				r.colleafRunOrigin = nil
				used[o.Ref] = true
				if hash != record.Fingerprint {
					r.Source.Historical = true
					d.warn("colleaf_run_source_historical", "Original PDF source facts are retained as historical after current run content or presentation changed")
				}
			}
		}
		return nil
	}
	var walk func([]Block) error
	walk = func(bs []Block) error {
		for i := range bs {
			b := &bs[i]
			if err := runs(b.Runs); err != nil {
				return err
			}
			if b.Section != nil {
				if err := walk(b.Section.Blocks); err != nil {
					return err
				}
				for _, s := range sectionStories(b.Section) {
					if s != nil {
						if err := walk(s.Blocks); err != nil {
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
		n := &d.Notes[i]

		if err := walk(n.Blocks); err != nil {
			return err
		}
		if err := runs(n.BodyLabelContent); err != nil {
			return err
		}
	}
	archive := map[string]bool{}
	for _, s := range d.UnboundSources {
		archive[string(sfAttrs(s))] = true
	}
	refs := make([]string, 0, len(pool))
	for ref := range pool {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	for _, ref := range refs {
		if !used[ref] {
			s := pool[ref].Source
			key := string(sfAttrs(s))
			if !archive[key] {
				d.UnboundSources = append(d.UnboundSources, *cloneSourceInfo(s))
				archive[key] = true
			}
			d.warn("colleaf_run_source_unbound", "Original PDF source facts are retained unbound after their native run binding disappeared")
		}
	}
	return nil
}
