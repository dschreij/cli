package gen

import (
	"strings"
	"testing"
)

func TestProcessSkipsTestFiles(t *testing.T) {
	content := generateFromSources(t, map[string]string{
		"model.go":      "package sample\n\ntype Article struct {\n\tID    uint\n\tTitle string\n}\n",
		"model_test.go": "package sample\n\ntype Fixture struct {\n\tID uint\n}\n",
	})

	if !strings.Contains(content, "var Article = struct") {
		t.Fatalf("expected helpers for Article, got:\n%s", content)
	}
	if strings.Contains(content, "var Fixture = struct") {
		t.Fatalf("test-only type Fixture must not get a helper, got:\n%s", content)
	}
	// And the broad form as well: a leak could surface as a field type rather than as a helper
	// block of its own, which a check for the block alone would not see.
	if strings.Contains(content, "Fixture") {
		t.Fatalf("no trace of a test-only type may reach the generated package, got:\n%s", content)
	}
}
