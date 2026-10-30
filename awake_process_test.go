package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestPickerProcessClassificationAndSearch(t *testing.T) {
	row := func(uid int, tty string, pid int, state, name string) string {
		return fmt.Sprintf("%d %s %d %s Fri Oct 30 10:00:00 2026 %s\n", uid, tty, pid, state, name)
	}
	data := row(501, "??", 10, "S", "/Applications/Example App.app/Contents/MacOS/Example App") +
		row(501, "ttys000", 11, "S+", "/usr/local/bin/codex") +
		row(501, "??", 12, "S", "/usr/libexec/helper") +
		row(0, "ttys000", 13, "S", "/usr/bin/other-user") +
		row(501, "ttys001", 14, "S", "/bin/zsh") +
		row(501, "ttys001", 15, "Z", "/bin/zombie") +
		row(501, "ttys001", 16, "S", "/usr/bin/clang") +
		"bad process record\n"
	p := parsePickerProcesses(data, []graphicalProcess{{PID: 10, Name: "Example App", BundlePath: "/Applications/Example App.app"}}, 501)
	if len(p) != 4 || p[0].Name != "Example App" || p[0].BundlePath == "" || p[1].Name != "codex" || p[1].BundlePath != "" || p[1].Identity != "Fri Oct 30 10:00:00 2026" {
		t.Fatalf("classification: %+v", p)
	}
	for _, tc := range []struct {
		index int
		query string
		want  bool
	}{{0, "example APP", true}, {0, "10", true}, {1, "CoDeX", true}, {1, "absent", false}, {2, "", false}, {2, "14", false}, {2, "zs", false}, {2, " zsh ", true}} {
		if got := matchesProcess(p[tc.index], tc.query); got != tc.want {
			t.Errorf("%s matches %q = %v", p[tc.index].Name, tc.query, got)
		}
	}
	for _, name := range []string{"zsh", "bash", "fish", "login", "-zsh", "sh", "dash", "ksh", "csh", "tcsh"} {
		p := runningProcess{PID: 1, Name: name}
		if matchesProcess(p, "") || !matchesProcess(p, strings.TrimPrefix(name, "-")) {
			t.Errorf("shell filter: %s", name)
		}
	}
}

func TestPickerAgentPriorityConfig(t *testing.T) {
	p := []runningProcess{{PID: 1, Name: "Safari"}, {PID: 2, Name: "codex-helper"}, {PID: 3, Name: "gemini"}, {PID: 4, Name: "Codex"}, {PID: 5, Name: "aider"}, {PID: 6, Name: "claude"}}
	sortPickerProcesses(p, defaultProcessAgents)
	var names []string
	for _, p := range p {
		names = append(names, p.Name)
	}
	if !reflect.DeepEqual(names, []string{"aider", "claude", "Codex", "gemini", "codex-helper", "Safari"}) {
		t.Fatal(names)
	}
	c := defaultConfig()
	c.AwakeAgents = "Safari, custom-agent"
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := saveConfig(path, c); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil || loaded.AwakeAgents != c.AwakeAgents {
		t.Fatalf("agent config: %v %v", loaded.AwakeAgents, err)
	}
	sortPickerProcesses(p, strings.Split(loaded.AwakeAgents, ","))
	if p[0].Name != "Safari" {
		t.Fatal(p)
	}
}

func TestPickerDebounceAndKeyboard(t *testing.T) {
	a := awakeTestApp(t)
	d := &awakeDialog{kind: "follow", processes: []runningProcess{{PID: 1, Name: "codex", Identity: "one"}, {PID: 2, Name: "claude", Identity: "two"}, {PID: 3, Name: "zsh", Identity: "three"}}}
	a.dialog = d
	tt := ui.NewTester(a.awakeDialogView, 440, 280)
	tt.SetLocale("en")
	tt.Key(0, ui.KeyDown)
	if d.selected != 2 {
		t.Fatalf("Down from search selected %d", d.selected)
	}
	tt.Key(0, ui.KeyUp)
	if d.selected != 1 {
		t.Fatal("Up did not select previous row")
	}
	tt.Type("cl")
	tt.Advance(60 * time.Millisecond)
	tt.Type("a")
	tt.Advance(60 * time.Millisecond)
	if d.filteredQuery != "" || len(d.filtered) != 2 {
		t.Fatalf("filter was not debounced: query=%q filtered=%q", d.search, d.filteredQuery)
	}
	tt.Advance(41 * time.Millisecond)
	if d.filteredQuery != "cla" || len(d.filtered) != 1 || d.selected != 2 {
		t.Fatalf("search result: query=%q filtered=%q selected=%d rows=%v", d.search, d.filteredQuery, d.selected, d.filtered)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("zsh")
	// Enter during debounce must use the current search, never the old PID.
	tt.Key(0, ui.KeyEnter)
	if a.dialog != nil || a.awake.snapshot().Session.PID != 3 {
		t.Fatalf("Enter followed stale search: %+v", a.awake.snapshot())
	}
}

func TestPickerVirtualListRemainsResponsiveWithSlowIcons(t *testing.T) {
	a := awakeTestApp(t)
	unblock := make(chan struct{})
	defer close(unblock)
	var calls atomic.Int32
	cache := newProcessIconCache(func(string) *ui.Bitmap {
		calls.Add(1)
		<-unblock
		return nil
	})
	d := &awakeDialog{kind: "follow", iconCache: cache}
	for i := 0; i < 2000; i++ {
		d.processes = append(d.processes, runningProcess{PID: i + 1, Name: fmt.Sprintf("App %04d", i), Identity: "start", BundlePath: fmt.Sprintf("/mock/%d.app", i)})
	}
	a.dialog = d
	start := time.Now()
	tt := ui.NewTester(a.awakeDialogView, 440, 280)
	elapsed := time.Since(start)
	if elapsed >= 200*time.Millisecond {
		t.Fatalf("2000-row first frame took %v", elapsed)
	}
	cache.mu.Lock()
	requested := len(cache.entries)
	cache.mu.Unlock()
	if requested == 0 || requested > 8 || tt.HasText("App 1000") || calls.Load() > 2 {
		t.Fatalf("not virtualized: requests=%d active=%d", requested, calls.Load())
	}
	for range 12 {
		tt.Key(0, ui.KeyDown)
	}
	if d.selected != 13 || !tt.HasText("App 0012") {
		t.Fatalf("keyboard did not scroll to selection: selected=%d texts=%v", d.selected, tt.Texts())
	}
	// The OS/icon loader is still blocked, but search and cancel must work.
	tt.Type("App 1999")
	tt.Advance(101 * time.Millisecond)
	if len(d.filtered) != 1 || d.selected != 2000 || !tt.HasText("App 1999") {
		t.Fatalf("large search: selected=%d results=%v", d.selected, d.filtered)
	}
	tt.Key(0, ui.KeyEscape)
	if a.dialog != nil {
		t.Fatal("slow icon blocked Escape")
	}
	t.Logf("2000 rows: first frame=%v, initial icon requests=%d (two workers max)", elapsed, requested)
}

func TestPickerIconCacheAndTTYFallback(t *testing.T) {
	var calls atomic.Int32
	ready := make(chan struct{}, 10)
	bitmap := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 2, 2)))
	cache := newProcessIconCache(func(string) *ui.Bitmap { calls.Add(1); return bitmap })
	if b, pending := cache.request("", func() { t.Error("TTY icon requested") }); b != nil || pending {
		t.Fatal("TTY must use terminal fallback")
	}
	for range 2 {
		cache.request("/mock/Same.app", func() { ready <- struct{}{} })
	}
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("icon never completed")
	}
	if b, pending := cache.request("/mock/Same.app", func() {}); b != bitmap || pending || calls.Load() != 1 {
		t.Fatal("bundle cache did not reuse bitmap")
	}
}

func TestPickerAsyncListCancelAndError(t *testing.T) {
	a := awakeTestApp(t)
	d := &awakeDialog{kind: "follow"}
	a.dialog = d
	started, ended := make(chan struct{}), make(chan struct{})
	d.loadProcesses(func(ctx context.Context) ([]runningProcess, error) {
		close(started)
		<-ctx.Done()
		close(ended)
		return nil, ctx.Err()
	}, nil, func() { t.Error("closed dialog invalidated") })
	<-started
	tt := ui.NewTester(a.awakeDialogView, 440, 280)
	tt.Key(0, ui.KeyEscape)
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("closing dialog did not cancel discovery")
	}
	d = &awakeDialog{kind: "follow"}
	ready := make(chan struct{}, 1)
	d.loadProcesses(func(context.Context) ([]runningProcess, error) { return nil, errors.New("ps failed") }, nil, func() { ready <- struct{}{} })
	<-ready
	a.dialog = d
	tt = ui.NewTester(a.awakeDialogView, 440, 280)
	if d.loading || d.errorKey != "ProcessListError" {
		t.Fatal("discovery error was not shown")
	}
	tt.Key(0, ui.KeyEnter)
	if a.dialog == nil || a.awake.snapshot().Active {
		t.Fatal("failed discovery allowed confirm")
	}
}

func TestPickerDiscoveryDeadlineShowsError(t *testing.T) {
	d := &awakeDialog{kind: "follow"}
	ready := make(chan struct{}, 1)
	d.loadProcesses(func(ctx context.Context) ([]runningProcess, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}, nil, func() { ready <- struct{}{} })
	select {
	case <-ready:
	case <-time.After(4 * time.Second):
		t.Fatal("deadline left picker spinning")
	}
	d.receiveProcesses()
	if d.loading || d.errorKey != "ProcessListError" {
		t.Fatal("deadline must stop spinner and report discovery failure")
	}
}

// Opt-in evidence uses the production background discovery and ui.Tester only.
// No window is shown, no activation APIs or notifications are invoked.
func TestPickerLocalLatency(t *testing.T) {
	dir := os.Getenv("CUBBIX_PICKER_EVIDENCE")
	if dir == "" {
		t.Skip("local picker evidence not requested")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	preparePickerNativeEvidence(t)
	a := awakeTestApp(t)
	for run := 0; run < 5; run++ {
		d := &awakeDialog{kind: "follow", iconCache: pickerIcons}
		a.dialog = d
		ready := make(chan struct{}, 1)
		start := time.Now()
		d.loadProcesses(listProcesses, defaultProcessAgents, func() { ready <- struct{}{} })
		tt := ui.NewTester(a.awakeDialogView, 440, 280)
		select {
		case <-ready:
		case <-time.After(3 * time.Second):
			t.Fatal("process discovery timed out")
		}
		tt.Frame()
		elapsed := time.Since(start)
		if d.errorKey != "" || len(d.filtered) == 0 || elapsed >= 200*time.Millisecond {
			t.Fatalf("local first list=%v rows=%d error=%s", elapsed, len(d.filtered), d.errorKey)
		}
		guiCount := 0
		for _, process := range d.processes {
			if process.BundlePath != "" {
				guiCount++
			}
		}
		if guiCount == 0 {
			t.Fatal("local evidence must exercise native GUI metadata too")
		}
		t.Logf("local run %d: discovery + first headless list frame=%v, candidates=%d shown=%d GUI=%d", run+1, elapsed, len(d.processes), len(d.filtered), guiCount)
		if run != 0 {
			continue
		}
		tt.SetScale(2)
		// Let visible icons finish while the tester stays on this goroutine.
		for range 60 {
			time.Sleep(25 * time.Millisecond)
			tt.Frame()
		}
		for _, locale := range []string{"zh-CN", "en"} {
			for _, dark := range []bool{false, true} {
				tt.SetLocale(locale)
				tt.SetDark(dark)
				mode := "light"
				if dark {
					mode = "dark"
				}
				file, err := os.Create(filepath.Join(dir, "picker-"+locale+"-"+mode+".png"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, tt.Image())
				file.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		// Also inspect actual GUI icons: agents may fill the initial viewport.
		for _, process := range d.processes {
			if process.BundlePath == "" {
				continue
			}
			d.search = process.Name
			d.filterProcesses()
			tt.Frame()
			deadline := time.Now().Add(2 * time.Second)
			var bitmap *ui.Bitmap
			for time.Now().Before(deadline) {
				tt.Frame()
				pickerIcons.mu.Lock()
				if entry := pickerIcons.entries[process.BundlePath]; entry != nil && entry.done {
					bitmap = entry.bitmap
				}
				pickerIcons.mu.Unlock()
				if bitmap != nil {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if bitmap == nil {
				pickerIcons.mu.Lock()
				entry := pickerIcons.entries[process.BundlePath]
				pickerIcons.mu.Unlock()
				first, last := d.list.Visible()
				t.Fatalf("visible GUI icon did not load: entry=%+v range=%d..%d filtered=%v", entry, first, last, d.filtered)
			}
			tt.Frame()
			file, err := os.Create(filepath.Join(dir, "picker-gui-icon.png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(file, tt.Image())
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
			t.Log("visible GUI icon loaded and rendered off the discovery path")
			break
		}
	}
}
