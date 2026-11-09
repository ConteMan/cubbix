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
// error: it means the module is not configured yet.
func (a *application) translateAPIKey() (string, error) {
	if a.translateSecret == nil {
		return "", nil
	}
	value, err := a.translateSecret.Get(translateSecretKey)
	if errors.Is(err, mygo.ErrSecretNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(value)), nil
}

func (a *application) setTranslateAPIKey(value string) error {
	value = strings.TrimSpace(value)
	a.translateKeyDraft = ""
	if a.translateSecret == nil {
		return errors.New("translate: secret store unavailable")
	}
	var err error
	if value == "" {
		err = a.translateSecret.Delete(translateSecretKey)
		a.translateKeySaved = false
	} else {
		if len(value) > mygo.MaxSecretSize {
			return errors.New("translate: key too long")
		}
		err = a.translateSecret.Set(translateSecretKey, []byte(value))
		a.translateKeySaved = err == nil
	}
	if err != nil {
		a.translateKeyError = "TranslateKeyError"
	} else {
		a.translateKeyError = ""
	}
	a.invalidateWindow()
	return err
}

func (a *application) clearTranslateAPIKey() error { return a.setTranslateAPIKey("") }

// refreshTranslateKey reads the status dot on a worker: the system may ask
// the user to unlock the keychain, which must not block the main thread.
func (a *application) refreshTranslateKey() {
	a.runTranslateWorker(func() {
		key, err := a.translateAPIKey()
		a.translateRunOnMain(func() {
			if err != nil {
				a.translateKeyError = "TranslateKeyError"
			} else {
				a.translateKeyError = ""
				a.translateKeySaved = key != ""
			}
			a.invalidateWindow()
		})
	})
}
