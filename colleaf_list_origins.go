package manydoc

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

const colleafListOriginProfile = "manydoc.list-origin"
const colleafItemOriginProfile = "manydoc.item-origin"
const colleafMarkerOriginProfile = "manydoc.marker-origin"

func colleafMarkerFingerprint(m *ListMarker) (string, error) {
	nodes, err := encodeSFBlocks(m.Blocks)
	if err != nil {
		return "", err
	}
	return colleafSourceFreeHash(nodes)
}

func colleafListFingerprint(b Block) (string, error) {
	nodes, err := encodeSFBlocks([]Block{b})
	if err != nil {
		return "", err
	}
	return colleafSourceFreeHash(nodes)
}
func colleafItemFingerprint(b Block, i int) (string, error) {
	if i < 0 || i >= len(b.Items) {
		return "", fmt.Errorf("invalid native item fingerprint scope")
	}
	fragment := b
	fragment.Source = nil
	item := b.Items[i]
	item.Source = nil
	fragment.Items = []ListItem{item}
	nodes, err := encodeSFBlocks([]Block{fragment})
	if err != nil {
		return "", err
	}
	return colleafSourceFreeHash(struct {
		Kind  string          `json:"kind"`
		Attrs json.RawMessage `json:"attrs,omitempty"`
		Index int             `json:"index"`
		Item  StructuredNode  `json:"item"`
	}{nodes[0].Type, nodes[0].Attrs, i, nodes[0].Content[0]})
}
func visitColleafListOrigins(snapshot *colleafNoteSnapshot, update bool, visit func(*colleafOrigin) error) error {
	body := func(b *colleafBody) error {
		for i := range b.Paragraphs {
			p := &b.Paragraphs[i]
			props, err := colleafStyleObject(p.Style)
			if err != nil {
				return err
			}
			changed := false
			for key, profile := range map[string]string{"manydocListOrigin": colleafListOriginProfile, "manydocItemOrigin": colleafItemOriginProfile, "manydocMarkerOrigin": colleafMarkerOriginProfile} {
				o, err := readColleafOriginProfile(props[key], profile)
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
					props[key] = sfAttrs(o)
					changed = true
				}
			}
			if changed {
				p.Style = sfAttrs(props)
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
func collectColleafListOrigins(out *colleafNoteSnapshot, e **colleafDocumentEvidence) error {
	pool := map[string]colleafOrigin{}
	err := visitColleafListOrigins(out, true, func(o *colleafOrigin) error {
		if o.Source == nil {
			return fmt.Errorf("native output list/item source lacks facts")
		}
		if old, ok := pool[o.Ref]; ok && !reflect.DeepEqual(old, *o) {
			return fmt.Errorf("conflicting native list/item source")
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
		(*e).ListOrigins = append((*e).ListOrigins, pool[ref])
	}
	return nil
}
func validateColleafListOrigin(o colleafOrigin, pages []PDFPage) error {
	if o.Profile != colleafListOriginProfile && o.Profile != colleafItemOriginProfile && o.Profile != colleafMarkerOriginProfile {
		return fmt.Errorf("invalid native list/item source profile")
	}
	return validateColleafOriginProfile(o, pages, o.Profile)
}
func restoreColleafListOrigins(d *Document, e *colleafDocumentEvidence, input colleafNoteSnapshot) error {
	pool := map[string]colleafOrigin{}
	if e != nil {
		for _, o := range e.ListOrigins {
			if err := validateColleafListOrigin(o, d.Pages); err != nil {
				return err
			}
			if _, ok := pool[o.Ref]; ok {
				return fmt.Errorf("duplicate native list/item source reference")
			}
			pool[o.Ref] = o
		}
	}
	lookup := func(o *colleafOrigin) (colleafOrigin, error) {
		record, ok := pool[o.Ref]
		if !ok || record.Profile != o.Profile {
			return record, fmt.Errorf("native list/item references missing source record")
		}
		if (o.Source != nil || o.Fingerprint != "") && !reflect.DeepEqual(*o, record) {
			return record, fmt.Errorf("contradictory native list/item source reference")
		}
		return record, nil
	}
	if err := visitColleafListOrigins(&input, false, func(o *colleafOrigin) error { _, err := lookup(o); return err }); err != nil {
		return err
	}
	used := map[string]bool{}
	restore := func(o *colleafOrigin, hash string) (*SourceInfo, error) {
		record, err := lookup(o)
		if err != nil {
			return nil, err
		}
		used[o.Ref] = true
		s := cloneSourceInfo(record.Source)
		if hash != record.Fingerprint {
			s.Historical = true
			d.warn("colleaf_list_source_historical", "Original list or item source facts are retained as historical after current content, numbering or ownership changed")
		}
		return s, nil
	}
	var walk func([]Block) error
	walk = func(bs []Block) error {
		for i := range bs {
			b := &bs[i]
			if b.Kind == ListBlock {
				for j := range b.Items {
					item := &b.Items[j]
					if len(item.Blocks) == 0 {
						continue
					}
					p := &item.Blocks[0]
					if p.Kind != Paragraph && p.Kind != Heading {
						continue
					}
					if j == 0 && p.colleafListOrigin != nil {
						hash, err := colleafListFingerprint(*b)
						if err != nil {
							return err
						}
						b.Source, err = restore(p.colleafListOrigin, hash)
						if err != nil {
							return err
						}
					}
					if p.colleafMarkerOrigin != nil && item.Marker != nil {
						hash, err := colleafMarkerFingerprint(item.Marker)
						if err != nil {
							return err
						}
						item.Marker.Source, err = restore(p.colleafMarkerOrigin, hash)
						if err != nil {
							return err
						}
					}
					if p.colleafItemOrigin != nil {
						hash, err := colleafItemFingerprint(*b, j)
						if err != nil {
							return err
						}
						item.Source, err = restore(p.colleafItemOrigin, hash)
						if err != nil {
							return err
						}
					}
				}
			}
			b.colleafListOrigin = nil
			b.colleafItemOrigin = nil
			b.colleafMarkerOrigin = nil
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
		d.warn("colleaf_list_source_unbound", "Original list or item source facts are retained unbound after their native list ownership disappeared")
	}
	return nil
}
