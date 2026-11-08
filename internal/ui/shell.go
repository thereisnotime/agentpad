package ui

import (
	"image/color"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// shellLayout: header on top; sidebar + content on wide windows, compact tab
// bar + content on narrow ones. Objects: header, sidebar, tabbar, content.
type shellLayout struct{}

func (shellLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	h, tabs, content := objs[0].MinSize(), objs[2].MinSize(), objs[3].MinSize()
	return fyne.NewSize(max(h.Width, tabs.Width, content.Width), h.Height+tabs.Height+content.Height)
}

func (shellLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	header, side, tabs, content := objs[0], objs[1], objs[2], objs[3]
	hh := header.MinSize().Height
	header.Move(fyne.NewPos(0, 0))
	header.Resize(fyne.NewSize(size.Width, hh))
	if isWide(size.Width) {
		tabs.Hide()
		side.Show()
		side.Move(fyne.NewPos(0, hh))
		side.Resize(fyne.NewSize(sidebarWidth, size.Height-hh))
		content.Move(fyne.NewPos(sidebarWidth, hh))
		content.Resize(fyne.NewSize(size.Width-sidebarWidth, size.Height-hh))
		return
	}
	side.Hide()
	tabs.Show()
	th := tabs.MinSize().Height
	tabs.Move(fyne.NewPos(0, hh))
	tabs.Resize(fyne.NewSize(size.Width, th))
	content.Move(fyne.NewPos(0, hh+th))
	content.Resize(fyne.NewSize(size.Width, size.Height-hh-th))
}

// headerLayout puts [brand, pills] on one row when they fit, else two rows.
type headerLayout struct{ lastW float32 }

const headerGap = 16

func pillsRowWidth(c *fyne.Container) float32 {
	var w float32
	n := 0
	for _, o := range c.Objects {
		if o.Visible() {
			w += o.MinSize().Width
			n++
		}
	}
	return w + float32(max(n-1, 0))*6
}

func (l *headerLayout) oneRow(objs []fyne.CanvasObject, w float32) bool {
	return objs[0].MinSize().Width+headerGap+pillsRowWidth(objs[1].(*fyne.Container)) <= w
}

func (l *headerLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	b, p := objs[0].MinSize(), objs[1].MinSize()
	if l.lastW > 0 && l.oneRow(objs, l.lastW) {
		return fyne.NewSize(max(b.Width, p.Width), max(b.Height, p.Height))
	}
	return fyne.NewSize(max(b.Width, p.Width), b.Height+8+p.Height)
}

func (l *headerLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	l.lastW = size.Width
	brand, pills := objs[0], objs[1]
	b := brand.MinSize()
	if l.oneRow(objs, size.Width) {
		pw := pillsRowWidth(pills.(*fyne.Container))
		ph := pills.MinSize().Height
		brand.Move(fyne.NewPos(0, (size.Height-b.Height)/2))
		brand.Resize(b)
		pills.Move(fyne.NewPos(size.Width-pw, (size.Height-ph)/2))
		pills.Resize(fyne.NewSize(pw, ph))
		return
	}
	brand.Move(fyne.NewPos(0, 0))
	brand.Resize(b)
	pills.Move(fyne.NewPos(0, b.Height+8))
	pills.Resize(fyne.NewSize(size.Width, pills.MinSize().Height))
}

// navItem is one navigation entry, drawn either as a sidebar row or a compact tab.
type navItem struct {
	*tapBox
	bg       *canvas.Rectangle
	bar      *canvas.Rectangle
	icon     *widget.Icon
	label    *canvas.Text
	res      fyne.Resource
	selected bool
	hovered  bool
}

func newNavItem(title string, res fyne.Resource, compact bool, onTap func()) *navItem {
	n := &navItem{bg: roundRect(color.Transparent, 10), bar: roundRect(color.Transparent, 2), icon: widget.NewIcon(res), res: res}
	var content fyne.CanvasObject
	if compact {
		n.label = text(title, 11, colText2, false)
		n.label.Alignment = fyne.TextAlignCenter
		n.bg.CornerRadius = 8
		col := container.NewVBox(container.NewCenter(fixed(20, 20, n.icon)), container.NewCenter(n.label))
		content = container.NewStack(n.bg, inset(6, 6, 2, 2, col),
			container.NewBorder(nil, fixed(0, 3, n.bar), nil, nil))
	} else {
		n.label = text(title, 14, colText2, false)
		row := container.NewHBox(vcenter(fixed(20, 20, n.icon)), vcenter(n.label))
		content = container.NewStack(n.bg, container.NewBorder(nil, nil, inset(8, 8, 0, 0, fixed(3, 0, n.bar)), nil, inset(9, 9, 10, 12, row)))
	}
	n.tapBox = newTapBox(content, onTap, func(h bool) { n.hovered = h; n.restyle() })
	n.restyle()
	return n
}

func (n *navItem) setSelected(on bool) {
	n.selected = on
	n.restyle()
}

func (n *navItem) restyle() {
	switch {
	case n.selected:
		n.bg.FillColor = colAccentSoft
		n.bar.FillColor = colAccent
		n.label.Color = colText
	case n.hovered:
		n.bg.FillColor = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x0d}
		n.bar.FillColor = color.Transparent
		n.label.Color = colText
	default:
		n.bg.FillColor = color.Transparent
		n.bar.FillColor = color.Transparent
		n.label.Color = colText2
	}
	n.label.TextStyle.Bold = n.selected
	n.icon.SetResource(themedIcon(n.res, n.selected))
	n.bg.Refresh()
	n.bar.Refresh()
	n.label.Refresh()
}

// toastLayout puts its single visible child at the bottom centre.
type toastLayout struct{ slide float32 } // extra downward offset while sliding in

func (*toastLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }
func (l *toastLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		m := o.MinSize()
		w := min(max(m.Width, 260), size.Width-32)
		o.Resize(fyne.NewSize(w, m.Height))
		o.Move(fyne.NewPos((size.Width-w)/2, size.Height-m.Height-20+l.slide))
	}
}

type toastKind int

const (
	toastInfo toastKind = iota
	toastSuccess
	toastError
)

// toaster shows short, non-modal status messages.
type toaster struct {
	box   *fyne.Container
	bar   *canvas.Rectangle
	label *widget.Label
	gen   int
	layer *fyne.Container
	lay   *toastLayout
	anim  *fyne.Animation
	// closed stops pending hide timers once the UI shut down (tests run
	// fyne.Do inline on the timer goroutine).
	closed atomic.Bool
}

func newToaster() *toaster {
	t := &toaster{bar: roundRect(colAccent, 2), label: wrapLabel("")}
	bg := roundRect(colSurface2, 12)
	bg.StrokeColor = rgb(0x3a3f4c)
	bg.StrokeWidth = 1
	t.box = container.NewStack(bg, container.NewBorder(nil, nil, inset(10, 10, 10, 0, fixed(4, 0, t.bar)), nil, inset(2, 2, 4, 8, t.label)))
	t.box.Hide()
	t.lay = &toastLayout{}
	t.layer = container.New(t.lay, t.box)
	return t
}

func (t *toaster) show(kind toastKind, msg string) {
	switch kind {
	case toastSuccess:
		t.bar.FillColor = colorSuccess
	case toastError:
		t.bar.FillColor = colorError
	default:
		t.bar.FillColor = colAccent
	}
	t.bar.Refresh()
	t.label.SetText(msg)
	t.box.Show()
	// slide in from below
	stopAnim(t.anim)
	t.anim = fyne.NewAnimation(220*time.Millisecond, func(p float32) {
		t.lay.slide = 48 * (1 - p)
		t.lay.Layout(t.layer.Objects, t.layer.Size())
	})
	t.anim.Curve = fyne.AnimationEaseOut
	t.anim.Start()
	t.gen++
	gen := t.gen
	d := 3500 * time.Millisecond
	if kind == toastError {
		d = 6 * time.Second
	}
	time.AfterFunc(d, func() {
		if t.closed.Load() {
			return
		}
		fyne.Do(func() {
			if t.gen == gen {
				t.box.Hide()
			}
		})
	})
}

// buildShell assembles header, navigation, screens and toast layer.
func (u *appUI) buildShell(screens []navScreen) fyne.CanvasObject {
	brandName := text("agentpad", 18, colText, true)
	brand := container.NewHBox(vcenter(fixed(32, 32, widget.NewIcon(iconApp))), vcenter(brandName))
	pills := container.NewHBox(u.pillConfig, u.pillPad, u.pillAgents, u.jumpBtn, u.pillRecv)
	headerBG := canvas.NewRectangle(colSurface)
	header := container.NewBorder(nil, widget.NewSeparator(), nil, nil,
		container.NewStack(headerBG, inset(10, 10, 16, 16, container.New(&headerLayout{}, brand, pills))))

	side := container.NewVBox()
	tabs := container.NewGridWithColumns(len(screens))
	stack := container.NewStack()
	u.nav = make([][2]*navItem, len(screens))
	for i, s := range screens {
		i := i
		a := newNavItem(s.title, s.icon, false, func() { u.navigate(i) })
		b := newNavItem(s.title, s.icon, true, func() { u.navigate(i) })
		u.nav[i] = [2]*navItem{a, b}
		side.Add(a)
		tabs.Add(b)
		stack.Add(s.content)
	}
	u.screens = stack
	sideBG := canvas.NewRectangle(colBackground)
	sidebar := container.NewStack(sideBG, container.NewBorder(nil, nil, nil, widget.NewSeparator(),
		inset(14, 14, 10, 10, container.NewBorder(nil, u.sideFooter(), nil, nil, side))))
	tabBG := canvas.NewRectangle(colSurface)
	tabbar := container.NewStack(tabBG, container.NewBorder(nil, widget.NewSeparator(), nil, nil, inset(4, 4, 6, 6, tabs)))

	shell := container.New(shellLayout{}, header, sidebar, tabbar, stack)
	u.navigate(screenProfiles)
	return container.NewStack(shell, u.toasts.layer)
}

func (u *appUI) sideFooter() fyne.CanvasObject {
	ver := u.deps.Version
	if ver != "" && ver[0] >= '0' && ver[0] <= '9' {
		ver = "v" + ver
	}
	v := text(ver, 11, colText3, false)
	return container.NewVBox(layout.NewSpacer(), inset(0, 0, 12, 0, v))
}

type navScreen struct {
	title   string
	icon    fyne.Resource
	content fyne.CanvasObject
}

// navigate shows screen i. UI goroutine.
func (u *appUI) navigate(i int) {
	for j, o := range u.screens.Objects {
		on := i == j
		if on {
			o.Show()
		} else {
			o.Hide()
		}
		u.nav[j][0].setSelected(on)
		u.nav[j][1].setSelected(on)
	}
	u.current = i
}
