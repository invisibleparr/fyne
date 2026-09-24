//go:build !wasm && !test_web_driver

package glfw

import (
	"image/color"
	"math"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
)

// Keymap help overlay: a drawn Xbox 360 style pad with red leader lines pointing
// from each control to its current binding, cloning the classic reference diagram.
// Everything is vector primitives in one fixed design space that scales to fit.
const (
	gamepadHelpDesignW = 980
	gamepadHelpDesignH = 430
)

var (
	helpPadGray    = color.NRGBA{R: 0x5b, G: 0x5e, B: 0x66, A: 0xff}
	helpPadDark    = color.NRGBA{R: 0x3a, G: 0x3d, B: 0x42, A: 0xff}
	helpPadOutline = color.NRGBA{R: 0x2b, G: 0x2d, B: 0x33, A: 0xff}
	helpStickCap   = color.NRGBA{R: 0x6a, G: 0x6d, B: 0x75, A: 0xff}
	helpLeaderRed  = color.NRGBA{R: 0xd2, G: 0x28, B: 0x28, A: 0xff}
	helpPillBg     = color.NRGBA{R: 0x14, G: 0x16, B: 0x1c, A: 0xf2}

	helpColorA = color.NRGBA{R: 0x8c, G: 0xc3, B: 0x4a, A: 0xff}
	helpColorB = color.NRGBA{R: 0xcd, G: 0x32, B: 0x3c, A: 0xff}
	helpColorX = color.NRGBA{R: 0x28, G: 0x8c, B: 0xd2, A: 0xff}
	helpColorY = color.NRGBA{R: 0xf5, G: 0xcd, B: 0x28, A: 0xff}
)

// gamepadHelpLabel maps one control to its label anchor in design coordinates.
type gamepadHelpLabel struct {
	button     desktop.GamepadButton
	name       string        // control name shown before the binding text
	from       fyne.Position // control center (leader line starts at its edge)
	radius     float32       // design-space radius: line leaves center+radius toward the label
	to         fyne.Position // label anchor point
	rightAlign bool          // left-side labels end at anchor instead of starting there
	centerX    bool          // top/bottom labels center horizontally on anchor
}

var gamepadHelpLabels = []gamepadHelpLabel{
	{desktop.GamepadLeftTrigger, "LT", fyne.NewPos(397, 52), 28, fyne.NewPos(250, 40), true, false},
	{desktop.GamepadLeftShoulder, "LB", fyne.NewPos(397, 76), 48, fyne.NewPos(250, 76), true, false},
	{desktop.GamepadLeftStick, "L3", fyne.NewPos(365, 165), 40, fyne.NewPos(250, 160), true, false},
	{desktop.GamepadDPadUp, "D-pad up", fyne.NewPos(415, 198), 2, fyne.NewPos(250, 318), true, false},
	{desktop.GamepadDPadDown, "D-pad down", fyne.NewPos(415, 262), 2, fyne.NewPos(250, 340), true, false},
	{desktop.GamepadDPadLeft, "D-pad left", fyne.NewPos(383, 230), 2, fyne.NewPos(250, 362), true, false},
	{desktop.GamepadDPadRight, "D-pad right", fyne.NewPos(447, 230), 2, fyne.NewPos(250, 384), true, false},
	{desktop.GamepadBack, "Back", fyne.NewPos(441, 150), 15, fyne.NewPos(380, 16), false, true},
	{desktop.GamepadStart, "Start", fyne.NewPos(539, 150), 15, fyne.NewPos(600, 16), false, true},
	{desktop.GamepadRightStick, "R3", fyne.NewPos(555, 230), 40, fyne.NewPos(730, 360), false, true},
	{desktop.GamepadY, "Y", fyne.NewPos(640, 123), 21, fyne.NewPos(730, 120), false, false},
	{desktop.GamepadB, "B", fyne.NewPos(682, 165), 21, fyne.NewPos(730, 158), false, false},
	{desktop.GamepadX, "X", fyne.NewPos(598, 165), 21, fyne.NewPos(730, 205), false, false},
	{desktop.GamepadA, "A", fyne.NewPos(640, 207), 21, fyne.NewPos(730, 234), false, false},
	{desktop.GamepadRightShoulder, "RB", fyne.NewPos(582, 76), 48, fyne.NewPos(730, 74), false, false},
	{desktop.GamepadRightTrigger, "RT", fyne.NewPos(582, 52), 28, fyne.NewPos(730, 40), false, false},
}

// gamepadHelpNotes document stick AXIS behaviors (cursor movement, scrolling).
// They are not button bindings, so they show even when every button is unbound.
var gamepadHelpNotes = []gamepadHelpLabel{
	{button: 0, name: "left stick moves cursor", from: fyne.NewPos(365, 165), radius: 40, to: fyne.NewPos(250, 200), rightAlign: true},
	{button: 0, name: "right stick scrolls", from: fyne.NewPos(555, 230), radius: 40, to: fyne.NewPos(730, 290)},
	{button: 0, name: "Guide opens keyboard", from: fyne.NewPos(490, 150), radius: 20, to: fyne.NewPos(490, 412), centerX: true},
}

// edgeStart moves from the control center toward the label so leader lines leave
// the control's rim instead of crossing over it and its neighbors.
func edgeStart(center fyne.Position, radius float32, toward fyne.Position) fyne.Position {
	dx, dy := toward.X-center.X, toward.Y-center.Y
	l := float32(math.Hypot(float64(dx), float64(dy)))
	if l > 0 && radius > 0 {
		return fyne.NewPos(center.X+dx/l*radius, center.Y+dy/l*radius)
	}
	return center
}

// buildGamepadHelp draws the diagram scaled to fit size. bindingFor is a parameter
// so tests can build it without an app or theme running. Labels are always white on
// dark rounded pills: readable over any theme and any pad part they cross.
func buildGamepadHelp(size fyne.Size, bindingFor func(desktop.GamepadButton) desktop.GamepadBinding) fyne.CanvasObject {
	s := fyne.Min((size.Width-24)/gamepadHelpDesignW, (size.Height-24)/gamepadHelpDesignH)
	if s <= 0 {
		s = 1 // tiny window: draw at natural size and let it clip
	}
	offX := (size.Width - gamepadHelpDesignW*s) / 2
	offY := (size.Height - gamepadHelpDesignH*s) / 2

	root := container.NewWithoutLayout()
	addGamepadPad(root, s, offX, offY)

	for _, lbl := range gamepadHelpLabels {
		binding := bindingFor(lbl.button)
		if binding.Kind == desktop.GamepadActionNone {
			continue // unbound buttons get no clutter
		}
		desc := lbl.name + ": " + strings.ReplaceAll(describeGamepadBinding(binding), " (cursor follows focus)", " +focus")
		addHelpLabel(root, lbl, desc, s, offX, offY)
	}
	for _, note := range gamepadHelpNotes {
		addHelpLabel(root, note, note.name, s, offX, offY)
	}
	return root
}

// addHelpLabel draws one leader line plus its text on a dark pill.
func addHelpLabel(root *fyne.Container, lbl gamepadHelpLabel, desc string, s, offX, offY float32) {
	p1 := helpPoint(edgeStart(lbl.from, lbl.radius, lbl.to), s, offX, offY)
	p2 := helpPoint(lbl.to, s, offX, offY)
	root.Add(helpLine(p1, p2))

	txt := canvas.NewText(desc, color.White)
	txt.TextSize = 13 * s
	w, h := txt.MinSize().Width, txt.MinSize().Height
	var tx float32
	switch {
	case lbl.rightAlign:
		tx = p2.X - w
	case lbl.centerX:
		tx = p2.X - w/2
	default:
		tx = p2.X
	}
	ty := p2.Y - h/2
	addHelpPill(root, fyne.NewPos(tx, ty), w, h, s)
	txt.Move(fyne.NewPos(tx, ty))
	root.Add(txt)
}

// addHelpPill draws the rounded background behind a label: ONE rectangle with
// corner radius = half height. (Old version stitched circle caps onto a rect; the
// anti-aliased circles lost half a pixel so caps looked smaller than the body.)
func addHelpPill(root *fyne.Container, pos fyne.Position, w, h, s float32) {
	pad := 4 * s
	bx, by := pos.X-pad*2, pos.Y-pad
	bw, bh := w+pad*4, h+pad*2
	bg := canvas.NewRectangle(helpPillBg)
	bg.CornerRadius = bh / 2
	bg.Resize(fyne.NewSize(bw, bh))
	bg.Move(fyne.NewPos(bx, by))
	root.Add(bg)
}

func helpPoint(p fyne.Position, s float32, offX, offY float32) fyne.Position {
	return fyne.NewPos(offX+p.X*s, offY+p.Y*s)
}

func helpLine(from, to fyne.Position) *canvas.Line {
	return &canvas.Line{Position1: from, Position2: to, StrokeColor: helpLeaderRed, StrokeWidth: 1.5}
}

// addGamepadPad draws the controller silhouette and all its controls (unlabeled).
// Silhouette clones the reference proportions: wide rounded body with a straight
// lower edge (dpad + right stick live in its lower half), two grips flaring
// down-left/down-right. The blob gets
// ONE clean outline by drawing every shape once inflated in outline color first, then
// the fills on top - internal seams vanish under neighbors. canvas.Circle is NOT an
// ellipse (it renders a min-dimension circle), so wide parts are rounded rectangles.
func addGamepadPad(root *fyne.Container, s, offX, offY float32) {
	rounded := func(x, y, w, h, r float32, fill color.Color, inflate float32) {
		rect := canvas.NewRectangle(fill)
		rect.CornerRadius = (r + inflate) * s
		rect.Resize(fyne.NewSize((w+2*inflate)*s, (h+2*inflate)*s))
		rect.Move(helpPoint(fyne.NewPos(x-inflate, y-inflate), s, offX, offY))
		root.Add(rect)
	}

	type blob struct{ x, y, w, h, r float32 } // rounded rect; circle = square with r=w/2
	blobs := []blob{
		{205, 205, 190, 190, 95}, // left grip
		{585, 205, 190, 190, 95}, // right grip
		{250, 78, 480, 210, 60},  // main body (dpad + right stick live in its lower half)
	}
	for _, b := range blobs { // pass one: outline color inflated = union silhouette edge
		rounded(b.x, b.y, b.w, b.h, b.r, helpPadOutline, 3)
	}
	for _, b := range blobs { // pass two: fills cover internal seams
		rounded(b.x, b.y, b.w, b.h, b.r, helpPadGray, 0)
	}

	rect := func(x, y, w, h float32, fill color.Color) {
		r := canvas.NewRectangle(fill)
		r.Resize(fyne.NewSize(w*s, h*s))
		r.Move(helpPoint(fyne.NewPos(x, y), s, offX, offY))
		root.Add(r)
	}
	circle := func(cx, cy, r float32, fill color.Color) {
		c := canvas.NewCircle(fill)
		c.StrokeColor = helpPadOutline
		c.StrokeWidth = 1.5 * s
		size := 2 * r * s
		c.Resize(fyne.NewSize(size, size))
		c.Move(helpPoint(fyne.NewPos(cx-r, cy-r), s, offX, offY))
		root.Add(c)
	}
	letter := func(center fyne.Position, name string, col color.Color) {
		txt := canvas.NewText(name, col)
		txt.TextSize = 13 * s
		w, h := txt.MinSize().Width, txt.MinSize().Height
		p := helpPoint(center, s, offX, offY)
		txt.Move(fyne.NewPos(p.X-w/2, p.Y-h/2))
		root.Add(txt)
	}

	// shoulders + trigger nubs on top edge (distinct parts, own dark tone)
	rect(350, 62, 95, 28, helpPadDark) // LB
	rect(535, 62, 95, 28, helpPadDark) // RB
	rect(370, 40, 55, 24, helpPadGray) // LT nub
	rect(555, 40, 55, 24, helpPadGray) // RT nub

	// sticks: dark well + lighter cap
	circle(365, 165, 38, helpPadDark)
	circle(365, 165, 24, helpStickCap)
	circle(555, 230, 38, helpPadDark)
	circle(555, 230, 24, helpStickCap)

	// d-pad cross (lower body, left of right stick)
	rect(401, 198, 28, 64, helpPadDark)
	rect(383, 216, 64, 28, helpPadDark)

	// back / start pills flanking the guide orb
	rect(428, 143, 26, 14, helpPadDark)
	rect(526, 143, 26, 14, helpPadDark)

	// guide button with green x (opens the on-screen keyboard, see gamepadHelpNotes)
	circle(490, 150, 20, color.NRGBA{R: 0xcf, G: 0xd2, B: 0xd8, A: 0xff})
	letter(fyne.NewPos(490, 150), "X", color.NRGBA{R: 0x3a, G: 0x7d, B: 0x2c, A: 0xff})

	// ABXY cluster in console colors (upper right)
	abxy := func(cx, cy float32, name string, fill, text color.Color) {
		circle(cx, cy, 19, fill)
		letter(fyne.NewPos(cx, cy), name, text)
	}
	black := color.NRGBA{A: 0xff}
	white := color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	abxy(640, 123, "Y", helpColorY, black)
	abxy(682, 165, "B", helpColorB, white)
	abxy(598, 165, "X", helpColorX, white)
	abxy(640, 207, "A", helpColorA, black)
}

// helpKeyNames prettifies key names that read badly on a gamepad diagram (they come
// straight from X11 keysym heritage: Prior/Next = page up/down).
var helpKeyNames = map[fyne.KeyName]string{
	fyne.KeyPageUp:   "Page Up",
	fyne.KeyPageDown: "Page Down",
	fyne.KeyReturn:   "Enter",
	fyne.KeyEscape:   "Esc",
}

// describeGamepadBinding renders a binding as human text for the help diagram.
func describeGamepadBinding(b desktop.GamepadBinding) string {
	switch b.Kind {
	case desktop.GamepadActionLeftClick:
		return "left click"
	case desktop.GamepadActionRightClick:
		return "right click"
	case desktop.GamepadActionMiddleClick:
		return "middle click"
	case desktop.GamepadActionSnapCenter:
		return "snap cursor to center"
	case desktop.GamepadActionFunc:
		return "custom function"
	case desktop.GamepadActionKey:
		s := ""
		if b.Modifier&fyne.KeyModifierControl != 0 {
			s += "Ctrl+"
		}
		if b.Modifier&fyne.KeyModifierShift != 0 {
			s += "Shift+"
		}
		if b.Modifier&fyne.KeyModifierAlt != 0 {
			s += "Alt+"
		}
		if b.Modifier&fyne.KeyModifierSuper != 0 {
			s += "Super+"
		}
		if nice, ok := helpKeyNames[b.Key]; ok {
			s += nice
		} else {
			s += string(b.Key)
		}
		if b.FollowFocus {
			s += " (cursor follows focus)"
		}
		return s
	}
	return ""
}
