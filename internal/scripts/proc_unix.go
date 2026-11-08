//go:build unix

package scripts

import (
	"os/exec"
	"syscall"
)

// setProcessGroup starts cmd in its own process group so a timeout can
// kill everything it spawned.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killGroup sends SIGTERM (or SIGKILL when hard) to cmd's process group.
func killGroup(cmd *exec.Cmd, hard bool) {
	if cmd.Process == nil {
		return
	}
	sig := syscall.SIGTERM
	if hard {
		sig = syscall.SIGKILL
	}
	_ = syscall.Kill(-cmd.Process.Pid, sig)
}
