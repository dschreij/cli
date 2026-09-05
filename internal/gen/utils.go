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
	typ = instantiateGeneric(typ)
	for _, t := range allowedInterfaces {
		iface, _ := t.Underlying().(*types.Interface)
		if types.Implements(typ, iface) || types.Implements(types.NewPointer(typ), iface) {
			return true
		}
	}
	return false
}

// instantiateGeneric instantiates a generic named type with any for each of its type
// parameters, so that its method set can be asked about: types.Implements is unspecified for
// an uninstantiated generic type. The constraints are not checked, because the question is
// which methods the type has, and Value and Scan do not mention the type parameters. A type
// that is not generic, or is already instantiated, is returned as it is.
func instantiateGeneric(typ types.Type) types.Type {
	named, ok := typ.(*types.Named)
	if !ok || named.TypeParams().Len() == 0 || named.TypeArgs().Len() > 0 {
		return typ
	}
	args := make([]types.Type, named.TypeParams().Len())
	for i := range args {
		args[i] = types.Universe.Lookup("any").Type()
	}
	instance, err := types.Instantiate(nil, named, args, false)
	if err != nil {
		return typ
	}
	return instance
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

// loadStructFromPackage loads a struct type definition from an external package by name
func loadNamedStructType(modRoot, pkgPath, name string) (*ast.StructType, error) {
	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedName,
		Dir:  modRoot,
	}

	pkgs, err := packages.Load(cfg, pkgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load package %q from %v: %w", pkgPath, modRoot, err)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found for path %q from %v", pkgPath, modRoot)
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
							return st, nil
						}
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("struct %s not found in package %s", name, pkgPath)
}

// hasSerializerTag reports whether the gorm tag declares a serializer, which makes the field a
// single serialized column regardless of its Go type.
func hasSerializerTag(fieldTag string) bool {
	settings := schema.ParseTagSetting(reflect.StructTag(fieldTag).Get("gorm"), ";")
	_, ok := settings["SERIALIZER"]
	return ok
}

// isNamedTypeCandidate reports whether a type expression could name a type a package scope
// holds, so that a container does not cost a package load to find nothing.
func isNamedTypeCandidate(goType, typName string) bool {
	if typName == "" || typName == "map" {
		return false
	}

	return !strings.HasPrefix(goType, "[]") && !strings.HasPrefix(goType, "map[")
}

// shortTypeName strips import paths from a type expression while keeping its shape:
// "[]*example.com/pkg.T" becomes "[]*pkg.T", where path.Base alone would drop the prefix.
func shortTypeName(goType string) string {
	switch {
	case strings.HasPrefix(goType, "[]"):
		return "[]" + shortTypeName(goType[2:])
	case strings.HasPrefix(goType, "*"):
		return "*" + shortTypeName(goType[1:])
	case strings.HasPrefix(goType, "map["):
		if end := closingBracket(goType, 3); end > 0 {
			return "map[" + shortTypeName(goType[4:end]) + "]" + shortTypeName(goType[end+1:])
		}
	}
	if open := strings.Index(goType, "["); open > 0 && strings.HasSuffix(goType, "]") {
		args := splitTypeArgs(goType[open+1 : len(goType)-1])
		for i, arg := range args {
			args[i] = shortTypeName(arg)
		}
		return path.Base(goType[:open]) + "[" + strings.Join(args, ", ") + "]"
	}
	return path.Base(goType)
}

// splitTypeArgs splits the arguments of a generic type at the commas of its own level, leaving
// the commas of a nested Pair[K, V] alone.
func splitTypeArgs(s string) []string {
	var (
		args  []string
		depth int
		start int
	)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(args, strings.TrimSpace(s[start:]))
}

// closingBracket returns the index of the bracket closing the one at open, or -1.
func closingBracket(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
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
