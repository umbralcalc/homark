package spine

import (
	"path/filepath"
	"testing"
)

func TestLoadCompletionsAnnual(t *testing.T) {
	path := filepath.Join("testdata", "minimal_completions_annual.csv")
	m, err := LoadCompletionsAnnual(path)
	if err != nil {
		t.Fatal(err)
	}
	if m["E08000035"][2020] == "" {
		t.Fatal("expected Leeds 2020 completions")
	}
}
