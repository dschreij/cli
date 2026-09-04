package gen

import (
	"strings"
	"testing"
)

// TestEmbeddedStructKeepsItsOwnFileAliases covers the case where the file declaring an embedded
// struct and the file embedding it bind the same alias to different import paths. The embedded
// field's type has to resolve through its own file, or it is looked up in the wrong package: a
// Valuer then stops being recognised and the column is generated as an association.
//
// The assertions read the embedding struct's own block. Base gets a helper of its own, built
// from its own file and so correct either way, which a search of the whole output would match
// instead.
func TestEmbeddedStructKeepsItsOwnFileAliases(t *testing.T) {
	content := generateFromSources(t, map[string]string{
		"money/money.go": "package money\n\nimport \"database/sql/driver\"\n\n" +
			"type Money struct{ Cents int64 }\n\n" +
			"func (m Money) Value() (driver.Value, error) { return m.Cents, nil }\n" +
			"func (m *Money) Scan(v any) error { return nil }\n",
		"other/other.go": "package other\n\ntype Thing struct{ Name string }\n",
		"base.go": "package sample\n\nimport m \"example.com/sample/money\"\n\n" +
			"type Base struct {\n\tAmount m.Money\n}\n",
		"model.go": "package sample\n\nimport m \"example.com/sample/other\"\n\n" +
			"type Row struct {\n\tBase\n\tID   uint\n\tOwn  m.Thing\n}\n",
	})

	row := structBlock(t, content, "Row")
	if !containsField(row, "Amount", "field.Field[money.Money]") {
		t.Errorf("the embedded field must resolve through its own file's alias, got:\n%s", row)
	}
	if !containsField(row, "Own", "field.Struct[other.Thing]") {
		t.Errorf("the embedding file's own alias must keep its meaning, got:\n%s", row)
	}
}

// structBlock returns the field list generated for one struct, so an assertion cannot be
// satisfied by the helper of a different struct that happens to carry the same field.
func structBlock(t *testing.T, content, name string) string {
	t.Helper()

	start := strings.Index(content, "var "+name+" = struct {")
	if start < 0 {
		t.Fatalf("no helper generated for %s, got:\n%s", name, content)
	}
	rest := content[start:]
	end := strings.Index(rest, "}{")
	if end < 0 {
		t.Fatalf("helper for %s is malformed, got:\n%s", name, rest)
	}

	return rest[:end]
}
