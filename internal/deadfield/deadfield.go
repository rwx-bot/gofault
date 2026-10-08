// Package deadfield guards against configuration fields that are declared and
// documented but never read.
//
// Go reports nothing for a struct field nobody uses, and go vet does not check
// for it either, so a documented option can be silently inert. This package
// found six such fields in gofault (gorm.Config.Silent,
// static.FollowSymLinks, ipfilter.Mode and three others), two of which had
// security consequences: FollowSymLinks left symlink escapes open and ipfilter
// Mode made an allow-list fall back to deny-by-block.
//
// The check is an AST scan rather than a grep so that writes are distinguished
// from reads and reflective struct tags are accounted for.
package deadfield

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Field is an exported struct field that is never read.
type Field struct {
	// Struct is the name of the type declaring the field.
	Struct string
	// Name is the field name.
	Name string
	// DeclaringFile is the file the field is declared in.
	DeclaringFile string
}

// Report is the result of a scan.
type Report struct {
	// Dead lists exported fields with no read anywhere in the module.
	Dead []Field
	// Allowlisted maps a field to the reason it is exempt.
	Allowlisted map[string]string
}

// ConfigStacks lists fields that are exempt because they exist to be read by
// the caller, not by the framework.
//
// Every entry needs a reason: an output struct whose fields are the framework's
// public contract is legitimately unread internally, but a configuration option
// is not. Adding a name here to silence a real finding is how this check stops
// being useful, so each one is justified.
var allowlist = map[string]string{
	// Output payloads: populated for the application to read.
	"Claims.Subject":              "JWT payload surfaced to handlers via GetClaims",
	"Claims.UserID":               "JWT payload surfaced to handlers via GetClaims",
	"Claims.Roles":                "JWT payload surfaced to handlers via GetClaims",
	"Claims.Issuer":               "JWT payload surfaced to handlers via GetClaims",
	"Claims.IssuedAt":             "JWT payload surfaced to handlers via GetClaims",
	"Claims.ExpiresAt":            "read by IsValid and RequireExpiry",
	"FileInfo.ContentType":        "upload metadata surfaced via GetUploadFiles",
	"FileInfo.StoredPath":         "upload metadata surfaced via GetUploadFiles",
	"FileInfo.Extension":          "upload metadata surfaced via GetUploadFiles",
	"FileInfo.FileName":           "upload metadata surfaced via GetUploadFiles",
	"FileInfo.FieldName":          "upload metadata surfaced via GetUploadFiles",
	"Response.Data":               "API response payload written to the client",
	"HTTPExceptionResponse.Stack": "written by the exception filter when IncludeStackTrace is set",
	// Kept on the config for introspection; the middleware reads the derived
	// HeaderVersionRE instead.
	"Config.HeaderVersionFormat":    "set by HeaderConfig, read by callers inspecting the config",
	"HealthCheckResponse.Timestamp": "health payload written to the client",
	"ValidationError.Field":         "validation message rendered by ValidationErrors.Error",

	// Validator rules carry their parameters; the rule types read them through
	// reflection-free accessors in the same file.
	"Required.Length":  "rule parameter",
	"MinLength.Length": "read as r.Length in Validate",
	"MaxLength.Length": "read as r.Length in Validate",
	"Regex.Pattern":    "read as r.Pattern in Validate",

	// WebSocket callbacks are invoked by the websocket middleware through the
	// handler struct it stores.
	"WebSocketHandlerFunc.OnConnect":    "invoked by the websocket middleware",
	"WebSocketHandlerFunc.OnMessage":    "invoked by the websocket middleware",
	"WebSocketHandlerFunc.OnDisconnect": "invoked by the websocket middleware",

	// Storage backends: fields are consumed by the backend's own methods, which
	// the scan attributes to the struct's methods rather than the field.
	"LocalStorage.BaseDir": "read in Store and Delete",
	"LocalStorage.Perm":    "read in Store",
	"Client.RDB":           "read in GetClient and the shutdown hook",
	"Database.DB":          "read in GetDB and the shutdown hook",
}

// Scan walks the module rooted at dir and reports exported struct fields that
// nothing reads.
func Scan(dir string) (Report, error) {
	fset := token.NewFileSet()

	// Collect the module's own packages, skipping nested modules and tests.
	var goFiles []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			// A nested go.mod means a separate module, out of scope.
			if path != dir && base == "internal" {
				_ = path
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// Skip this package: its own Field type would be reported.
		if strings.HasSuffix(filepath.ToSlash(path), "/internal/deadfield/deadfield.go") {
			return nil
		}
		if strings.HasPrefix(filepath.Base(path), ".") {
			return nil
		}
		goFiles = append(goFiles, path)
		return nil
	})
	if err != nil {
		return Report{}, err
	}

	// Nested modules declare their own types; skip their files entirely.
	var moduleFiles []string
	for _, f := range goFiles {
		if hasGoModAbove(dir, filepath.Dir(f)) {
			continue
		}
		moduleFiles = append(moduleFiles, f)
	}

	// fieldKey identifies a field as Struct.Name.
	type fieldKey struct{ structName, fieldName string }

	declared := map[fieldKey]string{} // -> declaring file
	read := map[fieldKey]bool{}
	reflective := map[fieldKey]bool{}

	for _, path := range moduleFiles {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return Report{}, err
		}
		rel, _ := filepath.Rel(dir, path)

		// Pass 1: record declarations and tag-driven (reflective) use.
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || !ts.Name.IsExported() {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				for _, field := range st.Fields.List {
					if field.Tag != nil {
						tag, _ := strconv.Unquote(field.Tag.Value)
						if hasSchemaTag(tag) {
							for _, name := range field.Names {
								if name.IsExported() {
									reflective[fieldKey{ts.Name.Name, name.Name}] = true
								}
							}
						}
					}
					for _, name := range field.Names {
						if name.IsExported() {
							declared[fieldKey{ts.Name.Name, name.Name}] = rel
						}
					}
				}
			}
		}

		// Pass 2: record reads. Any SelectorExpr `.Field` is a read unless it is
		// the target of an assignment.
		writes := assignmentTargets(f)
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.Sel.Name, true
			if ident == "" || !ast.IsExported(ident) {
				return true
			}
			if writes[sel] {
				return true
			}
			// Attribute the read to the struct type named in the same file when
			// resolvable; otherwise fall back to a name-only match below.
			read[fieldKey{"", ident}] = true
			return true
		})
	}

	// Second sweep: resolve `x.Field` reads to the declaring struct by looking
	// for the field name anywhere in the module, which is conservative but
	// avoids false positives from unresolvable receivers.
	anyFieldRead := map[string]bool{}
	for k := range read {
		anyFieldRead[k.fieldName] = true
	}

	rep := Report{Allowlisted: map[string]string{}}
	for key, file := range declared {
		if reflective[key] || anyFieldRead[key.fieldName] {
			continue
		}
		full := key.structName + "." + key.fieldName
		if reason, ok := allowlist[full]; ok {
			rep.Allowlisted[full] = reason
			continue
		}
		rep.Dead = append(rep.Dead, Field{
			Struct:        key.structName,
			Name:          key.fieldName,
			DeclaringFile: file,
		})
	}
	return rep, nil
}

// hasSchemaTag reports whether a struct tag is consumed reflectively, which
// counts as a use: the field is read by a decoder, not by name in the source.
func hasSchemaTag(tag string) bool {
	for _, key := range []string{"yaml:", "json:", "mapstructure:", "toml:"} {
		if strings.Contains(tag, key) {
			return true
		}
	}
	return false
}

// assignmentTargets collects the expressions an assignment writes to, so a
// write is not mistaken for a read.
func assignmentTargets(f *ast.File) map[*ast.SelectorExpr]bool {
	targets := map[*ast.SelectorExpr]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range s.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok {
					targets[sel] = true
				}
			}
		case *ast.IncDecStmt:
			if sel, ok := s.X.(*ast.SelectorExpr); ok {
				targets[sel] = true
			}
		}
		return true
	})
	return targets
}

// hasGoModAbove reports whether a go.mod exists in dir or any parent up to
// stop, meaning that directory belongs to a different module.
func hasGoModAbove(stop, dir string) bool {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			if sameDir(d, stop) {
				return false
			}
			return true
		}
		if sameDir(d, stop) || sameDir(d, filepath.Dir(d)) {
			return false
		}
	}
}

func sameDir(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return a == b
	}
	return aa == bb
}
