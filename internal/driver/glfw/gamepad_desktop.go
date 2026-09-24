//go:build !wasm && !test_web_driver

package glfw

import (
	"fmt"
	"image/color"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/internal/driver/common"
	"fyne.io/fyne/v2/internal/scale"

	"github.com/go-gl/glfw/v3.4/glfw"
)

// gamepadDebug enables stderr tracing with FYNE_GAMEPAD_DEBUG=1 (main-thread only).
var gamepadDebug = os.Getenv("FYNE_GAMEPAD_DEBUG") != ""

const (
	gamepadDeadzone    = 0.16 // raw axis slop below this is ignored (real pads drift!)
	gamepadCursorSpeed = 1200 // pixels per second at full stick tilt
	gamepadScrollRate  = 30   // wheel notches per second at full right-stick tilt (~750 px/s)
	gamepadMaxDelta    = 0.05 // clamp dt so a stall does not teleport the cursor
	gamepadMaxDeltaDur = time.Duration(gamepadMaxDelta * float64(time.Second))
	gamepadTakeoverPx  = 1.5 // real mouse moved this far past our warp -> it took over
)

// gamepadRawIndices translates public button ids to raw joystick button indices.
// Numbers are evidence from a real "Microsoft X-Box 360 pad" on linux xpad (see PLAN.md).
var gamepadRawIndices = map[desktop.GamepadButton]int{
	desktop.GamepadA:             0,
	desktop.GamepadB:             1,
	desktop.GamepadX:             2,
	desktop.GamepadY:             3,
	desktop.GamepadLeftShoulder:  4,
	desktop.GamepadRightShoulder: 5,
	desktop.GamepadBack:          6,
	desktop.GamepadStart:         7,
	desktop.GamepadLeftStick:     9,
	desktop.GamepadRightStick:    10,
	desktop.GamepadDPadUp:        11,
	desktop.GamepadDPadRight:     12,
	desktop.GamepadDPadDown:      13,
	desktop.GamepadDPadLeft:      14,
	desktop.GamepadGuide:         8, // center Xbox button; pads without it never report index 8
}

// gamepadTriggerAxes translates triggers to raw axis indices; they are analog axes at rest -1.
var gamepadTriggerAxes = map[desktop.GamepadButton]int{
	desktop.GamepadLeftTrigger:  2,
	desktop.GamepadRightTrigger: 5,
}

const gamepadTriggerThreshold = 0.5 // half travel counts as a press (analog triggers stay coarse for now)

// gamepadButtonRef keys edge detection per physical device:
// weird controllers enumerate as several joysticks, each must remember its own button state.
type gamepadButtonRef struct {
	joy glfw.Joystick
	btn desktop.GamepadButton
}

type gamepadState struct {
	ownsCursor  bool                      // stick owns cursor until real mouse moves again
	stickWasOut bool                      // stick outside deadzone last tick (touch-to-activate edge)
	prevPressed map[gamepadButtonRef]bool // rising edge memory per (joystick, button)
	lastWarpX   float64
	lastWarpY   float64
	helpHeld    time.Duration     // accumulated hold of the help combo
	helpOverlay fyne.CanvasObject // non-nil while keymap overlay is visible
	helpWin     *window           // window carrying the overlay
	osk         oskState          // on-screen keyboard (see gamepad_osk_desktop.go)
}

var gamepad gamepadState

var lastGamepadPoll time.Time

// gamepadRamp applies deadzone then quadratic ramp to a raw axis, returning -1..1.
// Slow near center for fine control, fast at full tilt.
func gamepadRamp(v float32) float64 {
	abs := float64(v)
	if abs < 0 {
		abs = -abs
	}
	if abs <= gamepadDeadzone {
		return 0
	}

	past := (abs - gamepadDeadzone) / (1.0 - gamepadDeadzone) // 0..1 beyond deadzone
	ramp := past * past                                       // quadratic
	if v < 0 {
		return -ramp
	}
	return ramp
}

// gamepadAxisDelta converts one raw axis value into pixels to move for this tick.
func gamepadAxisDelta(v float32, dt float64) float64 {
	return gamepadRamp(v) * float64(gamepadCursorSpeed) * dt
}

func presentJoysticks() []glfw.Joystick {
	var pads []glfw.Joystick
	for i := glfw.Joystick1; i <= glfw.Joystick15; i++ {
		if i.Present() {
			pads = append(pads, i)
		}
	}
	return pads
}

// gamepadSnapshot builds the raw public state for every present pad. Fresh maps each
// poll: published snapshots are never mutated, so readers need no locks of their own.
func gamepadSnapshot(pads []glfw.Joystick) []desktop.GamepadState {
	var out []desktop.GamepadState
	for _, p := range pads {
		st := desktop.GamepadState{Name: p.GetName(), Buttons: map[desktop.GamepadButton]bool{}}
		buttons := p.GetButtons()
		for btn, raw := range gamepadRawIndices {
			if raw < len(buttons) && buttons[raw] != glfw.Release {
				st.Buttons[btn] = true
			}
		}
		axes := p.GetAxes()
		if len(axes) >= 2 {
			st.LeftX, st.LeftY = axes[0], axes[1]
		}
		if len(axes) >= 5 {
			st.RightX, st.RightY = axes[3], axes[4]
		}
		st.LeftTrigger = gamepadTriggerTravel(axes, gamepadTriggerAxes[desktop.GamepadLeftTrigger])
		st.RightTrigger = gamepadTriggerTravel(axes, gamepadTriggerAxes[desktop.GamepadRightTrigger])
		if st.LeftTrigger > gamepadTriggerThreshold {
			st.Buttons[desktop.GamepadLeftTrigger] = true
		}
		if st.RightTrigger > gamepadTriggerThreshold {
			st.Buttons[desktop.GamepadRightTrigger] = true
		}
		out = append(out, st)
	}
	return out
}

// gamepadTriggerTravel normalizes a trigger axis (raw -1 at rest, +1 pressed) to 0..1.
func gamepadTriggerTravel(axes []float32, idx int) float32 {
	if idx >= len(axes) {
		return 0
	}
	v := clampF32((axes[idx]+1)/2, 0, 1)
	return v
}

// pollGamepad runs on the main thread from pollEvents, 60 times a second.
// Every present joystick is polled; input from all of them is accepted (some
// controllers enumerate as multiple devices). First pad pushing its left stick
// out of the deadzone drives the cursor for that tick.
func (d *gLDriver) pollGamepad() {
	now := time.Now()
	dtDur := now.Sub(lastGamepadPoll)
	lastGamepadPoll = now
	if dtDur <= 0 || dtDur > gamepadMaxDeltaDur {
		dtDur = gamepadMaxDeltaDur
	}

	pads := presentJoysticks()
	desktop.SetGamepadStates(gamepadSnapshot(pads)) // raw data always fresh, emulation or not
	if !desktop.GamepadEmulationEnabled() {
		gamepadHelpRelease() // nothing may linger on screen behind a game
		gamepadOSKClose()
		gamepadReleaseHeld(focusedWindow(d))
		return
	}

	w := focusedWindow(d)
	if len(pads) == 0 || w == nil || w.viewport == nil {
		gamepadHelpRelease()  // combo cannot be held without pad+focus: overlay must not linger
		gamepadOSKClose()     // keyboard with no pad or no window is a stranded keyboard
		gamepadReleaseHeld(w) // nobody left to release the buttons but us
		return
	}
	dt := dtDur.Seconds()

	if gamepad.osk.visible && gamepad.osk.win != w {
		gamepadOSKClose() // focus moved to another window: keyboard belongs to the old one
	}
	if gamepad.osk.visible {
		gamepadReleaseHeld(w)          // a button held when the keyboard opened must not stay pressed on the app below
		gamepadOSKTick(w, pads, dtDur) // sticks and triggers belong to the keyboard now
		gamepadHelpTick(w, pads, dtDur)
		return
	}

	var activeAxes []float32 // first pad pushing left stick out of deadzone owns the cursor this tick
	for _, p := range pads {
		if a := p.GetAxes(); len(a) >= 2 && (abs32(a[0]) > gamepadDeadzone || abs32(a[1]) > gamepadDeadzone) {
			activeAxes = a
			break
		}
	}

	x, y := w.viewport.GetCursorPos()
	gamepadUpdateOwnership(x, y, activeAxes) // nil axes = nobody pushing the stick
	if gamepad.ownsCursor && activeAxes != nil {
		gamepadWarpCursor(w, x, y, activeAxes[0], activeAxes[1], dt)
	}

	for _, p := range pads {
		axes := p.GetAxes()
		buttons := p.GetButtons()
		gamepadScroll(w, axes, dt) // no-op when right stick centered or pad lacks axes
		gamepadFire(w, p, buttons, axes)
	}

	gamepadHelpTick(w, pads, dtDur)
	gamepadOSKToggleCheck(w, pads) // the one and only way to open the keyboard: its toggle button
}

// gamepadUpdateOwnership implements last-mover-wins between stick and real mouse:
// pushing the stick out of its deadzone takes ownership; a real mouse move past our
// warp target hands it back. Touch-to-activate so a resting stick never fights the mouse.
func gamepadUpdateOwnership(x, y float64, axes []float32) {
	stickOut := len(axes) >= 2 && (abs32(axes[0]) > gamepadDeadzone || abs32(axes[1]) > gamepadDeadzone)

	if stickOut && !gamepad.stickWasOut {
		gamepad.ownsCursor = true // stick just got touched, grab the cursor where it sits
		gamepad.lastWarpX, gamepad.lastWarpY = x, y
	}
	gamepad.stickWasOut = stickOut

	if gamepad.ownsCursor && movedPast(gamepad.lastWarpX, gamepad.lastWarpY, x, y) {
		gamepad.ownsCursor = false // someone else (real mouse) moved the cursor
	}
}

func movedPast(fromX, fromY, toX, toY float64) bool {
	dx := toX - fromX
	dy := toY - fromY
	return dx > gamepadTakeoverPx || dx < -gamepadTakeoverPx || dy > gamepadTakeoverPx || dy < -gamepadTakeoverPx
}

func gamepadWarpCursor(w *window, curX, curY float64, axisX, axisY float32, dt float64) {
	dx := gamepadAxisDelta(axisX, dt)
	dy := gamepadAxisDelta(axisY, dt)
	if dx == 0 && dy == 0 {
		return
	}
	gamepadMoveCursorTo(w, curX+dx, curY+dy)
}

// gamepadScroll turns right stick tilt into continuous wheel notches at the cursor.
func gamepadScroll(w *window, axes []float32, dt float64) {
	if len(axes) < 5 {
		return
	}

	xoff := gamepadRamp(axes[3]) * gamepadScrollRate * dt
	yoff := -gamepadRamp(axes[4]) * gamepadScrollRate * dt // stick up = wheel up
	if xoff == 0 && yoff == 0 {
		return
	}
	w.processMouseScrolled(xoff, yoff)
}

// gamepadMoveCursorTo warps the cursor, feeds fyne the position (X11 warp silence fix)
// and takes cursor ownership for the controller.
func gamepadMoveCursorTo(w *window, x, y float64) {
	width, height := w.viewport.GetSize()
	x = clampFloat(x, 0, float64(width-1))
	y = clampFloat(y, 0, float64(height-1))
	w.viewport.SetCursorPos(x, y)
	w.mouseMoved(nil, x, y) // X11 does not reliably deliver a motion event for warps
	gamepad.ownsCursor = true
	gamepad.lastWarpX, gamepad.lastWarpY = x, y
}

// gamepadSnapToCenter parks the cursor mid-window (lost-cursor rescue).
func gamepadSnapToCenter(w *window) {
	width, height := w.viewport.GetSize()
	gamepadMoveCursorTo(w, float64(width)/2, float64(height)/2)
}

// gamepadFollowFocus warps the cursor onto whatever just got keyboard focus,
// so Tab navigation and fake-mouse clicking stay one consistent story.
func gamepadFollowFocus(w *window) {
	focusedObj, ok := w.canvas.Focused().(fyne.CanvasObject)
	if !ok || focusedObj == nil {
		return
	}

	var center fyne.Position
	found := false
	w.canvas.WalkTrees(func(node *common.RenderCacheNode, pos fyne.Position) {
		if found || node.Obj() != focusedObj {
			return
		}
		found = true
		size := node.Obj().Size()
		center = fyne.NewPos(pos.X+size.Width/2, pos.Y+size.Height/2)
	}, nil)
	if !found {
		return
	}

	x := float64(scale.ToScreenCoordinate(w.canvas, center.X))
	y := float64(scale.ToScreenCoordinate(w.canvas, center.Y))
	gamepadMoveCursorTo(w, x, y)
}

// gamepadFire drives the effective bindings: mouse-button kinds are held like real
// buttons (press on rising edge, release on falling) so Draggable widgets can be
// dragged with stick+button; key/func/snap kinds fire once per press.
func gamepadFire(w *window, joy glfw.Joystick, buttons []glfw.Action, axes []float32) {
	for button, raw := range gamepadRawIndices {
		gamepadEdgeFire(w, joy, button, raw < len(buttons) && buttons[raw] != glfw.Release)
	}
	for button, axis := range gamepadTriggerAxes {
		gamepadEdgeFire(w, joy, button, axis < len(axes) && float64(axes[axis]) > gamepadTriggerThreshold)
	}
}

func gamepadEdgeFire(w *window, joy glfw.Joystick, button desktop.GamepadButton, pressed bool) {
	ref := gamepadButtonRef{joy: joy, btn: button}
	wasPressed := gamepad.prevPressed[ref] // nil map read is fine in go
	if gamepad.prevPressed == nil {
		gamepad.prevPressed = map[gamepadButtonRef]bool{}
	}
	gamepad.prevPressed[ref] = pressed
	switch {
	case pressed && !wasPressed:
		gamepadExecute(w, desktop.GamepadBindingFor(button), true) // rising edge: button goes down
	case wasPressed && !pressed:
		gamepadExecute(w, desktop.GamepadBindingFor(button), false) // falling edge: lets go - drags end here
	}
}

// gamepadReleaseHeld drops fake mouse buttons that are still down when their context
// vanishes (keyboard opens, emulation off, pad or window gone). Without this fyne
// would keep an invisible button pressed and drag the UI forever.
func gamepadReleaseHeld(w *window) {
	if w == nil || len(gamepad.prevPressed) == 0 {
		return
	}
	for ref, held := range gamepad.prevPressed {
		if !held {
			continue
		}
		gamepadExecute(w, desktop.GamepadBindingFor(ref.btn), false)
		gamepad.prevPressed[ref] = false
	}
}

// down=true injects a button/key press, down=false the matching release.
// Non-mouse kinds (key, snap center, func) act on the press only.
func gamepadExecute(w *window, binding desktop.GamepadBinding, down bool) {
	if gamepadDebug {
		fmt.Fprintf(os.Stderr, "[gamepad] fire kind=%d key=%q mods=%v follow=%v\n",
			binding.Kind, binding.Key, binding.Modifier, binding.FollowFocus)
	}
	switch binding.Kind {
	case desktop.GamepadActionLeftClick:
		w.mouseClicked(nil, glfw.MouseButton1, gamepadMouseAction(down), 0)
		return
	case desktop.GamepadActionRightClick:
		w.mouseClicked(nil, glfw.MouseButton2, gamepadMouseAction(down), 0)
		return
	case desktop.GamepadActionMiddleClick:
		w.mouseClicked(nil, glfw.MouseButton3, gamepadMouseAction(down), 0)
		return
	}
	if down { // key, snap center and func are one-shot events on the press only
		gamepadFireOneShot(w, binding)
	}
}

func gamepadFireOneShot(w *window, binding desktop.GamepadBinding) {
	switch binding.Kind {
	case desktop.GamepadActionKey:
		if binding.Key == "" {
			return
		}
		// Feed processKeyPressed directly - it speaks fyne names, no glfw key conversion needed.
		// (And we MUST skip that conversion: glfw.GetKeyName with scancode 0 returns garbage,
		// e.g. KeyZ resolved to "Y" on a real test machine, silently breaking Ctrl+Z.)
		ascii := fyne.KeyUnknown
		if len(binding.Key) == 1 {
			ascii = binding.Key
		}
		if gamepadDebug {
			fmt.Fprintf(os.Stderr, "[gamepad] inject key=%q ascii=%q mods=%v\n", binding.Key, ascii, binding.Modifier)
		}
		w.processKeyPressed(binding.Key, ascii, 0, press, binding.Modifier)
		w.processKeyPressed(binding.Key, ascii, 0, release, binding.Modifier)
		if binding.FollowFocus {
			gamepadFollowFocus(w) // injected key moved focus synchronously already
		}
	case desktop.GamepadActionSnapCenter:
		gamepadSnapToCenter(w)
	case desktop.GamepadActionFunc:
		if binding.Func != nil {
			binding.Func(w) // main thread, right next to regular UI events
		}
	}
}

func gamepadMouseAction(down bool) glfw.Action {
	if down {
		return glfw.Press
	}
	return glfw.Release
}

// gamepadHelpPressed reports whether every button of the configured help combo
// is currently held on any pad (a button may come from different pads).
func gamepadHelpPressed(pads []glfw.Joystick) bool {
	buttons, _ := desktop.GamepadHelpCombo()
	if len(buttons) == 0 {
		return false // empty combo disables the overlay entirely
	}
	for _, b := range buttons {
		held := false
		for _, p := range pads {
			if gamepadButtonPressed(p, b) {
				held = true
				break
			}
		}
		if !held {
			return false
		}
	}
	return true
}

func gamepadButtonPressed(joy glfw.Joystick, button desktop.GamepadButton) bool {
	if raw, ok := gamepadRawIndices[button]; ok {
		buttons := joy.GetButtons()
		return raw < len(buttons) && buttons[raw] != glfw.Release
	}
	if axis, ok := gamepadTriggerAxes[button]; ok {
		axes := joy.GetAxes()
		return axis < len(axes) && float64(axes[axis]) > gamepadTriggerThreshold
	}
	return false
}

// gamepadHelpTimer advances the combo hold episode: held long enough -> visible,
// released -> hidden again. Overlay lives exactly as long as the fingers do.
// Pure so tests can hammer it.
func gamepadHelpTimer(pressed bool, held time.Duration, dt, hold time.Duration) (newHeld time.Duration, visible bool) {
	if !pressed {
		return 0, false
	}
	held += dt
	return held, held >= hold
}

func gamepadHelpTick(w *window, pads []glfw.Joystick, dt time.Duration) {
	_, hold := desktop.GamepadHelpCombo()
	held, visible := gamepadHelpTimer(gamepadHelpPressed(pads), gamepad.helpHeld, dt, hold)
	gamepad.helpHeld = held
	switch {
	case visible && gamepad.helpOverlay == nil:
		gamepadShowHelp(w)
	case !visible && gamepad.helpOverlay != nil:
		gamepadHideHelp()
	}
}

// gamepadHelpRelease ends the current hold episode (no pad or no focused window:
// combo cannot possibly be held, so an overlay that is up must come down).
func gamepadHelpRelease() {
	gamepad.helpHeld = 0
	gamepadHideHelp()
}

// gamepadShowHelp draws the controller diagram with live bindings as an overlay.
// It is not interactive: clicks pass through to the app below, and it stays up only
// while the help combo remains held (gamepadHelpTick hides on release).
func gamepadShowHelp(w *window) {
	content := container.NewStack(
		canvas.NewRectangle(color.NRGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xE6}),
		buildGamepadHelp(w.canvas.Size(), desktop.GamepadBindingFor),
	)
	w.canvas.Overlays().Add(content)
	gamepad.helpOverlay = content
	gamepad.helpWin = w
}

func gamepadHideHelp() {
	if gamepad.helpOverlay == nil {
		return
	}
	if gamepad.helpWin != nil {
		gamepad.helpWin.canvas.Overlays().Remove(gamepad.helpOverlay)
	}
	gamepad.helpOverlay = nil
	gamepad.helpWin = nil
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func focusedWindow(d *gLDriver) *window {
	for _, win := range d.windowList() {
		w, ok := win.(*window)
		if !ok || w.viewport == nil || w.closing || !w.visible {
			continue
		}
		if w.viewport.GetAttrib(glfw.Focused) != 0 {
			return w
		}
	}
	return nil
}

func clampFloat(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
