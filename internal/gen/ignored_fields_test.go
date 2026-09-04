package gen

import (
	"strings"
	"testing"
)

func TestIgnoredFieldsGetNoHelpers(t *testing.T) {
	content := generateFromSources(t, map[string]string{
		"model.go": "package sample\n\ntype Article struct {\n" +
			"\tID     uint\n" +
			"\tDraft  string `gorm:\"-\"`\n" +
			"\tCache  string `gorm:\"-:all\"`\n" +
			"\tLegacy string `gorm:\"-:migration\"`\n" +
			"}\n",
	})

	for _, absent := range []string{"Draft", "Cache"} {
		if strings.Contains(content, absent) {
			t.Errorf("field %s is ignored by GORM and must not get a helper, got:\n%s", absent, content)
		}
	}
	if !containsField(content, "Legacy", "field.String") {
		t.Errorf("field Legacy only skips migrations and must keep its helper, got:\n%s", content)
	}
}
