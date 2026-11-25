package manifold

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type sfSectionAttrs struct {
	Layout    *Layout                     `json:"layout,omitempty"`
	Start     SectionStart                `json:"start,omitempty"`
	FirstPage bool                        `json:"firstPage,omitempty"`
	Headers   map[string][]StructuredNode `json:"headers,omitempty"`
	Footers   map[string][]StructuredNode `json:"footers,omitempty"`
}

func encodeSFStories(stories RunningStories) (map[string][]StructuredNode, error) {
	out := map[string][]StructuredNode{}
	for _, s := range []struct {
		name  string
		story *RunningStory
	}{{"default", stories.Default}, {"first", stories.First}, {"even", stories.Even}} {
		if s.story == nil {
			continue
		}
		nodes, err := encodeSFBlocks(s.story.Blocks)
		if err != nil {
			return nil, err
		}
		if nodes == nil {
			nodes = []StructuredNode{}
		}
		out[s.name] = nodes
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
func decodeSFStories(nodes map[string][]StructuredNode, depth int) (RunningStories, error) {
	var out RunningStories
	for key, content := range nodes {
		var target **RunningStory
		switch key {
		case "default":
			target = &out.Default
		case "first":
			target = &out.First
		case "even":
			target = &out.Even
		default:
			return out, fmt.Errorf("unknown running story %q", key)
		}
		if content == nil {
			return out, fmt.Errorf("running story must be an array, not null")
		}
		blocks, err := decodeSFBlocks(content, depth+1)
		if err != nil {
			return out, err
		}
		*target = &RunningStory{blocks}
	}
	return out, nil
}
func encodeSFSection(s *Section) (StructuredNode, error) {
	a := sfSectionAttrs{Start: s.Start, FirstPage: s.FirstPage}
	if s.Layout != nil {
		l := cloneLayout(*s.Layout)
		a.Layout = &l
	}
	var err error
	if a.Headers, err = encodeSFStories(s.Headers); err != nil {
		return StructuredNode{}, err
	}
	if a.Footers, err = encodeSFStories(s.Footers); err != nil {
		return StructuredNode{}, err
	}
	content, err := encodeSFBlocks(s.Blocks)
	if err != nil {
		return StructuredNode{}, err
	}
	return StructuredNode{Type: "section", Attrs: sfAttrs(a), Content: content}, nil
}
func decodeSFSection(n StructuredNode, depth int) (*Section, error) {
	var raw map[string]json.RawMessage
	if len(n.Attrs) > 0 {
		if err := json.Unmarshal(n.Attrs, &raw); err != nil {
			return nil, err
		}
	}
	for _, name := range []string{"headers", "footers"} {
		if v, ok := raw[name]; ok && bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return nil, fmt.Errorf("%s must be an object", name)
		}
	}
	var a sfSectionAttrs
	if err := strictSFAttrs(n.Attrs, &a); err != nil {
		return nil, err
	}
	s := &Section{Start: a.Start, FirstPage: a.FirstPage}
	if a.Layout != nil {
		l := cloneLayout(*a.Layout)
		s.Layout = &l
	}
	var err error
	if s.Headers, err = decodeSFStories(a.Headers, depth); err != nil {
		return nil, err
	}
	if s.Footers, err = decodeSFStories(a.Footers, depth); err != nil {
		return nil, err
	}
	s.Blocks, err = decodeSFBlocks(n.Content, depth+1)
	return s, err
}
