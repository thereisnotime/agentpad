package ui

import (
	"fmt"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/thereisnotime/agentpad/internal/events"
)

type sessionsTab struct {
	u       *appUI
	content fyne.CanvasObject

	home     string
	sessions []events.Session

	bannerBG *canvas.Rectangle
	dot      *canvas.Circle
	title    *canvas.Text
	count    *widget.Label
	list     *fyne.Container
	cards    []*sessionCard
	empty    fyne.CanvasObject
}

func newSessionsTab(u *appUI) *sessionsTab {
	t := &sessionsTab{u: u}
	t.home, _ = os.UserHomeDir()

	t.bannerBG = roundRect(colSurface, 16)
	t.bannerBG.StrokeWidth = 1
	t.dot = canvas.NewCircle(colorIdle)
	t.title = text(stateTitle(events.StateIdle), 22, colText, true)
	t.count = secondary("")
	banner := container.NewStack(t.bannerBG, inset(16, 16, 18, 18,
		container.NewBorder(nil, nil, vcenter(fixed(18, 18, t.dot)), nil, tight(inset(0, 0, 10, 0, t.title), t.count))))

	t.list = newGrid(340)
	icon := widget.NewIcon(theme.ListIcon())
	setupBtn := widget.NewButtonWithIcon("Open Setup", theme.SettingsIcon(), func() { u.navigate(screenSetup) })
	setupBtn.Importance = widget.HighImportance
	emptyBody := container.NewVBox(
		container.NewCenter(fixed(40, 40, icon)),
		container.NewCenter(text("No agent sessions yet", 16, colText, true)),
		widget.NewLabelWithStyle("Add the hooks from Setup, then start Claude Code, Codex or OpenCode.", fyne.TextAlignCenter, fyne.TextStyle{}),
		container.NewCenter(setupBtn),
	)
	emptyBody.Objects[2].(*widget.Label).Wrapping = fyne.TextWrapWord
	emptyBody.Objects[2].(*widget.Label).Importance = widget.LowImportance
	t.empty = surface(inset(24, 24, 8, 8, emptyBody))

	t.content = screen(container.NewVBox(screenTitle("Sessions", ""), banner, t.empty, t.list))
	t.refresh()
	return t
}

// refresh reloads sessions from the tracker. UI goroutine.
func (t *sessionsTab) refresh() {
	t.sessions = t.u.tracker.Sessions()
	for len(t.cards) < len(t.sessions) {
		c := newSessionCard()
		t.cards = append(t.cards, c)
		t.list.Add(c.content)
	}
	now := time.Now()
	for i, c := range t.cards {
		if i < len(t.sessions) {
			c.show(t.sessions[i], t.home, now)
			c.content.Show()
		} else {
			c.content.Hide()
		}
	}
	if len(t.sessions) == 0 {
		t.empty.Show()
	} else {
		t.empty.Hide()
	}
	t.count.SetText(sessionSummary(t.sessions))
	t.list.Refresh()
}

// sessionSummary is the banner's second line.
func sessionSummary(ss []events.Session) string {
	if len(ss) == 0 {
		return "No sessions are reporting to agentpad."
	}
	var busy, att int
	for _, s := range ss {
		switch s.State {
		case events.StateBusy:
			busy++
		case events.StateAttention:
			att++
		}
	}
	return fmt.Sprintf("%s · %d waiting for you · %d busy · %d idle", plural(len(ss), "session"), att, busy, len(ss)-att-busy)
}

func (t *sessionsTab) showAggregate(s events.State) {
	c := stateColor(s)
	t.dot.FillColor = c
	t.dot.Refresh()
	t.bannerBG.FillColor = withAlpha(c, 0x1c)
	t.bannerBG.StrokeColor = withAlpha(c, 0x50)
	t.bannerBG.Refresh()
	t.title.Text = stateTitle(s)
	t.title.Refresh()
}

// sessionCard shows one agent session.
type sessionCard struct {
	content  fyne.CanvasObject
	badgeBG  *canvas.Circle
	letter   *canvas.Text
	agent    *canvas.Text
	stateDot *canvas.Circle
	state    *canvas.Text
	cwd      *widget.Label
	detail   *widget.Label
}

func newSessionCard() *sessionCard {
	c := &sessionCard{
		badgeBG: canvas.NewCircle(colorIdle), letter: text("", 16, colText, true),
		agent: text("", 15, colText, true), stateDot: canvas.NewCircle(colorIdle), state: text("", 12, colText2, false),
	}
	c.letter.Alignment = fyne.TextAlignCenter
	c.cwd = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	c.cwd.Truncation = fyne.TextTruncateEllipsis
	c.detail = widget.NewLabel("")
	c.detail.Importance = widget.LowImportance
	c.detail.Truncation = fyne.TextTruncateEllipsis
	badge := fixed(40, 40, c.badgeBG, container.NewCenter(c.letter))
	stateRow := container.NewHBox(vcenter(fixed(9, 9, c.stateDot)), vcenter(c.state))
	head := container.NewBorder(nil, nil, nil, stateRow, inset(0, 0, 10, 0, c.agent))
	c.content = surface(container.NewBorder(nil, nil, vcenter(badge), nil,
		inset(0, 0, 6, 0, tight(head, c.cwd, c.detail))))
	return c
}

func (c *sessionCard) show(s events.Session, home string, now time.Time) {
	letter, col := agentBadge(s.Agent)
	c.badgeBG.FillColor = col
	c.letter.Text = letter
	c.agent.Text = s.Agent
	c.stateDot.FillColor = stateColor(s.State)
	c.state.Text = string(s.State)
	c.state.Color = stateColor(s.State)
	cwd := shortenPath(s.Cwd, home)
	if cwd == "" {
		cwd = "unknown directory"
	}
	c.cwd.SetText(cwd)
	tool := s.LastTool
	if tool == "" {
		tool = "no tool yet"
	}
	c.detail.SetText(tool + " · " + relTime(s.LastEvent, now) + " · " + shortID(s.SessionID))
	for _, o := range []fyne.CanvasObject{c.badgeBG, c.letter, c.agent, c.stateDot, c.state} {
		o.Refresh()
	}
}
