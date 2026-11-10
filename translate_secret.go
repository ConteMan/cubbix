package main

import (
	"errors"
	"strings"

	"github.com/egoist/mygo"
)

// translateSecretStore is the small seam around mygo.Secrets. The key never
// enters Config and is never written to a log; tests replace the store so no
// case reads the real login keychain.
type translateSecretStore interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte) error
	Delete(key string) error
}

const translateSecretKey = "translate-api-key"

// translateAPIKey reads the key for a worker. A missing secret is not an
// error: it means the module is not configured yet. A read waits for a write
// still in flight, so opening the panel right after a paste sees the new key
// rather than the old one.
func (a *application) translateAPIKey() (string, error) {
	a.waitTranslateKeyWrite()
	return readTranslateAPIKey(a.translateSecret)
}

// waitTranslateKeyWrite blocks until the keychain write in flight settles. It
// is only ever called from a worker; the main thread never waits here.
func (a *application) waitTranslateKeyWrite() {
	for {
		a.translateSecretMu.Lock()
		pending := a.translateKeyPending
		a.translateSecretMu.Unlock()
		if pending == nil {
			return
		}
		<-pending
	}
}

func readTranslateAPIKey(store translateSecretStore) (string, error) {
	if store == nil {
		return "", nil
	}
	value, err := store.Get(translateSecretKey)
	if errors.Is(err, mygo.ErrSecretNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(value)), nil
}

func (a *application) setTranslateAPIKey(value string) {
	if a.translateKeyWriting {
		return
	}
	value = strings.TrimSpace(value)
	a.translateKeyDraft = ""
	if a.translateSecret == nil || len(value) > mygo.MaxSecretSize {
		a.translateKeyError = "TranslateKeyError"
		a.invalidateWindow()
		return
	}
	a.translateKeyGeneration++
	a.translateKeyWriting = true
	a.translateKeyError = ""
	done := make(chan struct{})
	a.translateSecretMu.Lock()
	a.translateKeyPending = done
	a.translateSecretMu.Unlock()
	a.invalidateWindow()
	a.runTranslateWorker(func() {
		var err error
		if value == "" {
			err = a.translateSecret.Delete(translateSecretKey)
			if errors.Is(err, mygo.ErrSecretNotFound) {
				err = nil
			}
		} else {
			err = a.translateSecret.Set(translateSecretKey, []byte(value))
		}
		a.translateSecretMu.Lock()
		if a.translateKeyPending == done {
			a.translateKeyPending = nil
		}
		a.translateSecretMu.Unlock()
		close(done)
		a.translateOnMain(func() {
			a.translateKeyWriting = false
			if err != nil {
				a.translateKeyError = "TranslateKeyError"
				// The keychain may still hold the old key: forget what was
				// committed so a retry writes again.
				a.translateKeyCommitted = ""
			} else {
				a.translateKeySaved = value != ""
				a.translateKeyEditing = false
				a.translateKeyError = ""
				a.translateKeyCommitted = value
			}
			a.invalidateWindow()
		})
	})
}

func (a *application) clearTranslateAPIKey() { a.setTranslateAPIKey("") }

// refreshTranslateKey reads the status dot on a worker: the system may ask
// the user to unlock the keychain, which must not block the main thread.
func (a *application) refreshTranslateKey() {
	generation := a.translateKeyGeneration
	a.runTranslateWorker(func() {
		key, err := a.translateAPIKey()
		a.translateOnMain(func() {
			if generation != a.translateKeyGeneration {
				return
			}
			if err != nil {
				a.translateKeyError = "TranslateKeyError"
			} else {
				a.translateKeyError = ""
				a.translateKeySaved = key != ""
				a.translateKeyCommitted = key
			}
			a.invalidateWindow()
		})
	})
}
