package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// translatePageHeight is the natural height of the sixth settings page. The
// settings card has four rows (① 48, ② 64, ③ 48, ① 48) and the shortcut
// card one; with a closed module only the shortcut card stays.
func translatePageHeight(t tokenValues, on bool) float32 {
	if !on {
		return sectionHeight(t, cardHeight(t, rowHeight(t, 3)))
	}
	settings := cardHeight(t, rowHeight(t, 1), rowHeight(t, 2), rowHeight(t, 3), rowHeight(t, 1))
	return sectionHeight(t, settings, cardHeight(t, rowHeight(t, 3)))
}

func (a *application) translateModulePage(c *ui.Context, t tokenValues, locale string, l settingsLayout, alert string) {
	text := func(key string) string { return a.text(locale, key) }
	on := l.translateOn
	a.pageHeader(c, t, locale, "text-quote", "Translate", "TranslatePageDesc", alert, func() {
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
		ui.TextInput(c.Key("translate-base-url"), &a.config.TranslateBaseURL).
			Width(240).Height(t.SizeSegmentHeight).PaddingX(t.Space8).
			Radius(t.RadiusControl).Border(t.StrokeHairline, ui.Hex(t.ColorSeparator)).
			Background(ui.Hex(t.ColorControl)).FontSize(t.FontSizeBody).
			Label(text("TranslateBaseURL")).Tooltip(text("TranslateBaseURLInfo")).
			Description(text("TranslateBaseURLInfo")).FocusRing(true).OnSubmit(a.commitTranslateBaseURL)
		if a.translateRowError != "" {
			a.alertIcon(c, t, text(a.translateRowError))
		}
	})
}

func (a *application) translateKeyRow(c *ui.Context, t tokenValues, locale string) {
	text := func(key string) string { return a.text(locale, key) }
	status := text("TranslateKeyMissing")
	if a.translateKeySaved {
		status = text("TranslateKeySaved")
	}
	a.settingsRow(c, t, text("TranslateAPIKey"), settingsRowOpts{level: 2, hint: text("TranslateKeyHint")}, func() {
		a.statusDot(c, t, a.translateKeySaved, status)
		ui.Text(c, status).FontSize(t.FontSizeSmall).TextColor(ui.Hex(t.ColorTextSecondary)).SingleLine()
		ui.TextInput(c.Key("translate-key"), &a.translateKeyDraft).
			Width(180).Height(t.SizeSegmentHeight).PaddingX(t.Space8).
			Radius(t.RadiusControl).Border(t.StrokeHairline, ui.Hex(t.ColorSeparator)).
			Background(ui.Hex(t.ColorControl)).FontSize(t.FontSizeBody).
			Placeholder(text("TranslateKeyPlaceholder")).Label(text("TranslateAPIKey")).
			FocusRing(true).Password().OnSubmit(a.commitTranslateKey)
		a.translateTextButton(c, t, "translate-key-replace", text("TranslateKeyReplace"), a.commitTranslateKey)
		a.translateTextButton(c, t, "translate-key-clear", text("TranslateKeyClear"), a.clearTranslateKeyAndDraft)
		if a.translateKeyError != "" {
			a.alertIcon(c, t, text(a.translateKeyError))
		}
	})
}

func (a *application) translateModelRow(c *ui.Context, t tokenValues, locale string) {
	text := func(key string) string { return a.text(locale, key) }
	a.settingsRow(c, t, text("TranslateModel"), settingsRowOpts{level: 3, tooltip: text("TranslateModelInfo")}, func() {
		ui.TextInput(c.Key("translate-model"), &a.config.TranslateModel).
			Width(200).Height(t.SizeSegmentHeight).PaddingX(t.Space8).
			Radius(t.RadiusControl).Border(t.StrokeHairline, ui.Hex(t.ColorSeparator)).
			Background(ui.Hex(t.ColorControl)).FontSize(t.FontSizeBody).
			Label(text("TranslateModel")).FocusRing(true).OnSubmit(a.commitTranslateModel)
		selected := a.config.TranslateModel
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
		test := ui.ButtonBase(c.Key("translate-test")).Size(t.SizeIconButton, t.SizeIconButton).
			Radius(t.RadiusControl).FocusRing(true).Label(testTip).Tooltip(testTip)
		if !a.translateTestRunning {
			test = test.OnClick(a.testTranslateEndpoint)
		}
		test.Children(func() {
			ui.Row(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				glyph := "refresh-cw"
				if a.translateTestRunning {
					glyph = "loader-circle"
				} else if a.translateTestKey == "TranslateTestOK" {
					glyph = "check"
				} else if a.translateTestKey != "" {
					glyph = "triangle-alert"
				}
				ui.Icon(c, icon(glyph)).Size(t.SizeIconSmall, t.SizeIconSmall).
					TextColor(ui.Hex(t.ColorTextSecondary))
			})
		})
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

func (a *application) translateAutoSpeakRow(c *ui.Context, t tokenValues, locale string) {
	text := func(key string) string { return a.text(locale, key) }
	on := a.config.TranslateAutoSpeak
	a.settingsRow(c, t, text("TranslateAutoSpeak"), settingsRowOpts{level: 1, hint: text("TranslateAutoSpeakHint")}, func() {
		// This step only stores the preference; speaking is a later step.
		a.switchControl(c, t, "translate-auto-speak", text("TranslateAutoSpeak"), &on, func() {
			a.changeTranslateAutoSpeak(!a.config.TranslateAutoSpeak)
		}, "", false)
	})
}

func (a *application) currentTranslateConfig() translateConfig {
	return translateConfig{BaseURL: strings.TrimSpace(a.config.TranslateBaseURL), Model: strings.TrimSpace(a.config.TranslateModel)}
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

func (a *application) commitTranslateBaseURL() {
	value := strings.TrimRight(strings.TrimSpace(a.config.TranslateBaseURL), "/")
	_, _, err := translateEndpoints(value)
	a.config.TranslateBaseURL = value
	a.setTranslateRowError(err)
	a.save()
}

func (a *application) commitTranslateModel() {
	a.config.TranslateModel = strings.TrimSpace(a.config.TranslateModel)
	a.save()
}

func (a *application) setTranslateModel(name string) {
	a.config.TranslateModel = strings.TrimSpace(name)
	a.save()
}

func (a *application) commitTranslateKey() {
	_ = a.setTranslateAPIKey(a.translateKeyDraft)
}

func (a *application) clearTranslateKeyAndDraft() {
	a.translateKeyDraft = ""
	_ = a.clearTranslateAPIKey()
	a.invalidateWindow()
}

func (a *application) fetchTranslateModelList() {
	if a.translateModelFetching {
		return
	}
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
