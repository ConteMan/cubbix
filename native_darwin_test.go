//go:build darwin

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

// The helper keeps its AppKit window hidden throughout. It never calls the
// application's show/ready paths, Focus, activation APIs or screen capture.
func TestMain(m *testing.M) {
	if os.Getenv("CUBBIX_JSON_NATIVE_HELPER") == "1" {
		os.Exit(jsonNativeMain())
	}
	if os.Getenv("CUBBIX_NATIVE_HELPER") != "1" {
		os.Exit(m.Run())
	}
	dir := os.Getenv("CUBBIX_NATIVE_EVIDENCE")
	mygo.App.SetName("Cubbix")
	mygo.App.SetPath(mygo.PathUserData, filepath.Join(dir, "mygo"))
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	a, err := newApplication(filepath.Join(dir, "config.toml"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	mygo.App.WhenReady(func() {
		a.createSettingsWindow()
		mygo.App.SetMenu(a.applicationMenu("zh-CN"))
		go func() {
			err := nativeWindowChecks(a, dir)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			fmt.Println("PASS: hidden native policy values, menu actions, minimum size and resize persistence; no focus or activation")
			mygo.RunOnMain(func() { a.quitting = true; a.updates.close(); mygo.App.Quit() })
		}()
	})
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func TestNativeWindowConstraints(t *testing.T) {
	dir := os.Getenv("CUBBIX_NATIVE_EVIDENCE")
	if dir == "" {
		dir = t.TempDir()
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// A direct background process, without open, UI automation or foreground requests.
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "CUBBIX_NATIVE_HELPER=1", "CUBBIX_NATIVE_EVIDENCE="+dir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hidden native checks: %v\n%s", err, output)
	}
	t.Log(string(output))
}

func nativeWindowChecks(a *application, dir string) error {
	run := func(fn func() error) error { var err error; mygo.RunOnMain(func() { err = fn() }); return err }
	nsapp := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
	checkHidden := func() error {
		if a.window.IsVisible() || a.window.IsFocused() || objc.Send[bool](nsapp, objc.RegisterName("isActive")) {
			return fmt.Errorf("native helper must stay hidden and inactive")
		}
		return nil
	}
	policy := func(want int) error {
		got := objc.Send[int](nsapp, objc.RegisterName("activationPolicy"))
		if got != want {
			return fmt.Errorf("native activation policy=%d, want %d", got, want)
		}
		fmt.Printf("hidden activationPolicy=%d\n", got)
		return checkHidden()
	}
	if err := run(func() error {
		if err := policy(1); err != nil {
			return err
		}
		// Change policy while the window stays hidden; only read NSApplication's value.
		a.visibility(true)
		if err := policy(0); err != nil {
			return err
		}
		a.visibility(false)
		if err := policy(1); err != nil {
			return err
		}
		menu := nsapp.Send(objc.RegisterName("mainMenu"))
		if objc.Send[int](menu, objc.RegisterName("numberOfItems")) != 3 {
			return fmt.Errorf("application menu not installed")
		}
		editMenu := menu.Send(objc.RegisterName("itemAtIndex:"), 2).Send(objc.RegisterName("submenu"))
		for _, item := range []struct {
			index  int
			action string
		}{{3, "cut:"}, {4, "copy:"}, {5, "paste:"}, {6, "selectAll:"}} {
			nativeItem := editMenu.Send(objc.RegisterName("itemAtIndex:"), item.index)
			if objc.Send[objc.SEL](nativeItem, objc.RegisterName("action")) != objc.RegisterName(item.action) {
				return fmt.Errorf("native edit action missing: %s", item.action)
			}
		}
		win := objc.ID(a.window.NativeHandle())
		style := objc.Send[uint](win, objc.RegisterName("styleMask"))
		if style&8 == 0 {
			return fmt.Errorf("native window is not resizable")
		}
		if win.Send(objc.RegisterName("standardWindowButton:"), 0) == 0 {
			return fmt.Errorf("native close button missing")
		}
		minimum := objc.Send[struct{ Width, Height float64 }](win, objc.RegisterName("minSize"))
		mw, mh := settingsMinimumSize()
		if int(minimum.Width) != mw || int(minimum.Height) != mh {
			return fmt.Errorf("native minimum=%+v, want %dx%d", minimum, mw, mh)
		}
		fmt.Printf("native minimum=%dx%d\n", mw, mh)
		a.window.SetSize(1000, 600)
		a.rememberWindowSize()
		saved, err := loadConfig(a.path)
		if err != nil {
			return err
		}
		if saved.WindowWidth != 1000 || saved.WindowHeight != 600 {
			return fmt.Errorf("resize not persisted: %+v", saved)
		}

		for _, kind := range []string{"custom", "until", "follow"} {
			d := &awakeDialog{kind: kind, hours: "1", minutes: "30", until: "02:30"}
			a.dialog = d
			a.createAwakeDialogWindow(d)
			if d.window.IsVisible() || d.window.IsFocused() {
				return fmt.Errorf("dialog became visible or focused")
			}
			native := objc.ID(d.window.NativeHandle())
			mask := objc.Send[uint](native, objc.RegisterName("styleMask"))
			if mask&8 != 0 || objc.Send[int](native, objc.RegisterName("titleVisibility")) != 1 || !objc.Send[bool](native, objc.RegisterName("titlebarAppearsTransparent")) {
				return fmt.Errorf("dialog has titlebar or is resizable: %s %d", kind, mask)
			}
			for button := 0; button < 3; button++ {
				if !objc.Send[bool](native.Send(objc.RegisterName("standardWindowButton:"), button), objc.RegisterName("isHidden")) {
					return fmt.Errorf("dialog system buttons visible")
				}
			}
			a.closeAwakeDialog()
			if err := checkHidden(); err != nil {
				return err
			}
		}
		data, _ := assets.ReadFile("assets/menubar-template.tiff")
		tray, err := mygo.NewTray(mygo.TrayOptions{Icon: data, IconIsTemplate: true, Menu: a.menuForLocale("en")})
		if err != nil {
			return err
		}
		defer tray.Destroy()
		active, _ := assets.ReadFile("assets/menubar-template-active.tiff")
		if err := tray.SetIcon(active, true); err != nil {
			return err
		}
		tray.SetMenu(a.menuForLocale("zh-CN"))
		if err := tray.SetIcon(data, true); err != nil {
			return err
		}
		const shortcut = "Ctrl+Alt+Cmd+Shift+F10"
		if err := mygo.GlobalShortcut.Register(shortcut, func() {}); err != nil {
			return fmt.Errorf("native global shortcut registration: %w", err)
		}
		if !mygo.GlobalShortcut.IsRegistered(shortcut) {
			return fmt.Errorf("global shortcut not registered")
		}
		mygo.GlobalShortcut.Unregister(shortcut)
		if mygo.GlobalShortcut.IsRegistered(shortcut) {
			return fmt.Errorf("global shortcut not released")
		}
		fmt.Println("PASS: three hidden frameless dialogs; native template icon swapping and translated tray menus; global shortcut registered and released")
		a.window.Close()
		return policy(1)
	}); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "native-result.txt"), []byte("PASS: hidden NSApplication policies 1→0→1; native resizable style, minimum 720×340; saved 1000×600; application/edit menu actions and close button present. Window stayed hidden and inactive. No native screenshots or foreground requests.\n"), 0600)
}
