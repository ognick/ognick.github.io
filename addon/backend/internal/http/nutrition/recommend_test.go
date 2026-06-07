package nutrition

import (
	"testing"
)

func TestComputeRequestHash(t *testing.T) {
	h1 := computeRequestHash("u1", "2026-W23", `{"calories":2000}`, "2026-06-07 10:00:00")
	h2 := computeRequestHash("u1", "2026-W23", `{"calories":2000}`, "2026-06-07 10:00:00")
	h3 := computeRequestHash("u1", "2026-W23", `{"calories":2500}`, "2026-06-07 10:00:00")

	if h1 != h2 {
		t.Error("same inputs should produce same hash")
	}
	if h1 == h3 {
		t.Error("different inputs should produce different hash")
	}
	if len(h1) != 64 {
		t.Errorf("SHA256 hash should be 64 hex chars, got %d", len(h1))
	}
}
