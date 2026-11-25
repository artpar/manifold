package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"sort"
)

const vdocsAnchorOriginProfile = "manydoc.anchor-origin"

func vdocsAnchorFingerprint(paragraphHash, name string) (string, error) {
	return vdocsSourceFreeHash(struct {
		Paragraph string
		Anchor    string
	}{paragraphHash, name})
}
func prepareVDOCSAnchorOrigins(b Block) (Block, error) {
	hash := ""
	var err error
	copied := false
	for i, r := range b.Runs {
		if r.Anchor == "" || r.Source == nil || r.vdocsAnchorOrigin != nil {
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
		fingerprint, err := vdocsAnchorFingerprint(hash, r.Anchor)
		if err != nil {
			return b, err
		}
		origin, err := newVDOCSSourceOrigin(r.Source, fingerprint, vdocsAnchorOriginProfile)
		if err != nil {
			return b, err
		}
		b.Runs[i].vdocsAnchorOrigin = origin
	}
	return b, nil
}
func visitVDOCSAnchorOrigins(snapshot *vdocsNoteSnapshot, update bool, visit func(*vdocsOrigin) error) error {
	body := func(b *vdocsBody) error {
		for i := range b.CustomRanges {
			a := &b.CustomRanges[i]
			if a.Type != 3 {
				continue
			}
			p, err := readVDOCSBookmark(*a)
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
	for _, stories := range []map[string]vdocsRunningStory{snapshot.Headers, snapshot.Footers} {
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
func collectVDOCSAnchorOrigins(out *vdocsNoteSnapshot, e **vdocsDocumentEvidence) error {
	pool := map[string]vdocsOrigin{}
	err := visitVDOCSAnchorOrigins(out, true, func(o *vdocsOrigin) error {
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
		*e = &vdocsDocumentEvidence{Version: 1}
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
func restoreVDOCSAnchorOrigins(d *Document, e *vdocsDocumentEvidence, input vdocsNoteSnapshot) error {
	pool := map[string]vdocsOrigin{}
	if e != nil {
		for _, o := range e.AnchorOrigins {
			if err := validateVDOCSOriginProfile(o, d.Pages, vdocsAnchorOriginProfile); err != nil {
				return err
			}
			if _, ok := pool[o.Ref]; ok {
				return fmt.Errorf("duplicate native anchor source reference")
			}
			pool[o.Ref] = o
		}
	}
	lookup := func(o *vdocsOrigin) (vdocsOrigin, error) {
		record, ok := pool[o.Ref]
		if !ok {
			return record, fmt.Errorf("native anchor references missing source record")
		}
		if (o.Source != nil || o.Fingerprint != "") && !reflect.DeepEqual(*o, record) {
			return record, fmt.Errorf("contradictory native anchor source reference")
		}
		return record, nil
	}
	if err := visitVDOCSAnchorOrigins(&input, false, func(o *vdocsOrigin) error { _, err := lookup(o); return err }); err != nil {
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
				o := r.vdocsAnchorOrigin
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
				current, err := vdocsAnchorFingerprint(hash, r.Anchor)
				if err != nil {
					return err
				}
				r.Source = cloneSourceInfo(record.Source)
				r.vdocsAnchorOrigin = nil
				used[o.Ref] = true
				if current != record.Fingerprint {
					r.Source.Historical = true
					d.warn("vdocs_anchor_source_historical", "Original anchor source facts are retained as historical after current point identity or paragraph context changed")
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
	for _, bs := range [][]Block{d.Blocks, d.Headers,

		// Avoid accidental range-ID aliases of real or explicitly unresolved SF names.
		// The native click handler tests both bookmarkId and rangeId in the same lookup.

		d.Footers} {
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
		d.warn("vdocs_anchor_source_unbound", "Original anchor source facts are retained unbound after their native point ownership disappeared")
	}
	return nil
}

const vdocsFlowOriginProfile = "manydoc.flow-origin"

func vdocsFlowFingerprint(b Block) (string, error) {
	nodes, err := encodeSFBlocks([]Block{b})
	if err != nil {
		return "", err
	}
	return vdocsSourceFreeHash(nodes)
}
func newVDOCSFlowOrigin(b Block) (*vdocsOrigin, error) {
	if b.Source == nil {
		return nil, nil
	}
	hash, err := vdocsFlowFingerprint(b)
	if err != nil {
		return nil, err
	}
	return newVDOCSSourceOrigin(b.Source, hash, vdocsFlowOriginProfile)
}
func visitVDOCSFlowOrigins(snapshot *vdocsNoteSnapshot, update bool, visit func(*vdocsOrigin) error) error {
	body := func(b *vdocsBody) error {
		for i := range b.SectionBreaks {
			s := &b.SectionBreaks[i]
			o, err := readVDOCSOriginProfile(s.Extra["manydocFlowOrigin"], vdocsFlowOriginProfile)
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
			props, err := vdocsStyleObject(p.Style)
			if err != nil {
				return err
			}
			o, err := readVDOCSOriginProfile(props["manydocFlowOrigin"], vdocsFlowOriginProfile)
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
			p, err := readVDOCSBookmark(*a)
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
	for _, stories := range []map[string]vdocsRunningStory{snapshot.Headers, snapshot.Footers} {
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
func collectVDOCSFlowOrigins(out *vdocsNoteSnapshot, e **vdocsDocumentEvidence) error {
	pool := map[string]vdocsOrigin{}
	err := visitVDOCSFlowOrigins(out, true, func(o *vdocsOrigin) error {
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
		*e = &vdocsDocumentEvidence{Version: 1}
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
func restoreVDOCSFlowOrigins(d *Document, e *vdocsDocumentEvidence, input vdocsNoteSnapshot) error {
	pool := map[string]vdocsOrigin{}
	if e != nil {
		for _, o := range e.FlowOrigins {
			if err := validateVDOCSOriginProfile(o, d.Pages, vdocsFlowOriginProfile); err != nil {
				return err
			}
			if _, ok := pool[o.Ref]; ok {
				return fmt.Errorf("duplicate native flow source reference")
			}
			pool[o.Ref] = o
		}
	}
	lookup := func(o *vdocsOrigin) (vdocsOrigin, error) {
		record, ok := pool[o.Ref]
		if !ok {
			return record, fmt.Errorf("native flow references missing source record")
		}
		if (o.Source != nil || o.Fingerprint != "") && !reflect.DeepEqual(*o, record) {
			return record, fmt.Errorf("contradictory native flow source reference")
		}
		return record, nil
	}
	if err := visitVDOCSFlowOrigins(&input, false, func(o *vdocsOrigin) error { _, err := lookup(o); return err }); err != nil {
		return err
	}
	used := map[string]bool{}
	var walk func([]Block) error
	walk = func(bs []Block) error {
		for i := range bs {
			b := &bs[i]
			if o := b.vdocsFlowOrigin; o != nil {
				record, err := lookup(o)
				if err != nil {
					return err
				}
				hash, err := vdocsFlowFingerprint(*b)
				if err != nil {
					return err
				}
				b.Source = cloneSourceInfo(record.Source)
				b.vdocsFlowOrigin = nil
				used[o.Ref] = true
				if hash != record.Fingerprint {
					b.Source.Historical = true
					d.warn("vdocs_flow_source_historical", "Original section, break or point source facts are retained as historical after current semantics changed")
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
		d.warn("vdocs_flow_source_unbound", "Original section, break or point source facts are retained unbound after native ownership disappeared")
	}
	return nil
}

const vdocsImageOriginProfile = "manydoc.image-origin"

func vdocsImageFingerprint(b Block) (string, error) {
	nodes, err := encodeSFBlocks([]Block{b})
	if err != nil {
		return "", err
	}
	return vdocsSourceFreeHash(nodes)
}
func visitVDOCSImageOrigins(snapshot *vdocsNoteSnapshot, update bool, visit func(*vdocsOrigin) error) error {
	drawings := func(items map[string]json.RawMessage) error {
		for id, raw := range items {
			props, err := vdocsStyleObject(raw)
			if err != nil {
				return err
			}
			o, err := readVDOCSOriginProfile(props["manydocOrigin"], vdocsImageOriginProfile)
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
	for _, stories := range []map[string]vdocsRunningStory{snapshot.Headers, snapshot.Footers} {
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
func collectVDOCSImageOrigins(out *vdocsNoteSnapshot, e **vdocsDocumentEvidence) error {
	pool := map[string]vdocsOrigin{}
	err := visitVDOCSImageOrigins(out, true, func(o *vdocsOrigin) error {
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
		*e = &vdocsDocumentEvidence{Version: 1}
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
func restoreVDOCSImageOrigins(d *Document, e *vdocsDocumentEvidence, input vdocsNoteSnapshot) error {
	pool := map[string]vdocsOrigin{}
	if e != nil {
		for _, o := range e.ImageOrigins {
			if err := validateVDOCSOriginProfile(o, d.Pages, vdocsImageOriginProfile); err != nil {
				return err
			}
			if _, ok := pool[o.Ref]; ok {
				return fmt.Errorf("duplicate native image source reference")
			}
			pool[o.Ref] = o
		}
	}
	lookup := func(o *vdocsOrigin) (vdocsOrigin, error) {
		record, ok := pool[o.Ref]
		if !ok {
			return record, fmt.Errorf("native image references missing source record")
		}
		if (o.Source != nil || o.Fingerprint != "") && !reflect.DeepEqual(*o, record) {
			return record, fmt.Errorf("contradictory native image source record")
		}
		return record, nil
	}
	if err := visitVDOCSImageOrigins(&input, false, func(o *vdocsOrigin) error { _, err := lookup(o); return err }); err != nil {
		return err
	}
	used := map[string]bool{}
	var walk func([]Block) error
	walk = func(bs []Block) error {
		for i := range bs {
			b := &bs[i]
			if o := b.vdocsImageOrigin; o != nil {
				record, err := lookup(o)
				if err != nil {
					return err
				}
				hash, err := vdocsImageFingerprint(*b)
				if err != nil {
					return err
				}
				b.Source = cloneSourceInfo(record.Source)
				b.vdocsImageOrigin = nil
				used[o.Ref] = true
				if hash != record.Fingerprint {
					b.Source.Historical = true
					d.warn("vdocs_image_source_historical", "Original image source facts are retained as historical after current asset, description or presentation changed")
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
		d.warn("vdocs_image_source_unbound", "Original image source facts are retained unbound after their native ownership disappeared")
	}
	return nil
}

const vdocsListOriginProfile = "manydoc.list-origin"
const vdocsItemOriginProfile = "manydoc.item-origin"
const vdocsMarkerOriginProfile = "manydoc.marker-origin"

func vdocsMarkerFingerprint(m *ListMarker) (string, error) {
	nodes, err := encodeSFBlocks(m.Blocks)
	if err != nil {
		return "", err
	}
	return vdocsSourceFreeHash(nodes)
}

func vdocsListFingerprint(b Block) (string, error) {
	nodes, err := encodeSFBlocks([]Block{b})
	if err != nil {
		return "", err
	}
	return vdocsSourceFreeHash(nodes)
}
func vdocsItemFingerprint(b Block, i int) (string, error) {
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
	return vdocsSourceFreeHash(struct {
		Kind  string          `json:"kind"`
		Attrs json.RawMessage `json:"attrs,omitempty"`
		Index int             `json:"index"`
		Item  StructuredNode  `json:"item"`
	}{nodes[0].Type, nodes[0].Attrs, i, nodes[0].Content[0]})
}
func visitVDOCSListOrigins(snapshot *vdocsNoteSnapshot, update bool, visit func(*vdocsOrigin) error) error {
	body := func(b *vdocsBody) error {
		for i := range b.Paragraphs {
			p := &b.Paragraphs[i]
			props, err := vdocsStyleObject(p.Style)
			if err != nil {
				return err
			}
			changed := false
			for key, profile := range map[string]string{"manydocListOrigin": vdocsListOriginProfile, "manydocItemOrigin": vdocsItemOriginProfile, "manydocMarkerOrigin": vdocsMarkerOriginProfile} {
				o, err := readVDOCSOriginProfile(props[key], profile)
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
	for _, stories := range []map[string]vdocsRunningStory{snapshot.Headers, snapshot.Footers} {
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
func collectVDOCSListOrigins(out *vdocsNoteSnapshot, e **vdocsDocumentEvidence) error {
	pool := map[string]vdocsOrigin{}
	err := visitVDOCSListOrigins(out, true, func(o *vdocsOrigin) error {
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
		*e = &vdocsDocumentEvidence{Version: 1}
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
func validateVDOCSListOrigin(o vdocsOrigin, pages []PDFPage) error {
	if o.Profile != vdocsListOriginProfile && o.Profile != vdocsItemOriginProfile && o.Profile != vdocsMarkerOriginProfile {
		return fmt.Errorf("invalid native list/item source profile")
	}
	return validateVDOCSOriginProfile(o, pages, o.Profile)
}
func restoreVDOCSListOrigins(d *Document, e *vdocsDocumentEvidence, input vdocsNoteSnapshot) error {
	pool := map[string]vdocsOrigin{}
	if e != nil {
		for _, o := range e.ListOrigins {
			if err := validateVDOCSListOrigin(o, d.Pages); err != nil {
				return err
			}
			if _, ok := pool[o.Ref]; ok {
				return fmt.Errorf("duplicate native list/item source reference")
			}
			pool[o.Ref] = o
		}
	}
	lookup := func(o *vdocsOrigin) (vdocsOrigin, error) {
		record, ok := pool[o.Ref]
		if !ok || record.Profile != o.Profile {
			return record, fmt.Errorf("native list/item references missing source record")
		}
		if (o.Source != nil || o.Fingerprint != "") && !reflect.DeepEqual(*o, record) {
			return record, fmt.Errorf("contradictory native list/item source reference")
		}
		return record, nil
	}
	if err := visitVDOCSListOrigins(&input, false, func(o *vdocsOrigin) error { _, err := lookup(o); return err }); err != nil {
		return err
	}
	used := map[string]bool{}
	restore := func(o *vdocsOrigin, hash string) (*SourceInfo, error) {
		record, err := lookup(o)
		if err != nil {
			return nil, err
		}
		used[o.Ref] = true
		s := cloneSourceInfo(record.Source)
		if hash != record.Fingerprint {
			s.Historical = true
			d.warn("vdocs_list_source_historical", "Original list or item source facts are retained as historical after current content, numbering or ownership changed")
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
					if j == 0 && p.vdocsListOrigin != nil {
						hash, err := vdocsListFingerprint(*b)
						if err != nil {
							return err
						}
						b.Source, err = restore(p.vdocsListOrigin, hash)
						if err != nil {
							return err
						}
					}
					if p.vdocsMarkerOrigin != nil && item.Marker != nil {
						hash, err := vdocsMarkerFingerprint(item.Marker)
						if err != nil {
							return err
						}
						item.Marker.Source, err = restore(p.vdocsMarkerOrigin, hash)
						if err != nil {
							return err
						}
					}
					if p.vdocsItemOrigin != nil {
						hash, err := vdocsItemFingerprint(*b, j)
						if err != nil {
							return err
						}
						item.Source, err = restore(p.vdocsItemOrigin, hash)
						if err != nil {
							return err
						}
					}
				}
			}
			b.vdocsListOrigin = nil
			b.vdocsItemOrigin = nil
			b.vdocsMarkerOrigin = nil
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

			// Validate that every source point survived rich-container interpretation. Recover
			// explicit missing targets from the final tree, rather than silently dropping or
			// inventing a destination. Heading identities use a separate scoped namespace.

		}
		d.warn("vdocs_list_source_unbound", "Original list or item source facts are retained unbound after their native list ownership disappeared")
	}
	return nil
}

type vdocsBookmarkHint struct {
	Version      int    `json:"version"`
	Role         string `json:"role"`
	ParagraphID  string `json:"paragraphId,omitempty"`
	OwnerID      string `json:"ownerId,omitempty"`
	PointScope   string `json:"pointScope,omitempty"`
	PointOrder   int    `json:"pointOrder,omitempty"`
	PointIndex   int    `json:"pointIndex,omitempty"`
	PointCarrier string `json:"pointCarrier,omitempty"`
}
type vdocsBookmarkProperties struct {
	FlowOrigin *vdocsOrigin       `json:"manydocFlowOrigin,omitempty"`
	Origin     *vdocsOrigin       `json:"manydocOrigin,omitempty"`
	BookmarkID string             `json:"bookmarkId,omitempty"`
	Hint       *vdocsBookmarkHint `json:"manydocNavigation,omitempty"`
}

func readVDOCSBookmark(a vdocsRangeRecord) (vdocsBookmarkProperties, error) {
	var p vdocsBookmarkProperties
	if len(a.Properties) > 0 {
		if err := validateTypedJSON(context.Background(), a.Properties, reflect.TypeFor[vdocsBookmarkProperties]()); err != nil {
			return p, err
		}
	}
	if len(a.Properties) > 0 {
		if err := json.Unmarshal(a.Properties, &p); err != nil {
			return p, err
		}
	}
	if p.FlowOrigin != nil {
		if _, err := readVDOCSOriginProfile(sfAttrs(p.FlowOrigin), vdocsFlowOriginProfile); err != nil {
			return p, err
		}
	}
	if p.Origin != nil {
		if _, err := readVDOCSOriginProfile(sfAttrs(p.Origin), vdocsAnchorOriginProfile); err != nil {
			return p, err
		}
	}
	if p.BookmarkID == "" {
		p.BookmarkID = a.ID
	}
	if err := destinationID(p.BookmarkID); err != nil {
		return p, err
	}
	if p.Hint != nil {
		fields, err := vdocsStyleObject(a.Properties)
		if err != nil {
			return p, err
		}
		if err = strictSFAttrs(fields["manydocNavigation"], p.Hint); err != nil {
			return p, err
		}
		validRole := p.Hint.Role == "run" || p.Hint.Role == "block" || p.Hint.Role == "table" || p.Hint.Role == "picture" || p.Hint.Role == "list" || p.Hint.Role == "break" || p.Hint.Role == "section" || p.Hint.Role == "point"
		if p.Hint.Version != 1 || !validRole {
			return p, fmt.Errorf("%w: native bookmark placement intent", ErrUnsupported)
		}
		structural := p.Hint.Role == "table" || p.Hint.Role == "picture" || p.Hint.Role == "list" || p.Hint.Role == "break" || p.Hint.Role == "section" || p.Hint.Role == "point"
		if structural && (p.Hint.OwnerID == "" || p.Hint.ParagraphID != "") || !structural && p.Hint.OwnerID != "" {
			return p, fmt.Errorf("invalid native bookmark owner binding")
		}
		if p.Hint.Role == "point" {
			if (p.Hint.PointScope != "section" && p.Hint.PointScope != "item") || p.Hint.PointOrder <= 0 || p.Hint.PointIndex < 0 {
				return p, fmt.Errorf("invalid standalone point binding")
			}
		} else if p.Hint.PointScope != "" || p.Hint.PointOrder != 0 || p.Hint.PointIndex != 0 || p.Hint.PointCarrier != "" {
			return p, fmt.Errorf("unexpected standalone point binding")
		}
	}
	return p, nil
}
func writeVDOCSBookmark(id string, offset int, pid, role string) vdocsRangeRecord {
	sum := sha256.Sum256([]byte(id))
	return vdocsRangeRecord{ID: fmt.Sprintf("manydoc-bookmark-%x", sum), Type: 3, StartIndex: offset, EndIndex: offset, Properties: sfAttrs(vdocsBookmarkProperties{BookmarkID: id, Hint: &vdocsBookmarkHint{Version: 1, Role: role, ParagraphID: pid}})}
}

type vdocsLinkProperties struct {
	URL      string `json:"url"`
	Bookmark string `json:"bookmarkId"`
	Heading  string `json:"headingId"`
	Mode     string `json:"textStyleMode"`
}

func vdocsNativeHeadingID(raw json.RawMessage) (string, error) {
	props, err := vdocsStyleObject(raw)
	if err != nil {
		return "", err
	}
	value, ok := props["headingId"]
	if !ok {
		return "", nil
	}
	var id *string
	if err = json.Unmarshal(value, &id); err != nil {
		return "", err
	}
	if id == nil {
		return "", nil
	}
	return *id, nil
}
func rebindVDOCSBookmark(a *vdocsRangeRecord, names map[string]string) error {
	if a.Type != 3 {
		return nil
	}
	p, err := readVDOCSBookmark(*a)
	if err != nil {
		return err
	}
	if p.Hint != nil {
		if name, ok := names[p.Hint.ParagraphID]; ok {
			p.Hint.ParagraphID = name
			a.Properties = sfAttrs(p)
		}
	}
	return nil
}
func vdocsParagraphStarts(index *vdocsBodyIndex) map[int]int {
	starts := map[int]int{}
	var walk func([]vdocsStreamNode)
	walk = func(nodes []vdocsStreamNode) {
		for _, n := range nodes {
			if n.kind == "paragraph" {
				starts[n.end-1] = n.start
			}
			walk(n.children)
		}
	}
	walk(index.sections)
	return starts
}
func vdocsHeadingID(scope, id string) string {
	if scope == "" {
		scope = "main"
	}
	sum := sha256.Sum256([]byte(scope + "\x00" + id))
	return fmt.Sprintf("univer-heading-%x", sum)
}
func vdocsInternalHref(href, id string) bool {
	u, err := url.Parse(href)
	return err == nil && u.Scheme == "" && u.Host == "" && u.Path == "" && u.RawQuery == "" && u.Opaque == "" && u.Fragment == id
}
func vdocsBookmarkAliases(body vdocsBody) (map[string]string, error) {
	aliases := map[string]string{}
	for _, a := range body.CustomRanges {
		if a.Type != 3 {
			continue
		}
		p, err := readVDOCSBookmark(a)
		if err != nil {
			return nil, err
		}
		for _, name := range []string{p.BookmarkID, a.ID} {
			if previous, ok := aliases[name]; ok && previous != p.BookmarkID {
				return nil, fmt.Errorf("ambiguous native bookmark alias")
			}
			aliases[name] = p.BookmarkID
		}
	}
	return aliases, nil
}

func scopeVDOCSBookmarkIDs(ctx context.Context, out *vdocsNoteSnapshot, d *Document) error {
	catalog, err := catalogDestinations(d, true)
	if err != nil {
		return err
	}
	reserved := map[string]bool{}
	for id := range catalog.anchors {
		reserved[id] = true
	}
	for id := range catalog.links {
		reserved[id] = true
	}
	for id := range catalog.missing {
		reserved[id] = true
	}
	bodies := []*vdocsBody{&out.Body}
	type storySlot struct {
		id     string
		header bool
		story  vdocsRunningStory
	}
	var slots []storySlot
	for _, header := range []bool{true, false} {
		source := out.Footers
		if header {
			source = out.Headers
		}
		ids := make([]string, 0, len(source))
		for id := range source {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			slots = append(slots, storySlot{id, header, source[id]})
		}
	}
	for i := range slots {
		bodies = append(bodies, &slots[i].story.Body)
	}
	noteIDs := make([]string, 0, len(out.Notes))
	for id := range out.Notes {
		noteIDs = append(noteIDs, id)
	}
	sort.Strings(noteIDs)
	notes := make([]vdocsNativeNote, len(noteIDs))
	for i, id := range noteIDs {
		notes[i] = out.Notes[id]
		bodies = append(bodies, &notes[i].Body)
	}
	for _, body := range bodies {
		for _, a := range body.CustomRanges {
			if a.Type != 3 {
				reserved[a.ID] = true
			}
		}
	}
	for _, body := range bodies {
		for i := range body.CustomRanges {
			if err := ctx.Err(); err != nil {
				return err
			}
			a := &body.CustomRanges[i]
			if a.Type != 3 {
				continue
			}
			p, err := readVDOCSBookmark(*a)
			if err != nil {
				return err
			}
			for salt := 0; ; salt++ {
				sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d", p.BookmarkID, salt)))
				id := fmt.Sprintf("manydoc-bookmark-%x", sum)
				if !reserved[id] {
					a.ID = id
					reserved[id] = true
					break
				}
			}
		}
	}
	for _, slot := range slots {
		if slot.header {
			out.Headers[slot.id] = slot.story
		} else {
			out.Footers[slot.id] = slot.story
		}
	}
	for i, id := range noteIDs {
		out.Notes[id] = notes[i]
	}
	return nil
}

func finishVDOCSNavigation(ctx context.Context, input vdocsNoteSnapshot, d *Document, declared []string) error {
	c, err := catalogDestinations(d, false)
	if err != nil {
		return err
	}
	expected := map[string]bool{}
	heading := false
	check := func(body vdocsBody, scope string) error {
		for _, a := range body.CustomRanges {
			if err := ctx.Err(); err != nil {
				return err
			}
			if a.Type != 3 {
				continue
			}
			p, err := readVDOCSBookmark(a)
			if err != nil {
				return err
			}
			if expected[p.BookmarkID] {
				return fmt.Errorf("duplicate native bookmark identity")
			}
			expected[p.BookmarkID] = true
			if !c.anchors[p.BookmarkID] {
				return fmt.Errorf("%w: native bookmark at an unrepresented structural cursor", ErrUnsupported)
			}
		}
		for _, p := range body.Paragraphs {
			nativeID, err := vdocsNativeHeadingID(p.Style)
			if err != nil {
				return err
			}
			if nativeID != "" {
				id := vdocsHeadingID(scope, nativeID)
				if expected[id] {
					return fmt.Errorf("duplicate native heading identity")
				}
				expected[id] = true
				if !c.anchors[id] {
					return fmt.Errorf("%w: native heading at an unrepresented structural cursor", ErrUnsupported)
				}
				heading = true
			}
		}

		return nil
	}
	if err = check(input.Body, ""); err != nil {
		return err
	}
	mainPoints := map[string]bool{}
	for id := range expected {
		mainPoints[id] = true
	}
	for id, s := range input.Headers {
		if err = check(s.Body, "header:"+id); err != nil {
			return err
		}
	}
	for id, s := range input.Footers {
		if err = check(s.Body, "footer:"+id); err != nil {
			return err
		}
	}
	for id, n := range input.Notes {
		if err = check(n.Body, "note:"+id); err != nil {
			return err
		}
	}
	if heading {
		d.warn("vdocs_heading_navigation", "Native heading identities become scoped SF navigation points; current internal targets are retained")
	}
	known := map[string]bool{}
	for _, id := range declared {
		if err = destinationID(id); err != nil {
			return err
		}
		if known[id] {
			return fmt.Errorf("duplicate native unresolved-target intent")
		}
		known[id] = true
	}
	missing := map[string]bool{}
	for id := range known {
		if !c.anchors[id] {
			missing[id] = true
		}
	}
	nonmain, newMissing := false, false
	for id := range c.links {
		nonmain = nonmain || (c.anchors[id] && !mainPoints[id])
		if !c.anchors[id] {
			missing[id] = true
			newMissing = newMissing || !known[id]
		}
	}
	if nonmain {
		d.warn("vdocs_nonmain_navigation", "Native internal lookup targets the main story only; nonmain point identities and link data are retained in SF")
	}
	if newMissing {
		d.warn("vdocs_unresolved_navigation", "Native internal link has no current target; its explicit unresolved identity is retained")
	}

	d.UnresolvedDestinations = nil
	for id := range missing {
		d.UnresolvedDestinations = append(d.UnresolvedDestinations, id)
	}
	sort.Strings(d.UnresolvedDestinations)
	return nil
}

// Native container points use their real token and source identity, never a
// synthetic paragraph or cached block. Current positions remain authoritative.
func (w *vdocsBodyWriter) containerAnchors(ids []string, offset int, role, owner string) error {
	for _, id := range ids {
		if err := destinationID(id); err != nil {
			return err
		}
		a := writeVDOCSBookmark(id, offset, "", role)
		a.Properties = sfAttrs(vdocsBookmarkProperties{BookmarkID: id, Hint: &vdocsBookmarkHint{Version: 1, Role: role, OwnerID: owner}})
		w.body.CustomRanges = append(w.body.CustomRanges, a)
	}
	return nil
}
func vdocsBookmarkLosses(doc *Document, a vdocsRangeRecord) error {
	fields, err := vdocsStyleObject(a.Properties)
	if err != nil {
		return err
	}
	for name := range fields {
		if name != "bookmarkId" && name != "manydocNavigation" && name != "manydocOrigin" && name != "manydocFlowOrigin" {
			doc.warn("vdocs_bookmark_metadata", "Native bookmark metadata outside identity and placement is not represented in SF")
		}
	}
	if a.StartIndex != a.EndIndex {
		doc.warn("vdocs_bookmark_span", "Native bookmark range editing extent becomes an SF point at its current start")
	}
	return nil
}
func (r *vdocsBodyReader) containerBookmark(a vdocsRangeRecord) (bool, error) {
	role, owner := "", ""
	if table, ok := r.index.tables[a.StartIndex]; ok {
		role, owner = "table", table.ID
	} else if atom, ok := r.index.blocks[a.StartIndex]; ok && atom.Type != nil && *atom.Type == 0 {
		role, owner = "picture", atom.ID
	}
	if role == "" {
		token, err := r.index.text.exclusive(a.StartIndex, a.StartIndex+1)
		if err == nil && token == "\f" {
			role, owner = "break", "page-break"
		}
	}
	if role == "" {
		return false, nil
	}
	p, err := readVDOCSBookmark(a)
	if err != nil {
		return false, err
	}
	if err = vdocsBookmarkLosses(r.doc, a); err != nil {
		return false, err
	}
	if p.Hint != nil && p.Hint.Role != "point" && (p.Hint.Role != role || p.Hint.OwnerID != owner) {
		r.doc.warn("vdocs_stale_bookmark_placement", "Edited native bookmark position binds to its current container rather than stale placement intent")
	}
	r.containerPoints[a.StartIndex] = append(r.containerPoints[a.StartIndex], p.BookmarkID)
	return true, nil
}

// A native list has no token of its own. Restore container ownership only when
// the current list starts at the point's current first paragraph; otherwise keep
// the real inline point and disclose the lost ownership.

// Collect text-free source records once per unique source/fingerprint. Native
// paragraph properties carry only references; no body/style cache is stored.

// Clean carriers only after actual list grouping has settled. Tentative grouping
// can be discarded; only point blocks in the final tree authorize carrier removal.

// SF has no row/cell control-token cursor. Retain each bookmark relationship at
// the corresponding logical boundary and disclose the native-position loss.

// Position-specific loss evidence can be numerous. Deduplicate this projection's
// warnings in constant time rather than scanning all prior cursor diagnostics.

// Visit every descriptor, including covered cells, independently of current SF
// ownership. Validation does not mutate the caller's native snapshot.

func (r *vdocsBodyReader) promoteListPoints(block *Block, firstID string, position int) {
	if len(block.Items) == 0 || len(block.Items[0].Blocks) == 0 {
		return
	}
	first := 0
	for first < len(block.Items[0].Blocks) && block.Items[0].Blocks[first].Kind == AnchorPoint {
		first++
	}
	if first == len(block.Items[0].Blocks) {
		return
	}
	p := &block.Items[0].Blocks[first]
	runs := make([]Run, 0, len(p.Runs))
	for _, run := range p.Runs {
		if owner, ok := r.listPointOwners[run.Anchor]; ok && owner == firstID && r.listPointPositions[run.Anchor] == r.paragraphStarts[position] {
			block.Anchors = append(block.Anchors, run.Anchor)
			r.promotedListPoints[run.Anchor] = true
		} else {
			runs = append(runs, run)
		}
	}
	if len(runs) == 0 {
		p.Runs = nil
	} else {
		p.Runs = runs
	}
}
func (r *vdocsBodyReader) finishListPoints() {
	for id := range r.listPointOwners {
		if !r.promotedListPoints[id] {
			r.doc.warn("vdocs_stale_bookmark_placement", "Native list point retains its current inline position because current list ownership differs")
			return
		}
	}
}

const vdocsOriginProfile = "manydoc.paragraph-origin"

type vdocsOrigin struct {
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
		b.Runs[i] = vdocsSourceFreeRun(b.Runs[i])
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
func newVDOCSOrigin(b Block) (*vdocsOrigin, error) {
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
	return &vdocsOrigin{Profile: vdocsOriginProfile, Version: 1, Ref: hex.EncodeToString(sum[:]), Fingerprint: fingerprint, Source: cloneSourceInfo(b.Source)}, nil
}
func readVDOCSOrigin(raw json.RawMessage) (*vdocsOrigin, error) {
	return readVDOCSOriginProfile(raw, vdocsOriginProfile)
}
func readVDOCSOriginProfile(raw json.RawMessage, profile string) (*vdocsOrigin, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var o vdocsOrigin
	if err := validateTypedJSON(context.Background(), raw, reflect.TypeFor[vdocsOrigin]()); err != nil {
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

func collectVDOCSOrigins(out *vdocsNoteSnapshot, e **vdocsDocumentEvidence) error {
	pool := map[string]vdocsOrigin{}
	process := func(body *vdocsBody) error {
		for i := range body.Paragraphs {
			p := &body.Paragraphs[i]
			props, err := vdocsStyleObject(p.Style)
			if err != nil {
				return err
			}
			o, err := readVDOCSOrigin(props["manydocOrigin"])
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
		*e = &vdocsDocumentEvidence{Version: 1}
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
func validateVDOCSOrigin(o vdocsOrigin, pages []PDFPage) error {
	return validateVDOCSOriginProfile(o, pages, vdocsOriginProfile)
}
func validateVDOCSOriginProfile(o vdocsOrigin, pages []PDFPage, profile string) error {
	checked, err := readVDOCSOriginProfile(sfAttrs(o), profile)
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
func restoreVDOCSOrigins(d *Document, e *vdocsDocumentEvidence) error {
	pool := map[string]vdocsOrigin{}
	if e != nil {
		for _, o := range e.Origins {
			if err := validateVDOCSOrigin(o, d.Pages); err != nil {
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
			if o := b.vdocsOrigin; o != nil {
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
				b.vdocsOrigin = nil
				used[o.Ref] = true
				if hash != record.Fingerprint {
					b.Source.Historical = true
					d.warn("vdocs_source_historical", "Original PDF source facts are retained as historical after current paragraph content or presentation changed")
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
			d.warn("vdocs_source_unbound", "Original PDF source facts are retained unbound after their native paragraph binding disappeared")
		}
	}
	return nil
}

func newVDOCSSourceOrigin(source *SourceInfo, hash, profile string) (*vdocsOrigin, error) {
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
	return &vdocsOrigin{Profile: profile, Version: 1, Ref: hex.EncodeToString(sum[:]), Fingerprint: hash, Source: cloneSourceInfo(source)}, nil
}

const vdocsRunOriginProfile = "manydoc.run-origin"

func vdocsSourceFreeRun(r Run) Run {
	r.Source = nil
	if r.Note != nil && r.Note.LabelContent != nil {
		n := *r.Note
		n.LabelContent = append([]Run(nil), n.LabelContent...)
		for i := range n.LabelContent {
			n.LabelContent[i] = vdocsSourceFreeRun(n.LabelContent[i])
		}
		r.Note = &n
	}
	return r
}
func vdocsRunFingerprint(r Run) (string, error) {
	r = vdocsSourceFreeRun(r)
	nodes, err := encodeSFBlocks([]Block{{Kind: Paragraph, Runs: []Run{r}}})
	if err != nil {
		return "", err
	}
	return vdocsSourceFreeHash(nodes)
}
func newVDOCSRunOrigin(r Run) (*vdocsOrigin, error) {
	if r.Source == nil {
		return nil, nil
	}
	hash, err := vdocsRunFingerprint(r)
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
	return &vdocsOrigin{Profile: vdocsRunOriginProfile, Version: 1, Ref: hex.EncodeToString(sum[:]), Fingerprint: hash, Source: cloneSourceInfo(r.Source)}, nil
}
func collectVDOCSRunOrigins(out *vdocsNoteSnapshot, e **vdocsDocumentEvidence) error {
	pool := map[string]vdocsOrigin{}
	style := func(raw *json.RawMessage) error {
		props, err := vdocsStyleObject(*raw)
		if err != nil {
			return err
		}
		o, err := readVDOCSOriginProfile(props["manydocOrigin"], vdocsRunOriginProfile)
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
	process := func(body *vdocsBody) error {
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
	for _, stories := range []map[string]vdocsRunningStory{out.Headers, out.Footers} {
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
		*e = &vdocsDocumentEvidence{Version: 1}
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
func restoreVDOCSRunOrigins(d *Document, e *vdocsDocumentEvidence) error {
	pool := map[string]vdocsOrigin{}
	if e != nil {
		for _, o := range e.RunOrigins {
			if err := validateVDOCSOriginProfile(o, d.Pages, vdocsRunOriginProfile); err != nil {
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
			if o := r.vdocsRunOrigin; o != nil {
				record, ok := pool[o.Ref]
				if !ok {
					return fmt.Errorf("native run references missing source record")
				}
				if (o.Source != nil || o.Fingerprint != "") && !reflect.DeepEqual(*o, record) {
					return fmt.Errorf("contradictory inline/native run source record")
				}
				hash, err := vdocsRunFingerprint(*r)
				if err != nil {
					return err
				}
				r.Source = cloneSourceInfo(record.Source)
				r.vdocsRunOrigin = nil
				used[o.Ref] = true
				if hash != record.Fingerprint {
					r.Source.Historical = true
					d.warn("vdocs_run_source_historical", "Original PDF source facts are retained as historical after current run content or presentation changed")
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
			d.warn("vdocs_run_source_unbound", "Original PDF source facts are retained unbound after their native run binding disappeared")
		}
	}
	return nil
}
func (w *vdocsBodyWriter) pointScopeID() string {
	w.pointScopeSerial++
	return fmt.Sprintf("manydoc-point-scope-%d", w.pointScopeSerial)
}
func (w *vdocsBodyWriter) pointFlow(blocks []Block, depth int, scope, owner string) error {
	oldScope, oldOwner := w.pointScope, w.pointOwner
	w.pointScope, w.pointOwner = scope, owner
	defer func() { w.pointScope, w.pointOwner = oldScope, oldOwner }()
	return w.flow(blocks, depth)
}
func (w *vdocsBodyWriter) standalonePoint(b Block) error {
	copy := b
	copy.Anchors = nil
	copy.Source = nil
	if !reflect.DeepEqual(copy, Block{Kind: AnchorPoint}) || len(b.Anchors) == 0 {
		return fmt.Errorf("invalid standalone native point")
	}
	origin, err := newVDOCSFlowOrigin(b)
	if err != nil {
		return err
	}
	w.pointSerial++
	for i, id := range b.Anchors {
		if err := destinationID(id); err != nil {
			return err
		}
		a := writeVDOCSBookmark(id, w.offset, "", "point")
		a.Properties = sfAttrs(vdocsBookmarkProperties{BookmarkID: id, Hint: &vdocsBookmarkHint{Version: 1, Role: "point", OwnerID: w.pointOwner, PointScope: w.pointScope, PointOrder: w.pointSerial, PointIndex: i}})
		if i == 0 && origin != nil {
			props, err := readVDOCSBookmark(a)
			if err != nil {
				return err
			}
			props.FlowOrigin = origin
			a.Properties = sfAttrs(props)
		}
		w.body.CustomRanges = append(w.body.CustomRanges, a)
	}
	return nil
}
func rebindVDOCSPointScope(body *vdocsBody, from, to string, first int) error {
	for i := first; i < len(body.CustomRanges); i++ {
		a := &body.CustomRanges[i]
		if a.Type != 3 {
			continue
		}
		p, err := readVDOCSBookmark(*a)
		if err != nil {
			return err
		}
		if p.Hint != nil && p.Hint.Role == "point" && p.Hint.OwnerID == from {
			p.Hint.OwnerID = to
			a.Properties = sfAttrs(p)
		}
	}
	return nil
}

type vdocsPointSlot struct {
	scope, owner           string
	position, order, index int
}
type vdocsStandalonePoint struct {
	origin                    *vdocsOrigin
	id, scope, owner, carrier string
	position, order, index    int
}

func (r *vdocsBodyReader) registerStandalone(a vdocsRangeRecord, p vdocsBookmarkProperties) error {
	h := p.Hint
	point := vdocsStandalonePoint{origin: p.FlowOrigin, id: p.BookmarkID, scope: h.PointScope, owner: h.OwnerID, carrier: h.PointCarrier, position: a.StartIndex, order: h.PointOrder, index: h.PointIndex}
	key := vdocsPointSlot{point.scope, point.owner, point.position, point.order, point.index}
	if r.pointSlots[key] {
		return fmt.Errorf("duplicate standalone point order slot")
	}
	r.pointSlots[key] = true
	r.standalone = append(r.standalone, point)
	r.standaloneIDs[p.BookmarkID] = true
	if h.PointCarrier != "" {
		r.pointCarriers[h.PointCarrier] = true
	}
	return vdocsBookmarkLosses(r.doc, a)
}
func (r *vdocsBodyReader) standaloneBlocks(scope, owner string, position int) []Block {
	var points []vdocsStandalonePoint
	for _, p := range r.pointsAt[position] {
		if p.scope == scope && p.owner == owner {
			points = append(points, p)
		}
	}
	sort.SliceStable(points, func(i, j int) bool {
		if points[i].order != points[j].order {
			return points[i].order < points[j].order
		}
		return points[i].index < points[j].index
	})
	var blocks []Block
	last := -1
	for _, p := range points {
		if p.order != last {
			blocks = append(blocks, Block{Kind: AnchorPoint})
			last = p.order
		}
		n := len(blocks) - 1
		blocks[n].Anchors = append(blocks[n].Anchors, p.id)
		if blocks[n].vdocsFlowOrigin == nil {
			blocks[n].vdocsFlowOrigin = p.origin
		}
	}
	return blocks
}

func (r *vdocsBodyReader) finishStandalone(blocks []Block) {
	promoted := map[string]bool{}
	var walk func([]Block, func(*Block))
	walk = func(bs []Block, f func(*Block)) {
		for i := range bs {
			b := &bs[i]
			f(b)
			if b.Section != nil {
				walk(b.Section.Blocks, f)
			}
			for j := range b.Items {
				if b.Items[j].Marker != nil {
					walk(b.Items[j].Marker.Blocks, f)
				}
				walk(b.Items[j].Blocks, f)
			}
			for j := range b.Rows {
				for k := range b.Rows[j].Cells {
					walk(b.Rows[j].Cells[k].Blocks, f)
				}
			}
		}
	}
	walk(blocks, func(b *Block) {
		if b.Kind == AnchorPoint {
			for _, id := range b.Anchors {
				if r.standaloneIDs[id] {
					promoted[id] = true
				}
			}
		}
	})
	walk(blocks, func(b *Block) {
		if b.Kind != AnchorPoint {
			var ids []string
			for _, id := range b.Anchors {
				if !promoted[id] {
					ids = append(ids, id)
				}
			}
			b.Anchors = ids
		}
		if b.Runs != nil {
			var runs []Run
			for _, run := range b.Runs {
				if run.Anchor == "" || !promoted[run.Anchor] {
					runs = append(runs, run)
				}
			}
			b.Runs = runs
		}
	})
	for _, p := range r.standalone {
		if !promoted[p.id] {
			r.doc.warn("vdocs_stale_point_ownership", "Standalone point retains its current native location because its owning flow or boundary changed")
			break
		}
	}
}
func (r *vdocsBodyReader) emptyPointCarrier(p vdocsParagraphRecord, start, end int, owner string, bs []Block) bool {
	if !r.pointCarriers[p.ID] {
		return false
	}
	bound := false
	for _, point := range r.pointsAt[start] {
		bound = bound || point.carrier == p.ID && point.scope == "section" && point.owner == owner
	}
	valid := bound && start == end && len(p.Style) == 0 && p.StyleID == "" && len(p.Bullet) == 0 && len(bs) == 1
	if valid {
		copy := bs[0]
		for _, run := range copy.Runs {
			if !r.standaloneIDs[run.Anchor] {
				valid = false
			}
		}
		copy.Runs = nil
		valid = valid && reflect.DeepEqual(copy, Block{Kind: Paragraph})
	}
	if !valid {
		r.doc.warn("vdocs_stale_point_carrier", "Edited native point carrier is retained as current visible content and presentation")
	}
	return valid
}

func vdocsStandaloneBlockIDs(blocks []Block) map[string]bool {
	found := map[string]bool{}
	var walk func([]Block)
	walk = func(bs []Block) {
		for _, b := range bs {
			if b.Kind == AnchorPoint {
				for _, id := range b.Anchors {
					found[id] = true
				}
			}
			if b.Section != nil {
				walk(b.Section.Blocks)
			}
			for _, item := range b.Items {
				if item.Marker != nil {
					walk(item.Marker.Blocks)
				}
				walk(item.Blocks)
			}
			for _, row := range b.Rows {
				for _, cell := range row.Cells {
					walk(cell.Blocks)
				}
			}
		}
	}
	walk(blocks)
	return found
}

func (r *vdocsBodyReader) indexStructuralCursors() error {
	r.cursorKinds = map[int]string{}
	r.cursorBookmarks = map[int][]vdocsRangeRecord{}
	var walk func([]vdocsStreamNode) error
	walk = func(nodes []vdocsStreamNode) error {
		for _, n := range nodes {
			if err := r.ctx.Err(); err != nil {
				return err
			}
			switch n.kind {
			case "section":
				r.cursorKinds[n.end-1] = "section terminator"
			case "table":
				r.cursorKinds[n.end-1] = "table terminator"
			case "row":
				r.cursorKinds[n.start] = "row start"
				r.cursorKinds[n.end-1] = "row end"
			case "cell":
				r.cursorKinds[n.start] = "cell start"
				r.cursorKinds[n.end-1] = "cell end"
			case "paragraph":
				if n.end-1 == n.start+1 {
					token, err := r.index.text.exclusive(n.start, n.start+1)
					if err != nil {
						return err
					}
					if token == "\f" {
						r.cursorKinds[n.end-1] = "page-break paragraph terminator"
					}
				}
				if len(n.children) > 0 && n.children[len(n.children)-1].end == n.end-1 {
					r.cursorKinds[n.end-1] = "table anchoring paragraph terminator"
				}
				if a, ok := r.index.blocks[n.start]; ok && a.Type != nil && *a.Type == 0 && n.end-1 == n.start+1 {
					r.cursorKinds[n.end-1] = "image paragraph terminator"
				}
			}
			if err := walk(n.children); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(r.index.sections)
}
func (r *vdocsBodyReader) cursorBlocks(position int, boundary string, ordinary bool) ([]Block, error) {
	records := r.cursorBookmarks[position]
	delete(r.cursorBookmarks, position)
	if ordinary {
		first := sort.Search(len(r.bookmarks), func(i int) bool { return r.bookmarks[i].StartIndex >= position })
		for _, a := range r.bookmarks[first:] {
			if a.StartIndex != position {
				break
			}
			if !r.bookmarkUsed[a.ID] {
				records = append(records, a)
				r.bookmarkUsed[a.ID] = true
			}
		}
	}
	var blocks []Block
	for _, a := range records {
		p, err := readVDOCSBookmark(a)
		if err != nil {
			return nil, err
		}
		losses := &Document{}
		if err = vdocsBookmarkLosses(losses, a); err != nil {
			return nil, err
		}
		for _, d := range losses.Diagnostics {
			r.cursorWarn(d.Code, d.Message)
		}
		kind := r.cursorKinds[position]
		if kind == "" {
			kind = "paragraph point"
		}
		location := fmt.Sprintf("UTF-16 offset %d (%s)", position, kind)
		if r.settings.NavigationScope != "" {
			location += fmt.Sprintf(" in story %q", r.settings.NavigationScope)
		}
		r.cursorWarn("vdocs_structural_cursor", fmt.Sprintf("Native bookmark at %s projects to %s; exact native control cursor identity is not retained", location, boundary))
		if p.Hint != nil {
			r.cursorWarn("vdocs_stale_bookmark_placement", "Native bookmark placement intent is normalized at its current structural boundary")
		}
		blocks = append(blocks, Block{Kind: AnchorPoint, Anchors: []string{p.BookmarkID}})
	}
	return blocks, nil
}

type vdocsCellCursorProjection struct {
	origin        *tableCellPosition
	before, after []Block
}

func (r *vdocsBodyReader) cellCursorProjection(row, cell vdocsStreamNode, index, count int, origin *tableCellPosition, covered bool) (vdocsCellCursorProjection, error) {
	out := vdocsCellCursorProjection{origin: origin}
	where := fmt.Sprintf("logical table cell origin row %d column %d", origin.Row, origin.Column)
	if covered {
		where = "merged continuation's " + where
	}
	take := func(position int, edge string, ordinary bool) error {
		blocks, err := r.cursorBlocks(position, where+" "+edge, ordinary)
		if err != nil {
			return err
		}
		if edge == "start" {
			out.before = append(out.before, blocks...)
		} else {
			out.after = append(out.after, blocks...)
		}
		return nil
	}
	if index == 0 {
		if err := take(row.start, "start", false); err != nil {
			return out, err
		}
	}
	if err := take(cell.start, "start", false); err != nil {
		return out, err
	}
	if covered {
		if err := take(cell.start+1, "start", true); err != nil {
			return out, err
		}
		if err := take(cell.end-2, "end", true); err != nil {
			return out, err
		}
	}
	if err := take(cell.end-1, "end", false); err != nil {
		return out, err
	}
	if index == count-1 {
		if err := take(row.end-1, "end", false); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (r *vdocsBodyReader) cursorWarn(code, message string) {
	if r.cursorWarningSet == nil {
		r.cursorWarningSet = map[string]bool{}
		for _, d := range r.doc.Diagnostics {
			r.cursorWarningSet[d.Code+"\x00"+d.Message] = true
		}
	}
	key := code + "\x00" + message
	if !r.cursorWarningSet[key] {
		r.cursorWarningSet[key] = true
		r.doc.Diagnostics = append(r.doc.Diagnostics, Diagnostic{Code: code, Message: message})
	}
}

const vdocsTableOriginProfile = "manydoc.table-origin"
const vdocsCellOriginProfile = "manydoc.cell-origin"

func vdocsTableFingerprint(b Block) (string, error) {
	nodes, err := encodeSFBlocks([]Block{b})
	if err != nil {
		return "", err
	}
	return vdocsSourceFreeHash(nodes)
}
func vdocsCellFingerprint(c Cell) (string, error) {
	nodes, err := encodeSFBlocks([]Block{{Kind: Table, Rows: []Row{{Cells: []Cell{c}}}}})
	if err != nil {
		return "", err
	}
	return vdocsSourceFreeHash(nodes[0].Content[0].Content[0])
}

func visitVDOCSTableOrigins(snapshot *vdocsNoteSnapshot, update bool, visit func(*vdocsOrigin) error) error {
	marker := func(props vdocsStyleMap, profile string) error {
		o, err := readVDOCSOriginProfile(props["manydocOrigin"], profile)
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
			props, err := vdocsStyleObject(raw)
			if err != nil {
				return err
			}
			if err = marker(props, vdocsTableOriginProfile); err != nil {
				return err
			}
			var rows []json.RawMessage
			if err = json.Unmarshal(props["tableRows"], &rows); err != nil {
				return err
			}
			for i, row := range rows {
				rp, err := vdocsStyleObject(row)
				if err != nil {
					return err
				}
				var cells []json.RawMessage
				if err = json.Unmarshal(rp["tableCells"], &cells); err != nil {
					return err
				}
				for j, cell := range cells {
					cp, err := vdocsStyleObject(cell)
					if err != nil {
						return err
					}
					if err = marker(cp, vdocsCellOriginProfile); err != nil {
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
	for _, stories := range []map[string]vdocsRunningStory{snapshot.Headers, snapshot.Footers} {
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
func collectVDOCSTableOrigins(out *vdocsNoteSnapshot, e **vdocsDocumentEvidence) error {
	pool := map[string]vdocsOrigin{}
	err := visitVDOCSTableOrigins(out, true, func(o *vdocsOrigin) error {
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
		*e = &vdocsDocumentEvidence{Version: 1}
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
func validateVDOCSTableOrigin(o vdocsOrigin, pages []PDFPage) error {
	if o.Profile != vdocsTableOriginProfile && o.Profile != vdocsCellOriginProfile {
		return fmt.Errorf("invalid native table/cell source profile")
	}
	return validateVDOCSOriginProfile(o, pages, o.Profile)
}
func restoreVDOCSTableOrigins(d *Document, e *vdocsDocumentEvidence, input vdocsNoteSnapshot) error {
	pool := map[string]vdocsOrigin{}
	if e != nil {
		for _, o := range e.TableOrigins {
			if err := validateVDOCSTableOrigin(o, d.Pages); err != nil {
				return err
			}
			if _, ok := pool[o.Ref]; ok {
				return fmt.Errorf("duplicate table/cell source reference")
			}
			pool[o.Ref] = o
		}
	}
	lookup := func(o *vdocsOrigin) (vdocsOrigin, error) {
		record, ok := pool[o.Ref]
		if !ok || record.Profile != o.Profile {
			return record, fmt.Errorf("native table/cell references missing source record")
		}
		if (o.Source != nil || o.Fingerprint != "") && !reflect.DeepEqual(*o, record) {
			return record, fmt.Errorf("contradictory native table/cell source record")
		}
		return record, nil
	}
	if err := visitVDOCSTableOrigins(&input, false, func(o *vdocsOrigin) error { _, err := lookup(o); return err }); err != nil {
		return err
	}
	used := map[string]bool{}
	restore := func(o *vdocsOrigin, hash string) (*SourceInfo, error) {
		record, err := lookup(o)
		if err != nil {
			return nil, err
		}
		used[o.Ref] = true
		source := cloneSourceInfo(record.Source)
		if hash != record.Fingerprint {
			source.Historical = true
			d.warn("vdocs_table_source_historical", "Original table or cell source facts are retained as historical after current content, grid or presentation changed")
		}
		return source, nil
	}
	var walk func([]Block) error
	walk = func(bs []Block) error {
		for i := range bs {
			b := &bs[i]
			if b.vdocsTableOrigin != nil {
				hash, err := vdocsTableFingerprint(*b)
				if err != nil {
					return err
				}
				b.Source, err = restore(b.vdocsTableOrigin, hash)
				if err != nil {
					return err
				}
				b.vdocsTableOrigin = nil
			}
			for ri := range b.Rows {
				for ci := range b.Rows[ri].Cells {
					c := &b.Rows[ri].Cells[ci]
					if c.vdocsOrigin != nil {
						hash, err := vdocsCellFingerprint(*c)
						if err != nil {
							return err
						}
						c.Source, err = restore(c.vdocsOrigin, hash)
						if err != nil {
							return err
						}
						c.vdocsOrigin = nil
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
		d.warn("vdocs_table_source_unbound", "Original table or cell source facts are retained unbound after their native ownership disappeared")
	}
	return nil
}
