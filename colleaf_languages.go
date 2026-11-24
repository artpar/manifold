package manydoc

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
)

const colleafLanguageProfile = "manydoc.run-languages"

// Native altLang remains authoritative. The marker identifies the SF slot it
// represents and retains the distinct authored complex-language setting, which
// the pinned engine does not expose as a separate rendering property.
type colleafLanguages struct {
	Profile         string `json:"profile"`
	Version         int    `json:"version"`
	AlternateKind   string `json:"alternateKind"`
	ComplexLanguage string `json:"complexLanguage,omitempty"`
}

func readColleafLanguages(raw json.RawMessage) (*colleafLanguages, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var value colleafLanguages
	if err := validateTypedJSON(context.Background(), raw, reflect.TypeFor[colleafLanguages]()); err != nil {
		return nil, err
	}
	if err := strictSFAttrs(raw, &value); err != nil {
		return nil, err
	}
	if value.Profile != colleafLanguageProfile || value.Version != 1 || value.AlternateKind != "eastAsia" {
		return nil, fmt.Errorf("%w: native run language intent profile/version/slot", ErrUnsupported)
	}
	if err := validateLanguage(value.ComplexLanguage); err != nil {
		return nil, err
	}
	return &value, nil
}
