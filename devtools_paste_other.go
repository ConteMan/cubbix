//go:build !darwin

package main

import "errors"

// devtoolsPostAccess answers false where synthesized key events do not exist:
// the window's ⌘回车 then copies and closes, without claiming a paste.
func devtoolsPostAccess() bool { return false }

func devtoolsFrontmostPID() int32 { return 0 }

func postDevtoolsCommandV(int32) error { return errors.New("devtools: key events unavailable") }
