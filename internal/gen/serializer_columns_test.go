package gen

import "testing"

func TestSerializedFieldsAreColumns(t *testing.T) {
	content := generateFromSources(t, map[string]string{
		"model.go": "package sample\n\n" +
			"type Attr struct {\n\tKey   string\n\tValue string\n}\n\n" +
			"type Meta struct {\n\tNote string\n}\n\n" +
			"type Options map[string]string\n\n" +
			"type Doc struct {\n" +
			"\tID    uint\n" +
			"\tAttrs []Attr   `gorm:\"serializer:json\"`\n" +
			"\tMeta  *Meta    `gorm:\"type:jsonb;serializer:json\"`\n" +
			"\tTags  []string `gorm:\"serializer:json\"`\n" +
			"\tOpts  Options `gorm:\"serializer:json\"`\n" +
			"\tRaw   Options\n" +
			"\tLinks []Attr\n" +
			"}\n",
	})

	for _, want := range [][2]string{
		{"Attrs", "field.Field[[]sample.Attr]"},
		{"Meta", "field.Field[sample.Meta]"},
		{"Tags", "field.Field[[]string]"},
		{"Opts", "field.Field[sample.Options]"},
		{"Raw", "field.Struct[sample.Options]"},
		{"Links", "field.Slice[sample.Attr]"},
	} {
		if !containsField(content, want[0], want[1]) {
			t.Errorf("expected helper %s %s, got:\n%s", want[0], want[1], content)
		}
	}
}

func TestShortTypeName(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"string", "string"},
		{"example.com/pkg.T", "pkg.T"},
		{"[]*example.com/pkg.T", "[]*pkg.T"},
		{"map[string]example.com/pkg.T", "map[string]pkg.T"},
		{"map[example.com/a.K][]example.com/b.V", "map[a.K][]b.V"},
		{"gorm.io/datatypes.JSONSlice[int]", "datatypes.JSONSlice[int]"},
		{"gorm.io/datatypes.JSONType[example.com/pkg.T]", "datatypes.JSONType[pkg.T]"},
	} {
		if got := shortTypeName(tt.in); got != tt.want {
			t.Errorf("shortTypeName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
