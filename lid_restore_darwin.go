package main

import (
	"context"
	"errors"
	"github.com/ConteMan/cubbix/internal/lid"
	"github.com/egoist/mygo"
	"net"
	"os/exec"
	"path/filepath"
	"time"
)

type commandSleepForcer struct{ admin func(string) error }

func (f commandSleepForcer) Force() error { return f.admin(lidForceScriptApple) }

// lidBundleCheckArgs verifies the installed app against the helper's client
// requirement. codesign reads a bare -R value as a file path; the leading "="
// marks it as requirement text.
func lidBundleCheckArgs() []string {
	return []string{"-v", "-R", "=" + lid.ClientRequirement, "/Applications/Cubbix.app"}
}

// Production authorization is wired only here. Construction does not execute it.
func (a *application) prepareLid() {
	c := &lidController{now: time.Now, marker: filepath.Join(filepath.Dir(a.path), "lid-restore-required"), snapshotValue: lidSnapshot{value: -1}}
	c.capable = !mygo.IsDev() && mygo.App.IsPackaged() && lidPackagedHelper() && (lid.SystemPower{}).HasInternalBattery()
	if !c.capable {
		a.lid = c
		return
	}
	c.supported = lidOSSupported() && lidBundlePath() == "/Applications/Cubbix.app"
	c.unavailable = "LidUnsupported"
	if c.supported {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := exec.CommandContext(ctx, "/usr/bin/codesign", lidBundleCheckArgs()...).Run()
		cancel()
		if err != nil {
			c.supported = false
			c.unavailable = "LidDevBuild"
		}
	}
	allowed := func() bool { return c.Available() }
	// Every manual action checks our Developer ID signature again; ad-hoc QA
	// bundles can render the UI but can never execute an authorization script.
	admin := func(program string) error {
		if !allowed() {
			return errors.New("LidUnsupported")
		}
		if program != lidInstallScriptApple && program != lidUninstallScriptApple && program != lidForceScriptApple {
			return errors.New("LidServiceBroken")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := exec.CommandContext(ctx, "/usr/bin/codesign", lidBundleCheckArgs()...).Run(); err != nil {
			return errors.New("LidDevBuild")
		}
		var command *exec.Cmd
		switch program {
		case lidInstallScriptApple:
			command = exec.Command("/usr/bin/osascript", "-e", lidInstallScriptApple)
		case lidUninstallScriptApple:
			command = exec.Command("/usr/bin/osascript", "-e", lidUninstallScriptApple)
		case lidForceScriptApple:
			command = exec.Command("/usr/bin/osascript", "-e", lidForceScriptApple)
		default:
			return errors.New("LidServiceBroken")
		}
		output, err := command.CombinedOutput()
		return lidAdminResult(output, err)
	}
	c.read = func() (int, error) {
		ctx, cancel := context.WithTimeout(context.Background(), lid.ReadTimeout)
		defer cancel()
		b, err := exec.CommandContext(ctx, "/usr/bin/pmset", "-g").Output()
		if err != nil {
			return -1, err
		}
		return lid.ParseSleep(b)
	}
	c.power = lid.SystemPower{}
	c.dial = func() (lidConnection, error) {
		return lid.Dial(lid.SocketPath, func(peer *net.UnixConn) error { return lid.VerifyPeer(peer, lid.HelperRequirement) })
	}
	c.service = &legacyLidService{admin: admin, allowed: allowed, restored: func() bool { v, err := c.read(); return err == nil && v == 0 && !c.markerExists() }}
	c.forcer = commandSleepForcer{admin: admin}
	a.lid = c
	a.lidAsync = true
}
