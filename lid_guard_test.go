package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestLidSourceSafetyGuard(t *testing.T) {
	forbidden := []string{"-a " + "disablesleep", "osa" + "script", "/" + "Library/"}
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != "." || d.Name() == "resources" || d.Name() == "build" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, "_test.go") {
			for _, word := range forbidden {
				if strings.Contains(string(data), word) {
					t.Errorf("test contains prohibited command/path at %s", path)
				}
			}
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, data, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CompositeLit:
				if typ, ok := n.Type.(*ast.SelectorExpr); ok && typ.Sel.Name == "PMSet" && path != filepath.Join("cmd", "cubbix-helper", "main.go") {
					t.Errorf("production write capability constructed in %s", path)
				}
				if typ, ok := n.Type.(*ast.Ident); ok && typ.Name == "commandSleepForcer" && path != "lid_restore_darwin.go" {
					t.Errorf("production recovery capability constructed in %s", path)
				}
			case *ast.CallExpr:
				fn, ok := n.Fun.(*ast.SelectorExpr)
				if !ok || (fn.Sel.Name != "Command" && fn.Sel.Name != "CommandContext") {
					return true
				}
				base, ok := fn.X.(*ast.Ident)
				if !ok || base.Name != "exec" {
					return true
				}
				pos := 0
				if fn.Sel.Name == "CommandContext" {
					pos = 1
				}
				if len(n.Args) <= pos {
					return true
				}
				lit, ok := n.Args[pos].(*ast.BasicLit)
				if !ok {
					if path != filepath.Join("cmd", "cubbix-helper", "main.go") {
						t.Errorf("dynamic executable in %s", path)
					}
					return true
				}
				name, _ := strconv.Unquote(lit.Value)
				if strings.HasSuffix(name, "launch"+"ctl") {
					t.Errorf("direct service command in %s", path)
				}
				if strings.HasSuffix(name, forbidden[1]) && path != "lid_restore_darwin.go" {
					t.Errorf("authorization outside fixed adapter in %s", path)
				}
				if strings.HasSuffix(name, "pmset") {
					for _, arg := range n.Args[pos+1:] {
						if text, ok := arg.(*ast.BasicLit); ok && text.Value == strconv.Quote("-a") {
							t.Errorf("direct power write in %s", path)
						}
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
func TestLidImmutableAuthorizationScripts(t *testing.T) {
	if lidUpdateScript != lidInstallScript {
		t.Fatal("install/update drift")
	}
	data, err := os.ReadFile("lid_scripts.go")
	if err != nil {
		t.Fatal(err)
	}
	f, err := parser.ParseFile(token.NewFileSet(), "lid_scripts.go", data, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range f.Decls {
		g, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		if g.Tok != token.CONST {
			t.Fatal("authorization script is mutable")
		}
		for _, spec := range g.Specs {
			v := spec.(*ast.ValueSpec)
			for _, value := range v.Values {
				switch value.(type) {
				case *ast.BasicLit, *ast.Ident:
				default:
					t.Fatal("authorization contains interpolation")
				}
			}
		}
	}
	for _, body := range []string{lidInstallScript, lidUninstallScript, lidForceScript} {
		if strings.Contains(body, "%") || strings.Contains(body, "cubbix://") {
			t.Fatal("runtime placeholder")
		}
	}
	verify := strings.Index(lidInstallScript, "codesign -v")
	rename := strings.Index(lidInstallScript, "/bin/mv")
	plist := strings.Index(lidInstallScript, "/bin/cat")
	bootstrap := strings.Index(lidInstallScript, "/bin/launch"+"ctl bootstrap")
	if verify < 0 || rename < verify || plist < rename || bootstrap < plist || !strings.Contains(lidInstallScript, ".new") || !strings.HasPrefix(lidInstallScript, "set -eu") {
		t.Fatal("install trust order")
	}
}
