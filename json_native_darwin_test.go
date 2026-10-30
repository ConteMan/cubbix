//go:build darwin

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestJSONHiddenWebKit(t *testing.T) {
	dir := os.Getenv("CUBBIX_JSON_EVIDENCE")
	if dir == "" {
		dir = t.TempDir()
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "CUBBIX_JSON_NATIVE_HELPER=1", "CUBBIX_JSON_EVIDENCE="+dir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hidden WebKit checks: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "PASS: hidden WebKit editor") {
		t.Fatalf("helper exited before completing checks: %s", output)
	}
	t.Log(string(output))
}
func jsonNativeMain() int {
	dir := os.Getenv("CUBBIX_JSON_EVIDENCE")
	mygo.App.SetName("Cubbix")
	mygo.App.SetPath(mygo.PathUserData, filepath.Join(dir, "native-profile"))
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	a, err := newApplication(filepath.Join(dir, "config.toml"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	a.setupJSON()
	mygo.App.OnWindowAllClosed(func() {})
	reads := 0
	a.jsonReadClipboard = func() string {
		reads++
		return `{"data":{"items":[{"val":"alpha"},{"val":"beta"},{"val":"gamma"}]},"total":3,"active":true}`
	}
	a.jsonCopy = func(value string) error {
		if !strings.Contains(value, "items") {
			return fmt.Errorf("copy mismatch")
		}
		return nil
	}
	a.jsonChooseSave = func(*mygo.Window) (string, error) { return filepath.Join(dir, "saved.json"), nil }
	// No test opens a native panel or a confirmation sheet.
	a.jsonChooseOpen = func(*mygo.Window) (string, error) { return filepath.Join(dir, "opened.json"), nil }
	mygo.App.WhenReady(func() {
		mygo.App.SetLanguages("zh-CN", "en")
		mygo.App.SetLocale("zh-CN")
		mygo.Theme.SetSource(mygo.ThemeLight)
		d := a.newJSONDocumentFromClipboard()
		go func() {
			err := jsonNativeChecks(a, d, dir, &reads)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			mygo.RunOnMain(func() {
				for _, doc := range append([]*jsonDocument(nil), a.jsonDocuments...) {
					a.destroyJSON(doc)
				}
				a.quitting = true
				a.updates.close()
				mygo.App.Quit()
			})
		}()
	})
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func jsonNativeChecks(a *application, d *jsonDocument, dir string, reads *int) error {
	page := d.window.Page()
	eval := func(code string) (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return page.EvalContext(ctx, code)
	}
	want := func(code string) error {
		v, err := eval(code)
		if err != nil {
			return fmt.Errorf("%s: %w", code, err)
		}
		if v != true {
			return fmt.Errorf("check failed: %s => %+v", code, v)
		}
		return nil
	}
	wait := func(code string) error {
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			v, err := eval(code)
			if err == nil && v == true {
				return nil
			}
			time.Sleep(40 * time.Millisecond)
		}
		v, err := eval(`({url:location.href,ready:document.readyState,body:document.body?.outerHTML.slice(0,2000),resources:performance.getEntriesByType("resource").map(x=>x.name),hasCM:!!document.querySelector(".cm-editor"),errors:window.jsonErrors,bridge:typeof mygo})`)
		return fmt.Errorf("hidden page did not settle: %s; %+v (%v)", code, v, err)
	}
	if err := wait("!!window.cubbix && window.cubbix.state().status === ''"); err != nil {
		return err
	}
	checkHidden := func() error {
		var err error
		mygo.RunOnMain(func() {
			nsapp := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
			if objc.Send[bool](nsapp, objc.RegisterName("isActive")) {
				err = fmt.Errorf("helper became active")
			}
			for _, doc := range a.jsonDocuments {
				if doc.window.IsVisible() || doc.window.IsFocused() {
					err = fmt.Errorf("JSON window became visible or focused")
				}
			}
		})
		return err
	}
	waitConfigSplit := func(expected int) error {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			var value int
			mygo.RunOnMain(func() { value = a.config.JSONSplit })
			if value == expected {
				return nil
			}
			time.Sleep(30 * time.Millisecond)
		}
		return fmt.Errorf("split width %d was not saved", expected)
	}
	if err := checkHidden(); err != nil {
		return err
	}
	if err := want(`document.querySelector('meta[http-equiv]').content.includes("connect-src 'none'")`); err != nil {
		return err
	}
	if err := want("window.cubbix.editor().state.doc.toString().includes('alpha') && document.querySelectorAll('#tools button').length===4"); err != nil {
		return err
	}
	capture := func(name string) error {
		expected := tokens(strings.HasSuffix(name, "dark"), "en").ColorCard
		if err := wait(fmt.Sprintf("getComputedStyle(document.documentElement).getPropertyValue('--ColorCard').trim() === %q", expected)); err != nil {
			return err
		}
		if err := checkHidden(); err != nil {
			return err
		}
		// Hidden windows do not receive regular animation frames. Flush the
		// pinned CodeMirror view's measurement phase before taking its page snapshot.
		if _, err := eval("window.cubbix.editor().measure()"); err != nil {
			return err
		}
		// Theme media-query changes and WebKit's compositor are asynchronous even
		// when the Worker has already replied. Keep the snapshot's named theme exact.
		time.Sleep(50 * time.Millisecond)
		data, err := d.window.CapturePage()
		if err != nil {
			return fmt.Errorf("hidden snapshot %s: %w", name, err)
		}
		return os.WriteFile(filepath.Join(dir, name+".png"), data, 0600)
	}
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			mode := "light"
			if dark {
				mode = "dark"
			}
			mygo.RunOnMain(func() {
				mygo.App.SetLocale(locale)
				if dark {
					mygo.Theme.SetSource(mygo.ThemeDark)
				} else {
					mygo.Theme.SetSource(mygo.ThemeLight)
				}
			})
			if err := wait("document.documentElement.lang === '" + locale + "'"); err != nil {
				return err
			}
			if err := want("await window.cubbix.evaluate('.data.items.length').then(v=>v.kind==='number'&&v.text==='3')"); err != nil {
				return err
			}
			if err := capture("json-number-" + locale + "-" + mode); err != nil {
				return err
			}
			if err := want("await window.cubbix.evaluate('.data.items.map(x=>x.val)').then(v=>v.kind==='array')"); err != nil {
				return err
			}
			if err := capture("json-array-" + locale + "-" + mode); err != nil {
				return err
			}
			if err := want("await window.cubbix.evaluate('.data').then(v=>v.kind==='object')"); err != nil {
				return err
			}
			if err := capture("json-object-" + locale + "-" + mode); err != nil {
				return err
			}
			if err := want("await window.cubbix.evaluate('.missing').then(v=>v.kind==='undefined')"); err != nil {
				return err
			}
			if err := capture("json-undefined-" + locale + "-" + mode); err != nil {
				return err
			}
			if err := want("await window.cubbix.evaluate('.missing.length').then(v=>v.kind==='error')"); err != nil {
				return err
			}
			if _, err := eval("document.getElementById('status').dispatchEvent(new PointerEvent('pointerover',{bubbles:true}))"); err != nil {
				return err
			}
			time.Sleep(480 * time.Millisecond)
			if err := capture("json-error-" + locale + "-" + mode); err != nil {
				return err
			}
			if err := want("await window.cubbix.evaluate(' && (()=>{while(true){}})()').then(v=>v.kind==='timeout')"); err != nil {
				return err
			}
			if _, err := eval("document.getElementById('status').dispatchEvent(new PointerEvent('pointerover',{bubbles:true}))"); err != nil {
				return err
			}
			time.Sleep(480 * time.Millisecond)
			if err := capture("json-timeout-" + locale + "-" + mode); err != nil {
				return err
			}
			if err := want("await window.cubbix.evaluate('.data.items.length').then(v=>v.text==='3')"); err != nil {
				return err
			}
		}
	}
	mygo.RunOnMain(func() { d.window.SetSize(672, 220) })
	if err := wait("innerWidth===672 && innerHeight===220"); err != nil {
		return err
	}
	if err := want("await window.cubbix.evaluate('.data.items').then(v=>v.kind==='array')"); err != nil {
		return err
	}
	if err := want("document.getElementById('tools').getBoundingClientRect().bottom<=innerHeight && document.getElementById('editor').getBoundingClientRect().height>0"); err != nil {
		return err
	}
	mygo.RunOnMain(func() { d.window.SetSize(840, 520) })
	if err := wait("innerWidth===840 && innerHeight===520"); err != nil {
		return err
	}
	if err := want(`(async()=>{const stale=window.cubbix.evaluate(' && (()=>{while(true){}})()');const latest=await window.cubbix.evaluate('.data.items.length');return (await stale).kind==='canceled'&&latest.text==='3'&&window.cubbix.state().result.text==='3'})()`); err != nil {
		return err
	}
	if _, err := eval("document.getElementById('fold').click()"); err != nil {
		return err
	}
	// Folding is applied in the next view update, not in the click handler.
	if err := wait("!!document.querySelector('#editor .cm-foldPlaceholder')"); err != nil {
		return err
	}
	if _, err := eval("document.getElementById('unfold').click()"); err != nil {
		return err
	}
	if err := wait("!document.querySelector('#editor .cm-foldPlaceholder')"); err != nil {
		return err
	}
	if _, err := eval("document.getElementById('copy').click();document.getElementById('minify').click()"); err != nil {
		return err
	}
	if err := want("await window.cubbix.evaluate(' && [typeof window,typeof document,typeof mygo]').then(v=>v.text.includes('undefined')&&!v.text.includes('object'))"); err != nil {
		return err
	}
	if err := want("(async()=>{await window.cubbix.pin();return window.cubbix.state().pinned})()"); err != nil {
		return err
	}
	var other *jsonDocument
	mygo.RunOnMain(func() { other = a.addJSONDocument("", "/selected/items.json", true); a.createJSONWindow(other) })
	if d.window.IsAlwaysOnTop() != true || other.window.IsAlwaysOnTop() {
		return fmt.Errorf("pin state leaked between windows")
	}
	if err := want("(async()=>{await window.cubbix.pin();return !window.cubbix.state().pinned})()"); err != nil {
		return err
	}
	if *reads != 1 {
		return fmt.Errorf("clipboard read after creation")
	}
	if err := want("(async()=>{await window.cubbix.save();return !window.cubbix.snapshot().dirty})()"); err != nil {
		return err
	}
	saved, err := os.ReadFile(filepath.Join(dir, "saved.json"))
	if err != nil || !strings.Contains(string(saved), "alpha") {
		return fmt.Errorf("bridge save failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "opened.json"), []byte("[1,2,3]"), 0600); err != nil {
		return err
	}
	if _, err := eval("window.cubbix.open()"); err != nil {
		return err
	}
	if err := wait("window.cubbix.editor().state.doc.toString().startsWith('[')"); err != nil {
		return err
	}
	// Invalid file contents remain visible and clear old results.
	if _, err := eval("window.cubbix.load('{broken',500,true)"); err != nil {
		return err
	}
	if err := wait("window.cubbix.state().status==='invalid'"); err != nil {
		return err
	}
	if err := capture("json-invalid-en-dark"); err != nil {
		return err
	}
	// Empty and long-line snapshots in both languages and themes.
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			mode := "light"
			if dark {
				mode = "dark"
			}
			mygo.RunOnMain(func() {
				mygo.App.SetLocale(locale)
				if dark {
					mygo.Theme.SetSource(mygo.ThemeDark)
				} else {
					mygo.Theme.SetSource(mygo.ThemeLight)
				}
			})
			if err := wait("document.documentElement.lang==='" + locale + "'"); err != nil {
				return err
			}
			if _, err := eval("window.cubbix.load('',600,true)"); err != nil {
				return err
			}
			if _, err := eval("window.cubbix.evaluate('')"); err != nil {
				return err
			}
			if err := capture("json-empty-" + locale + "-" + mode); err != nil {
				return err
			}
			if err := want("(()=>{window.cubbix.load(JSON.stringify({payload:'x'.repeat(120000)}),700,true);return window.cubbix.state().large && !document.getElementById('gauge').hidden && document.getElementById('fold').disabled})()"); err != nil {
				return err
			}
			if err := capture("json-large-" + locale + "-" + mode); err != nil {
				return err
			}
		}
	}
	// Measure the actual CodeMirror view in WebKit, without showing its window.
	metrics, err := eval(`(() => {
  const text='[\n'+('  {"value":"'+ 'x'.repeat(90)+'"},\n').repeat(99000)+'  {}\n]';
  const before=performance.now();window.cubbix.load(text,1000,true);const open=performance.now()-before;
  const samples=[];for(let i=0;i<20;i++){const t=performance.now();window.cubbix.editor().dispatch({changes:{from:5,to:5,insert:' '}});samples.push(performance.now()-t)}
  samples.sort((a,b)=>a-b);
  return {bytes:text.length,lines:window.cubbix.editor().state.doc.lines,openMS:open,editMedianMS:samples[10],editP95MS:samples[19],large:window.cubbix.state().large};
 })()`)
	if err != nil {
		return err
	}
	single, err := eval(`(()=>{const cases=[];for(const size of [64000,100000,120000,1000000]){const raw=JSON.stringify({payload:'x'.repeat(size)});const t=performance.now();window.cubbix.load(raw,2000+size,true);const opened=performance.now()-t;const edit=performance.now();window.cubbix.editor().dispatch({changes:{from:5,to:5,insert:' '}});cases.push({lineLength:raw.length,openMS:opened,editMS:performance.now()-edit,large:window.cubbix.state().large});}return cases})()`)
	if err != nil {
		return err
	}
	metrics.(map[string]any)["singleLines"] = single
	rawFile := "[\n" + strings.Repeat("  \""+strings.Repeat("x", 1040)+"\",\n", 10030) + "  \"end\"\n]"
	filePath := filepath.Join(dir, "10mb.json")
	if err := os.WriteFile(filePath, []byte(rawFile), 0600); err != nil {
		return err
	}
	// A selected file replaces the tool's contents directly, without a sheet.
	started := time.Now()
	mygo.RunOnMain(func() { a.replaceJSONFile(d, filePath) })
	if err := wait("window.cubbix.editor().state.doc.length > 10485760 && window.cubbix.editor().state.doc.line(2).text.includes('xxxx') && !window.cubbix.state().pending"); err != nil {
		return err
	}
	metrics.(map[string]any)["selectedFileBytes"] = len(rawFile)
	metrics.(map[string]any)["selectedFileReadyMS"] = float64(time.Since(started).Microseconds()) / 1000
	data, _ := json.MarshalIndent(metrics, "", "  ")
	fmt.Printf("WebKit CodeMirror performance: %s\n", data)
	if err := os.WriteFile(filepath.Join(dir, "performance.json"), append(data, '\n'), 0600); err != nil {
		return err
	}
	if metrics.(map[string]any)["large"] == true {
		return fmt.Errorf("10 MiB multiline unexpectedly degraded")
	}
	// Reset to a small nested document for the layout and naming checks.
	if _, err := eval(`window.cubbix.load('{"data":{"items":[1,2],"label":"alpha"},"ok":true,"none":null}',5000,true)`); err != nil {
		return err
	}
	if err := wait("!window.cubbix.state().pending && window.cubbix.state().status===''"); err != nil {
		return err
	}

	// Editor on the left, result on the right, this field under the editor.
	if err := want(`(()=>{
	  const box = id => document.getElementById(id).getBoundingClientRect();
	  const editor = box('editor'), splitter = box('splitter'), result = box('result'), input = box('expression'), tools = box('tools');
	  return editor.width > 0 && result.width > 0 && Math.abs(splitter.left - editor.right) < 1 &&
	    Math.abs(result.left - splitter.right) < 1 && Math.abs(input.width - editor.width) < 1 &&
	    input.right <= splitter.right + 1 && tools.right <= innerWidth + 1 && tools.width > 0 &&
	    document.querySelectorAll('#tools button').length === 4 &&
	    document.getElementById('splitter').getAttribute('aria-orientation') === 'vertical' &&
	    document.getElementById('result').childElementCount === 0;
	})()`); err != nil {
		return err
	}
	if err := want("Math.abs(document.getElementById('editor').getBoundingClientRect().width / document.getElementById('workspace').clientWidth - 0.6) < 0.02"); err != nil {
		return err
	}
	// Keys, strings, numbers and booleans read in their own tokens.
	if err := want(`(()=>{
	  const root = getComputedStyle(document.documentElement);
	  const toRGB = name => {
	    const hex = root.getPropertyValue(name).trim().slice(1);
	    const value = parseInt(hex, 16);
	    return 'rgb(' + ((value >> 16) & 255) + ', ' + ((value >> 8) & 255) + ', ' + (value & 255) + ')';
	  };
	  const painted = new Set([...document.querySelectorAll('#editor .cm-line span')].map(x => getComputedStyle(x).color));
	  const key = toRGB('--ColorSyntaxKey'), text = toRGB('--ColorSyntaxString'), number = toRGB('--ColorSyntaxNumber'), word = toRGB('--ColorSyntaxBool'), empty = toRGB('--ColorSyntaxNull');
	  return new Set([key, text, number, word]).size === 4 && word === empty && [key, text, number, word].every(x => painted.has(x));
	})()`); err != nil {
		return err
	}
	// Every indentation level draws one weak guide under the aligned content.
	if err := want(`(()=>{
	  const lines = [...document.querySelectorAll('#editor .cm-line.cm-guide')];
	  return lines.length > 0 && lines.every(line => Number(line.style.getPropertyValue('--indent')) > 0) &&
	    getComputedStyle(document.documentElement).getPropertyValue('--ColorSyntaxIndent').trim() !== '';
	})()`); err != nil {
		return err
	}
	// Dragging and arrow keys adjust the shared width and save the ratio.
	if _, err := eval("document.getElementById('splitter').dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowLeft',bubbles:true}))"); err != nil {
		return err
	}
	if err := want("Math.abs(window.cubbix.split().percent - 58) < 0.5"); err != nil {
		return err
	}
	if err := waitConfigSplit(58); err != nil {
		return err
	}
	if _, err := eval("window.mygo.call('JSON.Split', 5)"); err != nil {
		return err
	}
	if err := waitConfigSplit(jsonSplitMinimum); err != nil {
		return err
	}
	if _, err := eval("window.mygo.call('JSON.Split', 45)"); err != nil {
		return err
	}
	if err := waitConfigSplit(45); err != nil {
		return err
	}
	// Both windows are named for the app switcher; a new one never says "Untitled".
	var titles [3]string
	var extra *jsonDocument
	mygo.RunOnMain(func() {
		extra = a.addJSONDocument("", "", true)
		a.createJSONWindow(extra)
		titles = [3]string{d.window.Title(), other.window.Title(), extra.window.Title()}
	})
	if titles[0] != "10mb.json" || titles[1] != "items.json" || titles[2] != "JSON 3" {
		return fmt.Errorf("window titles: %q", titles)
	}
	// The page draws the same name next to the traffic lights.
	if err := want(`(()=>{
	  const title = document.getElementById('title');
	  const box = title.getBoundingClientRect(), toolbar = document.getElementById('toolbar').getBoundingClientRect();
	  return title.textContent === '10mb.json' && box.left >= 76 && box.left < toolbar.right && getComputedStyle(title).overflow === 'hidden';
	})()`); err != nil {
		return err
	}
	mygo.RunOnMain(func() { a.destroyJSON(extra) })
	// Neither deactivation nor an activation-policy change closes or hides a
	// window: only ⌘W, the red light, Close and quitting do.
	if err := checkHidden(); err != nil {
		return err
	}
	mygo.RunOnMain(func() { d.window.ShowInactive(); other.window.ShowInactive() })
	time.Sleep(250 * time.Millisecond)
	windowsState := func() (active, visible, focused bool, count int) {
		mygo.RunOnMain(func() {
			nsapp := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
			active = objc.Send[bool](nsapp, objc.RegisterName("isActive"))
			visible = d.window.IsVisible() && other.window.IsVisible()
			focused = d.window.IsFocused() || other.window.IsFocused()
			count = len(a.jsonDocuments)
		})
		return
	}
	if active, visible, focused, count := windowsState(); active || focused || !visible || count != 2 {
		return fmt.Errorf("inactive windows: active=%v focused=%v visible=%v count=%d", active, focused, visible, count)
	}
	mygo.RunOnMain(func() {
		// The user opened a window earlier, so the app is already regular.
		a.policy = mygo.ActivationPolicyRegular
		a.visibility(false)
	})
	time.Sleep(250 * time.Millisecond)
	if active, visible, focused, count := windowsState(); active || focused || !visible || count != 2 {
		return fmt.Errorf("after deactivation and policy change: active=%v focused=%v visible=%v count=%d", active, focused, visible, count)
	}
	mygo.RunOnMain(func() { d.window.Hide(); other.window.Hide() })
	if err := checkHidden(); err != nil {
		return err
	}
	// Hiding never discards documents; an explicit close does, without a prompt.
	if _, err := eval("window.cubbix.editor().dispatch({changes:{from:5,to:5,insert:' '}})"); err != nil {
		return err
	}
	if err := want("window.cubbix.snapshot().dirty"); err != nil {
		return err
	}
	mygo.RunOnMain(func() { d.window.Close() })
	if !d.window.IsDestroyed() || len(a.jsonDocuments) != 1 {
		return fmt.Errorf("dirty window did not close on request")
	}
	// An update restart also proceeds without asking about unsaved text.
	var relaunched bool
	mygo.RunOnMain(func() {
		a.relaunch = func() { relaunched = a.quitting }
		a.updates.state = updateState{Phase: updateReady}
		a.restartUpdate()
	})
	if !relaunched {
		return fmt.Errorf("update restart waited for an unsaved JSON document")
	}
	// Disabling the tile closes the remaining window and releases the shortcut.
	var registered, removed bool
	mygo.RunOnMain(func() {
		a.registerShortcut = func(string, func()) error { registered = true; return nil }
		a.unregisterShortcut = func(string) { removed = true }
		a.config.JSONEnabled = false
		a.changeJSONEnabled()
	})
	if len(a.jsonDocuments) != 0 || !other.window.IsDestroyed() || !removed || registered {
		return fmt.Errorf("disable did not close windows or unregister shortcut")
	}
	if !d.window.IsDestroyed() || !other.window.IsDestroyed() {
		return fmt.Errorf("WebKit window leaked after closing")
	}
	mygo.RunOnMain(func() {
		a.registerShortcut = mygo.GlobalShortcut.Register
		a.unregisterShortcut = mygo.GlobalShortcut.Unregister
		a.config.JSONShortcut = "Ctrl+Alt+Cmd+Shift+F9"
		a.config.JSONEnabled = true
		a.registerJSONShortcut()
	})
	if !mygo.GlobalShortcut.IsRegistered(a.config.JSONShortcut) {
		return fmt.Errorf("native JSON shortcut was not registered")
	}
	mygo.RunOnMain(func() { a.config.JSONEnabled = false; a.changeJSONEnabled() })
	if mygo.GlobalShortcut.IsRegistered(a.config.JSONShortcut) {
		return fmt.Errorf("native JSON shortcut remained registered")
	}
	fmt.Println("PASS: hidden WebKit editor, blob Worker states/timeout/recovery/isolation, split layout with saved ratio, syntax colors and indent guides, window names, focus-independent windows without save prompts, toggle cleanup; no activation")
	return nil
}

func TestJSONSettingsAndMenuScreenshots(t *testing.T) {
	dir := os.Getenv("CUBBIX_JSON_EVIDENCE")
	if dir == "" {
		t.Skip("evidence not requested")
	}
	a := awakeTestApp(t)
	a.addJSONDocument("", "", true)
	a.addJSONDocument("", "/selected/items.json", true)
	for _, locale := range []string{"zh-CN", "en"} {
		for _, dark := range []bool{false, true} {
			mode := "light"
			if dark {
				mode = "dark"
			}
			for _, kind := range []string{"tiles", "shortcuts", "menu"} {
				a.page = 2
				view := a.view
				w, h := 720, 340
				if kind == "shortcuts" {
					a.page = 3
				}
				if kind == "menu" {
					view = a.menuEvidenceView
					w, h = 352, 400
				}
				tt := ui.NewTester(view, w, h)
				tt.SetScale(2)
				tt.SetLocale(locale)
				tt.SetDark(dark)
				f, err := os.Create(filepath.Join(dir, kind+"-"+locale+"-"+mode+".png"))
				if err != nil {
					t.Fatal(err)
				}
				if err := png.Encode(f, tt.Image()); err != nil {
					t.Fatal(err)
				}
				f.Close()
			}
		}
	}
}
