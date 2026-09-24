// Command gamepad-advanced showcases every knob of Fyne's gamepad support:
// custom cursor images, live button remapping (all action kinds), the on-screen
// keyboard with language switching, and stick scrolling.
package main

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var allButtons = []desktop.GamepadButton{
	desktop.GamepadA, desktop.GamepadB, desktop.GamepadX, desktop.GamepadY,
	desktop.GamepadLeftShoulder, desktop.GamepadRightShoulder,
	desktop.GamepadBack, desktop.GamepadStart,
	desktop.GamepadLeftStick, desktop.GamepadRightStick,
	desktop.GamepadDPadUp, desktop.GamepadDPadDown, desktop.GamepadDPadLeft, desktop.GamepadDPadRight,
	desktop.GamepadLeftTrigger, desktop.GamepadRightTrigger, desktop.GamepadGuide,
}

func buttonName(b desktop.GamepadButton) string {
	names := map[desktop.GamepadButton]string{
		desktop.GamepadA: "A", desktop.GamepadB: "B", desktop.GamepadX: "X", desktop.GamepadY: "Y",
		desktop.GamepadLeftShoulder: "LB", desktop.GamepadRightShoulder: "RB",
		desktop.GamepadBack: "Back", desktop.GamepadStart: "Start",
		desktop.GamepadLeftStick: "L3", desktop.GamepadRightStick: "R3",
		desktop.GamepadDPadUp: "D-pad up", desktop.GamepadDPadDown: "D-pad down",
		desktop.GamepadDPadLeft: "D-pad left", desktop.GamepadDPadRight: "D-pad right",
		desktop.GamepadLeftTrigger: "LT", desktop.GamepadRightTrigger: "RT",
		desktop.GamepadGuide: "Guide",
	}
	return names[b]
}

func bindingText(b desktop.GamepadBinding) string {
	switch b.Kind {
	case desktop.GamepadActionNone:
		return "unbound"
	case desktop.GamepadActionLeftClick:
		return "left click"
	case desktop.GamepadActionRightClick:
		return "right click"
	case desktop.GamepadActionMiddleClick:
		return "middle click (paste)"
	case desktop.GamepadActionSnapCenter:
		return "snap cursor to center"
	case desktop.GamepadActionFunc:
		return "custom func()"
	case desktop.GamepadActionKey:
		s := "key " + string(b.Key)
		if m := modifierText(b.Modifier); m != "" {
			s = m + "+" + s
		}
		if b.FollowFocus {
			s += ", cursor follows focus"
		}
		return s
	}
	return "?"
}

func modifierText(m fyne.KeyModifier) string {
	var parts []string
	if m&fyne.KeyModifierControl != 0 {
		parts = append(parts, "Ctrl")
	}
	if m&fyne.KeyModifierShift != 0 {
		parts = append(parts, "Shift")
	}
	if m&fyne.KeyModifierAlt != 0 {
		parts = append(parts, "Alt")
	}
	if m&fyne.KeyModifierSuper != 0 {
		parts = append(parts, "Super")
	}
	return strings.Join(parts, "+")
}

// -- fancy cursors, drawn in code so the demo ships no assets --

func arrowCursor(c color.Color) image.Image {
	pts := [][2]float64{{1, 1}, {1, 30}, {9, 23}, {14, 38}, {19, 36}, {14, 21}, {24, 21}} // classic pointer
	img := image.NewRGBA(image.Rect(0, 0, 28, 42))
	for y := 0; y < 42; y++ {
		for x := 0; x < 28; x++ {
			if inPolygon(float64(x)+0.5, float64(y)+0.5, pts) {
				img.Set(x, y, c)
			} else if nearEdge(float64(x)+0.5, float64(y)+0.5, pts) {
				img.Set(x, y, color.Black) // outline keeps it visible on any background
			}
		}
	}
	return img
}

func ringCursor(c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 25, 25))
	for y := 0; y < 25; y++ {
		for x := 0; x < 25; x++ {
			dx, dy := float64(x-12), float64(y-12)
			r := dx*dx + dy*dy
			switch {
			case r <= 9: // crosshair center marks the exact hotspot
				img.Set(x, y, color.Black)
			case r >= 64 && r <= 121:
				img.Set(x, y, c)
			}
		}
	}
	return img
}

func inPolygon(x, y float64, pts [][2]float64) bool {
	inside := false
	for i, p := range pts {
		q := pts[(i+1)%len(pts)]
		if (p[1] > y) != (q[1] > y) && x < (q[0]-p[0])*(y-p[1])/(q[1]-p[1])+p[0] {
			inside = !inside
		}
	}
	return inside
}

func nearEdge(x, y float64, pts [][2]float64) bool {
	for i, p := range pts {
		q := pts[(i+1)%len(pts)]
		if distToSegment(x, y, p[0], p[1], q[0], q[1]) < 1.2 {
			return true
		}
	}
	return false
}

func distToSegment(px, py, x1, y1, x2, y2 float64) float64 {
	dx, dy := x2-x1, y2-y1
	l2 := dx*dx + dy*dy
	t := ((px-x1)*dx + (py-y1)*dy) / l2
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return math.Hypot(px-(x1+t*dx), py-(y1+t*dy))
}

// -- app --

type demo struct {
	bindings map[desktop.GamepadButton]*widget.Label
	count    int
	swapped  bool
	countLbl *widget.Label
	langLbl  *widget.Label
}

func (d *demo) refreshBindings() {
	for b, lbl := range d.bindings {
		if b == desktop.GamepadOSKToggle() {
			lbl.SetText("opens on-screen keyboard") // checked before any binding: the toggle wins
			continue
		}
		lbl.SetText(bindingText(desktop.GamepadBindingFor(b)))
	}
}

func (d *demo) cursorTab() fyne.CanvasObject {
	preview := canvas.NewImageFromImage(arrowCursor(color.NRGBA{R: 0xff, G: 0x33, B: 0xcc, A: 0xff}))
	preview.FillMode = canvas.ImageFillOriginal

	set := func(img image.Image, hx, hy int) {
		desktop.SetGamepadCursor(img, hx, hy)
		preview.Image = img
		preview.Refresh()
	}
	magenta := color.NRGBA{R: 0xff, G: 0x33, B: 0xcc, A: 0xff}
	lime := color.NRGBA{R: 0x66, G: 0xff, B: 0x33, A: 0xff}
	gold := color.NRGBA{R: 0xff, G: 0xc8, B: 0x22, A: 0xff}

	buttons := container.NewHBox(
		widget.NewButton("magenta arrow", func() { set(arrowCursor(magenta), 1, 1) }),
		widget.NewButton("lime ring", func() { set(ringCursor(lime), 12, 12) }),
		widget.NewButton("gold ring", func() { set(ringCursor(gold), 12, 12) }),
		widget.NewButtonWithIcon("default arrow", theme.CancelIcon(), func() {
			desktop.ResetGamepadCursor()
			preview.Image = image.NewRGBA(image.Rect(0, 0, 1, 1)) // blank: no giant arrow in the preview
			preview.Refresh()
		}),
	)

	info := widget.NewLabel("SetGamepadCursor themes the hardware cursor itself, so mouse users see it too. " +
		"Hover cursors (text I-beam...) still win. Takes effect as the pointer moves on.")
	return container.NewBorder(nil, nil, nil, nil,
		container.NewVBox(info, buttons, widget.NewSeparator(), preview))
}

func (d *demo) buttonsTab() fyne.CanvasObject {
	rows := []*widget.FormItem{}
	for _, b := range allButtons {
		lbl := widget.NewLabel("")
		d.bindings[b] = lbl
		rows = append(rows, widget.NewFormItem(buttonName(b), lbl))
	}
	d.refreshBindings()

	countUp := func(fyne.Window) { // GamepadActionFunc: main thread, next to regular UI events
		d.count++
		d.countLbl.SetText("func() fired " + strconv.Itoa(d.count) + "x")
	}

	var swapBtn *widget.Button
	swapBtn = widget.NewButton("swap A/B clicks", func() { // press again to swap back
		a, b := desktop.GamepadBinding{Kind: desktop.GamepadActionLeftClick}, desktop.GamepadBinding{Kind: desktop.GamepadActionRightClick}
		if d.swapped {
			d.swapped = false
			swapBtn.SetText("swap A/B clicks")
		} else {
			a, b = b, a
			d.swapped = true
			swapBtn.SetText("un-swap A/B clicks")
		}
		desktop.SetGamepadBinding(desktop.GamepadA, a)
		desktop.SetGamepadBinding(desktop.GamepadB, b)
		d.refreshBindings()
	})

	control := container.NewVBox(
		widget.NewLabelWithStyle("Try these remaps (live, no restart):", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewHBox(
			swapBtn,
			widget.NewButton("Y = func(): count presses", func() {
				desktop.SetGamepadBinding(desktop.GamepadY, desktop.GamepadBinding{Kind: desktop.GamepadActionFunc, Func: countUp})
				d.refreshBindings()
			}),
			widget.NewButton("X = Ctrl+C copy + follow focus", func() {
				desktop.SetGamepadBinding(desktop.GamepadX, desktop.GamepadBinding{
					Kind: desktop.GamepadActionKey, Key: fyne.KeyC, Modifier: fyne.KeyModifierControl, FollowFocus: true})
				d.refreshBindings()
			}),
			widget.NewButton("reset all", func() {
				desktop.ResetGamepadBindings()
				d.swapped = false // keep the swap button honest about reality
				swapBtn.SetText("swap A/B clicks")
				d.refreshBindings()
			}),
		),
	)

	d.countLbl = widget.NewLabel("func() fired 0x")
	grid := container.NewScroll(widget.NewForm(rows...))
	return container.NewBorder(nil, container.NewVBox(control, d.countLbl), nil, nil, grid)
}

func (d *demo) keyboardTab() fyne.CanvasObject {
	d.langLbl = widget.NewLabel("layout: " + desktop.GamepadOSKLanguage())
	toggle := widget.NewButton("switch layout en/de", func() {
		next := "de"
		if desktop.GamepadOSKLanguage() == "de" {
			next = "en"
		}
		desktop.SetGamepadOSKLanguage(next)
		d.langLbl.SetText("layout: " + next + " (applies when the keyboard opens next)")
	})

	entry := widget.NewEntry()
	entry.SetPlaceHolder("press Guide on your pad to type here...")
	pass := widget.NewPasswordEntry()
	pass.SetPlaceHolder("password field: preview stays masked")
	multi := widget.NewMultiLineEntry()
	multi.SetPlaceHolder("multi-line: the OSK preview mirrors every line")

	comboBtns, hold := desktop.GamepadHelpCombo()
	names := make([]string, 0, len(comboBtns))
	for _, b := range comboBtns {
		names = append(names, buttonName(b))
	}
	helpInfo := widget.NewLabel("keymap overlay: hold " + strings.Join(names, "+") + " for " + hold.String())

	return container.NewVBox(
		widget.NewLabelWithStyle("On-screen keyboard (press Guide):", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		entry, pass, multi, toggle, d.langLbl, widget.NewSeparator(), helpInfo)
}

func scrollTab() fyne.CanvasObject {
	items := make([]string, 100)
	for i := range items {
		items[i] = "row " + strconv.Itoa(i+1) + " - tilt the right stick to scroll"
	}
	return widget.NewList(
		func() int { return len(items) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.ListItemID, obj fyne.CanvasObject) { obj.(*widget.Label).SetText(items[id]) })
}

func main() {
	a := app.New()
	w := a.NewWindow("gamepad advanced")
	d := &demo{bindings: map[desktop.GamepadButton]*widget.Label{}}

	tabs := container.NewAppTabs(
		container.NewTabItem("Cursor", d.cursorTab()),
		container.NewTabItem("Buttons", d.buttonsTab()),
		container.NewTabItem("Keyboard", d.keyboardTab()),
		container.NewTabItem("Scroll", scrollTab()),
	)

	w.SetContent(tabs)
	w.Resize(fyne.NewSize(720, 560))
	w.ShowAndRun()
}
