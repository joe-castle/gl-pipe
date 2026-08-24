package components

// toggleSet flips a "staged" marker for key: turning it on sets it true,
// turning it off deletes the key entirely rather than setting it false.
//
// A map[K]bool where "unstaged" is stored as an explicit false rather than
// an absent key quietly breaks every len(set) == 0 check ("is anything
// staged?") and every len(set) count shown to the user — toggling the same
// key on and off leaves the key in the map forever, so the count only ever
// grows and "nothing staged" is never true again.
func toggleSet[K comparable](set map[K]bool, key K) {
	if set[key] {
		delete(set, key)
	} else {
		set[key] = true
	}
}
