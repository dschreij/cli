package gen

import (
	"bytes"
	_ "database/sql"
	_ "database/sql/driver"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"
	_ "gorm.io/gorm"
	"gorm.io/gorm/schema"
)

var allowedInterfaces = []types.Type{
	loadNamedType("", "database/sql", "Scanner"),
	loadNamedType("", "database/sql/driver", "Valuer"),
	loadNamedType("", "gorm.io/gorm", "Valuer"),
	loadNamedType("", "gorm.io/gorm/schema", "SerializerInterface"),
}

type ExtractedSQL struct {
	Raw    string
	Where  string
	Select string
}

func extractSQL(comment string, methodName string) ExtractedSQL {
	comment = strings.TrimSpace(comment)

	if index := strings.Index(comment, "\n\n"); index != -1 {
		if strings.Contains(comment[index+2:], methodName) {
			comment = comment[:index]
		} else {
			comment = comment[index+2:]
		}
	}

	sql := strings.TrimPrefix(comment, methodName)
	if strings.HasPrefix(sql, "where(") && strings.HasSuffix(sql, ")") {
		content := strings.TrimSuffix(strings.TrimPrefix(sql, "where("), ")")
		content = strings.Trim(content, "\"")
		content = strings.TrimSpace(content)
		return ExtractedSQL{Where: content}
	} else if strings.HasPrefix(sql, "select(") && strings.HasSuffix(sql, ")") {
		content := strings.TrimSuffix(strings.TrimPrefix(sql, "select("), ")")
		content = strings.Trim(content, "\"")
		content = strings.TrimSpace(content)
		return ExtractedSQL{Select: content}
	}
	return ExtractedSQL{Raw: sql}
}

// ImplementsAllowedInterfaces reports whether typ or *typ implements any allowed interface.
func ImplementsAllowedInterfaces(typ types.Type) bool {
	if ptr, ok := typ.(*types.Pointer); ok {
		typ = ptr.Elem()
	}
	for _, t := range allowedInterfaces {
		iface, _ := t.Underlying().(*types.Interface)
		if types.Implements(typ, iface) || types.Implements(types.NewPointer(typ), iface) {
			return true
		}
	}
	return false
}

func findGoModDir(filename string) string {
	cmd := exec.Command("go", "env", "GOMOD")
	cmd.Dir = filepath.Dir(filename)
	out, _ := cmd.Output()
	return filepath.Dir(string(out))
}

// getCurrentPackagePath gets the full import path of the current file's package
func getCurrentPackagePath(filename string) string {
	cfg := &packages.Config{
		Mode: packages.NeedName,
		Dir:  findGoModDir(filename),
	}

	pkgs, err := packages.Load(cfg, filepath.Dir(filename))
	if err == nil && len(pkgs) > 0 && pkgs[0].PkgPath != "" {
		return pkgs[0].PkgPath
	}
	return ""
}

// loadNamedType returns a named type from a package with basic caching.
func loadNamedType(modRoot, pkgPath, name string) types.Type {
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedName | packages.NeedDeps,
		Dir:  modRoot,
	}

	pkgs, err := packages.Load(cfg, pkgPath)
	if err != nil || len(pkgs) == 0 || pkgs[0].Types == nil {
		return nil
	}
	if obj := pkgs[0].Types.Scope().Lookup(name); obj != nil {
		return obj.Type()
	}
	return nil
}

type loadedStruct struct {
	st      *ast.StructType
	imports []Import
	err     error
}

var (
	loadedStructs   = map[string]loadedStruct{}
	loadedStructsMu sync.Mutex
)

// loadNamedStructType loads a struct type declaration by name from a package, together with the
// imports of the file declaring it, so callers can resolve the aliases its field types use.
//
// Memoised per module root, package and name: one base struct is commonly embedded by every model
// in a package, and each embedding would otherwise load the package again, which shells out to the
// go command.
func loadNamedStructType(modRoot, pkgPath, name string) (*ast.StructType, []Import, error) {
	key := modRoot + "\x00" + pkgPath + "\x00" + name

	// The lock guards the map, never the load: holding it across a load would serialise every
	// caller behind one shell-out to the go command. Two callers racing on the same key both
	// load, which costs one redundant load and no correctness.
	loadedStructsMu.Lock()
	cached, ok := loadedStructs[key]
	loadedStructsMu.Unlock()
	if ok {
		return cached.st, cached.imports, cached.err
	}

	st, imports, err := loadNamedStructTypeUncached(modRoot, pkgPath, name)

	loadedStructsMu.Lock()
	loadedStructs[key] = loadedStruct{st: st, imports: imports, err: err}
	loadedStructsMu.Unlock()

	return st, imports, err
}

func loadNamedStructTypeUncached(modRoot, pkgPath, name string) (*ast.StructType, []Import, error) {
	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedName,
		Dir:  modRoot,
	}

	pkgs, err := packages.Load(cfg, pkgPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load package %q from %v: %w", pkgPath, modRoot, err)
	}

	if len(pkgs) == 0 {
		return nil, nil, fmt.Errorf("no packages found for path %q from %v", pkgPath, modRoot)
	}

	for _, pkg := range pkgs {
		for _, syntax := range pkg.Syntax {
			for _, decl := range syntax.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, spec := range gen.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if ok && ts.Name.Name == name {
						if st, ok := ts.Type.(*ast.StructType); ok {
							return st, fileImports(syntax), nil
						}
					}
				}
			}
		}
	}

	return nil, nil, fmt.Errorf("struct %s not found in package %s", name, pkgPath)
}

// newImport builds an Import from a spec, keeping an explicit alias when there is one.
func newImport(spec *ast.ImportSpec) Import {
	importPath, _ := strconv.Unquote(spec.Path.Value)
	name := path.Base(importPath)
	if spec.Name != nil {
		name = spec.Name.Name
	}
	return Import{Name: name, Path: importPath}
}

// fileImports returns the imports declared by a parsed file.
func fileImports(f *ast.File) []Import {
	imports := make([]Import, 0, len(f.Imports))
	for _, spec := range f.Imports {
		imports = append(imports, newImport(spec))
	}
	return imports
}

// generateDBName generates database column name using GORM's NamingStrategy and COLUMN tag.
func generateDBName(fieldName, gormTag string) string {
	tagSettings := schema.ParseTagSetting(reflect.StructTag(gormTag).Get("gorm"), ";")
	if tagSettings["COLUMN"] != "" {
		return tagSettings["COLUMN"]
	}

	// Use GORM's NamingStrategy with IdentifierMaxLength: 64
	ns := schema.NamingStrategy{IdentifierMaxLength: 64}
	return ns.ColumnName("", fieldName)
}

// mergeImports appends imports from src into dst if not already present (by Path)
func mergeImports(dst *[]Import, src []Import) {
	existing := map[string]bool{}
	for _, i := range *dst {
		existing[i.Path] = true
	}
	for _, i := range src {
		if !existing[i.Path] {
			*dst = append(*dst, i)
			existing[i.Path] = true
		}
	}
}

// shouldSkipFile checks if a file contains the generated code header and should be skipped
func shouldSkipFile(filePath string) bool {
	if !strings.HasSuffix(filePath, ".go") {
		return true
	}

	content, err := os.ReadFile(filePath)
	return err == nil && bytes.Contains(content, []byte(codeGenHint))
}

// strLit returns the unquoted string if expr is a string literal; otherwise "".
func strLit(expr ast.Expr) string {
	if bl, ok := expr.(*ast.BasicLit); ok && bl.Kind == token.STRING {
		if s, err := strconv.Unquote(bl.Value); err == nil {
			return s
		}
	}

	return ""
}

func stripGeneric(s string) string {
	if i := strings.Index(s, "["); i >= 0 {
		return s[:i]
	}
	return s
}
