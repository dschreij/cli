package gen

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// generateFromSources writes files into a throwaway module, runs the generator over it and
// returns the concatenated generated code. Keys are file names relative to the module root.
func generateFromSources(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/sample\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create directory for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	out := filepath.Join(dir, "out")
	g := &Generator{Files: map[string]*File{}, outPath: out}
	if err := g.Process(dir); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if err := g.Gen(); err != nil {
		t.Fatalf("Gen: %v", err)
	}
	return readGeneratedTree(t, out)
}

// readGeneratedTree concatenates every generated file under dir, the nested packages included.
// readAllGeneratedGoFiles reads the top level only, which is enough for a fixture of one package
// but silently drops the output of one that spans several.
func readGeneratedTree(t *testing.T, dir string) string {
	t.Helper()

	var b strings.Builder
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		b.WriteString(string(content))
		b.WriteString("\n\n")

		return nil
	})
	if err != nil {
		t.Fatalf("read generated tree %s: %v", dir, err)
	}
	if b.Len() == 0 {
		t.Fatalf("no .go files under %s", dir)
	}

	return b.String()
}

// containsField reports whether the generated code declares a helper named name with the given
// type, tolerating the column alignment gofmt applies to struct fields.
func containsField(content, name, typ string) bool {
	re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `\s+` + regexp.QuoteMeta(typ) + `\s*$`)
	return re.MatchString(content)
}
