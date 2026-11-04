package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// cliLinkState is what the general page shows for the command line tool.
type cliLinkState int

const (
	cliAbsent       cliLinkState = iota // the bundle ships no CLI: draw no row
	cliMissing                          // the binary is there, no link yet
	cliInstalled                        // a link to this bundle's CLI
	cliBroken                           // a link to our CLI whose target is gone
	cliConflict                         // a link elsewhere, or a plain file
	cliTranslocated                     // the app runs from a random read-only path
)

// cliPaths is the one link and the binary it should point at.
type cliPaths struct {
	link   string // ~/.local/bin/cubbix
	target string // <resources>/bin/cubbix
	bundle string // the .app, for the translocation check
}

// cliPathsFor computes the paths from the running bundle. Tests replace it.
var cliPathsFor = func() (cliPaths, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return cliPaths{}, false
	}
	resources, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		return cliPaths{}, false
	}
	executable, err := os.Executable()
	if err != nil {
		return cliPaths{}, false
	}
	bundle := ""
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	parts := strings.Split(executable, "/")
	for index, part := range parts {
		if strings.HasSuffix(part, ".app") {
			bundle = strings.Join(parts[:index+1], "/")
			break
		}
	}
	return cliPaths{
		link:   filepath.Join(home, ".local", "bin", "cubbix"),
		target: filepath.Join(resources, "bin", "cubbix"),
		bundle: bundle,
	}, true
}

// available reports whether the bundle ships the tool. Without it the page
// draws no row at all, as the lid row disappears where lid keeping cannot work.
func (p cliPaths) available() bool {
	info, err := os.Stat(p.target)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

// translocated reports an app running from App Translocation: a link into it
// would break on the next launch.
func (p cliPaths) translocated() bool {
	return strings.Contains(p.bundle, "/AppTranslocation/")
}

// status reads the link, without changing anything.
//
// A link of ours whose binary disappeared is broken rather than absent: the row
// stays so the user can see what happened, even though reinstalling cannot repair
// it before the binary is back.
func (p cliPaths) status() cliLinkState {
	info, linkErr := os.Lstat(p.link)
	ours := false
	if linkErr == nil && info.Mode()&os.ModeSymlink != 0 {
		if dest, err := os.Readlink(p.link); err == nil && samePath(dest, p.target) {
			ours = true
		}
	}
	available := p.available()
	if !available && !ours {
		return cliAbsent
	}
	if p.translocated() {
		return cliTranslocated
	}
	if linkErr != nil {
		if errors.Is(linkErr, os.ErrNotExist) {
			return cliMissing
		}
		return cliConflict
	}
	if !ours {
		return cliConflict
	}
	if !available {
		return cliBroken
	}
	return cliInstalled
}

// samePath compares two paths after resolving what exists.
func samePath(first, second string) bool {
	resolve := func(path string) string {
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			return resolved
		}
		return filepath.Clean(path)
	}
	return resolve(first) == resolve(second)
}

// installCLILink creates the link, never replacing anything that is not ours.
func installCLILink(path cliPaths) error {
	if !path.available() {
		return errors.New("CLIInstallFailed")
	}
	if path.translocated() {
		return errors.New("CLITranslocated")
	}
	switch path.status() {
	case cliInstalled:
		return nil
	case cliConflict:
		return errors.New("CLILinkConflict")
	}
	if err := os.MkdirAll(filepath.Dir(path.link), 0o755); err != nil {
		return errors.New("CLIInstallFailed")
	}
	// A broken link of ours is replaced; anything else was refused above.
	if _, err := os.Lstat(path.link); err == nil {
		if err := os.Remove(path.link); err != nil {
			return errors.New("CLIInstallFailed")
		}
	}
	if err := os.Symlink(path.target, path.link); err != nil {
		return errors.New("CLIInstallFailed")
	}
	return nil
}

// removeCLILink removes the link only when it is ours.
func removeCLILink(path cliPaths) error {
	if path.status() != cliInstalled && path.status() != cliBroken {
		return errors.New("CLILinkConflict")
	}
	if err := os.Remove(path.link); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("CLIInstallFailed")
	}
	return nil
}

// panelResultCLI keeps a failed link change on the settings page, where the
// error icon belongs.
func (a *application) cliLinkResult(err error) {
	if err != nil {
		a.settingsError = err.Error()
	} else {
		a.settingsError = ""
	}
	a.invalidateWindow()
}

// cliSettingsRow draws the command line row of the general page. It draws
// nothing when the bundle ships no tool.
func (a *application) cliSettingsRow(c *ui.Context, t tokenValues, locale string) {
	paths, ok := cliPathsFor()
	if !ok {
		return
	}
	state := paths.status()
	if state == cliAbsent {
		return
	}
	text := func(key string) string { return a.text(locale, key) }
	ui.Divider(c)
	a.row(c, t, text("CLIName"), func() {
		install := func() { a.cliLinkResult(installCLILink(paths)) }
		remove := func() { a.cliLinkResult(removeCLILink(paths)) }
		switch state {
		case cliInstalled:
			ui.Icon(c, icon("check")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary)).
				Tooltip(strings.ReplaceAll(text("CLIInstalled"), "%s", paths.link))
			ui.ButtonBase(c.Key("cli-remove")).Size(t.SizeIconButton, t.SizeIconButton).Label(text("CLIRemove")).Tooltip(text("CLIRemove")).OnClick(remove).Children(func() {
				ui.Icon(c, icon("x")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
			})
		case cliBroken, cliConflict:
			reason := text("CLILinkConflict")
			if state == cliBroken {
				reason = text("CLILinkBroken")
			}
			ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorDanger)).
				Focusable().Label(reason).Description(reason).Tooltip(reason)
			ui.ButtonBase(c.Key("cli-install")).Size(t.SizeIconButton, t.SizeIconButton).Label(text("CLIRepair")).Tooltip(text("CLIRepair")).OnClick(install).Children(func() {
				ui.Icon(c, icon("terminal")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
			})
		default:
			button := ui.ButtonBase(c.Key("cli-install")).Size(t.SizeIconButton, t.SizeIconButton).Label(text("CLIInstall")).Tooltip(text("CLIInstall"))
			if state == cliTranslocated {
				button = button.Disabled(true).Opacity(t.OpacityDisabled)
				reason := text("CLITranslocated")
				ui.Icon(c, icon("triangle-alert")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorDanger)).
					Focusable().Label(reason).Description(reason).Tooltip(reason)
			} else {
				button.OnClick(install)
			}
			button.Children(func() {
				ui.Icon(c, icon("terminal")).Size(t.SizeIcon, t.SizeIcon).TextColor(ui.Hex(t.ColorTextSecondary))
			})
		}
	})
}
