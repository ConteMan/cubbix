package main

import (
	"os"
	"regexp"
	"testing"
)

// The install tests swap every command for a fixture, so they cannot notice a
// command that does not exist on macOS (chown lives in /usr/sbin, not /bin).
// Check the real paths the authorization scripts call.
func TestLidScriptCommandsExist(t *testing.T) {
	command := regexp.MustCompile(`(?m)(?:^|[\s;|&(!])(/(?:usr/)?s?bin/[A-Za-z0-9_.-]+)`)
	for name, script := range map[string]string{
		"install":   lidInstallScript,
		"uninstall": lidUninstallScript,
		"force":     lidForceScript,
	} {
		paths := command.FindAllStringSubmatch(script, -1)
		if len(paths) == 0 {
			t.Fatalf("%s: no commands found", name)
		}
		for _, m := range paths {
			info, err := os.Stat(m[1])
			if err != nil || info.IsDir() || info.Mode().Perm()&0111 == 0 {
				t.Errorf("%s calls %s, which is not an executable on this system", name, m[1])
			}
		}
	}
}
