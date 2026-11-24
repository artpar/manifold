package manydoc

// Resolve nested counter contexts on owned containers; source runs, assets and
// provenance remain shared read-only. A list's own start remains authoritative.
func resolveNumberingDocument(d *Document) *Document {
	type counter struct {
		value  int
		format string
	}
	var walk func([]Block, map[int]counter, int) []Block
	var stories func(RunningStories) RunningStories
	stories = func(s RunningStories) RunningStories {
		clone := func(v *RunningStory) *RunningStory {
			if v == nil {
				return nil
			}
			return &RunningStory{Blocks: walk(v.Blocks, nil, 0)}
		}
		return RunningStories{Default: clone(s.Default), First: clone(s.First), Even: clone(s.Even)}
	}
	walk = func(bs []Block, parents map[int]counter, depth int) []Block {
		if bs == nil {
			return nil
		}
		out := append([]Block(nil), bs...)
		for i := range out {
			b := &out[i]
			if b.Section != nil {
				s := *b.Section
				s.Blocks = walk(s.Blocks, nil, 0)
				s.Headers = stories(s.Headers)
				s.Footers = stories(s.Footers)
				b.Section = &s
			}
			if b.Numbering != nil {
				n := *b.Numbering
				n.Formats = append([]string(nil), n.Formats...)
				n.Ancestors = append([]int(nil), n.Ancestors...)
				for level, p := range parents {
					if !n.Independent && level < n.Level {
						n.Ancestors[level] = p.value
						n.Formats[level] = p.format
					}
				}
				b.Numbering = &n
			}
			if b.Rows != nil {
				b.Rows = append([]Row(nil), b.Rows...)
			}
			for ri := range b.Rows {
				row := &b.Rows[ri]
				row.Cells = append([]Cell(nil), row.Cells...)
				for ci := range row.Cells {
					row.Cells[ci].Blocks = walk(row.Cells[ci].Blocks, parents, depth)
				}
			}
			if b.Items != nil {
				b.Items = append([]ListItem(nil), b.Items...)
			}
			for ii := range b.Items {
				next := make(map[int]counter, len(parents)+1)
				for k, v := range parents {
					next[k] = v
				}
				level, format := depth, "decimal"
				if b.Numbering != nil {
					level = b.Numbering.Level
					format = b.Numbering.Formats[level]
					for k, v := range b.Numbering.Ancestors {
						next[k] = counter{v, b.Numbering.Formats[k]}
					}
				}
				if b.List == "number" {
					for k := range next {
						if k >= level {
							delete(next, k)
						}
					}
					next[level] = counter{listStart(*b) + ii, format}
				}
				if marker := b.Items[ii].Marker; marker != nil {
					m := *marker
					m.Blocks = walk(m.Blocks, next, level+1)
					b.Items[ii].Marker = &m
				}
				b.Items[ii].Blocks = walk(b.Items[ii].Blocks, next, level+1)
			}
		}
		return out
	}
	copy := *d
	copy.Blocks = walk(d.Blocks, nil, 0)
	copy.Headers = walk(d.Headers, nil, 0)
	copy.Footers = walk(d.Footers, nil, 0)
	copy.Notes = append([]Note(nil), d.Notes...)
	for i := range copy.Notes {
		copy.Notes[i].Blocks = walk(copy.Notes[i].Blocks, nil, 0)
	}
	return &copy
}
