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

func TestToggleSetAll_StagesEverythingWhenNoneStaged(t *testing.T) {
	set := map[int]bool{}
	toggleSetAll(set, []int{1, 2, 3})
	if len(set) != 3 || !set[1] || !set[2] || !set[3] {
		t.Fatalf("expected all 3 staged, got %+v", set)
	}
}

func TestToggleSetAll_UnstagesEverythingWhenAllStaged(t *testing.T) {
	set := map[int]bool{1: true, 2: true, 3: true}
	toggleSetAll(set, []int{1, 2, 3})
	if len(set) != 0 {
		t.Fatalf("expected all unstaged, got %+v", set)
	}
}

func TestToggleSetAll_PartiallyStagedGoesToAllStaged(t *testing.T) {
	set := map[int]bool{2: true}
	toggleSetAll(set, []int{1, 2, 3})
	if len(set) != 3 || !set[1] || !set[2] || !set[3] {
		t.Fatalf("expected a partial selection to become fully staged, got %+v", set)
	}
}

func TestToggleSetAll_OnlyTouchesGivenIDs(t *testing.T) {
	// A pre-existing staged entry outside the given ids (e.g. staged before
	// a filter was applied) must survive an unrelated toggleSetAll call.
	set := map[int]bool{99: true}
	toggleSetAll(set, []int{1, 2})
	if !set[99] {
		t.Fatal("expected an unrelated pre-existing entry to be left alone")
	}
	if !set[1] || !set[2] {
		t.Fatalf("expected ids 1 and 2 staged, got %+v", set)
	}
}

func TestToggleSetAll_EmptyIDsIsANoOp(t *testing.T) {
	set := map[int]bool{1: true}
	toggleSetAll(set, nil)
	if len(set) != 1 || !set[1] {
		t.Fatalf("expected an empty ids slice to change nothing, got %+v", set)
	}
}
