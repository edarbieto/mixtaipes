package prompts

import (
	"os/exec"
	"strings"
	"testing"
)

// TestNoCatalogDependency enforces ADR-007: nothing that builds LLM input may
// depend, directly or transitively, on catalog code.
func TestNoCatalogDependency(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	deps := strings.Fields(string(out))
	if len(deps) == 0 {
		t.Fatal("go list returned no packages")
	}
	for _, dep := range deps {
		if strings.HasSuffix(dep, "/internal/catalog") || strings.Contains(dep, "/internal/catalog/") {
			t.Errorf("prompts depends on %s (ADR-007)", dep)
		}
	}
}
