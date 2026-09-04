package gen

import (
	"strings"
	"testing"
)

func TestNamedTypesOverBasicKindsGetScalarHelpers(t *testing.T) {
	content := generateFromSources(t, map[string]string{
		"model.go": "package sample\n\nimport (\n\t\"encoding/json\"\n\t\"time\"\n)\n\n" +
			"type Status int32\n\ntype Enabled bool\n\ntype Code string\n\n" +
			"type Event struct {\n" +
			"\tID   uint\n" +
			"\tKind Status\n" +
			"\tPrev *Status\n" +
			"\tWait time.Duration\n" +
			"\tDay  time.Weekday\n" +
			"\tNum  json.Number\n" +
			"\tOn   Enabled\n" +
			"\tCode Code\n" +
			"\tRaw  json.RawMessage\n" +
			"}\n",
	})

	for _, want := range [][2]string{
		{"Kind", "field.Number[sample.Status]"},
		{"Prev", "field.Number[sample.Status]"},
		{"Wait", "field.Number[time.Duration]"},
		{"Day", "field.Number[time.Weekday]"},
		{"Num", "field.String"},
		{"On", "field.Bool"},
		{"Code", "field.String"},
		{"Raw", "field.Bytes"},
	} {
		if !containsField(content, want[0], want[1]) {
			t.Errorf("expected helper %s %s, got:\n%s", want[0], want[1], content)
		}
	}
	if strings.Contains(content, "field.Struct[") {
		t.Errorf("no field of Event is an association, got:\n%s", content)
	}
}
