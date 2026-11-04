package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

// Launch a separate test bundle with its own identifier; every window stays
// hidden and open -g never requests foreground activation.
func TestQuitHiddenPaletteAndDeadline(t *testing.T) {
	for _, mode := range []string{"normal", "canceled-window", "blocked-cleanup"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			bundle := filepath.Join(dir, "QuitTest.app")
			contents := filepath.Join(bundle, "Contents")
			exe := filepath.Join(contents, "MacOS", "QuitTest")
			if err := os.MkdirAll(filepath.Dir(exe), 0700); err != nil {
				t.Fatal(err)
			}
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			binary, err := os.ReadFile(self)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(exe, binary, 0700); err != nil {
				t.Fatal(err)
			}
			plist := fmt.Sprintf(`<?xml version="1.0"?><plist version="1.0"><dict><key>CFBundleExecutable</key><string>QuitTest</string><key>CFBundleIdentifier</key><string>org.example.cubbix.quittest.%d.%s</string><key>CFBundlePackageType</key><string>APPL</string><key>LSUIElement</key><true/></dict></plist>`, os.Getpid(), mode)
			if err := os.WriteFile(filepath.Join(contents, "Info.plist"), []byte(plist), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", bundle).CombinedOutput(); err != nil {
				t.Fatalf("sign test bundle: %v: %s", err, out)
			}
			t.Cleanup(func() {
				b, _ := os.ReadFile(filepath.Join(dir, "pid"))
				pid, _ := strconv.Atoi(string(b))
				if pid > 0 {
					comm, _ := exec.Command("/bin/ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
					if strings.TrimSpace(string(comm)) == exe {
						_ = syscall.Kill(pid, syscall.SIGKILL)
					}
				}
			})
			out, err := exec.Command("/usr/bin/open", "-g", "-n", "--env", "CUBBIX_QUIT_NATIVE_HELPER="+mode, "--env", "CUBBIX_QUIT_TEST_DIR="+dir, bundle).CombinedOutput()
			if err != nil {
				t.Fatalf("open background test bundle: %v: %s", err, out)
			}
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				result, _ := os.ReadFile(filepath.Join(dir, "result"))
				pidBytes, _ := os.ReadFile(filepath.Join(dir, "pid"))
				pid, _ := strconv.Atoi(string(pidBytes))
				if len(result) > 0 && pid > 0 && syscall.Kill(pid, 0) != nil {
					if string(result) != "PASS" {
						t.Fatal(string(result))
					}
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
			t.Fatal("background quit helper did not complete and exit")
		})
	}
}

func quitNativeMain() int {
	dir, mode := os.Getenv("CUBBIX_QUIT_TEST_DIR"), os.Getenv("CUBBIX_QUIT_NATIVE_HELPER")
	write := func(result string) { _ = os.WriteFile(filepath.Join(dir, "result"), []byte(result), 0600) }
	_ = os.WriteFile(filepath.Join(dir, "pid"), []byte(strconv.Itoa(os.Getpid())), 0600)
	mygo.App.SetPath(mygo.PathUserData, filepath.Join(dir, "profile"))
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	a, err := newApplication(filepath.Join(dir, "config.toml"))
	if err != nil {
		write(err.Error())
		return 1
	}
	a.installQuitWatchdog()
	a.quitGuard.budget = time.Second
	a.quitGuard.expire = func(bool) {
		if mode == "normal" {
			write("normal quit hit deadline")
		} else {
			write("PASS")
		}
		os.Exit(0)
	}
	mygo.App.OnBeforeQuit(a.beforeQuit)
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnQuit(func() {
		if mode == "normal" {
			write("PASS")
		} else {
			write("failure fixture did not reach deadline")
		}
	})
	mygo.App.WhenReady(func() {
		a.createSettingsWindow()
		a.palette = &paletteState{}
		a.buildPaletteWindow(a.palette)
		// Ordinary close hides and retains the panel; app quit must destroy it.
		a.palette.window.Close()
		if len(mygo.Windows()) != 2 {
			write("ordinary panel close destroyed a window")
			os.Exit(1)
		}
		nsapp := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
		for _, w := range mygo.Windows() {
			if w.IsVisible() || w.IsFocused() || objc.Send[bool](nsapp, objc.RegisterName("isActive")) {
				write("test activated a window")
				os.Exit(1)
			}
		}
		switch mode {
		case "canceled-window":
			a.palette.window.OnClose(func(e *mygo.CloseEvent) { e.PreventDefault() })
		case "blocked-cleanup":
			a.stopAwakeServices = func() { select {} }
		}
		mygo.App.Quit()
	})
	if err := mygo.App.Run(); err != nil {
		write(err.Error())
		return 1
	}
	return 0
}
