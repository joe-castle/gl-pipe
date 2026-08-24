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
