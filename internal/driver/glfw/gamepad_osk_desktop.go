//go:build !wasm && !test_web_driver

package glfw

import (
	"fmt"
	"image/color"
	"os"
	"strings"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"github.com/go-gl/glfw/v3.4/glfw"
)

// On-screen keyboard, Steam Deck style: two ghost cursors (one per stick, drawn as
// translucent circles over the keys), LT/RT type whatever is under their side's cursor.
// The overlay is driven ONLY by pollGamepad and an invisible tap-blocking sheet: while it
// is up nothing (mouse or A-click) reaches the app below, so the captured target Entry
// stays the one we type into. LESSON: OverlayStack.Add always
// installs a focus manager, so canvas.Focused() reports nil while any overlay is up;
// we track the target entry ourselves and type straight into it (physical keys too).

const (
	oskHeightFrac  = 0.42                   // keyboard occupies this fraction of window height
	oskRepeatDelay = 400 * time.Millisecond // hold a key this long before it repeats
	oskRepeatRate  = 50 * time.Millisecond  // then repeat at this rate
)

var (
	oskBgColor         = color.NRGBA{R: 0x18, G: 0x18, B: 0x1a, A: 0xf2}
	oskKeyFill         = color.NRGBA{R: 0x3c, G: 0x40, B: 0x46, A: 0xff}
	oskKeyHi           = color.NRGBA{R: 0x5a, G: 0x74, B: 0xb8, A: 0xff} // active shift/layer key
	oskTextColor       = color.NRGBA{R: 0xf2, G: 0xf2, B: 0xf2, A: 0xff}
	oskCursorLeftFill  = color.NRGBA{R: 0xff, G: 0x7a, B: 0x7a, A: 0x59} // left thumb sees red, right sees blue: never lose a cursor
	oskCursorLeftEdge  = color.NRGBA{R: 0xff, G: 0xb3, B: 0xb3, A: 0xcc}
	oskCursorRightFill = color.NRGBA{R: 0x7a, G: 0x9e, B: 0xff, A: 0x59}
	oskCursorRightEdge = color.NRGBA{R: 0xb3, G: 0xcc, B: 0xff, A: 0xcc}
)

type oskCmd int

const (
	oskCmdNone    oskCmd = iota
	oskCmdShift          // sticky one-shot shift
	oskCmdSymbols        // switch to symbols page
	oskCmdLetters        // back to letters page
	oskCmdPaste          // clipboard paste into the target entry
)

// oskKey is one key: types rune r, or fires special key name, or runs a layer command.
type oskKey struct {
	label string       // drawn on the key
	r     rune         // character typed (0 for specials/commands)
	key   fyne.KeyName // special key fired when r == 0
	cmd   oskCmd
	w     float32 // relative width; standard key = 1
}

func oskRunes(s string) []oskKey {
	keys := make([]oskKey, 0, len(s))
	for _, r := range s {
		keys = append(keys, oskKey{label: string(r), r: r, w: 1})
	}
	return keys
}

func oskRune(r rune) oskKey { return oskKey{label: string(r), r: r, w: 1} }

// oskBottomRow is shared by both pages; its first key is the smart layer switch:
// "?123" on letters, "ABC" on symbols.
func oskBottomRow(symbols bool) []oskKey {
	layer := oskKey{label: "?123", cmd: oskCmdSymbols, w: 1.4}
	if symbols {
		layer = oskKey{label: "ABC", cmd: oskCmdLetters, w: 1.4}
	}
	return []oskKey{
		layer,
		{label: ",", r: ',', w: 1},
		{label: "←", key: fyne.KeyLeft, w: 1},
		{label: "→", key: fyne.KeyRight, w: 1},
		{label: "paste", cmd: oskCmdPaste, w: 1.4},
		{label: "space", r: ' ', w: 3.6},
		{label: "enter", key: fyne.KeyReturn, w: 1.6},
	}
}

func oskLettersEN() [][]oskKey {
	return [][]oskKey{
		oskRunes("qwertyuiop"),
		oskRunes("asdfghjkl"),
		append([]oskKey{{label: "⇧", cmd: oskCmdShift, w: 1.6}}, append(oskRunes("zxcvbnm"),
			oskKey{label: "⌫", key: fyne.KeyBackspace, w: 1.6})...),
		oskBottomRow(false),
	}
}

// oskLettersDE is QWERTZ with umlauts appended; y sits in the bottom row like a real German board.
func oskLettersDE() [][]oskKey {
	return [][]oskKey{
		append(oskRunes("qwertzuiop"), oskRune('ü')),
		append(oskRunes("asdfghjkl"), oskRune('ö'), oskRune('ä')),
		append([]oskKey{{label: "⇧", cmd: oskCmdShift, w: 1.6}}, append(append(oskRunes("yxcvbnm"), oskRune('ß')),
			oskKey{label: "⌫", key: fyne.KeyBackspace, w: 1.6})...),
		oskBottomRow(false),
	}
}

func oskSymbols() [][]oskKey {
	return [][]oskKey{
		oskRunes("1234567890"),
		oskRunes(`!?"'%&/-_`),
		append(oskRunes(".,;:@#+="), oskKey{label: "⌫", key: fyne.KeyBackspace, w: 1.6}),
		oskBottomRow(true),
	}
}

func oskRowsFor(lang string, symbols bool) [][]oskKey {
	if symbols {
		return oskSymbols()
	}
	if lang == "de" {
		return oskLettersDE()
	}
	return oskLettersEN()
}

// oskRect is a plain geometry rectangle; fyne only ships the canvas object, not the shape.
type oskRect struct{ Min, Max fyne.Position }

func (r oskRect) Dx() float32 { return r.Max.X - r.Min.X }
func (r oskRect) Dy() float32 { return r.Max.Y - r.Min.Y }

// oskHit pairs a key with the rectangle it occupies in canvas coordinates.
type oskHit struct {
	rect oskRect
	key  oskKey
}

// layoutOSK computes key rectangles inside the bottom band of size.
func layoutOSK(size fyne.Size, rows [][]oskKey) ([]oskHit, oskRect) {
	kb := oskRect{
		Min: fyne.NewPos(size.Width*0.02, size.Height*(1-oskHeightFrac)),
		Max: fyne.NewPos(size.Width*0.98, size.Height),
	}
	gap := kb.Dy() * 0.035
	keyH := (kb.Dy() - gap*float32(len(rows)+1)) / float32(len(rows))

	var hits []oskHit
	y := kb.Min.Y + gap
	for _, row := range rows {
		units := float32(0)
		for _, k := range row {
			units += k.w
		}
		unitW := (kb.Dx() - gap*float32(len(row)-1)) / units
		x := kb.Min.X
		for _, k := range row {
			w := unitW * k.w
			hits = append(hits, oskHit{rect: oskRect{Min: fyne.NewPos(x, y), Max: fyne.NewPos(x+w, y+keyH)}, key: k})
			x += w + gap
		}
		y += keyH + gap
	}
	return hits, kb
}

// oskKeyAt returns the key under a point, or nil when between keys.
func oskKeyAt(hits []oskHit, p fyne.Position) *oskKey {
	for i := range hits {
		r := hits[i].rect
		if p.X >= r.Min.X && p.X <= r.Max.X && p.Y >= r.Min.Y && p.Y <= r.Max.Y {
			return &hits[i].key
		}
	}
	return nil
}

// oskMove advances a ghost cursor; full stick tilt crosses the keyboard in one second.
func oskMove(cur fyne.Position, ax, ay float32, kb oskRect, dt time.Duration) fyne.Position {
	speed := kb.Dx() * float32(dt.Seconds())
	return fyne.NewPos(
		clampF32(cur.X+float32(gamepadRamp(ax))*speed, kb.Min.X, kb.Max.X),
		clampF32(cur.Y+float32(gamepadRamp(ay))*speed, kb.Min.Y, kb.Max.Y),
	)
}

func clampF32(v, min, max float32) float32 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// oskTyped resolves the character a key produces under the current shift state.
func oskTyped(k oskKey, shift bool) rune {
	if k.r == 0 || !shift || !unicode.IsLetter(k.r) {
		return k.r
	}
	return unicode.ToUpper(k.r) // simple case mapping: ß becomes B, good enough for v1
}

// oskRepeatCount reports how many repeat fires a held typing key earned by now (the press edge itself is not counted).
func oskRepeatCount(held time.Duration) int {
	if held < oskRepeatDelay {
		return 0
	}
	return 1 + int((held-oskRepeatDelay)/oskRepeatRate)
}

// oskVisual is one built keyboard; rebuild it whenever size/lang/layer/text changes.
type oskVisual struct {
	content *fyne.Container
	hits    []oskHit
	kb      oskRect
	left    *canvas.Circle // ghost cursors, drawn last so they sit on top of everything
	right   *canvas.Circle
	cursorR float32
}

// oskBlocker is an invisible full-canvas sheet that swallows taps: while the keyboard
// is up neither real mouse nor A-click can reach the app below (modal by construction).
type oskBlocker struct{ *canvas.Rectangle }

func (o *oskBlocker) Tapped(*fyne.PointEvent) {}

func buildGamepadOSK(size fyne.Size, lang string, symbols, shift bool, text string) *oskVisual {
	rows := oskRowsFor(lang, symbols)
	hits, kb := layoutOSK(size, rows)
	keyH := hits[0].rect.Dy()

	root := container.NewWithoutLayout() // NOT a stack: StackLayout would resize every child to full canvas
	blocker := &oskBlocker{Rectangle: canvas.NewRectangle(color.NRGBA{A: 1})}
	blocker.Resize(size) // invisible sheet: swallows all taps so nothing below the keyboard is clickable
	root.Add(blocker)

	bg := canvas.NewRectangle(oskBgColor)
	bg.CornerRadius = keyH * 0.3
	bg.Resize(fyne.NewSize(kb.Dx(), kb.Dy()))
	bg.Move(kb.Min)
	root.Add(bg)

	for _, h := range hits {
		k := h.key
		fill := oskKeyFill
		if k.cmd == oskCmdShift && shift {
			fill = oskKeyHi
		}
		keyRect := canvas.NewRectangle(fill)
		keyRect.CornerRadius = keyH * 0.2
		keyRect.Resize(fyne.NewSize(h.rect.Dx(), h.rect.Dy()))
		keyRect.Move(h.rect.Min)
		root.Add(keyRect)

		label := k.label
		if r := oskTyped(k, shift); r != 0 && unicode.IsLetter(r) {
			label = string(r)
		}
		oskLabel(root, h.rect, label, keyH*0.42)
	}

	if text != "" {
		oskPreviewBubble(root, size, kb, keyH, text)
	}

	r := keyH * 0.32
	left, right := oskCursor(r, oskCursorLeftFill, oskCursorLeftEdge), oskCursor(r, oskCursorRightFill, oskCursorRightEdge)
	root.Add(left)
	root.Add(right)
	return &oskVisual{content: root, hits: hits, kb: kb, left: left, right: right, cursorR: r}
}

func oskCursor(r float32, fill, edge color.Color) *canvas.Circle {
	c := canvas.NewCircle(fill)
	c.StrokeColor = edge
	c.StrokeWidth = r * 0.1
	c.Resize(fyne.NewSize(2*r, 2*r))
	return c
}

func oskLabel(root *fyne.Container, rect oskRect, txt string, size float32) {
	t := canvas.NewText(txt, oskTextColor)
	t.TextSize = size
	ms := t.MinSize()
	t.Move(fyne.NewPos(rect.Min.X+(rect.Dx()-ms.Width)/2, rect.Min.Y+(rect.Dy()-ms.Height)/2))
	root.Add(t)
}

// oskPreviewBubble draws the live entry text above the keyboard as REAL lines (WYSIWYG):
// long lines keep their tail, too many lines drop from the top, pill grows to fit.
func oskPreviewBubble(root *fyne.Container, size fyne.Size, kb oskRect, keyH float32, text string) {
	availW := kb.Dx() - keyH*0.5 // room inside the pill padding

	var texts []*canvas.Text
	totalH, widest, gapV := float32(0), float32(0), keyH*0.12
	for _, line := range strings.Split(text, "\n") {
		t := canvas.NewText(line, oskTextColor)
		t.TextSize = keyH * 0.5
		runes := []rune(line) // shrink from the front, keep the tail: that is where typing happens
		for ms := t.MinSize(); ms.Width > availW && len(runes) > 1; ms = t.MinSize() {
			runes = runes[1:]
			t.Text = "…" + string(runes)
		}
		ms := t.MinSize()
		texts = append(texts, t)
		totalH += ms.Height
		widest = max32(widest, ms.Width)
	}
	totalH += gapV * float32(len(texts)-1)

	availH := kb.Min.Y - size.Height*0.04 // space above the keyboard
	for len(texts) > 1 && totalH > availH {
		totalH -= texts[0].MinSize().Height + gapV
		texts = texts[1:] // newest lines matter most
	}

	y := kb.Min.Y - keyH*0.35 - totalH
	if y < size.Height*0.02 {
		return // truly no room: skip the bubble rather than draw garbage
	}
	x := clampF32(kb.Min.X+(kb.Dx()-widest)/2, size.Width*0.02, size.Width-widest-size.Width*0.02)

	pad := keyH * 0.25 // rounded rect, NOT a pill: full radius turns into a moon on big blocks
	bg := canvas.NewRectangle(helpPillBg)
	bg.CornerRadius = keyH * 0.3
	bg.Resize(fyne.NewSize(widest+pad*2, totalH+pad*2))
	bg.Move(fyne.NewPos(x-pad, y-pad))
	root.Add(bg)

	ly := y // left-aligned like the entry itself: what you see is what you typed
	for _, t := range texts {
		t.Move(fyne.NewPos(x, ly))
		root.Add(t)
		ly += t.MinSize().Height + gapV
	}
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// moveCursors places both ghost cursor circles centered on their logical positions.
func (v *oskVisual) moveCursors(left, right fyne.Position) {
	o := fyne.NewPos(v.cursorR, v.cursorR)
	v.left.Move(left.Subtract(o))
	v.right.Move(right.Subtract(o))
}

// oskState lives inside gamepad; zero value = hidden.
type oskState struct {
	visible bool
	win     *window
	target  *widget.Entry // the field we type into: canvas.Focused() is shadowed while our overlay is up!

	visual      *oskVisual
	left, right fyne.Position
	symbols     bool
	shift       bool

	text         string // preview/layer state the current visual was built with
	size         fyne.Size
	lang         string
	builtSymbols bool
	builtShift   bool

	sidePrev map[gamepadButtonRef]bool          // edges for triggers/close/toggle
	heldDur  map[gamepadButtonRef]time.Duration // per held typing trigger
	fires    map[gamepadButtonRef]int           // repeat fires already delivered
}

// oskEntry returns the focused text field worth typing into, or nil.
func oskEntry(w *window) *widget.Entry {
	if ent, ok := w.canvas.Focused().(*widget.Entry); ok && !ent.Disabled() {
		return ent
	}
	return nil
}

func oskPreviewText(ent *widget.Entry) string {
	if ent == nil {
		return ""
	}
	t := strings.ReplaceAll(ent.Text, "\r", "") // keep \n: the pill renders real lines
	if ent.Password {
		masked := make([]rune, 0, len(t))
		for _, r := range t {
			if r == '\n' {
				masked = append(masked, r) // structure stays visible, content does not
			} else {
				masked = append(masked, '•')
			}
		}
		t = string(masked)
	}
	if r := []rune(t); len(r) > 400 {
		return "…" + string(r[len(r)-400:]) // hard cap; width fitting happens per line later
	}
	return t
}

func gamepadOSKOpen(w *window) {
	o := &gamepad.osk
	if o.visible {
		return
	}
	o.visible, o.win, o.target = true, w, oskEntry(w) // capture the field to type into; the modal blocker keeps it that way
	o.symbols, o.shift = false, false
	o.visual, o.text, o.size, o.lang = nil, "", fyne.Size{}, ""
	o.left, o.right = fyne.Position{}, fyne.Position{}
	o.heldDur, o.fires = nil, nil // sidePrev survives: the toggle press that opened us must not instantly close us

	gamepadOSKSync(w)
	if v := o.visual; v != nil {
		kb := v.kb
		o.left = fyne.NewPos(kb.Min.X+kb.Dx()*0.25, kb.Min.Y+kb.Dy()*0.5)
		o.right = fyne.NewPos(kb.Min.X+kb.Dx()*0.75, kb.Min.Y+kb.Dy()*0.5)
		v.moveCursors(o.left, o.right)
	}
	oskHideCursor(w) // the frozen arrow must not sit on top of our keys
}

// oskHideCursor hides the hardware cursor while the keyboard is up; re-asserted every tick
// because hover changes elsewhere can hand it back to GLFW.
func oskHideCursor(w *window) {
	if w.viewport != nil {
		w.viewport.SetInputMode(CursorMode, CursorHidden)
	}
}

func gamepadOSKClose() {
	o := &gamepad.osk
	if !o.visible {
		return
	}
	if o.win != nil {
		if o.visual != nil {
			o.win.canvas.Overlays().Remove(o.visual.content)
		}
		if o.win.viewport != nil {
			o.win.viewport.SetInputMode(CursorMode, CursorNormal) // arrow back on stage
		}
	}
	o.visible, o.win, o.target = false, nil, nil
	o.visual, o.heldDur, o.fires = nil, nil, nil
	gamepad.stickWasOut = false // re-arm touch-to-activate after the stick freeze
}

// gamepadOSKSync rebuilds the keyboard whenever anything visible about it changed.
func gamepadOSKSync(w *window) {
	o := &gamepad.osk
	size, lang := w.canvas.Size(), desktop.GamepadOSKLanguage()
	text := oskPreviewText(o.target) // Focused() is shadowed by our overlay: read the tracked entry
	if o.visual != nil && size == o.size && lang == o.lang && text == o.text &&
		o.symbols == o.builtSymbols && o.shift == o.builtShift {
		return
	}

	v := buildGamepadOSK(size, lang, o.symbols, o.shift, text)
	if o.visual != nil {
		o.win.canvas.Overlays().Remove(o.visual.content)
	}
	w.canvas.Overlays().Add(v.content)
	o.visual = v
	o.size, o.lang, o.text, o.builtSymbols, o.builtShift = size, lang, text, o.symbols, o.shift
	v.moveCursors(o.left, o.right)
}

func (o *oskState) visualContent() fyne.CanvasObject {
	if o.visual == nil {
		return nil
	}
	return o.visual.content
}

// gamepadOSKPresent reports whether our overlay is still on the canvas stack.
// OverlayStack.Remove evicts everything ABOVE the removed object: a dialog closing
// below us silently takes the keyboard with it (Enter in a file-save name field...).
func gamepadOSKPresent(c fyne.Canvas, content fyne.CanvasObject) bool {
	if content == nil {
		return false
	}
	for _, ov := range c.Overlays().List() {
		if ov == content {
			return true
		}
	}
	return false
}

// gamepadOSKTick runs INSTEAD of the normal cursor/scroll/fire path while visible:
// sticks drive ghost cursors, triggers type, B or the toggle button closes.
func gamepadOSKTick(w *window, pads []glfw.Joystick, dt time.Duration) {
	o := &gamepad.osk
	if !gamepadOSKPresent(w.canvas, o.visualContent()) { // evicted below by a closing dialog? give control back NOW
		gamepadOSKClose()
		return
	}
	gamepadOSKSync(w) // the modal blocker keeps our target stable; nothing to re-check here
	oskHideCursor(w)  // keep the arrow hidden even if hover logic handed it back

	leftDone, rightDone := false, false
	for _, p := range pads {
		axes := p.GetAxes()
		if len(axes) < 5 {
			continue
		}
		if !leftDone && (abs32(axes[0]) > gamepadDeadzone || abs32(axes[1]) > gamepadDeadzone) {
			o.left = oskMove(o.left, axes[0], axes[1], o.visual.kb, dt)
			leftDone = true
		}
		if !rightDone && (abs32(axes[3]) > gamepadDeadzone || abs32(axes[4]) > gamepadDeadzone) {
			o.right = oskMove(o.right, axes[3], axes[4], o.visual.kb, dt)
			rightDone = true
		}
	}

	for _, s := range []struct {
		btn desktop.GamepadButton
		pos fyne.Position
	}{{desktop.GamepadLeftTrigger, o.left}, {desktop.GamepadRightTrigger, o.right}} {
		for _, p := range pads {
			ref := gamepadButtonRef{joy: p, btn: s.btn}
			held := gamepadButtonPressed(p, s.btn)
			wasHeld := o.sidePrev[ref]
			if o.sidePrev == nil {
				o.sidePrev = map[gamepadButtonRef]bool{}
			}
			o.sidePrev[ref] = held
			if !held {
				delete(o.heldDur, ref)
				continue
			}
			if !wasHeld { // press edge types once
				gamepadOSKType(s.pos)
				if o.heldDur == nil {
					o.heldDur = map[gamepadButtonRef]time.Duration{}
				}
				if o.fires == nil {
					o.fires = map[gamepadButtonRef]int{}
				}
				o.heldDur[ref], o.fires[ref] = 0, 0
				continue
			}
			d := o.heldDur[ref] + dt
			o.heldDur[ref] = d
			if want := oskRepeatCount(d); want > o.fires[ref] { // held: repeat after delay
				gamepadOSKType(s.pos)
				o.fires[ref] = want
			}
		}
	}

	for _, p := range pads { // toggle button closes too (Guide by default; B stays free for the app)
		b := desktop.GamepadOSKToggle()
		ref := gamepadButtonRef{joy: p, btn: b}
		held := gamepadButtonPressed(p, b)
		rising := held && !o.sidePrev[ref]
		if o.sidePrev == nil {
			o.sidePrev = map[gamepadButtonRef]bool{}
		}
		o.sidePrev[ref] = held
		if rising {
			if gamepadDebug {
				fmt.Fprintf(os.Stderr, "[osk] close: toggle pressed on pad %v\n", p)
			}
			gamepadOSKClose()
			return
		}
	}

	o.visual.moveCursors(o.left, o.right)
}

// gamepadOSKType types (or acts on) the key under a ghost cursor. It injects into the
// tracked target entry directly: canvas.Focused() is shadowed by our overlay's focus manager.
func gamepadOSKType(pos fyne.Position) {
	o := &gamepad.osk
	if o.visual == nil {
		return
	}
	k := oskKeyAt(o.visual.hits, pos)
	if k == nil {
		return // between keys: triggers click on nothing, like real thumbs slipping
	}
	switch k.cmd {
	case oskCmdShift:
		o.shift = !o.shift
		return
	case oskCmdSymbols:
		o.symbols, o.shift = true, false
		return
	case oskCmdLetters:
		o.symbols, o.shift = false, false
		return
	case oskCmdPaste:
		if o.target != nil {
			o.target.TypedShortcut(&fyne.ShortcutPaste{Clipboard: fyne.CurrentApp().Clipboard()}) // same path the entry's context menu takes
		}
		return
	}

	if o.target == nil {
		return // hotkey-opened over no text field: layer keys still work, typing goes nowhere
	}
	if r := oskTyped(*k, o.shift); r != 0 {
		o.target.TypedRune(r) // umlauts need no dead-key dance
		if o.shift && unicode.IsLetter(k.r) {
			o.shift = false // sticky shift is one-shot
		}
		return
	}
	if k.key != "" {
		o.target.TypedKey(&fyne.KeyEvent{Name: k.key})
	}
}

// gamepadOSKToggleCheck opens the keyboard via its toggle button while hidden.
func gamepadOSKToggleCheck(w *window, pads []glfw.Joystick) {
	o := &gamepad.osk
	btn := desktop.GamepadOSKToggle()
	for _, p := range pads {
		ref := gamepadButtonRef{joy: p, btn: btn}
		held := gamepadButtonPressed(p, btn)
		rising := held && !o.sidePrev[ref]
		if o.sidePrev == nil {
			o.sidePrev = map[gamepadButtonRef]bool{}
		}
		o.sidePrev[ref] = held
		if rising {
			gamepadOSKOpen(w)
			return
		}
	}
}

// gamepadOSKPhysicalTarget returns the entry to deliver PHYSICAL keyboard events to while
// our overlay shadows canvas focus, or nil when no on-screen keyboard is up for this window.
func gamepadOSKPhysicalTarget(w *window) fyne.Focusable {
	if o := &gamepad.osk; o.visible && o.win == w && o.target != nil {
		return o.target
	}
	return nil
}
