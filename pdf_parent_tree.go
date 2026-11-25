package manydoc

import (
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// ParentTree is a reciprocal index, not an alternate source of document order.
// Missing reciprocal information is disclosed; contradictory ownership rejects.
func (s *pdfStructureRecovery) validateParentTree(value types.Object, kids []*pdfStructureNode) error {
	if value == nil {
		s.warn("pdf_parent_tree_missing", "Logical structure has no inverse ParentTree; forward ownership is recovered without reciprocal validation")
		return nil
	}
	entries := map[int]types.Object{}
	visited := map[string]bool{}
	count := 0
	var read func(types.Object, int) (int, int, error)
	read = func(value types.Object, depth int) (int, int, error) {
		if err := s.reader.ctx.Err(); err != nil {
			return 0, 0, err
		}
		count++
		if depth > 64 || count > 200000 {
			return 0, 0, fmt.Errorf("ParentTree exceeds nesting/entry limit")
		}
		if ref, ok := value.(types.IndirectRef); ok {
			id := fmt.Sprint(ref)
			if visited[id] {
				return 0, 0, fmt.Errorf("ParentTree cyclic or repeated node %s", id)
			}
			visited[id] = true
		}
		d, err := s.reader.pdf.DereferenceDict(value)
		if err != nil || d == nil {
			return 0, 0, fmt.Errorf("ParentTree node is not a dictionary")
		}
		if d["Nums"] != nil && d["Kids"] != nil {
			return 0, 0, fmt.Errorf("ParentTree node has both Nums and Kids")
		}
		min, max := -1, -1
		if d["Nums"] != nil {
			nums, e := s.reader.pdf.DereferenceArray(d["Nums"])
			if e != nil || nums == nil || len(nums)%2 != 0 || len(nums) > 400000 {
				return 0, 0, fmt.Errorf("invalid ParentTree Nums")
			}
			for i := 0; i < len(nums); i += 2 {
				count++
				if count > 200000 {
					return 0, 0, fmt.Errorf("ParentTree exceeds entry limit")
				}
				key, e := s.reader.pdf.DereferenceInteger(nums[i])
				if e != nil || key == nil || *key < 0 || int64(*key) > 2147483647 || int(*key) <= max {
					return 0, 0, fmt.Errorf("ParentTree keys must be increasing nonnegative integers")
				}
				if _, found := entries[int(*key)]; found {
					return 0, 0, fmt.Errorf("duplicate ParentTree key")
				}
				if min < 0 {
					min = int(*key)
				}
				max = int(*key)
				entries[max] = nums[i+1]
			}
		} else if d["Kids"] != nil {
			children, e := s.reader.pdf.DereferenceArray(d["Kids"])
			if e != nil || len(children) == 0 || len(children) > 200000 {
				return 0, 0, fmt.Errorf("invalid ParentTree Kids")
			}
			for _, child := range children {
				if _, ok := child.(types.IndirectRef); !ok {
					if _, direct := child.(types.Dict); !direct {
						return 0, 0, fmt.Errorf("ParentTree child is not a dictionary or indirect reference")
					}
					s.warn("pdf_parent_tree_direct_child", "ParentTree has a direct child dictionary instead of an indirect reference; reciprocal ownership is still checked, without claiming number-tree conformance")
				}
				lo, hi, e := read(child, depth+1)
				if e != nil {
					return 0, 0, e
				}
				if lo < 0 || lo <= max {
					return 0, 0, fmt.Errorf("ParentTree child ranges overlap or are empty")
				}
				if min < 0 {
					min = lo
				}
				max = hi
			}
		} else {
			return 0, 0, fmt.Errorf("ParentTree node has no Nums or Kids")
		}
		if d["Limits"] != nil {
			limits, e := s.reader.pdf.DereferenceArray(d["Limits"])
			if e != nil || len(limits) != 2 {
				return 0, 0, fmt.Errorf("invalid ParentTree Limits")
			}
			lo, e := s.reader.pdf.DereferenceInteger(limits[0])
			if e != nil || lo == nil {
				return 0, 0, fmt.Errorf("invalid ParentTree lower limit")
			}
			hi, e := s.reader.pdf.DereferenceInteger(limits[1])
			if e != nil || hi == nil || int(*lo) != min || int(*hi) != max {
				return 0, 0, fmt.Errorf("ParentTree Limits contradict keys")
			}
		}
		return min, max, nil
	}
	if _, _, err := read(value, 0); err != nil {
		return err
	}
	type slot struct{ key, mcid int }
	expected := map[slot]string{}
	containers := map[int]string{}
	forward := map[pdfContentKey]bool{}
	var walk func([]*pdfStructureNode, string) error
	walk = func(nodes []*pdfStructureNode, owner string) error {
		for _, n := range nodes {
			if err := s.reader.ctx.Err(); err != nil {
				return err
			}
			if n.kind == "StructElem" {
				if err := walk(n.kids, n.id); err != nil {
					return err
				}
				continue
			}
			var d types.Dict
			var container string
			var err error
			field := "StructParents"
			mcid := n.mcid
			if n.contentObject != nil {
				container = fmt.Sprint(n.contentObject)
				obj, e := s.reader.pdf.Dereference(n.contentObject)
				if e != nil {
					return e
				}
				switch obj := obj.(type) {
				case types.Dict:
					d = obj
				case types.StreamDict:
					d = obj.Dict
				default:
					return fmt.Errorf("ParentTree content container is not a dictionary")
				}
			} else {
				var ref *types.IndirectRef
				d, ref, _, err = s.reader.pdf.PageDict(n.page, false)
				if err != nil {
					return err
				}
				if ref != nil {
					container = fmt.Sprint(*ref)
				}
			}
			if n.kind == "OBJR" {
				field = "StructParent"
				mcid = -1
			}
			if d["StructParent"] != nil && d["StructParents"] != nil {
				return fmt.Errorf("content object has both StructParent and StructParents")
			}
			key, e := s.reader.pdf.DereferenceInteger(d[field])
			if e != nil {
				return e
			}
			if key == nil || owner == "" {
				s.warn("pdf_parent_tree_incomplete", "Forward content ownership lacks an indirect owner or reciprocal container key; forward content is retained")
				continue
			}
			if *key < 0 || int64(*key) > 2147483647 {
				return fmt.Errorf("invalid %s key", field)
			}
			if previous, ok := containers[int(*key)]; ok && previous != container {
				return fmt.Errorf("ParentTree key shared by different content containers")
			}
			containers[int(*key)] = container
			pos := slot{int(*key), mcid}
			if previous, ok := expected[pos]; ok && previous != owner {
				return fmt.Errorf("forward content has multiple logical owners")
			}
			if n.kind == "MCR" {
				physical := pdfContentKey{n.page, n.stream, n.mcid}
				if forward[physical] {
					return fmt.Errorf("marked-content sequence is referenced repeatedly")
				}
				forward[physical] = true
			}
			expected[pos] = owner
		}
		return nil
	}
	if err := walk(kids, ""); err != nil {
		return err
	}
	// Validate all listed references, including entries that have no forward owner.
	// Null array slots are allowed for unused or structurally unowned MCIDs.
	inverse := map[slot]string{}
	arrayEntries := map[int]bool{}
	ownerRef := func(value types.Object) (string, error) {
		if value == nil {
			return "", nil
		}
		ref, ok := value.(types.IndirectRef)
		if !ok {
			return "", fmt.Errorf("ParentTree owner must be an indirect structure element")
		}
		id := fmt.Sprint(ref)
		if !s.seen[id] {
			return "", fmt.Errorf("ParentTree owner is absent from forward structure")
		}
		return id, nil
	}
	for key, value := range entries {
		obj, e := s.reader.pdf.Dereference(value)
		if e != nil {
			return e
		}
		if array, ok := obj.(types.Array); ok {
			arrayEntries[key] = true
			count += len(array)
			if count > 400000 {
				return fmt.Errorf("ParentTree exceeds owner slot limit")
			}
			for mcid, v := range array {
				id, e := ownerRef(v)
				if e != nil {
					return e
				}
				if id != "" {
					inverse[slot{key, mcid}] = id
				}
			}
		} else {
			id, e := ownerRef(value)
			if e != nil {
				return e
			}
			if id == "" {
				return fmt.Errorf("ParentTree object entry is null")
			}
			inverse[slot{key, -1}] = id
		}
	}
	for pos, owner := range expected {
		if _, present := entries[pos.key]; present && arrayEntries[pos.key] != (pos.mcid >= 0) {
			return fmt.Errorf("ParentTree entry shape contradicts content container at key %d", pos.key)
		}
		actual := inverse[pos]
		if actual == "" {
			s.warn("pdf_parent_tree_incomplete", "Forward content lacks a reciprocal ParentTree entry; forward content is retained")
			continue
		}
		if actual != owner {
			return fmt.Errorf("ParentTree contradicts forward content owner at key %d MCID %d", pos.key, pos.mcid)
		}
	}
	for pos := range inverse {
		if _, ok := expected[pos]; !ok {
			s.warn("pdf_parent_tree_unclaimed", "ParentTree contains ownership absent from the forward structure; no additional content or ordering is invented")
		}
	}
	return nil
}
