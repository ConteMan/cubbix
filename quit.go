package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
)

const quitBudget = 5 * time.Second

// The timer stays armed until process exit, including MyGo's finish/relaunch.
// App.Exit also waits for the main thread, so it cannot enforce this deadline.
// The helper's EOF/lease recovery and caffeinate's PID binding cover forced exit.
type quitWatchdog struct {
	once    sync.Once
	restart atomic.Bool
	budget  time.Duration
	expire  func(bool)
}

func (g *quitWatchdog) arm(restart bool) {
	if restart {
		g.restart.Store(true)
	}
	g.once.Do(func() {
		time.AfterFunc(g.budget, func() { g.expire(g.restart.Load()) })
	})
}

func (a *application) installQuitWatchdog() {
	// Capture the logical bundle path before an updater renames/deletes it.
	exe, _ := os.Executable()
	a.quitGuard = &quitWatchdog{budget: quitBudget, expire: func(restart bool) {
		log.Print("quit deadline exceeded; terminating after cleanup budget")
		if restart {
			if err := relaunchAfterExit(exe, os.Getpid()); err != nil {
				log.Printf("fallback relaunch failed: %v", err)
				os.Exit(1)
			}
		}
		os.Exit(0)
	}}
	a.relaunch = func() {
		a.quitGuard.arm(true)
		mygo.App.Relaunch()
	}
}

// Keep the native relaunch path for normal completion. Only the deadline uses
// a waiter, so it cannot launch a second instance during a successful restart.
func relaunchAfterExit(exe string, pid int) error {
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if !filepath.IsAbs(exe) || !strings.HasSuffix(bundle, ".app") {
		return fmt.Errorf("relaunch requires an app bundle")
	}
	// Arguments are separate from the constant shell program. Wait for the old
	// process (and its single-instance lock) to disappear, then launch quietly.
	cmd := exec.Command("/bin/sh", "-c", `while /bin/kill -0 "$1" 2>/dev/null; do /bin/sleep 0.1; done
exec /usr/bin/open -g -n "$2"`, "cubbix-relaunch", fmt.Sprint(pid), bundle)
	cmd.Dir = "/"
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
