package ui

import (
	"fmt"
	"image/color"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/thereisnotime/agentpad/internal/scripts"
	"github.com/thereisnotime/agentpad/internal/settings"
)

const scriptsIntro = "Your own commands: run them from a pad control (action run:NAME), " +
	"when an agent event happens, or by hand. They run without a shell unless you pick one."

// scriptsTab is the Scripts screen: scripts, hooks and the last runs.
type scriptsTab struct {
	u       *appUI
	content fyne.CanvasObject

	list      *fyne.Container // script cards
	hooksBox  *fyne.Container // hook rows
	runsBox   *fyne.Container // run rows
	hookEvent *widget.Select
	hookAgent *widget.Select
	hookScrpt *widget.Select
	hookAdd   *widget.Button
	newBtn    *widget.Button
	open      map[scripts.RunID]bool // expanded run rows
}

func newScriptsTab(u *appUI) *scriptsTab {
	t := &scriptsTab{u: u, list: container.NewVBox(), hooksBox: container.NewVBox(), runsBox: container.NewVBox(),
		open: map[scripts.RunID]bool{}}

	t.newBtn = widget.NewButtonWithIcon("New script", theme.ContentAddIcon(), func() { t.edit("", scripts.Script{}) })
	t.newBtn.Importance = widget.HighImportance
	scriptsCard := surface(container.NewBorder(
		container.NewBorder(nil, nil, nil, container.NewVBox(t.newBtn), tight(boldLabel("Scripts"), secondary("Edit, run now, or put run:NAME on a pad control."))),
		nil, nil, nil, inset(8, 0, 0, 0, t.list)))

	labels := make([]string, 0, len(scripts.Events()))
	for _, ev := range scripts.Events() {
		labels = append(labels, eventLabel(ev))
	}
	t.hookEvent = widget.NewSelect(labels, nil)
	t.hookEvent.PlaceHolder = "When…"
	t.hookAgent = widget.NewSelect(append([]string{anyAgent}, scripts.Agents()...), nil)
	t.hookAgent.SetSelected(anyAgent)
	t.hookScrpt = widget.NewSelect(nil, nil)
	t.hookScrpt.PlaceHolder = "Run…"
	t.hookAdd = widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), t.addHook)
	addRow := container.New(&columnsLayout{cols: t.hookColumns, gap: 8}, t.hookEvent, t.hookAgent, t.hookScrpt, t.hookAdd)
	hooksCard := newCard("Hooks", "Run a script when something happens. Agent events can be limited to one agent.",
		container.NewVBox(t.hooksBox, widget.NewSeparator(), addRow))

	runsCard := newCard("Last runs", "The newest 50, also in scripts.jsonl next to the event log. Tap a run for its output.", t.runsBox)

	body := container.NewVBox(screenTitle("Scripts", scriptsIntro))
	if u.files == nil {
		warn := wrapLabel("Scripts live in settings.yaml, which could not be opened, so they are off for now.")
		warn.Importance = widget.WarningImportance
		body.Add(surface(warn))
		t.newBtn.Disable()
	}
	body.Add(scriptsCard)
	body.Add(hooksCard)
	body.Add(runsCard)
	t.content = screen(body)
	t.refresh()
	return t
}

// hookColumns lays the add-hook row out in 4 columns when wide, else 2.
func (t *scriptsTab) hookColumns() int {
	if t.u.win != nil && t.u.win.Canvas().Size().Width < 700 {
		return 2
	}
	return 4
}

// refresh rebuilds the script cards, hooks and runs. UI goroutine.
func (t *scriptsTab) refresh() {
	s := t.u.scriptSettings()
	names := s.ScriptNames()

	t.list.Objects = nil
	if len(names) == 0 {
		t.list.Add(secondary("No scripts yet. Add one with New script, or under scripts: in settings.yaml."))
	}
	for _, name := range names {
		t.list.Add(t.scriptCard(name, s.Scripts[name]))
	}
	t.list.Refresh()

	t.hooksBox.Objects = nil
	if len(s.Hooks) == 0 {
		t.hooksBox.Add(secondary("No hooks yet."))
	}
	for i, h := range s.Hooks {
		t.hooksBox.Add(t.hookRow(i, h))
	}
	t.hooksBox.Refresh()
	t.hookScrpt.Options = names
	if !slices.Contains(names, t.hookScrpt.Selected) {
		t.hookScrpt.ClearSelected()
	}
	t.hookScrpt.Refresh()
	if len(names) == 0 || t.u.files == nil {
		t.hookAdd.Disable()
	} else {
		t.hookAdd.Enable()
	}
	t.refreshRuns()
}

// dotColor maps a status dot kind to the palette.
func dotColor(k colorKind) color.NRGBA {
	switch k {
	case dotOK:
		return colorSuccess
	case dotFailed:
		return colorError
	case dotRunning:
		return colorBusy
	}
	return colorIdle
}

func statusDot(k colorKind) fyne.CanvasObject {
	c := canvas.NewCircle(dotColor(k))
	return vcenter(fixed(10, 10, c))
}

// scriptCard is one script: name, last status, command, Edit and Run now.
func (t *scriptsTab) scriptCard(name string, s scripts.Script) fyne.CanvasObject {
	last, ok := t.u.runner.Last(name)
	state := runColor(last, ok)
	if t.u.runner.Running(name) {
		state = dotRunning
	}
	status := "never run"
	if ok {
		status = runResult(last) + " · " + last.Start.Local().Format("15:04:05")
	}
	if state == dotRunning {
		status = "running"
	}
	title := container.NewHBox(statusDot(state), vcenter(text(name, 14, colText, true)), vcenter(text(status, 12, colText3, false)))
	cmd := monoText(s.Preview(), 13, colText2)
	var details []string
	if s.Shell != "" {
		details = append(details, "shell")
	}
	if s.Cwd != "" {
		details = append(details, "in "+s.Cwd)
	}
	if len(s.Env) > 0 {
		details = append(details, plural(len(s.Env), "env var"))
	}
	details = append(details, "timeout "+s.EffectiveTimeout().String())
	edit := widget.NewButtonWithIcon("Edit", theme.DocumentCreateIcon(), func() { t.edit(name, s) })
	edit.Importance = widget.LowImportance
	run := widget.NewButtonWithIcon("Run now", theme.MediaPlayIcon(), func() { t.u.runScriptNow(name) })
	if t.u.files == nil {
		edit.Disable()
	}
	info := container.NewVBox(title, inset(0, 0, 18, 0, cmd), inset(0, 0, 18, 0, text(strings.Join(details, " · "), 12, colText3, false)))
	bg := roundRect(colSurface2, 10)
	return container.NewStack(bg, inset(6, 6, 10, 10, container.NewBorder(nil, nil, nil, vcenter(container.NewHBox(edit, run)), info)))
}

// hookRow is one hook with a remove button.
func (t *scriptsTab) hookRow(i int, h scripts.Hook) fyne.CanvasObject {
	agent := "any agent"
	if h.Agent != "" {
		agent = agentShort(h.Agent) + " only"
	}
	if !isAgentEvent(h.On) {
		agent = ""
	}
	label := eventLabel(h.On)
	if agent != "" {
		label += " · " + agent
	}
	remove := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() { t.removeHook(i, h) })
	remove.Importance = widget.LowImportance
	if t.u.files == nil {
		remove.Disable()
	}
	row := container.NewHBox(vcenter(text(label, 13, colText, false)), vcenter(text("→", 13, colText3, false)),
		vcenter(monoText(h.Script, 13, colAccent)))
	return container.NewBorder(nil, nil, nil, remove, row)
}

// isAgentEvent reports whether ev comes from an agent session.
func isAgentEvent(ev string) bool {
	switch ev {
	case scripts.EventAttention, scripts.EventBusy, scripts.EventIdle,
		scripts.EventSessionStart, scripts.EventSessionEnd, scripts.EventJump:
		return true
	}
	return false
}

func (t *scriptsTab) addHook() {
	ev := eventFromLabel(t.hookEvent.Selected)
	name := t.hookScrpt.Selected
	if ev == "" || name == "" {
		t.u.toasts.show(toastError, "Pick an event and a script first.")
		return
	}
	h := scripts.Hook{On: ev, Script: name}
	if a := t.hookAgent.Selected; a != anyAgent && a != "" && isAgentEvent(ev) {
		h.Agent = a
	}
	err := t.u.updateScripts(func(s *settings.Settings) {
		if !slices.Contains(s.Hooks, h) {
			s.Hooks = append(s.Hooks, h)
		}
	})
	if err != nil {
		t.u.toasts.show(toastError, "Hook not added: "+errorText(err))
		return
	}
	t.u.toasts.show(toastSuccess, "Hook added: "+h.String()+".")
}

func (t *scriptsTab) removeHook(i int, h scripts.Hook) {
	err := t.u.updateScripts(func(s *settings.Settings) {
		if i < len(s.Hooks) && s.Hooks[i] == h {
			s.Hooks = slices.Delete(s.Hooks, i, i+1)
		}
		if len(s.Hooks) == 0 {
			s.Hooks = nil
		}
	})
	if err != nil {
		t.u.toasts.show(toastError, "Hook not removed: "+errorText(err))
	}
}

// edit opens the script editor; name "" adds a new script.
func (t *scriptsTab) edit(name string, s scripts.Script) {
	f := formFromScript(name, s)
	nameE := widget.NewEntry()
	nameE.SetText(f.Name)
	nameE.SetPlaceHolder("mic-off")
	cmdE := widget.NewEntry()
	cmdE.SetText(f.Command)
	cmdE.TextStyle.Monospace = true
	shellC := widget.NewCheck("Run through /bin/sh -c (pipes, $VARS, ~ everywhere)", nil)
	shellC.SetChecked(f.Shell)
	cmdHint := secondary("")
	setHint := func(shell bool) {
		if shell {
			cmdE.SetPlaceHolder("pactl set-source-mute @DEFAULT_SOURCE@ toggle")
			cmdHint.SetText("A shell command line. Use it only when you need shell features.")
		} else {
			cmdE.SetPlaceHolder("notify-send 'agentpad' 'hello'")
			cmdE.Refresh()
			cmdHint.SetText("Command and arguments, quoted like in a shell but run directly. ~ works in the command itself.")
		}
	}
	setHint(f.Shell)
	shellC.OnChanged = setHint
	cwdE := widget.NewEntry()
	cwdE.SetText(f.Cwd)
	cwdE.SetPlaceHolder("~/projects/api (optional)")
	envE := widget.NewMultiLineEntry()
	envE.SetText(f.Env)
	envE.SetPlaceHolder("STAGE=prod\nTOPIC=my-agentpad")
	envE.SetMinRowsVisible(3)
	envE.TextStyle.Monospace = true
	toE := widget.NewEntry()
	toE.SetText(f.Timeout)
	toE.SetPlaceHolder(scripts.DefaultTimeout.String() + " (max " + scripts.MaxTimeout.String() + ")")
	errL := wrapLabel("")
	errL.Importance = widget.DangerImportance
	errL.Hide()

	field := func(label string, o fyne.CanvasObject) fyne.CanvasObject {
		return container.NewVBox(text(label, 12, colText2, true), o)
	}
	body := container.NewVBox(
		field("Name", nameE),
		field("Command", container.NewVBox(cmdE, shellC, cmdHint)),
		field("Working directory", cwdE),
		field("Environment (KEY=value per line)", envE),
		field("Timeout", toE),
		secondary("Scripts also get the event as JSON on stdin and AGENTPAD_* variables, see docs/scripts.md."),
		errL,
	)
	title := "New script"
	if name != "" {
		title = "Edit " + name
	}
	d := dialog.NewCustomWithoutButtons(title, fixedWidth(520, body), t.u.win)
	save := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
		nf := scriptForm{Name: nameE.Text, Command: cmdE.Text, Shell: shellC.Checked, Cwd: cwdE.Text, Env: envE.Text, Timeout: toE.Text}
		if err := t.save(name, nf); err != nil {
			errL.SetText(errorText(err))
			errL.Show()
			return
		}
		d.Hide()
	})
	save.Importance = widget.HighImportance
	buttons := []fyne.CanvasObject{widget.NewButton("Cancel", d.Hide)}
	if name != "" {
		del := widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), func() {
			d.Hide()
			t.confirmDelete(name)
		})
		del.Importance = widget.DangerImportance
		buttons = append(buttons, del)
	}
	d.SetButtons(append(buttons, save))
	d.Show()
}

// save stores the edited script; renaming moves its hooks along.
func (t *scriptsTab) save(oldName string, f scriptForm) error {
	name, s, err := f.script()
	if err != nil {
		return err
	}
	cur := t.u.scriptSettings()
	if name != oldName && cur.HasScript(name) {
		return fmt.Errorf("a script named %s already exists", name)
	}
	err = t.u.updateScripts(func(x *settings.Settings) {
		if x.Scripts == nil {
			x.Scripts = map[string]scripts.Script{}
		}
		if oldName != "" && oldName != name {
			delete(x.Scripts, oldName)
			for i := range x.Hooks {
				if x.Hooks[i].Script == oldName {
					x.Hooks[i].Script = name
				}
			}
		}
		x.Scripts[name] = s
	})
	if err != nil {
		return err
	}
	if oldName != "" && oldName != name {
		t.u.toasts.show(toastSuccess, "Renamed "+oldName+" to "+name+". Pad controls using run:"+oldName+" need updating.")
	} else {
		t.u.toasts.show(toastSuccess, "Saved "+name+".")
	}
	return nil
}

func (t *scriptsTab) confirmDelete(name string) {
	cur := t.u.scriptSettings()
	n := 0
	for _, h := range cur.Hooks {
		if h.Script == name {
			n++
		}
	}
	msg := fmt.Sprintf("Delete the script %s?", name)
	if n > 0 {
		msg += fmt.Sprintf(" Its %s go too.", plural(n, "hook"))
	}
	dialog.ShowConfirm("Delete script", msg, func(ok bool) {
		if !ok {
			return
		}
		err := t.u.updateScripts(func(x *settings.Settings) {
			delete(x.Scripts, name)
			x.Hooks = slices.DeleteFunc(x.Hooks, func(h scripts.Hook) bool { return h.Script == name })
			if len(x.Scripts) == 0 {
				x.Scripts = nil
			}
			if len(x.Hooks) == 0 {
				x.Hooks = nil
			}
		})
		if err != nil {
			t.u.toasts.show(toastError, "Not deleted: "+errorText(err))
			return
		}
		t.u.toasts.show(toastInfo, "Deleted "+name+".")
	}, t.u.win)
}

// runUpdated reflects a run change. UI goroutine.
func (t *scriptsTab) runUpdated(r scripts.Run) {
	if r.Status == scripts.StatusQueued {
		return // shows up as running a moment later
	}
	t.refresh()
}

// runColumns: time, script, trigger, result, duration.
var runColumns = []float32{80, 130, 170, 90, 80}

// refreshRuns rebuilds the Last runs rows. UI goroutine.
func (t *scriptsTab) refreshRuns() {
	runs := t.u.runner.Runs()
	t.runsBox.Objects = nil
	if len(runs) == 0 {
		t.runsBox.Add(secondary("Nothing ran yet."))
		t.runsBox.Refresh()
		return
	}
	head := container.New(&runRowLayout{}, runHeader("Time"), runHeader("Script"), runHeader("Trigger"), runHeader("Result"), runHeader("Took"))
	t.runsBox.Add(head)
	for _, r := range runs {
		t.runsBox.Add(t.runRow(r))
	}
	t.runsBox.Refresh()
}

func runHeader(s string) fyne.CanvasObject { return text(s, 12, colText3, true) }

// runRow is one run; tapping it shows the captured output.
func (t *scriptsTab) runRow(r scripts.Run) fyne.CanvasObject {
	k := runColor(r, true)
	result := container.NewHBox(statusDot(k), monoText(runResult(r), 12, dotColor(k)))
	trigger := r.Trigger
	if trigger == "" {
		trigger = r.Event
	}
	row := container.New(&runRowLayout{},
		monoText(r.Start.Local().Format("15:04:05"), 12, colText2),
		ellipsized(r.Script, 12, colText, true),
		ellipsized(trigger, 12, colText2, false),
		result,
		monoText(durationText(r.Duration), 12, colText2),
	)
	out := runOutput(r)
	out.Hidden = !t.open[r.ID]
	id := r.ID
	box := container.NewVBox()
	tap := newTapBox(inset(3, 3, 0, 0, row), func() {
		t.open[id] = !t.open[id]
		out.Hidden = !t.open[id]
		box.Refresh()
	}, nil)
	box.Objects = []fyne.CanvasObject{tap, out}
	return box
}

// runOutput is the expanded output of a run.
func runOutput(r scripts.Run) *fyne.Container {
	var parts []string
	if r.Err != "" {
		parts = append(parts, r.Err)
	}
	if r.Stdout != "" {
		parts = append(parts, "stdout:\n"+strings.TrimRight(r.Stdout, "\n"))
	}
	if r.Stderr != "" {
		parts = append(parts, "stderr:\n"+strings.TrimRight(r.Stderr, "\n"))
	}
	if r.Truncated {
		parts = append(parts, "(output cut at 64 KiB)")
	}
	if len(parts) == 0 {
		parts = append(parts, "No output.")
	}
	l := widget.NewLabelWithStyle(capRunes(strings.Join(parts, "\n\n"), 4000), fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	l.Wrapping = fyne.TextWrapBreak
	bg := roundRect(colBackground, 8)
	return container.NewStack(bg, inset(2, 2, 6, 6, l))
}

// capRunes cuts s to n runes for display.
func capRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "\n…"
}

// ellipsized is a single-line label that never grows its column.
func ellipsized(s string, size float32, c color.Color, bold bool) fyne.CanvasObject {
	t := text(s, size, c, bold)
	return container.New(clipLayout{}, t)
}

// clipLayout reports no min width so long text cannot widen a column.
type clipLayout struct{}

func (clipLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, objs[0].MinSize().Height)
}

func (clipLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(size.Width, objs[0].MinSize().Height))
}

// runRowLayout places the five run columns; the trigger column takes the
// slack, and on narrow widths trigger and duration are dropped.
type runRowLayout struct{}

func (runRowLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var h float32
	for _, o := range objs {
		h = max(h, o.MinSize().Height)
	}
	return fyne.NewSize(runColumns[0]+runColumns[1]+runColumns[3], h)
}

func (runRowLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	w := slices.Clone(runColumns)
	total := w[0] + w[1] + w[2] + w[3] + w[4]
	if size.Width < total {
		w[2], w[4] = 0, 0 // narrow: time, script, result
		w[1] = max(size.Width-w[0]-w[3], 60)
	} else {
		w[2] += size.Width - total
	}
	x := float32(0)
	for i, o := range objs {
		if w[i] == 0 {
			o.Hide()
			continue
		}
		o.Show()
		m := o.MinSize()
		o.Move(fyne.NewPos(x, (size.Height-m.Height)/2))
		o.Resize(fyne.NewSize(w[i]-8, m.Height))
		x += w[i]
	}
}

var _ fyne.Layout = runRowLayout{}
