package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo"
)

const jsonFileLimit = 64 * 1024 * 1024
const jsonWindowSubmenuAt = 6

// jsonWindowNameLimit keeps an inline rename readable in the title bar and the
// card list.
const jsonWindowNameLimit = 64

type jsonDocument struct {
	number                       int
	window                       *mygo.Window
	text, path, custom           string
	revision                     uint64
	dirty, pinned, ready, closed bool
}

type jsonSnapshot struct {
	Text     string `json:"text"`
	Revision uint64 `json:"revision"`
}

// defaultName is the name the document has without a rename: the open file's
// name, or the fixed JSON name for a window that never opened a file.
func (d *jsonDocument) defaultName(a *application, locale string) string {
	if d.path != "" {
		return filepath.Base(d.path)
	}
	return a.text(locale, "JSONWindowName")
}

// name is the one name used by the window title, the system title, the menu and
// the card list: a rename while it applies, otherwise the default name.
func (d *jsonDocument) name(a *application, locale string) string {
	if d.custom != "" {
		return d.custom
	}
	return d.defaultName(a, locale)
}

// rename applies an inline title edit. An empty name, or the default name,
// restores the default instead of pinning a copy of it.
func (d *jsonDocument) rename(a *application, locale, name string) {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > jsonWindowNameLimit {
		name = strings.TrimSpace(string([]rune(name)[:jsonWindowNameLimit]))
	}
	if name == "" || name == d.defaultName(a, locale) {
		d.custom = ""
		return
	}
	d.custom = name
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
