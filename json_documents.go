package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

const jsonFileLimit = 64 * 1024 * 1024
const jsonWindowSubmenuAt = 6

type jsonDocument struct {
	number                       int
	window                       *mygo.Window
	text, path                   string
	revision                     uint64
	dirty, pinned, ready, closed bool
}

type jsonSnapshot struct {
	Text     string `json:"text"`
	Revision uint64 `json:"revision"`
	Dirty    bool   `json:"dirty"`
}

func (d *jsonDocument) name(a *application, locale string) string {
	if d.path != "" {
		return filepath.Base(d.path)
	}
	return fmt.Sprintf(a.text(locale, "JSONUntitled"), d.number)
}

func (d *jsonDocument) changed(revision uint64, dirty bool) {
	if revision >= d.revision {
		d.revision, d.dirty = revision, dirty
	}
}

func (a *application) addJSONDocument(text, path string, clean bool) *jsonDocument {
	a.jsonNext++
	d := &jsonDocument{number: a.jsonNext, text: text, path: path, revision: 1, dirty: !clean && text != ""}
	a.jsonDocuments = append(a.jsonDocuments, d)
	return d
}

func (a *application) removeJSONDocument(d *jsonDocument) {
	d.closed = true
	for i, candidate := range a.jsonDocuments {
		if candidate == d {
			a.jsonDocuments = append(a.jsonDocuments[:i:i], a.jsonDocuments[i+1:]...)
			break
		}
	}
}

func (a *application) jsonForWindow(w *mygo.Window) *jsonDocument {
	for _, d := range a.jsonDocuments {
		if d.window == w && !d.closed {
			return d
		}
	}
	return nil
}

func (a *application) jsonMenu(locale string) []*mygo.MenuItem {
	if !a.config.JSONEnabled {
		return nil
	}
	items := []*mygo.MenuItem{{ID: "json-new", Label: a.text(locale, "R07NewWindow"), Accelerator: a.config.JSONShortcut, Click: func(*mygo.MenuItem, *mygo.Window) { a.newJSONWindow() }}}
	windows := []*mygo.MenuItem{}
	for _, d := range a.jsonDocuments {
		windows = append(windows, &mygo.MenuItem{ID: fmt.Sprintf("json-window-%d", d.number), Label: d.name(a, locale), Click: func(*mygo.MenuItem, *mygo.Window) { a.showJSON(d) }})
	}
	if len(windows) >= jsonWindowSubmenuAt {
		items = append(items, &mygo.MenuItem{ID: "json-windows", Label: a.text(locale, "JSONWindows"), Submenu: windows})
	} else {
		items = append(items, windows...)
	}
	return append(items, mygo.Separator())
}

func readJSONFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("JSONReadError")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("JSONReadError")
	}
	if info.Size() > jsonFileLimit {
		return "", errors.New("JSONFileLimit")
	}
	data, err := io.ReadAll(io.LimitReader(file, jsonFileLimit+1))
	if err != nil {
		return "", errors.New("JSONReadError")
	}
	if len(data) > jsonFileLimit {
		return "", errors.New("JSONFileLimit")
	}
	// JSON validity belongs to the editor; invalid input must remain editable.
	return string(data), nil
}

func writeJSONFile(path, text string) error {
	// Resolve a selected symlink before replacing the target atomically.
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".cubbix-json-*")
	if err != nil {
		return errors.New("JSONWriteError")
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if info, err := os.Stat(path); err == nil {
		if err = file.Chmod(info.Mode().Perm()); err != nil {
			return errors.New("JSONWriteError")
		}
	}
	if _, err = file.WriteString(text); err != nil {
		return errors.New("JSONWriteError")
	}
	if err = file.Sync(); err != nil {
		return errors.New("JSONWriteError")
	}
	if err = file.Close(); err != nil {
		return errors.New("JSONWriteError")
	}
	if err = os.Rename(file.Name(), path); err != nil {
		return errors.New("JSONWriteError")
	}
	return nil
}

func validClipboardJSON(text string) string {
	if len(text) > jsonFileLimit || !json.Valid([]byte(text)) {
		return ""
	}
	return text
}
