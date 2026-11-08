package jump

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/thereisnotime/agentpad/internal/events"
)

// execRunner runs name with args and returns stdout; on failure the error
// carries the trimmed stderr.
func execRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return out, fmt.Errorf("%w: %s", err, firstLine(msg))
		}
		return out, err
	}
	return out, nil
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

func (j *Jumper) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if j.Run == nil {
		return nil, errors.New("no command runner")
	}
	ctx, cancel := context.WithTimeout(ctx, j.execTimeout())
	defer cancel()
	return j.Run(ctx, name, args...)
}

func (j *Jumper) multiplexer(ctx context.Context, o events.Origin) []Step {
	var steps []Step
	if o.ZellijSession != "" || o.ZellijPane != "" {
		steps = append(steps, j.zellij(ctx, o))
	}
	if o.TmuxPane != "" || o.Tmux != "" {
		steps = append(steps, j.tmux(ctx, o))
	}
	if len(steps) == 0 {
		steps = append(steps, skipped(StepMultiplexer, "", "no zellij or tmux info in origin"))
	}
	return steps
}

type zellijPane struct {
	ID       int  `json:"id"`
	IsPlugin bool `json:"is_plugin"`
	TabID    int  `json:"tab_id"`
}

// zellij focuses terminal_<pane>. From outside the session (agentpad is not
// a zellij client) `focus-pane-id` does not move an attached client's focus
// in 0.44.3, so the step goes to the pane's tab (`go-to-tab-by-id`) and then
// walks `focus-next-pane` until `list-clients` reports the pane, bounded by the
// number of panes in that tab.
func (j *Jumper) zellij(ctx context.Context, o events.Origin) Step {
	const be = "zellij"
	if o.ZellijSession == "" {
		return skipped(StepMultiplexer, be, "no zellij session name in origin")
	}
	id, err := strconv.Atoi(strings.TrimPrefix(o.ZellijPane, "terminal_"))
	if err != nil || id < 0 {
		return skipped(StepMultiplexer, be, "no usable zellij pane id in origin (%q)", o.ZellijPane)
	}
	target := "terminal_" + strconv.Itoa(id)
	z := func(args ...string) ([]byte, error) {
		return j.run(ctx, "zellij", append([]string{"--session", o.ZellijSession, "action"}, args...)...)
	}

	out, err := z("list-panes", "--json", "--tab")
	if err != nil {
		return failed(StepMultiplexer, be, "list-panes in session %q: %v", o.ZellijSession, err)
	}
	var panes []zellijPane
	if err := json.Unmarshal(out, &panes); err != nil {
		return failed(StepMultiplexer, be, "parse list-panes: %v", err)
	}
	tab, inTab := -1, 0
	for _, p := range panes {
		if !p.IsPlugin && p.ID == id {
			tab = p.TabID
		}
	}
	if tab < 0 {
		return failed(StepMultiplexer, be, "pane %s not found in session %q", target, o.ZellijSession)
	}
	for _, p := range panes {
		if !p.IsPlugin && p.TabID == tab {
			inTab++
		}
	}
	if _, err := z("go-to-tab-by-id", strconv.Itoa(tab)); err != nil {
		return failed(StepMultiplexer, be, "go-to-tab-by-id %d: %v", tab, err)
	}
	// Cheap first try; works when zellij applies it to the attached client.
	_, _ = z("focus-pane-id", target)

	clients, err := z("list-clients")
	if err != nil {
		return failed(StepMultiplexer, be, "list-clients: %v", err)
	}
	if len(zellijClientPanes(clients)) == 0 {
		return done(StepMultiplexer, be, "switched to tab %d of session %q; no client attached, pane focus not verifiable", tab, o.ZellijSession)
	}
	for i := 0; i <= inTab; i++ {
		if zellijClientOn(clients, target) {
			return done(StepMultiplexer, be, "focused %s in tab %d of session %q", target, tab, o.ZellijSession)
		}
		if i == inTab {
			break
		}
		if _, err := z("focus-next-pane"); err != nil {
			return failed(StepMultiplexer, be, "focus-next-pane: %v", err)
		}
		if clients, err = z("list-clients"); err != nil {
			return failed(StepMultiplexer, be, "list-clients: %v", err)
		}
	}
	return failed(StepMultiplexer, be, "switched to tab %d but could not focus %s", tab, target)
}

// zellijClientPanes parses `zellij action list-clients`:
//
//	CLIENT_ID ZELLIJ_PANE_ID RUNNING_COMMAND
//	1         terminal_2     N/A
func zellijClientPanes(out []byte) []string {
	var panes []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 2 || f[0] == "CLIENT_ID" {
			continue
		}
		panes = append(panes, f[1])
	}
	return panes
}

func zellijClientOn(out []byte, target string) bool {
	for _, p := range zellijClientPanes(out) {
		if p == target {
			return true
		}
	}
	return false
}

// tmux selects the pane's window and the pane, on the server named by the
// socket path in $TMUX ("socket,pid,session").
func (j *Jumper) tmux(ctx context.Context, o events.Origin) Step {
	const be = "tmux"
	if o.TmuxPane == "" {
		return skipped(StepMultiplexer, be, "no tmux pane id in origin")
	}
	var base []string
	if sock, _, _ := strings.Cut(o.Tmux, ","); sock != "" {
		base = []string{"-S", sock}
	}
	for _, cmd := range []string{"select-window", "select-pane"} {
		if _, err := j.run(ctx, "tmux", append(append([]string{}, base...), cmd, "-t", o.TmuxPane)...); err != nil {
			return failed(StepMultiplexer, be, "%s -t %s: %v", cmd, o.TmuxPane, err)
		}
	}
	return done(StepMultiplexer, be, "selected window and pane %s", o.TmuxPane)
}
