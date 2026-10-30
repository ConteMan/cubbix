package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
)

type runningProcess struct {
	PID            int
	Name, Identity string
	BundlePath     string // Nonempty only for regular graphical applications.
}

type graphicalProcess struct {
	PID              int
	Name, BundlePath string
}

var defaultProcessAgents = []string{"codex", "claude", "gemini", "aider"}

// Both native discovery and ps run on the caller's background goroutine.
// A single ps snapshot supplies start identities, never one subprocess per PID.
func listProcesses(ctx context.Context) ([]runningProcess, error) {
	apps := runningGUIApplications()
	data, err := exec.CommandContext(ctx, "/bin/ps", "-axo", "uid=,tty=,pid=,stat=,lstart=,comm=").Output()
	if err != nil {
		return nil, err
	}
	return parsePickerProcesses(string(data), apps, os.Getuid()), nil
}

func parsePickerProcesses(data string, apps []graphicalProcess, uid int) []runningProcess {
	gui := make(map[int]graphicalProcess, len(apps))
	for _, app := range apps {
		gui[app.PID] = app
	}
	var out []runningProcess
	for _, line := range strings.Split(data, "\n") {
		f := strings.Fields(line)
		if len(f) < 10 || strings.Contains(f[3], "Z") {
			continue
		}
		owner, e1 := strconv.Atoi(f[0])
		pid, e2 := strconv.Atoi(f[2])
		if e1 != nil || e2 != nil || pid <= 0 {
			continue
		}
		app, graphical := gui[pid]
		if !graphical && (owner != uid || f[1] == "??" || f[1] == "?") {
			continue
		}
		p := runningProcess{PID: pid, Name: filepath.Base(strings.Join(f[9:], " ")), Identity: strings.Join(f[4:9], " ")}
		if graphical {
			p.Name, p.BundlePath = app.Name, app.BundlePath
		}
		out = append(out, p)
	}
	return out
}

func sortPickerProcesses(processes []runningProcess, agents []string) {
	preferred := make(map[string]bool, len(agents))
	for _, name := range agents {
		preferred[strings.ToLower(strings.TrimSpace(name))] = true
	}
	sort.SliceStable(processes, func(i, j int) bool {
		a, b := strings.ToLower(processes[i].Name), strings.ToLower(processes[j].Name)
		if preferred[a] != preferred[b] {
			return preferred[a]
		}
		if a == b {
			return processes[i].PID < processes[j].PID
		}
		return a < b
	})
}

func matchesProcess(p runningProcess, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	name := strings.ToLower(p.Name)
	if p.BundlePath == "" {
		switch strings.TrimPrefix(name, "-") {
		case "zsh", "bash", "fish", "sh", "dash", "ksh", "csh", "tcsh", "login":
			// A PID or partial name does not explicitly ask for a shell.
			if query != strings.TrimPrefix(name, "-") {
				return false
			}
		}
	}
	return strings.Contains(name+" "+strconv.Itoa(p.PID), query)
}

type processListResult struct {
	processes []runningProcess
	err       error
}

func (d *awakeDialog) loadProcesses(load func(context.Context) ([]runningProcess, error), agents []string, invalidate func()) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	d.cancelLoad, d.loading = cancel, true
	results := make(chan processListResult, 1)
	d.results = results
	agents = append([]string(nil), agents...)
	go func() {
		defer cancel()
		processes, err := load(ctx)
		sortPickerProcesses(processes, agents)
		if ctx.Err() == context.Canceled {
			return
		}
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		results <- processListResult{processes, err}
		invalidate()
	}()
}

// Results are adopted only by a frame, so closing a dialog cannot deliver stale
// data into a new one and worker goroutines never mutate UI state.
func (d *awakeDialog) receiveProcesses() {
	select {
	case result := <-d.results:
		d.loading = false
		if result.err != nil {
			d.errorKey = "ProcessListError"
		} else {
			d.processes = result.processes
			d.filterProcesses()
		}
	default:
	}
}

func (d *awakeDialog) filterProcesses() {
	d.filtered = d.filtered[:0]
	for _, p := range d.processes {
		if matchesProcess(p, d.search) {
			d.filtered = append(d.filtered, p)
		}
	}
	d.filteredQuery, d.filterReady = d.search, true
	d.selected = 0
	if len(d.filtered) > 0 {
		d.selected = d.filtered[0].PID
	}
	d.list.ScrollTo(0, ui.Start)
}

func (d *awakeDialog) moveProcessSelection(delta int) {
	// Confirm/navigation must never act on results from an older query.
	if !d.filterReady || d.filteredQuery != d.search {
		d.filterProcesses()
	}
	if len(d.filtered) == 0 {
		return
	}
	index := 0
	for i, p := range d.filtered {
		if p.PID == d.selected {
			index = max(0, min(len(d.filtered)-1, i+delta))
			break
		}
	}
	d.selected = d.filtered[index].PID
	d.list.ScrollIntoView(index)
}

type processIconEntry struct {
	bitmap *ui.Bitmap
	done   bool
}

// Bitmaps are built off-thread and then owned by the UI. The mutex protects
// publication only; no native calls, decoding, or waiting happen under it.
type processIconCache struct {
	mu      sync.Mutex
	entries map[string]*processIconEntry
	workers chan struct{}
	load    func(string) *ui.Bitmap
}

func newProcessIconCache(load func(string) *ui.Bitmap) *processIconCache {
	return &processIconCache{entries: map[string]*processIconEntry{}, workers: make(chan struct{}, 2), load: load}
}

func (cache *processIconCache) request(path string, invalidate func()) (*ui.Bitmap, bool) {
	if path == "" {
		return nil, false
	}
	cache.mu.Lock()
	if entry, ok := cache.entries[path]; ok {
		bitmap, done := entry.bitmap, entry.done
		cache.mu.Unlock()
		return bitmap, !done
	}
	cache.entries[path] = &processIconEntry{}
	cache.mu.Unlock()
	go func() {
		cache.workers <- struct{}{}
		bitmap := cache.load(path)
		<-cache.workers
		cache.mu.Lock()
		cache.entries[path] = &processIconEntry{bitmap: bitmap, done: true}
		cache.mu.Unlock()
		invalidate()
	}()
	return nil, true
}

var pickerIcons = newProcessIconCache(graphicalAppIcon)
