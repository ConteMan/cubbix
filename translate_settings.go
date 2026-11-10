package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// translatePageHeight is the natural height of the sixth settings page. The
// settings card has five rows (① 48, ② 64, ③ 48, ① 48, ③ 48) and the shortcut
// card one; with a closed module only the shortcut card stays.
func translatePageHeight(t tokenValues, on bool) float32 {
	if !on {
		return sectionHeight(t, cardHeight(t, rowHeight(t, 3)))
	}
	settings := cardHeight(t, rowHeight(t, 1), rowHeight(t, 2), rowHeight(t, 3),
		rowHeight(t, 1), rowHeight(t, 3))
	return sectionHeight(t, settings, cardHeight(t, rowHeight(t, 3)))
}

func (a *application) translateModulePage(c *ui.Context, t tokenValues, locale string, l settingsLayout, alert string) {
	text := func(key string) string { return a.text(locale, key) }
	a.syncTranslateDrafts()
	on := l.translateOn
	a.pageHeader(c, t, locale, "languages", "Translate", "TranslatePageDesc", alert, func() {
		a.switchControl(c, t, "translate-enabled", text("Translate"), &on, func() {
			a.changeTranslateEnabled(!a.config.TranslateEnabled)
		}, "", false)
	})
	if on {
		a.settingsSection(c, t, text("GroupSettings"), func() {
			a.translateBaseURLRow(c, t, locale)
			ui.Divider(c)
			a.translateKeyRow(c, t, locale)
			ui.Divider(c)
			a.translateModelRow(c, t, locale)
			ui.Divider(c)
			a.translateAutoSpeakRow(c, t, locale)
			ui.Divider(c)
			a.translateSelectionRow(c, t, locale)
		})
	}
	a.settingsSection(c, t, text("GroupShortcuts"), func() {
		a.shortcutSettingsRow(c, t, locale, "translate")
	})
}

// translateBaseURLRow keeps the short level-1 sentence beside the title. The
// full address rule lives in a tooltip on the field. A rejected address is
// reported by the row's own alert when the value is saved or tested, never by
// another sentence beside the title.
func (a *application) translateBaseURLRow(c *ui.Context, t tokenValues, locale string) {
	text := func(key string) string { return a.text(locale, key) }
	a.settingsRow(c, t, text("TranslateBaseURL"), settingsRowOpts{level: 1, hint: text("TranslateEndpointLocalHint")}, func() {
		input := ui.TextInput(c.Key("translate-base-url"), &a.translateBaseURLDraft).
			Width(240).Height(t.SizeSegmentHeight).PaddingX(t.Space8).
			Radius(t.RadiusControl).Border(t.StrokeHairline, ui.Hex(t.ColorSeparator)).
			Background(ui.Hex(t.ColorControl)).FontSize(t.FontSizeBody).
			Label(text("TranslateBaseURL")).Tooltip(text("TranslateBaseURLInfo")).
			Description(text("TranslateBaseURLInfo")).FocusRing(true).OnSubmit(a.commitTranslateBaseURL)
		input.OnChange(func() { a.translateBaseURLDirty = true })
		a.trackTranslateFieldFocus(translateFieldBaseURL, input)
		if a.translateRowError != "" {
			a.alertIcon(c, t, text(a.translateRowError))
		}
	})
}

func (a *application) translateKeyRow(c *ui.Context, t tokenValues, locale string) {
	text := func(key string) string { return a.text(locale, key) }
	a.settingsRow(c, t, text("TranslateAPIKey"), settingsRowOpts{level: 2, hint: text("TranslateKeyHint")}, func() {
		ui.Row(c).Gap(t.Space8).AlignItems(ui.Center).Disabled(a.translateKeyWriting).Children(func() {
			if a.translateKeySaved && !a.translateKeyEditing {
				ui.Text(c, text("TranslateKeySaved")).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary)).SingleLine()
				a.translateTextButton(c, t, "translate-key-replace", text("TranslateKeyReplace"), func() {
					a.translateKeyEditing = true
					a.translateKeyFocus = true
					a.invalidateWindow()
				})
				a.translateTextButton(c, t, "translate-key-clear", text("TranslateKeyClear"), a.clearTranslateKeyAndDraft)
				// No input is drawn while the saved state is shown, so the
				// focus record must not claim the field still has the keyboard.
				delete(a.translateFieldFocused, translateFieldKey)
				return
			}
			input := ui.TextInput(c.Key("translate-key"), &a.translateKeyDraft).
				Width(180).Height(t.SizeSegmentHeight).PaddingX(t.Space8).
				Radius(t.RadiusControl).Border(t.StrokeHairline, ui.Hex(t.ColorSeparator)).
				Background(ui.Hex(t.ColorControl)).FontSize(t.FontSizeBody).
				Placeholder(text("TranslateKeyPlaceholder")).Label(text("TranslateAPIKey")).
				FocusRing(true).Password().OnSubmit(a.commitTranslateKey)
			if a.translateKeyFocus {
				a.translateKeyFocus = false
				input.Focus()
			}
			a.trackTranslateFieldFocus(translateFieldKey, input)
		})
		if a.translateKeyWriting {
			ui.Spinner(c).Size(t.SizeIcon, t.SizeIcon).Label(text("TranslateAPIKey"))
		}
		if a.translateKeyError != "" {
			a.alertIcon(c, t, text(a.translateKeyError))
		}
	})
}

func (a *application) translateModelRow(c *ui.Context, t tokenValues, locale string) {
	text := func(key string) string { return a.text(locale, key) }
	a.settingsRow(c, t, text("TranslateModel"), settingsRowOpts{level: 3, tooltip: text("TranslateModelInfo")}, func() {
		input := ui.TextInput(c.Key("translate-model"), &a.translateModelDraft).
			Width(200).Height(t.SizeSegmentHeight).PaddingX(t.Space8).
			Radius(t.RadiusControl).Border(t.StrokeHairline, ui.Hex(t.ColorSeparator)).
			Background(ui.Hex(t.ColorControl)).FontSize(t.FontSizeBody).
			Label(text("TranslateModel")).FocusRing(true).OnSubmit(a.commitTranslateModel)
		input.OnChange(func() { a.translateModelDirty = true })
		a.trackTranslateFieldFocus(translateFieldModel, input)
		selected := a.translateModelDraft
		sel := ui.SelectBase(c.Key("translate-model-list"), &selected)
		trigger := sel.Trigger.Size(t.SizeIconButton, t.SizeIconButton).Radius(t.RadiusControl).
			Background(ui.Hex(t.ColorControl)).FocusRing(true).
			Label(text("TranslateModelFetch")).Tooltip(text("TranslateModelFetch")).
			OnClick(a.fetchTranslateModelList)
		trigger.Children(func() {
			ui.Row(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Icon(c, icon("chevron-down")).Size(t.SizeIconSmall, t.SizeIconSmall).
					TextColor(ui.Hex(t.ColorTextSecondary))
			})
		})
		sel.Popup(func(panel ui.Element) {
			panel.Padding(t.Space4).Radius(t.RadiusCard).
				Border(t.StrokeHairline, ui.Hex(t.ColorSeparator)).Background(ui.Hex(t.ColorCard)).Children(func() {
				if len(a.translateModelList) == 0 {
					ui.Text(c, text("TranslateModelListEmpty")).Height(t.SizeRowHeight).
						FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary))
					return
				}
				for _, name := range a.translateModelList {
					model := name
					item := sel.Item(model).Height(t.SizeRowHeight - t.Space8).PaddingX(t.Space8).
						Radius(t.RadiusControl).Label(model).OnClick(func() { a.setTranslateModel(model) })
					item.Children(func() {
						ui.Text(c, model).FontSize(t.FontSizeBody).SingleLine()
					})
				}
			})
		})
		testTip := text("TranslateTest")
		if a.translateTestRunning {
			testTip = text("TranslateTestRunning")
		} else if a.translateTestKey != "" {
			testTip = text(a.translateTestKey)
		}
		// A named button, not a lone refresh glyph: the maintainer did not
		// recognise the icon as the connection test. The result keeps the
		// existing short-lived glyph beside it.
		test := ui.ButtonBase(c.Key("translate-test")).Height(t.SizeSegmentHeight).PaddingX(t.Space8).
			Radius(t.RadiusControl).FocusRing(true).Label(testTip).Tooltip(testTip)
		if !a.translateTestRunning {
			test = test.OnClick(a.testTranslateEndpoint)
		}
		test.Children(func() {
			ui.Text(c, text("TranslateTest")).FontSize(t.FontSizeSmall).
				TextColor(ui.Hex(t.ColorText)).SingleLine()
		})
		a.translateTestStatus(c, t, text)
		if a.translateModelListError != "" {
			a.alertIcon(c, t, text(a.translateModelListError))
		}
	})
	// A short-lived status is cleared by a scheduled frame, not a timer.
	if a.translateTestKey != "" && !a.translateTestRunning {
		now := c.Now()
		if now.Before(a.translateTestUntil) {
			c.After(a.translateTestUntil.Sub(now))
		} else {
			a.translateTestKey = ""
		}
	}
}

// translateTestStatus is the connection test's result, beside its button:
// a loader while it runs, then a check or an alert with the reason in its
// tooltip. It replaces the old result glyph that used to sit in the button.
func (a *application) translateTestStatus(c *ui.Context, t tokenValues, text func(string) string) {
	switch {
	case a.translateTestRunning:
		ui.Spinner(c).Size(t.SizeIcon, t.SizeIcon).Label(text("TranslateTestRunning"))
	case a.translateTestKey == "":
		return
	case a.translateTestKey == "TranslateTestOK":
		a.checkIcon(c, t, text("TranslateTestOK"))
	default:
		a.alertIcon(c, t, text(a.translateTestKey))
	}
}

// checkIcon is the success marker: the same neutral check the panel uses, with
// its meaning in a tooltip, so the result is never a wordless shape.
func (a *application) checkIcon(c *ui.Context, t tokenValues, message string) {
	e := ui.Icon(c, icon("check")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary)).
		Focusable().Label(message).Description(message)
	ui.TooltipBase(c, e, func(tip ui.Element) {
		tip.Padding(t.Space6, t.Space8).Radius(t.RadiusControl).MaxWidth(t.SizeMenubarPanelWidth).
			Background(ui.Hex(t.ColorTooltip)).TextColor(ui.Hex(t.ColorTooltipText)).Children(func() {
			ui.Text(c, message).FontSize(t.FontSizeSmall).LineHeight(t.LineHeightBody)
		})
	})
}

func (a *application) translateAutoSpeakRow(c *ui.Context, t tokenValues, locale string) {
	text := func(key string) string { return a.text(locale, key) }
	on := a.config.TranslateAutoSpeak
	a.settingsRow(c, t, text("TranslateAutoSpeak"), settingsRowOpts{level: 1, hint: text("TranslateAutoSpeakHint")}, func() {
		// Reads a single English word once, after a definition or a translation.
		a.switchControl(c, t, "translate-auto-speak", text("TranslateAutoSpeak"), &on, func() {
			a.changeTranslateAutoSpeak(!a.config.TranslateAutoSpeak)
		}, "", false)
	})
}

// translateSelectionRow shows the Accessibility state. Level 3 is the right
// level from 007: the status changes with the system permission and the
// explanation needs more than one short sentence. The text button is a
// user action, never an automatic system window.
func (a *application) translateSelectionRow(c *ui.Context, t tokenValues, locale string) {
	text := func(key string) string { return a.text(locale, key) }
	status := text("TranslateSelectionUnauthorized")
	if a.translateSelectionTrusted {
		status = text("TranslateSelectionAuthorized")
	}
	a.settingsRow(c, t, text("TranslateSelectionLabel"),
		settingsRowOpts{level: 3, tooltip: text("TranslateSelectionInfo")}, func() {
			ui.Text(c, status).FontSize(t.FontSizeSmall).LineHeight(t.LineHeightBody).
				TextColor(ui.Hex(t.ColorTextSecondary)).SingleLine().Label(status)
			a.translateTextButton(c, t, "translate-selection-grant",
				text("TranslateGrantAccess"), a.grantAccessibility)
		})
}

func (a *application) currentTranslateConfig() translateConfig {
	return translateConfig{BaseURL: strings.TrimSpace(a.config.TranslateBaseURL), Model: strings.TrimSpace(a.config.TranslateModel)}
}

// translateFields are the three settings inputs that commit on Return. MyGo
// has no per-element blur callback, so the frame itself reports the focus:
// an input that had it last frame and no longer does is a blur.
const (
	translateFieldBaseURL = "base-url"
	translateFieldKey     = "key"
	translateFieldModel   = "model"
)

// trackTranslateFieldFocus notes this frame's focus for one field. A field
// that loses the focus commits with the same rule as Return, in the same
// frame, so a click on another control cannot leave a draft behind.
func (a *application) trackTranslateFieldFocus(name string, input ui.Element) {
	focused := input.Focused()
	previous := a.translateFieldFocused[name]
	if previous && !focused {
		a.commitTranslateField(name)
	}
	if a.translateFieldFocused == nil {
		a.translateFieldFocused = map[string]bool{}
	}
	a.translateFieldFocused[name] = focused
}

// commitTranslateField applies the Return rule to one field: an unchanged
// value is not written, and an empty key draft never deletes the saved key.
func (a *application) commitTranslateField(name string) {
	switch name {
	case translateFieldBaseURL:
		a.commitTranslateBaseURL()
	case translateFieldKey:
		a.commitTranslateKey()
	case translateFieldModel:
		a.commitTranslateModel()
	}
}

// commitTranslateDrafts commits every uncommitted field. The settings window
// losing the focus, a page switch, a window close, a connection test and the
// floating panel all call it, so no draft depends on Return.
func (a *application) commitTranslateDrafts() {
	a.commitTranslateBaseURL()
	a.commitTranslateKey()
	a.commitTranslateModel()
}

// resetTranslateFieldFocus drops the focus record when the settings view is
// not on screen, so a later frame cannot read a stale "was focused" as a
// blur and commit a value the user is still typing.
func (a *application) resetTranslateFieldFocus() {
	a.translateFieldFocused = nil
}

func (a *application) setTranslateRowError(err error) {
	switch {
	case errors.Is(err, errTranslateHTTPRejected):
		a.translateRowError = "TranslateEndpointHTTPRejected"
	case err != nil:
		a.translateRowError = "TranslateNetworkError"
	default:
		a.translateRowError = ""
	}
}

// syncTranslateDrafts copies the saved values into any field the user is not
// editing, so a write from elsewhere (the command line, the model list) still
// shows up in the input while a rejected draft keeps its own text.
func (a *application) syncTranslateDrafts() {
	if !a.translateBaseURLDirty {
		a.translateBaseURLDraft = a.config.TranslateBaseURL
	}
	if !a.translateModelDirty {
		a.translateModelDraft = a.config.TranslateModel
	}
}

// commitTranslateBaseURL applies the Return rule to the address field. A
// rejected address keeps its draft and the row's own inline error, and is
// never written; an unchanged one writes nothing.
func (a *application) commitTranslateBaseURL() {
	if !a.translateBaseURLDirty {
		// Nothing typed since the last commit: follow the saved value, and
		// leave the row's own error alone so a rejection is not cleared by an
		// unrelated commit.
		a.translateBaseURLDraft = a.config.TranslateBaseURL
		return
	}
	value := strings.TrimRight(strings.TrimSpace(a.translateBaseURLDraft), "/")
	if value == "" {
		// An empty address is "not configured", not a malformed one: it must
		// not raise the network error on its own.
		a.setTranslateRowError(nil)
		a.translateBaseURLDraft, a.translateBaseURLDirty = "", false
		if a.config.TranslateBaseURL == "" {
			return
		}
		a.config.TranslateBaseURL = ""
		a.save()
		return
	}
	_, _, err := translateEndpoints(value)
	if err != nil {
		a.setTranslateRowError(err)
		return
	}
	a.setTranslateRowError(nil)
	a.translateBaseURLDraft, a.translateBaseURLDirty = value, false
	if value == a.config.TranslateBaseURL {
		return
	}
	a.config.TranslateBaseURL = value
	a.save()
}

// commitTranslateModel applies the same rule to the model field.
func (a *application) commitTranslateModel() {
	if !a.translateModelDirty {
		a.translateModelDraft = a.config.TranslateModel
		return
	}
	value := strings.TrimSpace(a.translateModelDraft)
	a.translateModelDraft, a.translateModelDirty = value, false
	if value == a.config.TranslateModel {
		return
	}
	a.config.TranslateModel = value
	a.save()
}

// setTranslateModel is the list's own choice: it is a value the user picked,
// so it is dirty and commits like a typed one.
func (a *application) setTranslateModel(name string) {
	a.translateModelDraft, a.translateModelDirty = strings.TrimSpace(name), true
	a.commitTranslateModel()
}

func (a *application) commitTranslateKey() {
	// An empty draft is not a delete: only the Clear button removes a key.
	value := strings.TrimSpace(a.translateKeyDraft)
	if value == "" {
		return
	}
	if value == a.translateKeyCommitted {
		// The same key again: nothing to write, so the field just collapses.
		a.translateKeyDraft = ""
		a.translateKeyEditing = false
		a.invalidateWindow()
		return
	}
	a.setTranslateAPIKey(value)
}

func (a *application) clearTranslateKeyAndDraft() {
	a.translateKeyDraft = ""
	a.clearTranslateAPIKey()
	a.invalidateWindow()
}

func (a *application) fetchTranslateModelList() {
	if a.translateModelFetching {
		return
	}
	a.commitTranslateDrafts()
	cfg := a.currentTranslateConfig()
	_, _, err := translateEndpoints(cfg.BaseURL)
	if err != nil {
		a.setTranslateRowError(err)
		a.translateModelListError = "TranslateModelFetchFailed"
		a.invalidateWindow()
		return
	}
	a.translateModelFetching = true
	a.translateModelListError = ""
	a.invalidateWindow()
	a.runTranslateWorker(func() {
		key, keyErr := a.translateAPIKey()
		cfg.Key = key
		ctx, cancel := context.WithTimeout(context.Background(), translateProbeTimeout)
		defer cancel()
		var models []string
		var fetchErr error
		if keyErr != nil {
			fetchErr = keyErr
		} else {
			models, fetchErr = a.translateModelsFunction()(ctx, cfg)
		}
		a.translateOnMain(func() {
			a.translateModelFetching = false
			switch {
			case fetchErr != nil:
				a.translateModelList = nil
				a.translateModelListError = "TranslateModelFetchFailed"
			case len(models) == 0:
				a.translateModelList = nil
				a.translateModelListError = "TranslateModelListEmpty"
			default:
				a.translateModelList = models
				a.translateModelListError = ""
			}
			a.invalidateWindow()
		})
	})
}

func (a *application) testTranslateEndpoint() {
	if a.translateTestRunning {
		return
	}
	// The test reads the saved values, so a draft the user just typed must
	// be committed first, exactly as Return would.
	a.commitTranslateDrafts()
	cfg := a.currentTranslateConfig()
	_, _, err := translateEndpoints(cfg.BaseURL)
	a.setTranslateRowError(err)
	if err != nil {
		a.translateTestKey = translateErrorKey(err)
		a.translateTestUntil = time.Now().Add(2 * time.Second)
		a.invalidateWindow()
		return
	}
	a.translateTestRunning = true
	a.translateTestKey = ""
	a.invalidateWindow()
	a.runTranslateWorker(func() {
		key, keyErr := a.translateAPIKey()
		cfg.Key = key
		result := ""
		switch {
		case keyErr != nil:
			result = "TranslateKeyError"
		case strings.TrimSpace(cfg.Model) == "" || key == "":
			result = "TranslateNotConfigured"
		default:
			ctx, cancel := context.WithTimeout(context.Background(), translateProbeTimeout)
			defer cancel()
			probe, probeErr := a.translateTestFunction()(ctx, cfg)
			if probeErr != nil {
				result = "TranslateServerError"
			} else if probe == "" {
				result = "TranslateTestOK"
			} else {
				result = probe
			}
		}
		a.translateOnMain(func() {
			a.translateTestRunning = false
			a.translateTestKey = result
			a.translateTestUntil = time.Now().Add(2 * time.Second)
			a.invalidateWindow()
		})
	})
}
