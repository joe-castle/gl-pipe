package components

import "testing"

func TestToggleSet_OnThenOffRemovesTheKey(t *testing.T) {
	set := map[int]bool{}

	toggleSet(set, 42)
	if !set[42] || len(set) != 1 {
		t.Fatalf("expected key present and true after first toggle, got %+v", set)
	}

	toggleSet(set, 42)
	if len(set) != 0 {
		t.Fatalf("expected the key deleted (not left as false) after second toggle, got %+v", set)
	}
}

func TestToggleSet_RepeatedCyclesNeverLeakEntries(t *testing.T) {
	set := map[string]bool{}
	for i := 0; i < 10; i++ {
		toggleSet(set, "a")
		toggleSet(set, "a")
	}
	if len(set) != 0 {
		t.Fatalf("expected 0 entries after 10 on/off cycles, got %d", len(set))
	}
}
