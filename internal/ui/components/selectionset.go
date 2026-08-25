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

// toggleSetAll stages every key in ids, unless every one of them is
// already staged, in which case it unstages all of them instead — one key
// toggles between "select all (visible)" and "select none", the way a
// tri-state "select all" checkbox usually behaves. ids is meant to be
// whatever's currently visible (post-filter), not necessarily the full
// underlying set, so selecting all respects an active filter.
func toggleSetAll[K comparable](set map[K]bool, ids []K) {
	all := len(ids) > 0
	for _, id := range ids {
		if !set[id] {
			all = false
			break
		}
	}
	for _, id := range ids {
		if all {
			delete(set, id)
		} else {
			set[id] = true
		}
	}
}
