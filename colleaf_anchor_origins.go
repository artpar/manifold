package manifold

import (
	"fmt"
	"reflect"
	"sort"
)

const colleafAnchorOriginProfile = "manydoc.anchor-origin"

func colleafAnchorFingerprint(paragraphHash, name string) (string, error) {
	return colleafSourceFreeHash(struct {
		Paragraph string
		Anchor    string
	}{paragraphHash, name})
}
func prepareColleafAnchorOrigins(b Block) (Block, error) {
	hash := ""
	var err error
	copied := false
	for i, r := range b.Runs {
		if r.Anchor == "" || r.Source == nil || r.colleafAnchorOrigin != nil {
			continue
		}
		if !copied {
			b.Runs = append([]Run(nil), b.Runs...)
			copied = true
		}
		if hash == "" {
			hash, err = paragraphFingerprint(b)
			if err != nil {
				return b, err
			}
		}
		fingerprint, err := colleafAnchorFingerprint(hash, r.Anchor)
		if err != nil {
			return b, err
		}
		origin, err := newColleafSourceOrigin(r.Source, fingerprint, colleafAnchorOriginProfile)
		if err != nil {
			return b, err
		}
		b.Runs[i].colleafAnchorOrigin = origin
	}
	return b, nil
}
func visitColleafAnchorOrigins(snapshot *colleafNoteSnapshot, update bool, visit func(*colleafOrigin) error) error {
	body := func(b *colleafBody) error {
		for i := range b.CustomRanges {
			a := &b.CustomRanges[i]
			if a.Type != 3 {
				continue
			}
			p, err := readColleafBookmark(*a)
			if err != nil {
				return err
			}
			if p.Origin == nil {
				continue
			}
			if err = visit(p.Origin); err != nil {
				return err
			}
			if update {
				a.Properties = sfAttrs(p)
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
func collectColleafAnchorOrigins(out *colleafNoteSnapshot, e **colleafDocumentEvidence) error {
	pool := map[string]colleafOrigin{}
	err := visitColleafAnchorOrigins(out, true, func(o *colleafOrigin) error {
		if o.Source == nil {
			return fmt.Errorf("native output anchor origin lacks source")
		}
		if old, ok := pool[o.Ref]; ok && !reflect.DeepEqual(old, *o) {
			return fmt.Errorf("conflicting anchor source reference")
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
		(*e).AnchorOrigins = append((*e).AnchorOrigins, pool[ref])
	}
	return nil
}
func restoreColleafAnchorOrigins(d *Document, e *colleafDocumentEvidence, input colleafNoteSnapshot) error {
	pool := map[string]colleafOrigin{}
	if e != nil {
		for _, o := range e.AnchorOrigins {
			if err := validateColleafOriginProfile(o, d.Pages, colleafAnchorOriginProfile); err != nil {
				return err
			}
			if _, ok := pool[o.Ref]; ok {
				return fmt.Errorf("duplicate native anchor source reference")
			}
			pool[o.Ref] = o
		}
	}
	lookup := func(o *colleafOrigin) (colleafOrigin, error) {
		record, ok := pool[o.Ref]
		if !ok {
			return record, fmt.Errorf("native anchor references missing source record")
		}
		if (o.Source != nil || o.Fingerprint != "") && !reflect.DeepEqual(*o, record) {
			return record, fmt.Errorf("contradictory native anchor source reference")
		}
		return record, nil
	}
	if err := visitColleafAnchorOrigins(&input, false, func(o *colleafOrigin) error { _, err := lookup(o); return err }); err != nil {
		return err
	}
	used := map[string]bool{}
	var walk func([]Block) error
	walk = func(bs []Block) error {
		for i := range bs {
			b := &bs[i]
			hash := ""
			for j := range b.Runs {
				r := &b.Runs[j]
				o := r.colleafAnchorOrigin
				if o == nil {
					continue
				}
				if r.Anchor == "" {
					return fmt.Errorf("anchor source marker lacks current inline point")
				}
				record, err := lookup(o)
				if err != nil {
					return err
				}
				if hash == "" {
					hash, err = paragraphFingerprint(*b)
					if err != nil {
						return err
					}
				}
				current, err := colleafAnchorFingerprint(hash, r.Anchor)
				if err != nil {
					return err
				}
				r.Source = cloneSourceInfo(record.Source)
				r.colleafAnchorOrigin = nil
				used[o.Ref] = true
				if current != record.Fingerprint {
					r.Source.Historical = true
					d.warn("colleaf_anchor_source_historical", "Original anchor source facts are retained as historical after current point identity or paragraph context changed")
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
		d.warn("colleaf_anchor_source_unbound", "Original anchor source facts are retained unbound after their native point ownership disappeared")
	}
	return nil
}
