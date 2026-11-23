package manydoc

func listStart(b Block) int {
	if b.Start != nil {
		return *b.Start
	}
	return 1
}
func listStartPointer(value int) *int {
	if value == 1 {
		return nil
	}
	return &value
}

// legacyLists converts consecutive legacy paragraph list runs to explicit list
// blocks. Explicit list blocks are never coalesced: their boundaries matter.
// It does not mutate the caller's values or backing slices.
func legacyLists(bs []Block) []Block {
	var result []Block
	for _, b := range bs {
		if (b.Kind == Paragraph || b.Kind == Heading) && b.List != "" {
			kind := b.List
			b.List = ""
			item := ListItem{Blocks: []Block{b}}
			if len(result) > 0 && result[len(result)-1].Kind == ListBlock && result[len(result)-1].docxNumberID == -1 && result[len(result)-1].List == kind {
				last := &result[len(result)-1]
				last.Items = append(last.Items, item)
			} else {
				result = append(result, Block{Kind: ListBlock, List: kind, Items: []ListItem{item}, docxNumberID: -1})
			}
		} else {
			result = append(result, b)
		}
	}
	// The marker distinguishes legacy runs only during this call.
	for i := range result {
		if result[i].docxNumberID == -1 {
			result[i].docxNumberID = 0
		}
	}
	return result
}
