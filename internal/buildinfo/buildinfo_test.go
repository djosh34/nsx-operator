package buildinfo

import "testing"

func TestProjectNameIdentifiesOperator(t *testing.T) {
	if ProjectName() != "nsx-operator" {
		t.Fatalf("ProjectName() = %q, want %q", ProjectName(), "nsx-operator")
	}
}
