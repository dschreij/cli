package gen

import (
	"regexp"
	"testing"
)

func TestEmbeddedStructDeclaredInAnotherFile(t *testing.T) {
	content := generateFromSources(t, map[string]string{
		"base.go": "package sample\n\nimport \"time\"\n\n" +
			"type Base struct {\n\tID        uint `gorm:\"primaryKey\"`\n\tCreatedAt time.Time\n}\n",
		"model.go": "package sample\n\n" +
			"type Article struct {\n\tBase\n\tTitle string\n}\n\n" +
			"type Comment struct {\n\t*Base\n\tBody string\n}\n",
	})

	for _, want := range [][2]string{
		{"ID", "field.Number[uint]"},
		{"CreatedAt", "field.Time"},
		{"Title", "field.String"},
		{"Body", "field.String"},
	} {
		if !containsField(content, want[0], want[1]) {
			t.Errorf("expected helper %s %s, got:\n%s", want[0], want[1], content)
		}
	}

	// Base gets its own helper, and both the value and the pointer embedding carry its fields.
	if got := len(regexp.MustCompile(`(?m)^\s*ID\s+field\.Number\[uint\]\s*$`).FindAllString(content, -1)); got != 3 {
		t.Errorf("expected ID helper on Base, Article and Comment, found %d, got:\n%s", got, content)
	}
}
