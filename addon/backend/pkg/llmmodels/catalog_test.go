package llmmodels

import "testing"

func TestCatalog_HasKnownModels(t *testing.T) {
	known := []string{
		"minimax-m3", "kimi-k2.6", "deepseek-v4-flash", "qwen3.7-max", "glm-5.1",
	}
	for _, id := range known {
		meta, ok := Catalog[id]
		if !ok {
			t.Errorf("model %q missing from catalog", id)
			continue
		}
		if meta.Speed < 1 || meta.Speed > 5 {
			t.Errorf("%q: speed %d out of range 1..5", id, meta.Speed)
		}
		if meta.Intelligence < 1 || meta.Intelligence > 5 {
			t.Errorf("%q: intelligence %d out of range 1..5", id, meta.Intelligence)
		}
		if meta.Family == "" {
			t.Errorf("%q: empty family", id)
		}
	}
}

func TestDefaultMeta(t *testing.T) {
	m := DefaultMeta("nonexistent-model")
	if m.Speed != 3 || m.Intelligence != 3 {
		t.Errorf("default meta should be neutral 3/3, got %+v", m)
	}
	if m.Family != "unknown" {
		t.Errorf("default family: got %q, want unknown", m.Family)
	}
}
