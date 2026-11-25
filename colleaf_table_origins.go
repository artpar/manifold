package manifold

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

const colleafTableOriginProfile = "manydoc.table-origin"
const colleafCellOriginProfile = "manydoc.cell-origin"

func colleafTableFingerprint(b Block) (string, error) {
	nodes, err := encodeSFBlocks([]Block{b})
	if err != nil {
		return "", err
	}
	return colleafSourceFreeHash(nodes)
}
func colleafCellFingerprint(c Cell) (string, error) {
	nodes, err := encodeSFBlocks([]Block{{Kind: Table, Rows: []Row{{Cells: []Cell{c}}}}})
	if err != nil {
		return "", err
	}
	return colleafSourceFreeHash(nodes[0].Content[0].Content[0])
}

// Visit every descriptor, including covered cells, independently of current SF
// ownership. Validation does not mutate the caller's native snapshot.
func visitColleafTableOrigins(snapshot *colleafNoteSnapshot, update bool, visit func(*colleafOrigin) error) error {
	marker := func(props colleafStyleMap, profile string) error {
		o, err := readColleafOriginProfile(props["manydocOrigin"], profile)
		if err != nil {
			return err
		}
		if o == nil {
			return nil
		}
		if err = visit(o); err != nil {
			return err
		}
		if update {
			props["manydocOrigin"] = sfAttrs(o)
		}
		return nil
	}
	tables := func(items map[string]json.RawMessage) error {
		for id, raw := range items {
			props, err := colleafStyleObject(raw)
			if err != nil {
				return err
			}
			if err = marker(props, colleafTableOriginProfile); err != nil {
				return err
			}
			var rows []json.RawMessage
			if err = json.Unmarshal(props["tableRows"], &rows); err != nil {
				return err
			}
			for i, row := range rows {
				rp, err := colleafStyleObject(row)
				if err != nil {
					return err
				}
				var cells []json.RawMessage
				if err = json.Unmarshal(rp["tableCells"], &cells); err != nil {
					return err
				}
				for j, cell := range cells {
					cp, err := colleafStyleObject(cell)
					if err != nil {
						return err
					}
					if err = marker(cp, colleafCellOriginProfile); err != nil {
						return err
					}
					if update {
						cells[j] = sfAttrs(cp)
					}
				}
				if update {
					rp["tableCells"] = sfAttrs(cells)
					rows[i] = sfAttrs(rp)
				}
			}
			if update {
				props["tableRows"] = sfAttrs(rows)
				items[id] = sfAttrs(props)
			}
		}
		return nil
	}
	if err := tables(snapshot.Tables); err != nil {
		return err
	}
	for _, stories := range []map[string]colleafRunningStory{snapshot.Headers, snapshot.Footers} {
		for _, story := range stories {
			if err := tables(story.Tables); err != nil {
				return err
			}
		}
	}
	for _, note := range snapshot.Notes {
		if err := tables(note.Tables); err != nil {
			return err
		}
	}
	return nil
}
func collectColleafTableOrigins(out *colleafNoteSnapshot, e **colleafDocumentEvidence) error {
	pool := map[string]colleafOrigin{}
	err := visitColleafTableOrigins(out, true, func(o *colleafOrigin) error {
		if o.Source == nil {
			return fmt.Errorf("native output table/cell origin lacks source")
		}
		if old, ok := pool[o.Ref]; ok && !reflect.DeepEqual(old, *o) {
			return fmt.Errorf("conflicting table/cell origin")
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
		(*e).TableOrigins = append((*e).TableOrigins, pool[ref])
	}
	return nil
}
func validateColleafTableOrigin(o colleafOrigin, pages []PDFPage) error {
	if o.Profile != colleafTableOriginProfile && o.Profile != colleafCellOriginProfile {
		return fmt.Errorf("invalid native table/cell source profile")
	}
	return validateColleafOriginProfile(o, pages, o.Profile)
}
func restoreColleafTableOrigins(d *Document, e *colleafDocumentEvidence, input colleafNoteSnapshot) error {
	pool := map[string]colleafOrigin{}
	if e != nil {
		for _, o := range e.TableOrigins {
			if err := validateColleafTableOrigin(o, d.Pages); err != nil {
				return err
			}
			if _, ok := pool[o.Ref]; ok {
				return fmt.Errorf("duplicate table/cell source reference")
			}
			pool[o.Ref] = o
		}
	}
	lookup := func(o *colleafOrigin) (colleafOrigin, error) {
		record, ok := pool[o.Ref]
		if !ok || record.Profile != o.Profile {
			return record, fmt.Errorf("native table/cell references missing source record")
		}
		if (o.Source != nil || o.Fingerprint != "") && !reflect.DeepEqual(*o, record) {
			return record, fmt.Errorf("contradictory native table/cell source record")
		}
		return record, nil
	}
	if err := visitColleafTableOrigins(&input, false, func(o *colleafOrigin) error { _, err := lookup(o); return err }); err != nil {
		return err
	}
	used := map[string]bool{}
	restore := func(o *colleafOrigin, hash string) (*SourceInfo, error) {
		record, err := lookup(o)
		if err != nil {
			return nil, err
		}
		used[o.Ref] = true
		source := cloneSourceInfo(record.Source)
		if hash != record.Fingerprint {
			source.Historical = true
			d.warn("colleaf_table_source_historical", "Original table or cell source facts are retained as historical after current content, grid or presentation changed")
		}
		return source, nil
	}
	var walk func([]Block) error
	walk = func(bs []Block) error {
		for i := range bs {
			b := &bs[i]
			if b.colleafTableOrigin != nil {
				hash, err := colleafTableFingerprint(*b)
				if err != nil {
					return err
				}
				b.Source, err = restore(b.colleafTableOrigin, hash)
				if err != nil {
					return err
				}
				b.colleafTableOrigin = nil
			}
			for ri := range b.Rows {
				for ci := range b.Rows[ri].Cells {
					c := &b.Rows[ri].Cells[ci]
					if c.colleafOrigin != nil {
						hash, err := colleafCellFingerprint(*c)
						if err != nil {
							return err
						}
						c.Source, err = restore(c.colleafOrigin, hash)
						if err != nil {
							return err
						}
						c.colleafOrigin = nil
					}
					if err := walk(c.Blocks); err != nil {
						return err
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
		d.warn("colleaf_table_source_unbound", "Original table or cell source facts are retained unbound after their native ownership disappeared")
	}
	return nil
}
