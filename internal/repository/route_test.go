package repository

import (
	"testing"
)

// ── whereBuilder (unit tests, no DB needed) ────────────────────────

func TestWhereBuilder_InitialState(t *testing.T) {
	wb := newWhereBuilder("loc-123")

	if len(wb.conds) != 2 {
		t.Fatalf("initial conditions = %d, want 2", len(wb.conds))
	}
	if wb.conds[0] != "r.location_id = $1" {
		t.Errorf("conds[0] = %q, want %q", wb.conds[0], "r.location_id = $1")
	}
	if wb.conds[1] != "r.deleted_at IS NULL" {
		t.Errorf("conds[1] = %q, want %q", wb.conds[1], "r.deleted_at IS NULL")
	}
	if len(wb.args) != 1 {
		t.Fatalf("args len = %d, want 1", len(wb.args))
	}
	if wb.args[0] != "loc-123" {
		t.Errorf("args[0] = %v, want %q", wb.args[0], "loc-123")
	}
	if wb.argN != 2 {
		t.Errorf("argN = %d, want 2", wb.argN)
	}
}

func TestWhereBuilder_AddEq(t *testing.T) {
	wb := newWhereBuilder("loc-1")

	wb.addEq("r.wall_id", "wall-1")

	if len(wb.conds) != 3 {
		t.Fatalf("conditions after addEq = %d, want 3", len(wb.conds))
	}
	if wb.conds[2] != "r.wall_id = $2" {
		t.Errorf("conds[2] = %q, want %q", wb.conds[2], "r.wall_id = $2")
	}
	if len(wb.args) != 2 {
		t.Fatalf("args after addEq = %d, want 2", len(wb.args))
	}
	if wb.args[1] != "wall-1" {
		t.Errorf("args[1] = %v, want %q", wb.args[1], "wall-1")
	}
	if wb.argN != 3 {
		t.Errorf("argN = %d, want 3", wb.argN)
	}
}

func TestWhereBuilder_AddEq_EmptyValue(t *testing.T) {
	wb := newWhereBuilder("loc-1")

	wb.addEq("r.wall_id", "") // empty should be skipped

	if len(wb.conds) != 2 {
		t.Errorf("conditions after empty addEq = %d, want 2 (no change)", len(wb.conds))
	}
	if len(wb.args) != 1 {
		t.Errorf("args after empty addEq = %d, want 1 (no change)", len(wb.args))
	}
}

func TestWhereBuilder_MultipleAdds(t *testing.T) {
	wb := newWhereBuilder("loc-1")

	wb.addEq("r.wall_id", "wall-1")
	wb.addEq("r.status", "active")
	wb.addEq("r.route_type", "") // should be skipped
	wb.addEq("r.grading_system", "v_scale")

	// Initial 2 + 3 non-empty adds = 5
	if len(wb.conds) != 5 {
		t.Fatalf("conditions = %d, want 5", len(wb.conds))
	}
	// Args: loc-1, wall-1, active, v_scale = 4
	if len(wb.args) != 4 {
		t.Fatalf("args = %d, want 4", len(wb.args))
	}
	// argN should be at 5 (next would be $5)
	if wb.argN != 5 {
		t.Errorf("argN = %d, want 5", wb.argN)
	}

	// Check positional parameter numbering
	if wb.conds[2] != "r.wall_id = $2" {
		t.Errorf("conds[2] = %q, want %q", wb.conds[2], "r.wall_id = $2")
	}
	if wb.conds[3] != "r.status = $3" {
		t.Errorf("conds[3] = %q, want %q", wb.conds[3], "r.status = $3")
	}
	if wb.conds[4] != "r.grading_system = $4" {
		t.Errorf("conds[4] = %q, want %q", wb.conds[4], "r.grading_system = $4")
	}
}

func TestWhereBuilder_AddGte(t *testing.T) {
	wb := newWhereBuilder("loc-1")

	wb.addGte("r.date_set", "2026-01-01")

	if len(wb.conds) != 3 {
		t.Fatalf("conditions = %d, want 3", len(wb.conds))
	}
}

func TestWhereBuilder_AddGte_EmptyValue(t *testing.T) {
	wb := newWhereBuilder("loc-1")

	wb.addGte("r.date_set", "")

	if len(wb.conds) != 2 {
		t.Errorf("conditions = %d, want 2 (empty should be skipped)", len(wb.conds))
	}
}

func TestWhereBuilder_AddGte_Operator(t *testing.T) {
	wb := newWhereBuilder("loc-1")
	wb.addGte("r.date_set", "2026-01-01")

	if wb.conds[2] != "r.date_set >= $2" {
		t.Errorf("conds[2] = %q, want %q", wb.conds[2], "r.date_set >= $2")
	}
}

func TestWhereBuilder_AddLte(t *testing.T) {
	wb := newWhereBuilder("loc-1")
	wb.addLte("r.date_set", "2026-12-31")

	if len(wb.conds) != 3 {
		t.Fatalf("conditions = %d, want 3", len(wb.conds))
	}
	if wb.conds[2] != "r.date_set <= $2" {
		t.Errorf("conds[2] = %q, want %q", wb.conds[2], "r.date_set <= $2")
	}
}

func TestWhereBuilder_AddLte_EmptyValue(t *testing.T) {
	wb := newWhereBuilder("loc-1")
	wb.addLte("r.date_set", "")

	if len(wb.conds) != 2 {
		t.Errorf("conditions = %d, want 2 (empty should be skipped)", len(wb.conds))
	}
}

func TestWhereBuilder_AddIn(t *testing.T) {
	wb := newWhereBuilder("loc-1")
	wb.addIn("r.status", []string{"active", "flagged"})

	if len(wb.conds) != 3 {
		t.Fatalf("conditions = %d, want 3", len(wb.conds))
	}
	// Should produce: r.status IN ($2,$3)
	if wb.conds[2] != "r.status IN ($2,$3)" {
		t.Errorf("conds[2] = %q, want %q", wb.conds[2], "r.status IN ($2,$3)")
	}
	if len(wb.args) != 3 {
		t.Fatalf("args = %d, want 3", len(wb.args))
	}
	if wb.args[1] != "active" {
		t.Errorf("args[1] = %v, want %q", wb.args[1], "active")
	}
	if wb.args[2] != "flagged" {
		t.Errorf("args[2] = %v, want %q", wb.args[2], "flagged")
	}
}

func TestWhereBuilder_AddIn_EmptySlice(t *testing.T) {
	wb := newWhereBuilder("loc-1")
	wb.addIn("r.status", []string{})

	if len(wb.conds) != 2 {
		t.Errorf("conditions = %d, want 2 (empty slice should be skipped)", len(wb.conds))
	}
}

func TestWhereBuilder_AddIn_SingleValue(t *testing.T) {
	wb := newWhereBuilder("loc-1")
	wb.addIn("r.status", []string{"active"})

	if wb.conds[2] != "r.status IN ($2)" {
		t.Errorf("conds[2] = %q, want %q", wb.conds[2], "r.status IN ($2)")
	}
}

func TestWhereBuilder_Clause(t *testing.T) {
	wb := newWhereBuilder("loc-1")
	wb.addEq("r.wall_id", "wall-1")

	clause := wb.clause()
	expected := "r.location_id = $1 AND r.deleted_at IS NULL AND r.wall_id = $2"
	if clause != expected {
		t.Errorf("clause() = %q, want %q", clause, expected)
	}
}

func TestWhereBuilder_NextArg(t *testing.T) {
	wb := newWhereBuilder("loc-1")
	if wb.nextArg() != 2 {
		t.Errorf("nextArg() = %d, want 2", wb.nextArg())
	}

	wb.addEq("r.wall_id", "wall-1")
	if wb.nextArg() != 3 {
		t.Errorf("nextArg() = %d, want 3", wb.nextArg())
	}

	wb.addIn("r.status", []string{"a", "b", "c"})
	if wb.nextArg() != 6 {
		t.Errorf("nextArg() = %d, want 6", wb.nextArg())
	}
}

func TestWhereBuilder_MixedOperators(t *testing.T) {
	wb := newWhereBuilder("loc-1")
	wb.addEq("r.wall_id", "wall-1")
	wb.addGte("r.date_set", "2026-01-01")
	wb.addLte("r.date_set", "2026-12-31")
	wb.addIn("r.status", []string{"active", "flagged"})

	// 2 initial + 4 adds = 6
	if len(wb.conds) != 6 {
		t.Fatalf("conditions = %d, want 6", len(wb.conds))
	}
	// Args: loc-1, wall-1, 2026-01-01, 2026-12-31, active, flagged = 6
	if len(wb.args) != 6 {
		t.Fatalf("args = %d, want 6", len(wb.args))
	}
	// argN = 7 (next param would be $7)
	if wb.argN != 7 {
		t.Errorf("argN = %d, want 7", wb.argN)
	}
}
