package main

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/ConteMan/cubbix/internal/lid"
)

func TestLidBundleCheckArgsUseRequirementText(t *testing.T) {
	args := lidBundleCheckArgs()
	if len(args) != 4 || args[0] != "-v" || args[1] != "-R" || args[3] != "/Applications/Cubbix.app" {
		t.Fatalf("args = %q", args)
	}
	if args[2] != "="+lid.ClientRequirement {
		t.Fatalf("requirement must be passed as text, got %q", args[2])
	}
}

// codesign treats a bare -R value as a requirement file. Check the parsing
// against the system tool with a bundle that cannot satisfy the requirement:
// the text form must fail on the signature, not on reading a file.
func TestLidBundleCheckArgsParseAsRequirement(t *testing.T) {
	args := lidBundleCheckArgs()
	args[3] = "/usr/bin/true"
	out, err := exec.Command("/usr/bin/codesign", args...).CombinedOutput()
	if err == nil {
		t.Fatal("system binary unexpectedly satisfied the Cubbix requirement")
	}
	if strings.Contains(string(out), "No such file") || strings.Contains(string(out), "invalid requirement") {
		t.Fatalf("requirement was not parsed as text: %s", out)
	}
}
