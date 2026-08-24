package components

import "github.com/charmbracelet/bubbles/table"

// setRows replaces a table's rows and re-clamps its cursor.
//
// bubbles/table's own SetRows only clamps the cursor downward (when it's
// now larger than the last row index) — it never recovers a cursor that
// was previously forced to -1 by a 0-row SetRows call. A table that starts
// empty (nothing synced yet) and later receives its first real rows is
// left permanently unable to select anything otherwise: Cursor() stays -1,
// every "highlighted row" lookup fails, and every key bound to the
// highlighted row silently does nothing.
func setRows(t *table.Model, rows []table.Row) {
	t.SetRows(rows)
	t.SetCursor(t.Cursor())
}

// tableColumnOverhead is bubbles/table's per-column padding (Padding(0, 1)
// in its default Styles, 1 space each side) that isn't part of a Column's
// declared Width — every flex-width calculation below has to account for
// it or the row silently overflows the table's actual viewport width.
const tableColumnOverhead = 2

// flexColumnWidth computes how wide a flexible text column (e.g. a repo
// path) should be, given the table's total available width, the widths of
// every other (fixed) column, and a floor so it never collapses to
// nothing on a narrow terminal.
//
// Every gl-pipe table used to declare its widest, most-important column
// (repo path, job name, ...) with a hard-coded constant — fine at the
// width someone happened to test with, silently truncating (with a "…")
// on anything narrower, and wasting the rest of the row as blank space on
// anything wider. This is the fix: recompute columns from the real
// terminal width every time it changes, rather than once at construction.
func flexColumnWidth(total int, fixed []int, min int) int {
	sum := 0
	for _, w := range fixed {
		sum += w
	}
	flex := total - sum - tableColumnOverhead*(len(fixed)+1)
	if flex < min {
		return min
	}
	return flex
}

// splitFlexWidth divides the width left over after every fixed column
// between two flexible columns (e.g. Project and Job name), in the given
// ratio (a's share, 0–1), each respecting its own floor.
func splitFlexWidth(total int, fixed []int, ratio float64, minA, minB int) (a, b int) {
	sum := 0
	for _, w := range fixed {
		sum += w
	}
	remaining := total - sum - tableColumnOverhead*(len(fixed)+2)
	if remaining < minA+minB {
		return minA, minB
	}
	a = int(float64(remaining) * ratio)
	if a < minA {
		a = minA
	}
	b = remaining - a
	if b < minB {
		b = minB
	}
	return a, b
}
