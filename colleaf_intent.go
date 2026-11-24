package manydoc

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

const colleafIntentProfile = "manydoc.native-intent"
const colleafIntentRangeID = "manydoc-intent-v1"

type colleafIntentCarrier struct {
	Profile string            `json:"profile"`
	Version int               `json:"version"`
	Records []colleafResource `json:"records"`
}

func colleafOwnIntent(name string) bool {
	switch name {
	case colleafNoteIdentityResource, colleafListResource, colleafImageResource:
		return true
	}
	return false
}

// Unregistered resources are removed by the actual FDocument.save API. A native
// CUSTOM annotation on the terminal LF survives that API and normal text edits.
// Its payload contains checked missing intent only, never a whole SF/text cache.
func sealColleafIntent(snapshot *colleafNoteSnapshot) error {
	var records, remaining []colleafResource
	for _, resource := range snapshot.Resources {
		if colleafOwnIntent(resource.Name) {
			records = append(records, resource)
		} else {
			remaining = append(remaining, resource)
		}
	}
	if len(records) == 0 {
		return nil
	}
	position := colleafUTF16Len(snapshot.Body.DataStream) - 1
	if position < 0 || !strings.HasSuffix(snapshot.Body.DataStream, "\r\n") {
		return fmt.Errorf("native intent carrier requires final paragraph/section terminators")
	}
	for _, a := range snapshot.Body.CustomRanges {
		if a.ID == colleafIntentRangeID {
			return fmt.Errorf("native intent carrier identity collision")
		}
	}
	snapshot.Body.CustomRanges = append(snapshot.Body.CustomRanges, colleafRangeRecord{StartIndex: position, EndIndex: position, ID: colleafIntentRangeID, Type: 5, Properties: sfAttrs(colleafIntentCarrier{Profile: colleafIntentProfile, Version: 1, Records: records})})
	snapshot.Resources = remaining
	return nil
}
func openColleafIntent(ctx context.Context, snapshot colleafNoteSnapshot) (colleafNoteSnapshot, error) {
	// Copy slices before filtering: caller-owned native records remain untouched.
	snapshot.Resources = append([]colleafResource(nil), snapshot.Resources...)
	var ranges []colleafRangeRecord
	found := false
	for _, a := range snapshot.Body.CustomRanges {
		var marker struct {
			Profile string `json:"profile"`
		}
		_ = json.Unmarshal(a.Properties, &marker)
		if a.Type != 5 || marker.Profile != colleafIntentProfile {
			ranges = append(ranges, a)
			continue
		}
		if found {
			return snapshot, fmt.Errorf("duplicate native intent carrier")
		}
		found = true
		var carrier colleafIntentCarrier
		if err := strictSFAttrs(a.Properties, &carrier); err != nil {
			return snapshot, err
		}
		if carrier.Version != 1 {
			return snapshot, fmt.Errorf("%w: native intent version%d", ErrUnsupported, carrier.Version)
		}
		index, err := newColleafTextIndex(ctx, snapshot.Body.DataStream)
		if err != nil {
			return snapshot, err
		}
		// Verify the scalar range before treating the annotation as metadata. TextX
		// may expand/move an annotation during editing; its bounds do not own text.
		if _, err = index.inclusive(a.StartIndex, a.EndIndex); err != nil {
			return snapshot, err
		}
		for _, record := range carrier.Records {
			if !colleafOwnIntent(record.Name) {
				return snapshot, fmt.Errorf("unknown record in native intent carrier %q", record.Name)
			}
			snapshot.Resources = append(snapshot.Resources, record)
		}
	}
	snapshot.Body.CustomRanges = ranges
	return snapshot, nil
}
func emptyColleafBuiltinResource(resource colleafResource) bool {
	switch resource.Name {
	case "DOC_OBJECT_PERMISSION_PLUGIN", "DOC_WORD_STYLES_PLUGIN", "DOC_NOTE_PLUGIN":
		value := strings.TrimSpace(resource.Data)
		return value == "" || value == "{}" || value == "null"
	}
	return false
}

// The registered drawing resource may mirror a subset of persisted story
// drawings. Ignore it only after checking every mirrored descriptor and order ID.
func checkColleafDrawingMirror(snapshot colleafNoteSnapshot, resource colleafResource) error {
	var mirror struct {
		Data  map[string]json.RawMessage `json:"data"`
		Order []string                   `json:"order"`
	}
	if err := strictSFAttrs(json.RawMessage(resource.Data), &mirror); err != nil {
		return err
	}
	sources := map[string]json.RawMessage{}
	add := func(drawings map[string]json.RawMessage) error {
		for id, raw := range drawings {
			if _, ok := sources[id]; ok {
				return fmt.Errorf("ambiguous native drawing ID across stories")
			}
			sources[id] = raw
		}
		return nil
	}
	if err := add(snapshot.Drawings); err != nil {
		return err
	}
	for _, note := range snapshot.Notes {
		if err := add(note.Drawings); err != nil {
			return err
		}
	}
	for id, raw := range mirror.Data {
		source, ok := sources[id]
		if !ok {
			return fmt.Errorf("native drawing resource lacks persisted source %q", id)
		}
		var a, b any
		if err := json.Unmarshal(raw, &a); err != nil {
			return err
		}
		if err := json.Unmarshal(source, &b); err != nil {
			return err
		}
		if !reflect.DeepEqual(a, b) {
			return fmt.Errorf("contradictory native drawing resource/source %q", id)
		}
	}
	seen := map[string]bool{}
	for _, id := range mirror.Order {
		if seen[id] {
			return fmt.Errorf("duplicate native drawing resource order")
		}
		if _, ok := mirror.Data[id]; !ok {
			return fmt.Errorf("native drawing resource order lacks data")
		}
		seen[id] = true
	}
	return nil
}
