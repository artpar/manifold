package manydoc

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

const colleafHyperlinkResource = "DOC_HYPER_LINK_PLUGIN"

type colleafHyperlinkResourceData struct {
	Links *[]colleafHyperlinkResourceEntry `json:"links"`
}
type colleafHyperlinkResourceEntry struct {
	ID      *string `json:"id"`
	Payload *string `json:"payload"`
}

// Pinned plugin onLoad applies resource payloads to main/header/footer ranges by
// global rangeId. Notes are not visited. Follow that precedence explicitly and
// reject ambiguous identities instead of depending on map/story iteration order.
// Copy only the slices/maps modified here; caller-owned snapshots remain intact.
func applyColleafHyperlinkResource(ctx context.Context, input colleafNoteSnapshot) (colleafNoteSnapshot, []Diagnostic, error) {
	var resource *colleafResource
	var remaining []colleafResource
	for _, r := range input.Resources {
		if r.Name == colleafHyperlinkResource {
			if resource != nil {
				return input, nil, fmt.Errorf("duplicate native hyperlink resource")
			}
			copy := r
			resource = &copy
		} else {
			remaining = append(remaining, r)
		}
	}
	if resource == nil {
		return input, nil, ctx.Err()
	}
	raw := []byte(resource.Data)
	if err := validateTypedJSON(ctx, raw, reflect.TypeFor[colleafHyperlinkResourceData]()); err != nil {
		return input, nil, fmt.Errorf("native hyperlink resource JSON: %w", err)
	}
	var data colleafHyperlinkResourceData
	if err := strictSFAttrs(raw, &data); err != nil {
		return input, nil, err
	}
	if data.Links == nil {
		return input, nil, fmt.Errorf("native hyperlink resource requires links array")
	}
	out := input
	out.Resources = remaining
	ranges := map[string]*colleafRangeRecord{}
	index := func(body *colleafBody) error {
		body.CustomRanges = append([]colleafRangeRecord(nil), body.CustomRanges...)
		for i := range body.CustomRanges {
			if err := ctx.Err(); err != nil {
				return err
			}
			a := &body.CustomRanges[i]
			if a.Type != 0 {
				continue
			}
			if a.ID == "" || ranges[a.ID] != nil {
				return fmt.Errorf("ambiguous native hyperlink ID across stories")
			}
			ranges[a.ID] = a
		}
		return nil
	}
	if err := index(&out.Body); err != nil {
		return input, nil, err
	}
	copyStories := func(source map[string]colleafRunningStory) (map[string]colleafRunningStory, error) {
		if source == nil {
			return nil, nil
		}
		copied := make(map[string]colleafRunningStory, len(source))
		ids := make([]string, 0, len(source))
		for id := range source {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			story := source[id]
			if err := index(&story.Body); err != nil {
				return nil, err
			}
			copied[id] = story
		}
		return copied, nil
	}
	var err error
	if out.Headers, err = copyStories(input.Headers); err != nil {
		return input, nil, err
	}
	if out.Footers, err = copyStories(input.Footers); err != nil {
		return input, nil, err
	}
	seen := map[string]bool{}
	changed, orphan := false, false
	for _, entry := range *data.Links {
		if err := ctx.Err(); err != nil {
			return input, nil, err
		}
		if entry.ID == nil || *entry.ID == "" || entry.Payload == nil || seen[*entry.ID] {
			return input, nil, fmt.Errorf("invalid/duplicate native hyperlink resource entry")
		}
		seen[*entry.ID] = true
		a, ok := ranges[*entry.ID]
		if !ok {
			orphan = true
			continue
		}
		props, err := colleafStyleObject(a.Properties)
		if err != nil {
			return input, nil, err
		}
		var current string
		if raw, ok := props["url"]; ok {
			if err = json.Unmarshal(raw, &current); err != nil {
				return input, nil, err
			}
		}
		if *entry.Payload != "" {
			for _, key := range []string{"bookmarkId", "headingId"} {
				var internal string
				if raw, ok := props[key]; ok {
					if err = json.Unmarshal(raw, &internal); err != nil {
						return input, nil, err
					}
				}
				if internal != "" && !colleafInternalHref(*entry.Payload, internal) {
					return input, nil, fmt.Errorf("%w: native hyperlink resource has conflicting external/internal targets", ErrUnsupported)
				}
			}
		}
		if current != *entry.Payload {
			props["url"] = sfAttrs(*entry.Payload)
			a.Properties = sfAttrs(props)
			changed = true
		}
	}
	var diagnostics []Diagnostic
	if changed {
		diagnostics = append(diagnostics, Diagnostic{"colleaf_hyperlink_resource_target", "Native hyperlink resource overrides a different body URL; the current editor target is retained"})
	}
	if orphan {
		diagnostics = append(diagnostics, Diagnostic{"colleaf_hyperlink_resource_orphan", "Native hyperlink resource contains inactive identities; current visible hyperlink ranges are retained"})
	}
	return out, diagnostics, nil
}
