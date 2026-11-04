package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var lidAppleNames = map[string]bool{"lidInstallScriptApple": true, "lidUninstallScriptApple": true, "lidForceScriptApple": true}

func lidASTText(node ast.Node) string {
	var out bytes.Buffer
	if err := format.Node(&out, token.NewFileSet(), node); err != nil {
		return ""
	}
	return out.String()
}
func lidStaticString(expr ast.Expr) (string, bool) {
	switch value := expr.(type) {
	case *ast.BasicLit:
		if value.Kind == token.STRING {
			s, err := strconv.Unquote(value.Value)
			return s, err == nil
		}
	case *ast.BinaryExpr:
		if value.Op == token.ADD {
			left, lok := lidStaticString(value.X)
			right, rok := lidStaticString(value.Y)
			return left + right, lok && rok
		}
	}
	return "", false
}
func lidCallStrings(args []ast.Expr) ([]string, bool) {
	var values []string
	for _, arg := range args {
		value, ok := lidStaticString(arg)
		if !ok {
			return nil, false
		}
		values = append(values, value)
	}
	return values, true
}
func lidSameStrings(values []string, want ...string) bool {
	if len(values) != len(want) {
		return false
	}
	for i := range want {
		if values[i] != want[i] {
			return false
		}
	}
	return true
}
func lidReadOnlyPMSet(args []string) bool {
	return lidSameStrings(args, "-g") || lidSameStrings(args, "-g", "batt")
}
func lidCapabilityName(expr ast.Expr) string {
	switch typ := expr.(type) {
	case *ast.Ident:
		return typ.Name
	case *ast.SelectorExpr:
		return typ.Sel.Name
	}
	return ""
}

// Only this exact, replaceable PMSet.Run closure may use a dynamic executable.
// Any additional command in helper main is checked normally.
func lidHelperBridge(literal *ast.CompositeLit) (*ast.FuncLit, bool) {
	if len(literal.Elts) < 1 || len(literal.Elts) > 2 {
		return nil, false
	}
	var field *ast.KeyValueExpr
	for _, elt := range literal.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			return nil, false
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			return nil, false
		}
		switch key.Name {
		case "Run":
			if field != nil {
				return nil, false
			}
			field = kv
		case "Logf":
			if lidASTText(kv.Value) != "log.Printf" {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	if field == nil {
		return nil, false
	}
	runner, ok := field.Value.(*ast.FuncLit)
	if !ok {
		return nil, false
	}
	expected, err := parser.ParseExpr(`func(ctx context.Context,name string,args ...string)([]byte,error){return exec.CommandContext(ctx,name,args...).Output()}`)
	return runner, err == nil && lidASTText(runner) == lidASTText(expected)
}

// This one fixed waiter can only wait for a PID and open an app in the
// background. No caller-supplied text is evaluated as shell code.
func lidQuitRelaunchBridge(call *ast.CallExpr) bool {
	expected, err := parser.ParseExpr("exec.Command(\"/bin/sh\", \"-c\", `while /bin/kill -0 \"$1\" 2>/dev/null; do /bin/sleep 0.1; done\nexec /usr/bin/open -g -n \"$2\"`, \"cubbix-relaunch\", fmt.Sprint(pid), bundle)")
	return err == nil && lidASTText(call) == lidASTText(expected)
}

func lidSourceViolations(path string, source []byte) []string {
	path = filepath.ToSlash(path)
	file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
	if err != nil {
		return []string{err.Error()}
	}
	var violations []string
	reject := func(message string) { violations = append(violations, message) }
	execNames := map[string]bool{}
	for _, imp := range file.Imports {
		value, _ := strconv.Unquote(imp.Path.Value)
		if value == "os/exec" {
			name := "exec"
			if imp.Name != nil {
				name = imp.Name.Name
			}
			if name == "." {
				reject("dot import of process execution")
			}
			execNames[name] = true
		}
	}
	execConstructor := func(expr ast.Expr) bool {
		selector, ok := expr.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		base, ok := selector.X.(*ast.Ident)
		return ok && execNames[base.Name] && (selector.Sel.Name == "Command" || selector.Sel.Name == "CommandContext")
	}
	permittedTypes := map[ast.Node]bool{}
	bridges := map[*ast.CallExpr]bool{}
	var parents []ast.Node
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			parents = parents[:len(parents)-1]
			return true
		}
		parents = append(parents, node)
		var function *ast.FuncDecl
		var closure *ast.FuncLit
		for _, parent := range parents {
			if fn, ok := parent.(*ast.FuncDecl); ok {
				function = fn
			}
			if fn, ok := parent.(*ast.FuncLit); ok {
				closure = fn
			}
		}
		functionName := ""
		if function != nil {
			functionName = function.Name.Name
		}
		isAdmin := false
		if path == "lid_restore_darwin.go" && functionName == "prepareLid" && closure != nil {
			for _, parent := range parents {
				if assignment, ok := parent.(*ast.AssignStmt); ok && len(assignment.Lhs) == 1 && len(assignment.Rhs) == 1 && assignment.Rhs[0] == closure {
					if name, ok := assignment.Lhs[0].(*ast.Ident); ok && name.Name == "admin" {
						isAdmin = true
					}
				}
			}
		}
		switch n := node.(type) {
		case *ast.FuncDecl:
			if n.Recv != nil {
				for _, field := range n.Recv.List {
					typ := field.Type
					if ptr, ok := typ.(*ast.StarExpr); ok {
						typ = ptr.X
					}
					name := lidCapabilityName(typ)
					if name == "PMSet" && path == "internal/lid/pmset.go" || name == "commandSleepForcer" && path == "lid_restore_darwin.go" {
						permittedTypes[typ] = true
					}
				}
			}
		case *ast.TypeSpec:
			if n.Name.Name == "PMSet" && path == "internal/lid/pmset.go" || n.Name.Name == "commandSleepForcer" && path == "lid_restore_darwin.go" {
				permittedTypes[n.Name] = true
			}
		case *ast.CompositeLit:
			name := lidCapabilityName(n.Type)
			switch name {
			case "PMSet":
				runner, valid := lidHelperBridge(n)
				if path != "cmd/cubbix-helper/main.go" || functionName != "main" || !valid {
					reject("write capability outside the exact helper bridge")
				} else {
					permittedTypes[n.Type] = true
					ast.Inspect(runner, func(child ast.Node) bool {
						if call, ok := child.(*ast.CallExpr); ok && execConstructor(call.Fun) {
							bridges[call] = true
						}
						return true
					})
				}
			case "commandSleepForcer":
				if path != "lid_restore_darwin.go" || functionName != "prepareLid" || lidASTText(n) != "commandSleepForcer{admin: admin}" {
					reject("recovery capability outside the fixed adapter")
				} else {
					permittedTypes[n.Type] = true
				}
			}
			if typ, ok := n.Type.(*ast.SelectorExpr); ok && typ.Sel.Name == "Cmd" {
				if base, ok := typ.X.(*ast.Ident); ok && execNames[base.Name] {
					reject("direct process struct bypasses command checks")
				}
			}
		case *ast.AssignStmt:
			for _, lhs := range n.Lhs {
				if field, ok := lhs.(*ast.SelectorExpr); ok && (field.Sel.Name == "Run" || field.Sel.Name == "admin" || field.Sel.Name == "Path" || field.Sel.Name == "Args") {
					reject("mutable execution capability or process arguments")
				}
				if name, ok := lhs.(*ast.Ident); ok && lidAppleNames[name.Name] {
					reject("shadowed authorization constant")
				}
			}
		case *ast.ValueSpec:
			for _, name := range n.Names {
				if lidAppleNames[name.Name] && path != "lid_scripts.go" {
					reject("shadowed authorization constant")
				}
			}
		case *ast.Field:
			for _, name := range n.Names {
				if lidAppleNames[name.Name] {
					reject("shadowed authorization parameter")
				}
			}
		case *ast.SelectorExpr:
			if n.Sel.Name == "Run" && path == "cmd/cubbix-helper/main.go" {
				reject("helper cannot borrow or mutate the injected power runner")
			}
			if n.Sel.Name == "Run" && path == "internal/lid/pmset.go" {
				if receiver, ok := n.X.(*ast.Ident); ok && receiver.Name == "p" {
					call, direct := parents[len(parents)-2].(*ast.CallExpr)
					if !direct || call.Fun != n {
						reject("aliased injected power runner")
					}
				}
			}
			if n.Sel.Name == "PMSet" && !permittedTypes[n] {
				reject("write capability type outside approved construction")
			}
			if execConstructor(n) {
				if len(parents) < 2 {
					reject("aliased process constructor")
				} else if call, ok := parents[len(parents)-2].(*ast.CallExpr); !ok || call.Fun != n {
					reject("aliased process constructor")
				}
			}
		case *ast.Ident:
			if n.Name == "PMSet" || n.Name == "commandSleepForcer" {
				selectorChild := false
				if len(parents) > 1 {
					if selector, ok := parents[len(parents)-2].(*ast.SelectorExpr); ok && selector.Sel == n {
						selectorChild = true
					}
				}
				if !selectorChild && !permittedTypes[n] {
					reject("execution capability type outside approved construction")
				}
			}
		case *ast.CallExpr:
			// PMSet's injected runner is restricted to the same fixed read/write argv.
			if method, ok := n.Fun.(*ast.SelectorExpr); ok && method.Sel.Name == "Run" && path == "internal/lid/pmset.go" {
				if receiver, ok := method.X.(*ast.Ident); ok && receiver.Name == "p" {
					if len(n.Args) == 0 {
						reject("missing power runner arguments")
						break
					}
					values, static := lidCallStrings(n.Args[1:])
					valid := static && len(values) > 0 && values[0] == "/usr/bin/pmset" && !n.Ellipsis.IsValid()
					if valid {
						valid = functionName == "Read" && lidReadOnlyPMSet(values[1:]) || functionName == "Set" && (lidSameStrings(values[1:], "-a", "disablesleep", "0") || lidSameStrings(values[1:], "-a", "disablesleep", "1"))
					}
					if !valid {
						reject("power runner escaped fixed read/write arguments")
					}
				}
			}
			if !execConstructor(n.Fun) {
				break
			}
			if bridges[n] {
				break
			}
			if path == "quit.go" && functionName == "relaunchAfterExit" && lidQuitRelaunchBridge(n) {
				break
			}
			method := n.Fun.(*ast.SelectorExpr)
			offset := 0
			if method.Sel.Name == "CommandContext" {
				offset = 1
			}
			if len(n.Args) <= offset {
				reject("missing executable")
				break
			}
			executable, static := lidStaticString(n.Args[offset])
			if !static {
				reject("dynamic executable outside exact helper bridge")
				break
			}
			args := n.Args[offset+1:]
			// Unknown executables can proxy a shell or a power-setting write.
			// Adding another process boundary requires an explicit guard update.
			switch executable {
			// /usr/bin/open: the command line tool launches the app bundle the
			// tool belongs to (006 §3.4). It is not privileged and cannot set
			// power options; it starts an app, as LaunchServices would.
			case "/bin/ps", "/usr/bin/caffeinate", "/usr/bin/codesign", "/usr/bin/pmset", "/usr/bin/" + "osa" + "script", "/usr/bin/open":
			default:
				reject("unreviewed executable or command proxy")
				break
			}
			switch filepath.Base(executable) {
			case "sh", "bash", "zsh", "dash", "ksh", "fish":
				reject("direct shell execution")
			case "launch" + "ctl", "install":
				reject("direct privileged service or install command")
			case "pmset":
				values, known := lidCallStrings(args)
				if !known || n.Ellipsis.IsValid() || !lidReadOnlyPMSet(values) {
					reject("power command is not an explicit read-only query")
				}
			case "osa" + "script":
				valid := isAdmin && executable == "/usr/bin/"+"osa"+"script" && len(args) == 2 && !n.Ellipsis.IsValid()
				if valid {
					flag, ok := lidStaticString(args[0])
					script, identifier := args[1].(*ast.Ident)
					valid = ok && flag == "-e" && identifier && lidAppleNames[script.Name]
				}
				if !valid {
					reject("authorization command is not one of three constants in the fixed adapter")
				}
			}
		}
		return true
	})
	return violations
}

func TestLidQuitWaiterBoundary(t *testing.T) {
	source, err := os.ReadFile("quit.go")
	if err != nil {
		t.Fatal(err)
	}
	if violations := lidSourceViolations("quit.go", source); len(violations) != 0 {
		t.Fatal(violations)
	}
	for _, source := range []string{
		strings.Replace(string(source), "do /bin/sleep 0.1", "do arbitrary-command", 1),
		strings.Replace(string(source), "func relaunchAfterExit(exe", "func otherFunction(exe", 1),
		strings.Replace(string(source), `exec.Command("/bin/sh"`, `exec.Command(exe`, 1),
		strings.Replace(string(source), "fmt.Sprint(pid), bundle)", "bundle, bundle)", 1),
	} {
		if len(lidSourceViolations("quit.go", []byte(source))) == 0 {
			t.Fatal("modified waiter escaped command checks")
		}
	}
}

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
		for _, violation := range lidSourceViolations(path, data) {
			t.Errorf("%s: %s", path, violation)
		}
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
		checked := strings.NewReplacer(
			`printf "cubbix-step=%s\ncubbix-exit=%s\n"`, `printf "fixed-markers"`,
			`printf 'cubbix-command-exit=%s\n'`, `printf 'fixed-marker'`,
		).Replace(body)
		if strings.Contains(checked, "%") || strings.Contains(body, "cubbix://") {
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
	steps := []string{
		"/bin/chmod 0644", "/bin/launch" + "ctl bootout",
		"while /bin/launch" + "ctl print", "unload_attempt + 1", "/bin/sleep 0.1",
		"/bin/launch" + "ctl bootstrap", "bootstrap_attempt + 1", "/bin/sleep 1",
		"if ! /bin/launch" + "ctl print", "cubbix-error=confirm-failed", "exit 1",
	}
	rest := lidInstallScript
	for _, step := range steps {
		at := strings.Index(rest, step)
		if at < 0 {
			t.Fatal("missing or reordered install step", step)
		}
		rest = rest[at+len(step):]
	}
}

// Decode only the string literal grammar used by our fixed AppleScript wrapper.
// No interpreter or system authorization is involved in these comparisons.
func lidDecodeApple(script string) ([]byte, error) {
	const prefix = `do shell script "`
	const suffix = `" with administrator privileges`
	if !strings.HasPrefix(script, prefix) || !strings.HasSuffix(script, suffix) {
		return nil, fmt.Errorf("unexpected authorization wrapper")
	}
	input := script[len(prefix) : len(script)-len(suffix)]
	out := make([]byte, 0, len(input))
	for i := 0; i < len(input); i++ {
		switch input[i] {
		case '"':
			return nil, fmt.Errorf("unescaped quote")
		case '\\':
			i++
			if i == len(input) {
				return nil, fmt.Errorf("truncated escape")
			}
			switch input[i] {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case '"', '\\':
				out = append(out, input[i])
			default:
				return nil, fmt.Errorf("unsupported escape")
			}
		default:
			out = append(out, input[i])
		}
	}
	return out, nil
}
func TestLidAppleScriptsMatchByteForByte(t *testing.T) {
	for _, pair := range []struct{ name, apple, body string }{{"install/update", lidInstallScriptApple, lidInstallScript}, {"uninstall", lidUninstallScriptApple, lidUninstallScript}, {"force", lidForceScriptApple, lidForceScript}} {
		t.Run(pair.name, func(t *testing.T) {
			decoded, err := lidDecodeApple(pair.apple)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(decoded, []byte(pair.body)) {
				t.Fatal("authorization wrapper differs from reviewed script bytes")
			}
		})
	}
	for _, script := range []string{`do shell script "x\q" with administrator privileges`, `do shell script "a" & "b" with administrator privileges`, `do shell script "x"`, `do shell script "x\" with administrator privileges`} {
		if _, err := lidDecodeApple(script); err == nil {
			t.Fatal("invalid wrapper accepted")
		}
	}
}
func TestLidGuardRejectsExecutionBypasses(t *testing.T) {
	process := func(command string) string {
		return "package main\nimport (\"os/exec\";\"context\")\nfunc bad(ctx context.Context){" + command + "}"
	}
	helper := `package main
import("context";"os/exec";"github.com/ConteMan/cubbix/internal/lid")
func main(){sleep:=lid.PMSet{Run:func(ctx context.Context,name string,args ...string)([]byte,error){return exec.CommandContext(ctx,name,args...).Output()}};_ = sleep;EXTRA}`
	appleExe := strconv.Quote("/usr/bin/" + "osa" + "script")
	admin := func(command string) string {
		return "package main\nimport \"os/exec\"\nfunc(a *application)prepareLid(){admin:=func(program string)error{" + command + ";return nil};_ = admin}"
	}
	cases := []struct{ name, path, source string }{
		{"implicit-write", "extra.go", process(`exec.Command("/usr/bin/pmset","disablesleep","1")`)},
		{"battery-write", "extra.go", process(`exec.CommandContext(ctx,"/usr/bin/pmset","-b","disablesleep","1")`)},
		{"other-setting", "extra.go", process(`exec.Command("/usr/bin/pmset","sleep","0")`)},
		{"no-query-args", "extra.go", process(`exec.Command("/usr/bin/pmset")`)},
		{"dynamic-power-args", "extra.go", process(`args:=[]string{"-g"};exec.Command("/usr/bin/pmset",args...)`)},
		{"composed-write", "extra.go", process(`exec.Command("/usr/bin/"+"pmset","sleep","0")`)},
		{"dynamic-executable", "extra.go", process(`name:="/usr/bin/pmset";exec.Command(name,"-g")`)},
		{"command-proxy", "extra.go", process(`exec.Command("/usr/bin/env","pmset","sleep","0")`)},
		{"authorization-proxy", "extra.go", process(`exec.Command("/usr/bin/sudo","pmset","sleep","0")`)},
		{"shell", "extra.go", process(`exec.Command("/bin/sh","-c","ignored")`)},
		{"shell-arg-vector", "extra.go", process(`args:=[]string{"-c","ignored"};exec.CommandContext(ctx,"/bin/sh",args...)`)},
		{"aliased-constructor", "extra.go", process(`run:=exec.Command;run("/usr/bin/pmset","sleep","0")`)},
		{"aliased-import", "extra.go", `package main;import run "os/exec";func bad(){run.Command("/usr/bin/pmset","sleep","0")}`},
		{"process-struct", "extra.go", `package main;import "os/exec";func bad(){c:=exec.Cmd{Path:"/usr/bin/pmset"};c.Run()}`},
		{"mutate-process-args", "extra.go", process(`c:=exec.Command("/usr/bin/pmset","-g");c.Args=[]string{"pmset","sleep","0"}`)},
		{"dynamic-authorization", "lid_restore_darwin.go", admin(`exec.Command(` + appleExe + `,"-e",program)`)},
		{"other-authorization-constant", "lid_restore_darwin.go", admin(`exec.Command(` + appleExe + `,"-e",otherScriptApple)`)},
		{"inline-authorization", "lid_restore_darwin.go", admin(`exec.Command(` + appleExe + `,"-e","ignored")`)},
		{"authorization-file-exemption", "lid_restore_darwin.go", process(`exec.Command(` + appleExe + `,"-e",lidForceScriptApple)`)},
		{"shadowed-authorization-constant", "lid_restore_darwin.go", admin(`lidForceScriptApple:=program;exec.Command(` + appleExe + `,"-e",lidForceScriptApple)`)},
		{"helper-extra-write", "cmd/cubbix-helper/main.go", strings.Replace(helper, "EXTRA", `exec.Command("/usr/bin/pmset","sleep","0")`, 1)},
		{"helper-extra-dynamic-command", "cmd/cubbix-helper/main.go", strings.Replace(helper, "EXTRA", `name:="ignored";exec.Command(name)`, 1)},
		{"helper-altered-bridge", "cmd/cubbix-helper/main.go", strings.Replace(strings.Replace(helper, "EXTRA", "", 1), `return exec.CommandContext(ctx,name,args...).Output()`, `args=append(args,"ignored");return exec.CommandContext(ctx,name,args...).Output()`, 1)},
		{"helper-mutates-runner", "cmd/cubbix-helper/main.go", strings.Replace(helper, "EXTRA", `sleep.Run=otherRunner`, 1)},
		{"helper-borrows-runner", "cmd/cubbix-helper/main.go", strings.Replace(helper, "EXTRA", `sleep.Run(ctx,"/usr/bin/pmset","sleep","0")`, 1)},
		{"zero-write-capability", "extra.go", `package main;import "github.com/ConteMan/cubbix/internal/lid";var s lid.PMSet;func bad(){s.Run=otherRunner}`},
		{"new-write-capability", "extra.go", `package main;import "github.com/ConteMan/cubbix/internal/lid";func bad(){s:=new(lid.PMSet);s.Run=otherRunner}`},
		{"aliased-write-type", "extra.go", `package main;import "github.com/ConteMan/cubbix/internal/lid";type X=lid.PMSet`},
		{"zero-recovery-capability", "lid_restore_darwin.go", `package main;func bad(){var f commandSleepForcer;f.admin=otherRunner}`},
		{"wrong-recovery-runner", "lid_restore_darwin.go", `package main;func(a *application)prepareLid(){f:=commandSleepForcer{admin:otherRunner};_ = f}`},
		{"changed-injected-write", "internal/lid/pmset.go", `package lid;func(p PMSet)Set(v int)error{_,err:=p.Run(ctx,"/usr/bin/pmset","sleep","0");return err}`},
		{"empty-injected-write", "internal/lid/pmset.go", `package lid;func(p PMSet)Set(v int)error{_,err:=p.Run();return err}`},
		{"aliased-injected-runner", "internal/lid/pmset.go", `package lid;func(p PMSet)Set(v int)error{run:=p.Run;_,err:=run(ctx,"/usr/bin/pmset","sleep","0");return err}`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parser.ParseFile(token.NewFileSet(), test.path, test.source, 0); err != nil {
				t.Fatalf("invalid guard fixture: %v", err)
			}
			if violations := lidSourceViolations(test.path, []byte(test.source)); len(violations) == 0 {
				t.Fatal("unsafe source fixture passed the guard")
			}
		})
	}
	allowed := []struct{ path, source string }{
		{"extra.go", process(`exec.Command("/usr/bin/pmset","-g");exec.CommandContext(ctx,"/usr/bin/pmset","-g","batt")`)},
		{"cmd/cubbix-helper/main.go", strings.Replace(helper, "EXTRA", "", 1)},
		{"lid_restore_darwin.go", admin(`exec.Command(` + appleExe + `,"-e",lidInstallScriptApple);exec.Command(` + appleExe + `,"-e",lidUninstallScriptApple);exec.Command(` + appleExe + `,"-e",lidForceScriptApple)`)},
	}
	for _, test := range allowed {
		if violations := lidSourceViolations(test.path, []byte(test.source)); len(violations) != 0 {
			t.Fatalf("approved source rejected in %s: %v", test.path, violations)
		}
	}
}
