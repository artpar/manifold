package manydoc

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

const colleafImageOriginProfile = "manydoc.image-origin"

func colleafImageFingerprint(b Block) (string, error) {
	nodes, err := encodeSFBlocks([]Block{b})
	if err != nil {
		return "", err
	}
	return colleafSourceFreeHash(nodes)
}
func visitColleafImageOrigins(snapshot *colleafNoteSnapshot, update bool, visit func(*colleafOrigin) error) error {
	drawings := func(items map[string]json.RawMessage) error {
		for id, raw := range items {
			props, err := colleafStyleObject(raw)
			if err != nil {
				return err
			}
			o, err := readColleafOriginProfile(props["manydocOrigin"], colleafImageOriginProfile)
			if err != nil {
				return err
			}
			if o == nil {
				continue
			}
			if err = visit(o); err != nil {
				return err
			}
			if update {
				props["manydocOrigin"] = sfAttrs(o)
				items[id] = sfAttrs(props)
			}
		}
		return nil
	}
	if err := drawings(snapshot.Drawings); err != nil {
		return err
	}
	for _, stories := range []map[string]colleafRunningStory{snapshot.Headers, snapshot.Footers} {
		for _, s := range stories {
			if err := drawings(s.Drawings); err != nil {
				return err
			}
		}
	}
	for _, n := range snapshot.Notes {
		if err := drawings(n.Drawings); err != nil {
			return err
		}
	}
	return nil
}
func collectColleafImageOrigins(out *colleafNoteSnapshot, e **colleafDocumentEvidence) error {
	pool := map[string]colleafOrigin{}
	err := visitColleafImageOrigins(out, true, func(o *colleafOrigin) error {
		if o.Source == nil {
			return fmt.Errorf("native output image origin lacks source")
		}
		if old, ok := pool[o.Ref]; ok && !reflect.DeepEqual(old, *o) {
			return fmt.Errorf("conflicting image source reference")
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
		(*e).ImageOrigins = append((*e).ImageOrigins, pool[ref])
	}
	return nil
}
func restoreColleafImageOrigins(d *Document, e *colleafDocumentEvidence, input colleafNoteSnapshot) error {
	pool := map[string]colleafOrigin{}
	if e != nil {
		for _, o := range e.ImageOrigins {
			if err := validateColleafOriginProfile(o, d.Pages, colleafImageOriginProfile); err != nil {
				return err
			}
			if _, ok := pool[o.Ref]; ok {
				return fmt.Errorf("duplicate native image source reference")
			}
			pool[o.Ref] = o
		}
	}
	lookup := func(o *colleafOrigin) (colleafOrigin, error) {
		record, ok := pool[o.Ref]
		if !ok {
			return record, fmt.Errorf("native image references missing source record")
		}
		if (o.Source != nil || o.Fingerprint != "") && !reflect.DeepEqual(*o, record) {
			return record, fmt.Errorf("contradictory native image source record")
		}
		return record, nil
	}
	if err := visitColleafImageOrigins(&input, false, func(o *colleafOrigin) error { _, err := lookup(o); return err }); err != nil {
		return err
	}
	used := map[string]bool{}
	var walk func([]Block) error
	walk = func(bs []Block) error {
		for i := range bs {
			b := &bs[i]
			if o := b.colleafImageOrigin; o != nil {
				record, err := lookup(o)
				if err != nil {
					return err
				}
				hash, err := colleafImageFingerprint(*b)
				if err != nil {
					return err
				}
				b.Source = cloneSourceInfo(record.Source)
				b.colleafImageOrigin = nil
				used[o.Ref] = true
				if hash != record.Fingerprint {
					b.Source.Historical = true
					d.warn("colleaf_image_source_historical", "Original image source facts are retained as historical after current asset, description or presentation changed")
				}
			}
			for j := range b.Rows {
				for k := range b.Rows[j].Cells {
					if err := walk(b.Rows[j].Cells[k].Blocks); err != nil {
						return err
					}
				}
			}
			for j := range b.Items {
				if err := walk(b.Items[j].Blocks); err != nil {
					return err
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
		d.warn("colleaf_image_source_unbound", "Original image source facts are retained unbound after their native ownership disappeared")
	}
	return nil
}
