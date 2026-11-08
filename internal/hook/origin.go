package hook

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/thereisnotime/agentpad/internal/events"
)

const (
	// maxAncestors bounds the /proc walk.
	maxAncestors = 32
	// maxCmdline bounds how much of /proc/<pid>/cmdline is read.
	maxCmdline = 4096
)

// ProcInfo is what the origin capture needs from /proc/<pid>/stat and cmdline.
type ProcInfo struct {
	PID   int
	PPID  int
	Comm  string
	TTYNr int
	Argv  []string
}

// ReadProc reads /proc/<pid>/stat and /proc/<pid>/cmdline under procRoot
// (normally "/proc"). Two small file reads, no exec.
func ReadProc(procRoot string, pid int) (ProcInfo, bool) {
	dir := filepath.Join(procRoot, strconv.Itoa(pid))
	stat, err := os.ReadFile(filepath.Join(dir, "stat"))
	if err != nil {
		return ProcInfo{}, false
	}
	p, ok := parseStat(stat)
	if !ok {
		return ProcInfo{}, false
	}
	p.PID = pid
	if f, err := os.Open(filepath.Join(dir, "cmdline")); err == nil {
		buf := make([]byte, maxCmdline)
		n, _ := f.Read(buf)
		_ = f.Close()
		p.Argv = splitCmdline(buf[:n])
	}
	return p, true
}

// parseStat parses "pid (comm) state ppid pgrp session tty_nr ...". comm may
// contain spaces and parentheses, so split at the last ')'.
func parseStat(b []byte) (ProcInfo, bool) {
	l, r := bytes.IndexByte(b, '('), bytes.LastIndexByte(b, ')')
	if l < 0 || r < l {
		return ProcInfo{}, false
	}
	f := strings.Fields(string(b[r+1:]))
	if len(f) < 5 {
		return ProcInfo{}, false
	}
	ppid, err1 := strconv.Atoi(f[1])
	tty, err2 := strconv.Atoi(f[4])
	if err1 != nil || err2 != nil {
		return ProcInfo{}, false
	}
	return ProcInfo{Comm: string(b[l+1 : r]), PPID: ppid, TTYNr: tty}, true
}

func splitCmdline(b []byte) []string {
	b = bytes.TrimRight(b, "\x00")
	if len(b) == 0 {
		return nil
	}
	parts := bytes.Split(b, []byte{0})
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = string(p)
	}
	return out
}

// Ancestors returns pid's ancestors starting with its parent, nearest first,
// stopping at pid 1 or after maxAncestors steps.
func Ancestors(procRoot string, pid int) []ProcInfo {
	var out []ProcInfo
	seen := map[int]bool{pid: true}
	cur, ok := ReadProc(procRoot, pid)
	for i := 0; ok && i < maxAncestors && cur.PPID > 1 && !seen[cur.PPID]; i++ {
		seen[cur.PPID] = true
		cur, ok = ReadProc(procRoot, cur.PPID)
		if ok {
			out = append(out, cur)
		}
	}
	return out
}

// MatchesAgent reports whether p looks like the given agent's process: comm or
// argv[0] basename equals the agent name, or a node/bun/deno runtime whose
// arguments mention it (npm-installed CLIs).
func MatchesAgent(p ProcInfo, agent string) bool {
	names := []string{p.Comm}
	if len(p.Argv) > 0 {
		names = append(names, filepath.Base(p.Argv[0]))
	}
	runtime := false
	for _, n := range names {
		n = strings.TrimPrefix(n, ".")
		if n == agent {
			return true
		}
		switch n {
		case "node", "bun", "deno", "nodejs":
			runtime = true
		}
	}
	if !runtime {
		return false
	}
	for _, a := range p.Argv[min(1, len(p.Argv)):] {
		if strings.Contains(a, agent) {
			return true
		}
	}
	return false
}

// ttyName maps a /proc stat tty_nr to a device path for the common cases
// (pseudo-terminals and virtual consoles); "" when unknown.
func ttyName(nr int) string {
	if nr <= 0 {
		return ""
	}
	major := (nr >> 8) & 0xfff
	minor := (nr & 0xff) | ((nr >> 12) & 0xfff00)
	switch {
	case major >= 136 && major <= 143:
		return "/dev/pts/" + strconv.Itoa((major-136)*256+minor)
	case major == 4 && minor < 64:
		return "/dev/tty" + strconv.Itoa(minor)
	}
	return ""
}

// CaptureOrigin builds the origin for an agent hook running as pid with the
// given environment. It only reads environ and a bounded number of /proc
// files; it never execs. The agent PID is the first ancestor matching the
// agent (see MatchesAgent), else the parent.
func CaptureOrigin(agent string, environ []string, procRoot string, pid int) events.Origin {
	env := make(map[string]string, 16)
	for _, kv := range environ {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		switch k {
		case "KONSOLE_DBUS_SERVICE", "KONSOLE_DBUS_WINDOW", "KONSOLE_DBUS_SESSION",
			"WINDOWID", "ZELLIJ_SESSION_NAME", "ZELLIJ_PANE_ID", "TMUX", "TMUX_PANE", "TERM_PROGRAM":
			env[k] = v
		}
	}
	o := events.Origin{
		TermProgram:    env["TERM_PROGRAM"],
		WindowID:       env["WINDOWID"],
		KonsoleService: env["KONSOLE_DBUS_SERVICE"],
		KonsoleWindow:  env["KONSOLE_DBUS_WINDOW"],
		KonsoleSession: env["KONSOLE_DBUS_SESSION"],
		ZellijSession:  env["ZELLIJ_SESSION_NAME"],
		ZellijPane:     env["ZELLIJ_PANE_ID"],
		Tmux:           env["TMUX"],
		TmuxPane:       env["TMUX_PANE"],
	}
	anc := Ancestors(procRoot, pid)
	if len(anc) == 0 {
		return o
	}
	agentProc := anc[0]
	for _, p := range anc {
		if MatchesAgent(p, agent) {
			agentProc = p
			break
		}
	}
	o.AgentPID = agentProc.PID
	o.TTY = ttyName(agentProc.TTYNr)
	return o
}
