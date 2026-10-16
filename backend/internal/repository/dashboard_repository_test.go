package repository

import (
	"errors"
	"testing"
)

// fakePgError implements the structural pgError interface used by
// isUndefinedTable (SQLState() string).
type fakePgError struct {
	state string
}

func (e *fakePgError) Error() string { return "pg error " + e.state }
func (e *fakePgError) SQLState() string {
	return e.state
}

func TestIsUndefinedTable(t *testing.T) {
	if !isUndefinedTable(&fakePgError{state: "42P01"}) {
		t.Error("42P01 should be detected as undefined table")
	}
	if isUndefinedTable(&fakePgError{state: "23505"}) {
		t.Error("23505 (unique violation) should NOT be undefined table")
	}
	if isUndefinedTable(errors.New("some other error")) {
		t.Error("non-pg error should NOT be undefined table")
	}
	if isUndefinedTable(nil) {
		t.Error("nil should NOT be undefined table")
	}
}
