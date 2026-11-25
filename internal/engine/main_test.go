package engine

import (
	"fmt"
	"os"
	"testing"
)

// The reference corpus and optional QA outputs are repository-wide resources.
// Set one stable working directory before any tests or fuzz workers start.
func TestMain(m *testing.M) {
	if err := os.Chdir("../.."); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
