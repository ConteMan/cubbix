package main

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
)

//go:generate ./scripts/build-frontend.sh

// The JSON window is a tool, not a document: it closes on request, never to
// protect unsaved text. The editor shares its width with the this field.
const (
	jsonSplitDefault = 60
	jsonSplitMinimum = 25
	jsonSplitMaximum = 75
)

func clampJSONSplit(percent int) int {
	if percent < jsonSplitMinimum {
		return jsonSplitMinimum
	}
	if percent > jsonSplitMaximum {
		return jsonSplitMaximum
	}
	return percent
}

func (a *application) setupJSON() {
	frontend, _ := fs.Sub(jsonAssets, "frontend/dist")
	mygo.SetFrontend(frontend)
	mygo.BindAs("JSON", &jsonBridge{app: a})
	a.jsonReadClipboard = mygo.Clipboard.ReadText
	a.jsonCopy = func(text string) error { return mygo.Clipboard.Write(transfer.TextData(text)) }
	a.jsonChooseOpen = func(w *mygo.Window) (string, error) {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: w, Title: a.text(mygo.App.Locale(), "R07Open"), Filters: []mygo.FileFilter{{Name: "JSON", Extensions: []string{"json", "txt"}}}})
		if err != nil || len(paths) == 0 {
			return "", err
		}
		return paths[0], nil
	}
	a.jsonChooseSave = func(w *mygo.Window) (string, error) {
		return mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: w, Title: a.text(mygo.App.Locale(), "JSONSaveAs"), DefaultPath: "untitled.json", Filters: []mygo.FileFilter{{Name: "JSON", Extensions: []string{"json"}}}})
	}
}

func (a *application) registerJSONShortcut() {
	if !a.config.JSONEnabled {
		return
	}
	a.jsonShortcutError = ""
	if reason := a.shortcutConflict(a.config.JSONShortcut, "json"); reason != "" {
		a.jsonShortcutError = reason
		return
	}
	if err := a.registerShortcut(a.config.JSONShortcut, a.newJSONWindow); err != nil {
		a.jsonShortcutError = "ShortcutUnavailable"
	}
}

func (a *application) changeJSONEnabled() {
	if a.config.JSONEnabled {
		a.registerJSONShortcut()
		if a.refreshApplicationMenu != nil {
			a.refreshApplicationMenu()
		}
		a.save()
		a.refreshAwake()
		return
	}
	a.config.JSONEnabled = false
	if a.refreshApplicationMenu != nil {
		a.refreshApplicationMenu()
	}
	a.unregisterShortcut(a.config.JSONShortcut)
	for _, d := range append([]*jsonDocument(nil), a.jsonDocuments...) {
		a.destroyJSON(d)
	}
	a.save()
	a.refreshAwake()
}

func (a *application) createJSONWindow(d *jsonDocument) {
	t := tokens(mygo.Theme.IsDark(), mygo.App.Locale())
	width, height := int(t.SizeJSONWindowWidth), int(t.SizeJSONWindowHeight)
	if a.config.JSONWidth >= int(t.SizeJSONEmptyWidth) {
		width = a.config.JSONWidth
	}
	if a.config.JSONHeight >= int(t.SizeJSONEmptyHeight) {
		height = a.config.JSONHeight
	}
	d.window = mygo.NewWindow(mygo.WindowOptions{Title: d.name(a, mygo.App.Locale()), Width: width, Height: height, MinWidth: int(t.SizeJSONEmptyWidth), MinHeight: int(t.SizeJSONEmptyHeight), Hidden: true, TitleBarStyle: mygo.TitleBarHidden, TrafficLightPosition: &mygo.Point{X: int(t.Space12), Y: int(t.Space16)}, DisableFullScreen: true, Page: mygo.PageOptions{DevTools: mygo.DevToolsDisabled}, BackgroundColor: t.ColorCard})
	d.window.Page().SetWindowOpenHandler(func(mygo.WindowOpenRequest) *mygo.WindowOptions { return nil })
	d.window.Page().OnWillNavigate(func(e *mygo.NavigateEvent) {
		if d.ready || e.URL != "mygo://localhost/" {
			e.PreventDefault()
		}
	})
	d.window.Page().OnPageTitleUpdated(func(e *mygo.TitleEvent) { e.PreventDefault() })
	// Showing is explicit: the app never gains windows by activation or focus.
	d.window.OnHide(func() { a.visibility(false) })
	d.window.OnClosed(func() { a.removeJSONDocument(d); a.visibility(false); a.refreshAwake() })
	d.window.OnResize(func() {
		width, height := d.window.Size()
		a.config.JSONWidth, a.config.JSONHeight = width, height
		a.save()
	})
	d.window.OnFileDrop(func(e *mygo.FileDropEvent) {
		if len(e.Paths) == 0 {
			return
		}
		// Paths originate in the native drop event, never from a bridge argument.
		path := e.Paths[0]
		a.replaceJSONFile(d, path)
	})
	_ = d.window.Page().LoadURL("/")
}

func (a *application) newJSONWindow() {
	if !a.config.JSONEnabled {
		return
	}
	d := a.newJSONDocumentFromClipboard()
	a.showJSON(d)
	a.refreshAwake()
}

// newJSONWindowBlank opens an empty document without reading the clipboard.
func (a *application) newJSONWindowBlank() {
	if !a.config.JSONEnabled {
		return
	}
	d := a.addJSONDocument("", "", true)
	a.createJSONWindow(d)
	a.showJSON(d)
	a.refreshAwake()
}

// openJSONText opens a document whose text arrives in a cubbix:// URL. It
// never reads the clipboard and never evaluates the text.
func (a *application) openJSONText(text string) {
	if a.paletteOpenJSON != nil {
		a.paletteOpenJSON(text)
		return
	}
	d := a.addJSONDocument(text, "", true)
	a.createJSONWindow(d)
	a.showJSON(d)
	a.refreshAwake()
}

// setJSONPinned shares the pin state between the page bridge and the palette.
func (a *application) setJSONPinned(d *jsonDocument, pinned bool) {
	if d == nil || d.window == nil {
		return
	}
	d.pinned = pinned
	d.window.SetAlwaysOnTop(pinned)
}

func (a *application) newJSONDocumentFromClipboard() *jsonDocument {
	text := ""
	if a.jsonReadClipboard != nil {
		text = validClipboardJSON(a.jsonReadClipboard())
	}
	d := a.addJSONDocument(text, "", text == "")
	a.createJSONWindow(d)
	return d
}

func (a *application) showJSON(d *jsonDocument) {
	if d.closed || d.window == nil {
		return
	}
	// A JSON window takes the focus and activates the app, so the card panel
	// closes first instead of lingering behind it.
	a.hideMenubarPanel()
	a.visibility(true)
	d.window.Show()
	d.window.Focus()
}
func (a *application) destroyJSON(d *jsonDocument) {
	if d.window != nil {
		d.window.Destroy()
	} else {
		a.removeJSONDocument(d)
	}
	a.refreshAwake()
}

func (a *application) refreshJSONTitles(locale string) {
	for _, d := range a.jsonDocuments {
		if d.window == nil {
			continue
		}
		// The window manager, the app switcher and the page show one name.
		name := d.name(a, locale)
		d.window.SetTitle(name)
		go a.setJSONPageTitle(d, name)
	}
}

func (a *application) setJSONPageTitle(d *jsonDocument, name string) {
	var page *mygo.Page
	mygo.RunOnMain(func() {
		if d.window != nil && !d.closed {
			page = d.window.Page()
		}
	})
	if page == nil {
		return
	}
	encoded, _ := json.Marshal(name)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _ = page.EvalContext(ctx, "window.cubbix?.title("+string(encoded)+")")
}

func (a *application) openJSONFile(d *jsonDocument) {
	if a.jsonChooseOpen == nil {
		return
	}
	go func() {
		path, err := a.jsonChooseOpen(d.window)
		if err != nil {
			a.jsonError(d, "read")
			return
		}
		if path != "" {
			mygo.RunOnMain(func() { a.replaceJSONFile(d, path) })
		}
	}()
}
func (a *application) replaceJSONFile(d *jsonDocument, path string) {
	if d.closed {
		return
	}
	go func() {
		data, err := readJSONFile(path)
		if err != nil {
			key := "read"
			if err.Error() == "JSONFileLimit" {
				key = "fileLimit"
			}
			a.jsonError(d, key)
			return
		}
		mygo.RunOnMain(func() {
			if d.closed {
				return
			}
			d.path = path
			d.text = data
			d.revision++
			d.dirty = false
			a.refreshJSONTitles(mygo.App.Locale())
			a.refreshAwake()
			encoded, _ := json.Marshal(data)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := d.window.Page().EvalContext(ctx, "window.cubbix.load("+string(encoded)+","+formatRevision(d.revision)+",true)"); err != nil {
				a.jsonError(d, "read")
			}
		})
	}()
}
func formatRevision(revision uint64) string { data, _ := json.Marshal(revision); return string(data) }

func (a *application) jsonError(d *jsonDocument, status string) {
	if d.window == nil {
		return
	}
	encoded, _ := json.Marshal(status)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _ = d.window.Page().EvalContext(ctx, "window.cubbix?.error("+string(encoded)+")")
}

func (a *application) saveJSON(d *jsonDocument, snapshot jsonSnapshot, as bool) (bool, error) {
	var path string
	mygo.RunOnMain(func() { path = d.path })
	if as || path == "" {
		if a.jsonChooseSave == nil {
			return false, nil
		}
		var err error
		path, err = a.jsonChooseSave(d.window)
		if err != nil {
			return false, err
		}
		if path == "" {
			return false, nil
		}
	}
	if err := writeJSONFile(path, snapshot.Text); err != nil {
		return false, err
	}
	mygo.RunOnMain(func() {
		if d.closed {
			return
		}
		d.path = filepath.Clean(path)
		if snapshot.Revision >= d.revision {
			d.changed(snapshot.Revision, false)
		}
		a.refreshJSONTitles(mygo.App.Locale())
		a.refreshAwake()
	})
	return true, nil
}

// Only an app-owned JSON page may use these methods. Paths are kept in Go;
// bridge callers cannot nominate an arbitrary file to read or write.
type jsonBridge struct{ app *application }

func (b *jsonBridge) document(ctx context.Context) (*jsonDocument, error) {
	var d *jsonDocument
	mygo.RunOnMain(func() {
		d = b.app.jsonForWindow(mygo.CallerWindow(ctx))
		if d != nil && mygo.CallerPage(ctx) != d.window.Page() {
			d = nil
		}
	})
	if d == nil || ctx.Err() != nil {
		return nil, errors.New("JSON window unavailable")
	}
	return d, nil
}
func (b *jsonBridge) Appearance(ctx context.Context) (map[string]any, error) {
	if _, err := b.document(ctx); err != nil {
		return nil, err
	}
	return b.app.jsonAppearance(), nil
}
func (a *application) jsonAppearance() map[string]any {
	locale := mygo.App.Locale()
	if locale == "" {
		locale = "zh-CN"
	}
	data, _ := assets.ReadFile("design/locales/" + locale + ".json")
	messages := map[string]string{}
	_ = json.Unmarshal(data, &messages)
	for key := range messages {
		if !strings.HasPrefix(key, "JSON") && !strings.HasPrefix(key, "R07") {
			delete(messages, key)
		}
	}
	return map[string]any{"locale": locale, "messages": messages, "light": tokens(false, locale), "dark": tokens(true, locale)}
}
func (b *jsonBridge) Boot(ctx context.Context) (map[string]any, error) {
	d, err := b.document(ctx)
	if err != nil {
		return nil, err
	}
	result := b.app.jsonAppearance()
	mygo.RunOnMain(func() {
		result["text"] = d.text
		result["revision"] = d.revision
		result["clean"] = !d.dirty
		result["pinned"] = d.pinned
		result["split"] = clampJSONSplit(b.app.config.JSONSplit)
		result["name"] = d.name(b.app, mygo.App.Locale())
		d.ready = true
	})
	return result, nil
}
func (b *jsonBridge) Changed(ctx context.Context, revision uint64, dirty bool) error {
	d, err := b.document(ctx)
	if err != nil {
		return err
	}
	mygo.RunOnMain(func() { d.changed(revision, dirty) })
	return nil
}
func (b *jsonBridge) Split(ctx context.Context, percent int) error {
	if _, err := b.document(ctx); err != nil {
		return err
	}
	mygo.RunOnMain(func() {
		clamped := clampJSONSplit(percent)
		if b.app.config.JSONSplit == clamped {
			return
		}
		b.app.config.JSONSplit = clamped
		b.app.save()
	})
	return nil
}
func (b *jsonBridge) Copy(ctx context.Context, text string) error {
	if _, err := b.document(ctx); err != nil {
		return err
	}
	if len(text) > jsonFileLimit {
		return errors.New("JSONFileLimit")
	}
	return b.app.jsonCopy(text)
}
func (b *jsonBridge) Pin(ctx context.Context) (bool, error) {
	d, err := b.document(ctx)
	if err != nil {
		return false, err
	}
	var pinned bool
	mygo.RunOnMain(func() { b.app.setJSONPinned(d, !d.pinned); pinned = d.pinned })
	return pinned, nil
}
func (b *jsonBridge) Open(ctx context.Context) error {
	d, err := b.document(ctx)
	if err != nil {
		return err
	}
	mygo.RunOnMain(func() { b.app.openJSONFile(d) })
	return nil
}
func (b *jsonBridge) Save(ctx context.Context, text string, revision uint64, as bool) (bool, error) {
	d, err := b.document(ctx)
	if err != nil {
		return false, err
	}
	if len(text) > jsonFileLimit {
		return false, errors.New("JSONFileLimit")
	}
	return b.app.saveJSON(d, jsonSnapshot{Text: text, Revision: revision}, as)
}
func (b *jsonBridge) Close(ctx context.Context) error {
	d, err := b.document(ctx)
	if err != nil {
		return err
	}
	mygo.RunOnMain(func() { d.window.Close() })
	return nil
}

func (a *application) jsonPageAction(w *mygo.Window, action string) {
	if d := a.jsonForWindow(w); d != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			_, _ = w.Page().EvalContext(ctx, "window.cubbix."+action)
		}()
	}
}
