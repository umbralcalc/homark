package ladata

import "testing"

func TestLoadTargets(t *testing.T) {
	t.Run("embedded targets load and are non-empty", func(t *testing.T) {
		got, err := LoadTargets()
		if err != nil {
			t.Fatal(err)
		}
		if len(got) < 3 {
			t.Fatalf("expected at least 3 targets, got %d", len(got))
		}
		for _, a := range got {
			if a.Name == "" || a.AreaCode == "" {
				t.Fatalf("invalid authority: %+v", a)
			}
		}
	})
}
