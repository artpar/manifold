package manydoc

import (
	"fmt"
	"reflect"
	"sort"
)

const colleafFlowOriginProfile = "manydoc.flow-origin"

func colleafFlowFingerprint(b Block) (string, error) {
	nodes, err := encodeSFBlocks([]Block{b})
	if err != nil {
		return "", err
	}
	return colleafSourceFreeHash(nodes)
}
func newColleafFlowOrigin(b Block) (*colleafOrigin, error) {
	if b.Source == nil {
		return nil, nil
	}
	hash, err := colleafFlowFingerprint(b)
	if err != nil {
		return nil, err
	}
	return newColleafSourceOrigin(b.Source, hash, colleafFlowOriginProfile)
}
func visitColleafFlowOrigins(snapshot *colleafNoteSnapshot, update bool, visit func(*colleafOrigin) error) error {
	body := func(b *colleafBody) error {
		for i := range b.SectionBreaks {
			s := &b.SectionBreaks[i]
			o, err := readColleafOriginProfile(s.Extra["manydocFlowOrigin"], colleafFlowOriginProfile)
			if err != nil {
				return err
			}
			if o != nil {
				if err = visit(o); err != nil {
					return err
				}
				if update {
					s.Extra["manydocFlowOrigin"] = sfAttrs(o)
				}
			}
		}
		for i := range b.Paragraphs {
			p := &b.Paragraphs[i]
			props, err := colleafStyleObject(p.Style)
			if err != nil {
				return err
			}
			o, err := readColleafOriginProfile(props["manydocFlowOrigin"], colleafFlowOriginProfile)
			if err != nil {
				return err
			}
			if o != nil {
				if err = visit(o); err != nil {
					return err
				}
				if update {
					props["manydocFlowOrigin"] = sfAttrs(o)
					p.Style = sfAttrs(props)
				}
			}
		}
		for i := range b.CustomRanges {
			a := &b.CustomRanges[i]
			if a.Type != 3 {
				continue
			}
			p, err := readColleafBookmark(*a)
			if err != nil {
				return err
			}
			if p.FlowOrigin != nil {
				if err = visit(p.FlowOrigin); err != nil {
					return err
				}
				if update {
					a.Properties = sfAttrs(p)
				}
			}
		}
		return nil
	}
	if err := body(&snapshot.Body); err != nil {
		return err
	}
	for _, stories := range []map[string]colleafRunningStory{snapshot.Headers, snapshot.Footers} {
		for id, s := range stories {
			if err := body(&s.Body); err != nil {
				return err
			}
			if update {
				stories[id] = s
			}
		}
	}
	for id, n := range snapshot.Notes {
		if err := body(&n.Body); err != nil {
			return err
		}
		if update {
			snapshot.Notes[id] = n
		}
	}
	return nil
}
func collectColleafFlowOrigins(out *colleafNoteSnapshot, e **colleafDocumentEvidence) error {
	pool := map[string]colleafOrigin{}
	err := visitColleafFlowOrigins(out, true, func(o *colleafOrigin) error {
		if o.Source == nil {
			return fmt.Errorf("native output flow origin lacks source")
		}
		if old, ok := pool[o.Ref]; ok && !reflect.DeepEqual(old, *o) {
			return fmt.Errorf("conflicting flow source reference")
		}
		pool[o.Ref] = *o
		o.Source = nil
		o.Fingerprint = ""
		return nil
	})
	if err != nil {
		return err
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
		(*e).FlowOrigins = append((*e).FlowOrigins, pool[ref])
	}
	return nil
}
func restoreColleafFlowOrigins(d *Document, e *colleafDocumentEvidence, input colleafNoteSnapshot) error {
	pool := map[string]colleafOrigin{}
	if e != nil {
		for _, o := range e.FlowOrigins {
			if err := validateColleafOriginProfile(o, d.Pages, colleafFlowOriginProfile); err != nil {
				return err
			}
			if _, ok := pool[o.Ref]; ok {
				return fmt.Errorf("duplicate native flow source reference")
			}
			pool[o.Ref] = o
		}
	}
	lookup := func(o *colleafOrigin) (colleafOrigin, error) {
		record, ok := pool[o.Ref]
		if !ok {
			return record, fmt.Errorf("native flow references missing source record")
		}
		if (o.Source != nil || o.Fingerprint != "") && !reflect.DeepEqual(*o, record) {
			return record, fmt.Errorf("contradictory native flow source reference")
		}
		return record, nil
	}
	if err := visitColleafFlowOrigins(&input, false, func(o *colleafOrigin) error { _, err := lookup(o); return err }); err != nil {
		return err
	}
	used := map[string]bool{}
	var walk func([]Block) error
	walk = func(bs []Block) error {
		for i := range bs {
			b := &bs[i]
			if o := b.colleafFlowOrigin; o != nil {
				record, err := lookup(o)
				if err != nil {
					return err
				}
				hash, err := colleafFlowFingerprint(*b)
				if err != nil {
					return err
				}
				b.Source = cloneSourceInfo(record.Source)
				b.colleafFlowOrigin = nil
				used[o.Ref] = true
				if hash != record.Fingerprint {
					b.Source.Historical = true
					d.warn("colleaf_flow_source_historical", "Original section, break or point source facts are retained as historical after current semantics changed")
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
	archived := map[string]bool{}
	for _, s := range d.UnboundSources {
		archived[string(sfAttrs(s))] = true
	}
	refs := make([]string, 0, len(pool))
	for ref := range pool {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	for _, ref := range refs {
		if used[ref] {
			continue
		}
		source := pool[ref].Source
		key := string(sfAttrs(source))
		if !archived[key] {
			d.UnboundSources = append(d.UnboundSources, *cloneSourceInfo(source))
			archived[key] = true
		}
		d.warn("colleaf_flow_source_unbound", "Original section, break or point source facts are retained unbound after native ownership disappeared")
	}
	return nil
}
