package engine

import (
	"encoding/hex"
	"fmt"
	"golang.org/x/text/language"
	"strings"
	"unicode"
)

func validateDirection(value string) error {
	if value != "" && value != "ltr" && value != "rtl" {
		return fmt.Errorf("direction must be ltr, rtl or empty")
	}
	return nil
}
func validateLanguage(value string) error {
	if value == "" {
		return nil
	}
	if len(value) > 255 || strings.TrimSpace(value) != value || strings.ContainsAny(value, "_\t\r\n ") {
		return fmt.Errorf("invalid BCP47 language tag")
	}
	if _, err := language.Parse(value); err != nil {
		return fmt.Errorf("invalid BCP47 language tag %q: %w", value, err)
	}
	return nil
}
func validateTextStyle(r Run) error {
	if err := validateDirection(r.Direction); err != nil {
		return err
	}
	for _, value := range []string{r.Language, r.EastAsiaLanguage, r.ComplexLanguage} {
		if err := validateLanguage(value); err != nil {
			return err
		}
	}

	if err := validateString(r.FontFamily); err != nil {
		return err
	}
	if strings.IndexFunc(r.FontFamily, unicode.IsControl) >= 0 {
		return fmt.Errorf("font family contains control characters")
	}
	if strings.TrimSpace(r.FontFamily) != r.FontFamily {
		return fmt.Errorf("font family has surrounding whitespace")
	}
	if !finite(r.FontSize) || r.FontSize < 0 {
		return fmt.Errorf("font size must be finite and nonnegative")
	}
	if r.Color != "" && r.Color != "auto" {
		if len(r.Color) != 7 || r.Color[0] != '#' {
			return fmt.Errorf("text color must be #RRGGBB or auto")
		}
		if _, err := hex.DecodeString(r.Color[1:]); err != nil {
			return fmt.Errorf("invalid text color")
		}
	}
	return nil
}
func textRGB(color string) (int, int, int) {
	if color == "" || color == "auto" {
		return 0, 0, 0
	}
	b, _ := hex.DecodeString(color[1:])
	return int(b[0]), int(b[1]), int(b[2])
}
func textSize(r Run, fallback float64) float64 {
	if r.FontSize > 0 {
		return r.FontSize
	}
	return fallback
}

func canonicalTextColor(color string) string {
	if color == "auto" {
		return color
	}
	return strings.ToUpper(color)
}
