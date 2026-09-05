package gen

import (
	"strings"
	"testing"
)

func TestGenericColumnTypesKeepTheirArgumentsAndInterfaces(t *testing.T) {
	// Box implements driver.Valuer and sql.Scanner, so a Box column is one column whatever its
	// type argument, as datatypes.JSONType is. Pair implements neither and stays an association,
	// but with the arguments it was declared with rather than "any".
	content := generateFromSources(t, map[string]string{
		"model.go": "package sample\n\nimport (\n\t\"database/sql/driver\"\n\t\"encoding/json\"\n)\n\n" +
			"type Box[T any] struct{ Data T }\n\n" +
			"func (b Box[T]) Value() (driver.Value, error) { return json.Marshal(b.Data) }\n" +
			"func (b *Box[T]) Scan(v any) error { return json.Unmarshal(v.([]byte), &b.Data) }\n\n" +
			"type Pair[K comparable, V any] struct {\n\tKey K\n\tVal V\n}\n\n" +
			"type Plain struct{ Name string }\n\n" +
			"type Row struct {\n" +
			"\tID     uint\n" +
			"\tMeta   Box[map[string]string]\n" +
			"\tLocal  Box[Plain]\n" +
			"\tOpt    *Box[int]\n" +
			"\tPair   Pair[string, int]\n" +
			"\tMixed  Pair[Plain, int]\n" +
			"\tBoxes  []Box[Plain]\n" +
			"}\n",
	})

	for _, want := range [][2]string{
		{"Meta", "field.Field[sample.Box[map[string]string]]"},
		{"Local", "field.Field[sample.Box[sample.Plain]]"},
		{"Opt", "field.Field[sample.Box[int]]"},
		{"Pair", "field.Struct[sample.Pair[string, int]]"},
		{"Mixed", "field.Struct[sample.Pair[sample.Plain, int]]"},
		{"Boxes", "field.Slice[sample.Box[sample.Plain]]"},
	} {
		if !containsField(content, want[0], want[1]) {
			t.Errorf("expected helper %s %s, got:\n%s", want[0], want[1], content)
		}
	}
	if strings.Contains(content, "[any]") {
		t.Errorf("a type argument was rendered as any, got:\n%s", content)
	}
}

// TestGenericColumnTypesResolveThroughAFullImportPath covers a generic Valuer declared in another
// package. The type's package and name are split off the part before the type arguments, and that
// path is already full, so getFullImportPath finds no alias for it and returns it unchanged, which
// is exactly what the package load needs.
func TestGenericColumnTypesResolveThroughAFullImportPath(t *testing.T) {
	content := generateFromSources(t, map[string]string{
		"box/box.go": "package box\n\nimport (\n\t\"database/sql/driver\"\n\t\"encoding/json\"\n)\n\n" +
			"type Box[T any] struct{ Data T }\n\n" +
			"func (b Box[T]) Value() (driver.Value, error) { return json.Marshal(b.Data) }\n" +
			"func (b *Box[T]) Scan(v any) error { return nil }\n",
		"model.go": "package sample\n\nimport \"example.com/sample/box\"\n\n" +
			"type Row struct {\n\tID   uint\n\tMeta box.Box[map[string]string]\n}\n",
	})

	if !containsField(content, "Meta", "field.Field[box.Box[map[string]string]]") {
		t.Errorf("an external generic Valuer must be a column helper, got:\n%s", content)
	}
}

// TestMapColumnsAreNotAssociations covers a map whose key or value names a package. The type
// printer renders the map now, so the qualifier inside it would otherwise put the field down the
// association branch, which tests for a dot anywhere in the type.
func TestMapColumnsAreNotAssociations(t *testing.T) {
	content := generateFromSources(t, map[string]string{
		"model.go": "package sample\n\n" +
			"type Attr struct{ Key string }\n\n" +
			"type Row struct {\n" +
			"\tID    uint\n" +
			"\tPlain map[string]string\n" +
			"\tQual  map[string]Attr\n" +
			"\tNest  map[string][]Attr\n" +
			"}\n",
	})

	for _, want := range [][2]string{
		{"Plain", "field.Field[map[string]string]"},
		{"Qual", "field.Field[map[string]sample.Attr]"},
		{"Nest", "field.Field[map[string][]sample.Attr]"},
	} {
		if !containsField(content, want[0], want[1]) {
			t.Errorf("expected helper %s %s, got:\n%s", want[0], want[1], content)
		}
	}
	if strings.Contains(content, "field.Struct[map[") {
		t.Errorf("a map is a column, never an association, got:\n%s", content)
	}
}
