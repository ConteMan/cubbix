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
	if !strings.Contains(string(output), "PASS: hidden WebKit editor") || !strings.Contains(string(output), "PASS: actual quit with dirty JSON tool") {
		t.Fatalf("helper exited before completing checks: %s", output)
	}
	t.Log(string(output))
}
func jsonNativeMain() int {
	dir := os.Getenv("CUBBIX_JSON_EVIDENCE")
	profile, err := os.MkdirTemp("", "cubbix-json-profile-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(profile)
	mygo.App.SetName("Cubbix")
	mygo.App.SetPath(mygo.PathUserData, filepath.Join(profile, "native-profile"))
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	a, err := newApplication(filepath.Join(profile, "config.toml"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	a.setupJSON()
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		a.beforeQuit(e)
		fmt.Println("PASS: actual quit with dirty JSON tool, no confirmation")
	})
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
				// Quit a dirty tool through the production OnBeforeQuit handler.
				dirty := a.addJSONDocument("{broken", "", false)
				a.createJSONWindow(dirty)
				a.quitting = false
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
	space12 := int(tokens(false, "zh-CN").Space12)
	space16 := int(tokens(false, "zh-CN").Space16)
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
	// A fresh window has no this result: no preview, no separator, and the
	// editor owns the whole workspace row.
	if err := want(`(()=>{
	  const box = id => document.getElementById(id).getBoundingClientRect();
	  const state = window.cubbix.state();
	  return !state.preview && state.status === '' &&
	    getComputedStyle(document.getElementById('result')).display === 'none' &&
	    getComputedStyle(document.getElementById('splitter')).display === 'none' &&
	    Math.abs(box('editor').width - box('workspace').width) < 1;
	})()`); err != nil {
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
			// The error is one alert icon in the preview; its reason stays in the tooltip.
			if err := want(`(()=>{
			  const pane = document.getElementById('result'), status = document.getElementById('status');
			  return window.cubbix.state().preview && getComputedStyle(pane).display !== 'none' &&
			    status.classList.contains('error') && !!status.querySelector('svg') && pane.textContent.trim() === '';
			})()`); err != nil {
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
	if err := wait("(window.cubbix.editor().measure(), !!document.querySelector('#editor .cm-foldPlaceholder'))"); err != nil {
		return err
	}
	if _, err := eval("document.getElementById('unfold').click()"); err != nil {
		return err
	}
	if err := wait("(window.cubbix.editor().measure(), !document.querySelector('#editor .cm-foldPlaceholder'))"); err != nil {
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
			// An emptied document drops the preview again; the editor fills the row.
			if err := want(`(()=>{
			  const box = id => document.getElementById(id).getBoundingClientRect();
			  return !window.cubbix.state().preview &&
			    getComputedStyle(document.getElementById('result')).display === 'none' &&
			    Math.abs(box('editor').width - box('workspace').width) < 1;
			})()`); err != nil {
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

	// A result brings the separator and the preview.
	if err := want("await window.cubbix.evaluate('.data.items.length').then(v=>v.kind==='number')"); err != nil {
		return err
	}
	// Editor left, preview right; one full-width footer stops both at its top.
	if err := want(fmt.Sprintf(`(()=>{
	  const box = id => document.getElementById(id).getBoundingClientRect();
	  const centre = r => (r.top + r.bottom) / 2;
	  const editor = box('editor'), splitter = box('splitter'), result = box('result');
	  const workspace = box('workspace'), expression = box('expression'), input = box('suffix'), tools = box('tools');
	  const footer = document.querySelector('footer').getBoundingClientRect();
	  return window.cubbix.state().preview && editor.width > 0 && result.width > 0 &&
	    Math.abs(splitter.left - editor.right) < 1 && Math.abs(result.left - splitter.right) < 1 &&
	    Math.abs(splitter.bottom - footer.top) < 1 && Math.abs(editor.bottom - footer.top) < 1 &&
	    Math.abs(result.bottom - footer.top) < 1 && Math.abs(workspace.bottom - footer.top) < 1 &&
	    footer.left <= 0.5 && footer.right >= innerWidth - 0.5 && Math.abs(footer.bottom - innerHeight) < 1 &&
	    Math.abs(expression.left - footer.left - %d) < 1 && Math.abs(input.left - expression.left) > 1 &&
	    input.width > editor.width / 2 && Math.abs(tools.left - input.right - %d) < 1 &&
	    Math.abs(innerWidth - tools.right - %d) < 1 && Math.abs(centre(input) - centre(tools)) < 1 &&
	    document.querySelectorAll('#tools button').length === 4 &&
	    document.getElementById('splitter').getAttribute('aria-orientation') === 'vertical';
	})()`, space12, space12, space12)); err != nil {
		return err
	}
	if err := want("Math.abs(document.getElementById('editor').getBoundingClientRect().width / document.getElementById('workspace').clientWidth - 0.6) < 0.02"); err != nil {
		return err
	}
	// The line number (44) and the fold arrow (24) share one row and its centre.
	gutter, err := eval(`(()=>{
	  const centre = r => (r.top + r.bottom) / 2;
	  const element = (selector, text) => [...document.querySelectorAll(selector)]
	    .find(candidate => getComputedStyle(candidate).visibility !== 'hidden' && (!text || candidate.textContent === text));
	  const number = element('#editor .cm-lineNumbers .cm-gutterElement', '1');
	  const fold = element('#editor .cm-foldGutter .cm-gutterElement', '');
	  const glyph = target => {
	    const range = document.createRange();
	    range.selectNodeContents(target);
	    return range.getBoundingClientRect();
	  };
	  const numberBox = number.getBoundingClientRect(), foldBox = fold.getBoundingClientRect();
	  const numberGlyph = glyph(number), foldGlyph = glyph(fold.firstElementChild || fold);
	  return {
	    number: {width: numberBox.width, align: getComputedStyle(number).alignItems, display: getComputedStyle(number).display, centre: centre(numberGlyph)},
	    fold: {width: foldBox.width, align: getComputedStyle(fold).alignItems, display: getComputedStyle(fold).display, centre: centre(foldGlyph)},
	    rows: Math.abs(centre(numberBox) - centre(foldBox)),
	  };
	})()`)
	if err != nil {
		return err
	}
	measured := gutter.(map[string]any)
	number := measured["number"].(map[string]any)
	fold := measured["fold"].(map[string]any)
	within := func(rect map[string]any, key string, want float64) bool {
		got := rect[key].(float64)
		return got >= want-1 && got <= want+1
	}
	centred := func(rect map[string]any) bool {
		return rect["align"] == "center" && rect["display"] == "flex"
	}
	if !within(number, "width", 44) || !within(fold, "width", 24) || !centred(number) || !centred(fold) ||
		!within(fold, "centre", number["centre"].(float64)) || measured["rows"].(float64) > 1 {
		return fmt.Errorf("gutter geometry: %+v", measured)
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
	  return lines.length > 0 && lines.every(line => {
      const levels = Number(line.style.getPropertyValue('--indent'));
      const style = getComputedStyle(line), step = line.style.getPropertyValue('--indent-step');
      const expected = Math.floor((line.textContent.match(/^ */)[0].length) / 2);
      return levels === expected && levels > 0 && step === '2ch' &&
        style.backgroundImage.includes('gradient') && parseFloat(style.backgroundSize) > 0;
    }) &&
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
	// Wide layouts and pointer events must stay within the same persisted limits.
	mygo.RunOnMain(func() { d.window.SetSize(1600, 520) })
	if err := wait("innerWidth===1600"); err != nil {
		return err
	}
	if err := want(`(()=>{
      const bar=document.getElementById('splitter');
      // Synthetic pointers cannot acquire capture, so stub capture only; the
      // production down/move/up handlers and native persistence still run.
      const capture=bar.setPointerCapture;
      bar.setPointerCapture=()=>{};
      try {
        bar.dispatchEvent(new PointerEvent('pointerdown',{pointerId:1,bubbles:true}));
        bar.dispatchEvent(new PointerEvent('pointermove',{pointerId:1,clientX:1599,bubbles:true}));
        bar.dispatchEvent(new PointerEvent('pointerup',{pointerId:1,bubbles:true}));
      } finally { bar.setPointerCapture=capture; }
      return window.cubbix.split().percent===75;
    })()`); err != nil {
		return err
	}
	if err := waitConfigSplit(75); err != nil {
		return err
	}
	persisted, err := loadConfig(a.path)
	if err != nil || persisted.JSONSplit != 75 {
		return fmt.Errorf("split disk roundtrip: %+v, %v", persisted.JSONSplit, err)
	}
	mygo.RunOnMain(func() { d.window.SetSize(840, 520) })
	if err := wait("innerWidth===840"); err != nil {
		return err
	}
	// Scalar results use the same syntax tokens as object/array contents.
	for _, dark := range []bool{false, true} {
		mygo.RunOnMain(func() {
			if dark {
				mygo.Theme.SetSource(mygo.ThemeDark)
			} else {
				mygo.Theme.SetSource(mygo.ThemeLight)
			}
		})
		expected := tokens(dark, "en").ColorCard
		if err := wait(fmt.Sprintf("getComputedStyle(document.documentElement).getPropertyValue('--ColorCard').trim() === %q", expected)); err != nil {
			return err
		}
		for _, item := range []struct{ suffix, token string }{{".data.label", "ColorSyntaxString"}, {".data.items.length", "ColorSyntaxNumber"}, {".ok", "ColorSyntaxBool"}, {".none", "ColorSyntaxNull"}} {
			if err := want(fmt.Sprintf(`(async()=>{
              await window.cubbix.evaluate(%q);
              const pre=document.querySelector('#result pre');
              const probe=document.createElement('span');probe.style.color='var(--'+%q+')';document.body.append(probe);
              const same=!!pre&&getComputedStyle(pre).color===getComputedStyle(probe).color;
              probe.remove();return same;
            })()`, item.suffix, item.token)); err != nil {
				return err
			}
		}
	}
	// A window without a file shows the fixed name, never a number.
	var titles [3]string
	var extra *jsonDocument
	mygo.RunOnMain(func() {
		extra = a.addJSONDocument("", "", true)
		a.createJSONWindow(extra)
		titles = [3]string{d.window.Title(), other.window.Title(), extra.window.Title()}
	})
	if titles[0] != "10mb.json" || titles[1] != "items.json" || titles[2] != "JSON" {
		return fmt.Errorf("window titles: %q", titles)
	}
	// The page draws the same name, centred on the 44-high bar and inset past
	// the traffic lights by 12 + 52 + Space16.
	if err := want(fmt.Sprintf(`(()=>{
	  const title = document.getElementById('title');
	  const box = title.getBoundingClientRect(), toolbar = document.getElementById('toolbar').getBoundingClientRect();
	  const centre = r => (r.top + r.bottom) / 2;
	  return title.textContent === '10mb.json' && Math.abs(box.left - %d) < 1 && box.left < toolbar.right &&
	    Math.abs(centre(box) - centre(toolbar)) < 1 && Math.abs(centre(box) - 22) < 1 &&
	    getComputedStyle(title).overflow === 'hidden';
	})()`, space12+52+space16)); err != nil {
		return err
	}
	// One click edits the name in place: Enter confirms through the bridge.
	if err := want(`(async()=>{
	  const click = () => document.getElementById('title').dispatchEvent(new MouseEvent('click',{bubbles:true}));
	  const field = () => document.getElementById('title-edit');
	  const key = (target, name) => target.dispatchEvent(new KeyboardEvent('keydown',{key:name,bubbles:true}));
	  click();
	  const fieldBox = field().getBoundingClientRect(), fieldStyle = getComputedStyle(field());
	  const opened = !field().hidden && window.cubbix.state().editing && document.getElementById('title').hidden &&
	    document.activeElement === field() && Math.abs(fieldBox.height - 24) < 1 &&
	    fieldStyle.borderTopWidth === '1px' && fieldBox.top > 0;
	  field().value = 'orders view';
	  key(field(), 'Enter');
	  await new Promise(resolve => setTimeout(resolve, 200));
	  return opened && field().hidden && !document.getElementById('title').hidden &&
	    document.getElementById('title').textContent === 'orders view' && window.cubbix.state().name === 'orders view';
	})()`); err != nil {
		return err
	}
	// The window, the card list and the palette command share the new name.
	var renamed string
	mygo.RunOnMain(func() { renamed = d.window.Title() })
	if renamed != "orders view" {
		return fmt.Errorf("renamed window title %q", renamed)
	}
	if rows := a.menubarPanelRows(); rows.documents[0] != d || d.name(a, "en") != "orders view" {
		return fmt.Errorf("card list kept the old name")
	}
	for _, command := range a.commands() {
		if command.ID == fmt.Sprintf("json.window.%d", d.number) && command.Title(a, "en") != "orders view" {
			return fmt.Errorf("palette command kept the old name")
		}
	}
	// Esc cancels, an empty name restores the default, another file brings its
	// own name.
	if err := want(`(async()=>{
	  const click = () => document.getElementById('title').dispatchEvent(new MouseEvent('click',{bubbles:true}));
	  const field = () => document.getElementById('title-edit');
	  const key = (target, name) => target.dispatchEvent(new KeyboardEvent('keydown',{key:name,bubbles:true}));
	  click();
	  field().value = 'discarded';
	  key(field(), 'Escape');
	  const cancelled = field().hidden && document.getElementById('title').textContent === 'orders view';
	  click();
	  field().value = '   ';
	  key(field(), 'Enter');
	  await new Promise(resolve => setTimeout(resolve, 200));
	  return cancelled && document.getElementById('title').textContent === '10mb.json';
	})()`); err != nil {
		return err
	}
	otherPath := filepath.Join(dir, "opened.json")
	if err := os.WriteFile(otherPath, []byte(`{"another":true}`), 0600); err != nil {
		return err
	}
	mygo.RunOnMain(func() { a.replaceJSONFile(d, otherPath) })
	if err := wait(`document.getElementById('title').textContent === 'opened.json' && !window.cubbix.state().pending`); err != nil {
		return err
	}
	var opened string
	mygo.RunOnMain(func() { opened = d.window.Title() })
	if opened != "opened.json" {
		return fmt.Errorf("new file title %q", opened)
	}
	mygo.RunOnMain(func() {
		// The traffic-light close action reaches the same native close path.
		objc.ID(extra.window.NativeHandle()).Send(objc.RegisterName("performClose:"), objc.ID(0))
	})
	if !extra.window.IsDestroyed() {
		return fmt.Errorf("native close action did not destroy tool")
	}
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
	var focusedEvents, blurredEvents, resignedEvents int
	mygo.RunOnMain(func() {
		nsapp := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
		// Set the real native policy without MyGo's activating wrapper.
		nsapp.Send(objc.RegisterName("setActivationPolicy:"), 0)
		a.policy = mygo.ActivationPolicyRegular
		for _, doc := range []*jsonDocument{d, other} {
			doc.window.OnFocus(func() { focusedEvents++ })
			doc.window.OnBlur(func() { blurredEvents++ })
			delegate := objc.ID(doc.window.NativeHandle()).Send(objc.RegisterName("delegate"))
			// Exercise AppKit -> MyGo event callbacks without making a real key
			// window, activating the app, or disturbing the foreground Space.
			delegate.Send(objc.RegisterName("windowDidBecomeKey:"), objc.ID(0))
			delegate.Send(objc.RegisterName("windowDidResignKey:"), objc.ID(0))
		}
		mygo.App.OnDidResignActive(func() { resignedEvents++ })
		nsapp.Send(objc.RegisterName("deactivate"))
		nsapp.Send(objc.RegisterName("delegate")).Send(objc.RegisterName("applicationDidResignActive:"), objc.ID(0))
		a.visibility(false)
	})
	if focusedEvents != 2 || blurredEvents != 2 || resignedEvents < 1 {
		return fmt.Errorf("native event paths not exercised: focus=%d blur=%d resign=%d", focusedEvents, blurredEvents, resignedEvents)
	}
	fmt.Println("Focus evidence: native regular policy, two non-key windows, 2 simulated native focus/blur delegate pairs, resign-active delegate + deactivate; no foreground focus switching")
	time.Sleep(250 * time.Millisecond)
	if active, visible, focused, count := windowsState(); active || focused || !visible || count != 2 {
		return fmt.Errorf("after native focus/blur callbacks and deactivation: active=%v focused=%v visible=%v count=%d", active, focused, visible, count)
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
	// Send the shortcut through the real page bridge and close event.
	_, _ = eval("document.dispatchEvent(new KeyboardEvent('keydown',{key:'w',metaKey:true,bubbles:true}))")
	deadline := time.Now().Add(5 * time.Second)
	for !d.window.IsDestroyed() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !d.window.IsDestroyed() || len(a.jsonDocuments) != 1 {
		return fmt.Errorf("dirty window did not close on request")
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
			for _, kind := range []string{"tiles", "shortcuts"} {
				a.page = 2
				view := a.view
				w, h := 720, 340
				if kind == "shortcuts" {
					a.page = 3
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
