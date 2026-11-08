//go:build !unix

package scripts

import "os/exec"

func setProcessGroup(*exec.Cmd) {}

func killGroup(cmd *exec.Cmd, _ bool) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
