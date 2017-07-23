package restic

import "testing"

func TestIDMethods(t *testing.T) {
	var id ID

	if id.Str() != "[null]" {
		t.Errorf("ID.Str() returned wrong value, want %v, got %v", "[null]", id.Str())
	}

	var pid *ID
	if pid.Str() != "[nil]" {
		t.Errorf("ID.Str() returned wrong value, want %v, got %v", "[nil]", pid.Str())
	}
}
