package desktop

import (
	"image"
	"sync"
	"time"

	"fyne.io/fyne/v2"
)

// GamepadButton identifies a physical control on an Xbox 360 style gamepad.
// Triggers are treated as digital buttons that fire past their analog travel;
// the Guide/Xbox button is not included, it belongs to the operating system.
//
// Since: 2.9
type GamepadButton int

// Gamepad button identifiers using the Xbox 360 layout as lowest common denominator.
const (
	GamepadA GamepadButton = iota
	GamepadB
	GamepadX
	GamepadY
	GamepadLeftShoulder
	GamepadRightShoulder
	GamepadBack
	GamepadStart
	GamepadLeftStick
	GamepadRightStick
	GamepadDPadUp
	GamepadDPadDown
	GamepadDPadLeft
	GamepadDPadRight
	GamepadLeftTrigger
	GamepadRightTrigger
	// GamepadGuide is the center Xbox/Guide button. It has no default binding and
	// doubles as the on-screen keyboard toggle; pads without it never fire it.
	GamepadGuide
)

// GamepadActionKind selects what a gamepad binding does.
//
// Since: 2.9
type GamepadActionKind int

// Gamepad action kinds.
const (
	// GamepadActionNone disables the button. It is also the zero value, so unlisted buttons do nothing.
	GamepadActionNone GamepadActionKind = iota
	GamepadActionLeftClick
	GamepadActionRightClick
	GamepadActionMiddleClick
	// GamepadActionKey synthesizes a key press and release of Binding.Key with Binding.Modifier.
	GamepadActionKey
	// GamepadActionSnapCenter warps the mouse cursor to the center of the focused window.
	GamepadActionSnapCenter
	// GamepadActionFunc calls Binding.Func on the main thread, next to regular UI events.
	GamepadActionFunc
)

// GamepadBinding describes what one physical gamepad button does.
// The default bindings emulate a mouse and keyboard: see Fyne gamepad documentation.
//
// Since: 2.9
type GamepadBinding struct {
	Kind     GamepadActionKind
	Key      fyne.KeyName      // used by GamepadActionKey
	Modifier fyne.KeyModifier  // optional modifier for GamepadActionKey
	Func     func(fyne.Window) // used by GamepadActionFunc; called on the main thread

	// FollowFocus warps the mouse cursor onto whatever widget has keyboard focus
	// after this binding's key was delivered. Combine with KeyTab for gamepad navigation.
	FollowFocus bool
}

var (
	gamepadBindingsLock sync.RWMutex
	gamepadOverrides    = map[GamepadButton]GamepadBinding{}

	gamepadCursorLock sync.RWMutex
	gamepadCursor     Cursor // replaces the default arrow while set; hover cursors still win
)

// gamepadImageCursor adapts a raw image to the Cursor interface.
type gamepadImageCursor struct {
	img        image.Image
	hotX, hotY int
}

func (c *gamepadImageCursor) Image() (image.Image, int, int) { return c.img, c.hotX, c.hotY }

// SetGamepadCursor replaces the default mouse cursor image (the arrow) with a custom one,
// letting apps theme the pointer that gamepad navigation warps around.
// It applies to the window's hardware cursor itself, so a real mouse shows it too -
// one cursor for all devices. Cursors of hovered objects (text I-beam and friends) still win.
// Takes effect as the pointer moves on. Pass nil or call ResetGamepadCursor to restore the arrow.
//
// Since: 2.9
func SetGamepadCursor(img image.Image, hotspotX, hotspotY int) {
	gamepadCursorLock.Lock()
	defer gamepadCursorLock.Unlock()
	if img == nil {
		gamepadCursor = nil
		return
	}
	gamepadCursor = &gamepadImageCursor{img: img, hotX: hotspotX, hotY: hotspotY}
}

// ResetGamepadCursor restores the standard default cursor.
//
// Since: 2.9
func ResetGamepadCursor() {
	gamepadCursorLock.Lock()
	defer gamepadCursorLock.Unlock()
	gamepadCursor = nil
}

// GamepadCursor returns the custom default cursor, or nil when the standard arrow is in use.
//
// Since: 2.9
func GamepadCursor() Cursor {
	gamepadCursorLock.RLock()
	defer gamepadCursorLock.RUnlock()
	return gamepadCursor
}

// default bindings: controller becomes fake mouse + keyboard, see PLAN discussion in fyne-controller-support.
var gamepadDefaultBindings = map[GamepadButton]GamepadBinding{
	GamepadA:             {Kind: GamepadActionLeftClick},
	GamepadB:             {Kind: GamepadActionRightClick},
	GamepadX:             {Kind: GamepadActionKey, Key: fyne.KeyReturn},
	GamepadY:             {Kind: GamepadActionKey, Key: fyne.KeyEscape},
	GamepadLeftShoulder:  {Kind: GamepadActionKey, Key: fyne.KeyPageUp},
	GamepadRightShoulder: {Kind: GamepadActionKey, Key: fyne.KeyPageDown},
	GamepadBack:          {Kind: GamepadActionKey, Key: fyne.KeyTab, Modifier: fyne.KeyModifierShift, FollowFocus: true},
	GamepadStart:         {Kind: GamepadActionKey, Key: fyne.KeyTab, FollowFocus: true},
	GamepadLeftStick:     {Kind: GamepadActionMiddleClick},
	GamepadRightStick:    {Kind: GamepadActionSnapCenter},
	GamepadDPadUp:        {Kind: GamepadActionKey, Key: fyne.KeyUp},
	GamepadDPadDown:      {Kind: GamepadActionKey, Key: fyne.KeyDown},
	GamepadDPadLeft:      {Kind: GamepadActionKey, Key: fyne.KeyLeft},
	GamepadDPadRight:     {Kind: GamepadActionKey, Key: fyne.KeyRight},
}

// SetGamepadBinding overrides the binding of one gamepad button.
// It may be called at any time and takes effect on the next press.
//
// Since: 2.9
func SetGamepadBinding(button GamepadButton, binding GamepadBinding) {
	gamepadBindingsLock.Lock()
	defer gamepadBindingsLock.Unlock()
	gamepadOverrides[button] = binding
}

// ResetGamepadBindings restores all gamepad buttons to their default bindings.
//
// Since: 2.9
func ResetGamepadBindings() {
	gamepadBindingsLock.Lock()
	defer gamepadBindingsLock.Unlock()
	gamepadOverrides = map[GamepadButton]GamepadBinding{}
}

// GamepadBindingFor returns the effective binding of a gamepad button,
// either the developer override or the default.
//
// Since: 2.9
func GamepadBindingFor(button GamepadButton) GamepadBinding {
	gamepadBindingsLock.RLock()
	override, ok := gamepadOverrides[button]
	gamepadBindingsLock.RUnlock()
	if ok {
		return override
	}
	return gamepadDefaultBindings[button] // missing entry = zero value = GamepadActionNone
}

var (
	gamepadHelpButtons  = []GamepadButton{GamepadBack, GamepadStart}
	gamepadHelpHoldTime = 2 * time.Second
)

// SetGamepadHelpCombo configures which buttons must be held together, and for how long,
// to show the keymap help overlay. Pass an empty slice to disable the overlay entirely.
// Defaults: Back+Start held for two seconds.
//
// Since: 2.9
func SetGamepadHelpCombo(buttons []GamepadButton, hold time.Duration) {
	gamepadBindingsLock.Lock()
	defer gamepadBindingsLock.Unlock()
	gamepadHelpButtons = append([]GamepadButton(nil), buttons...)
	if hold > 0 {
		gamepadHelpHoldTime = hold
	}
}

// GamepadHelpCombo returns the current help overlay combo and required hold time.
//
// Since: 2.9
func GamepadHelpCombo() ([]GamepadButton, time.Duration) {
	gamepadBindingsLock.RLock()
	defer gamepadBindingsLock.RUnlock()
	return append([]GamepadButton(nil), gamepadHelpButtons...), gamepadHelpHoldTime
}

var gamepadOSK = struct {
	toggle GamepadButton
	lang   string
}{toggle: GamepadGuide, lang: "en"}

// SetGamepadOSKToggle picks the button that opens and closes the on-screen keyboard.
// Default: GamepadGuide (pads without one should pick another button; it has no default
// binding of its own so any free button works).
//
// Since: 2.9
func SetGamepadOSKToggle(button GamepadButton) {
	gamepadBindingsLock.Lock()
	defer gamepadBindingsLock.Unlock()
	gamepadOSK.toggle = button
}

// GamepadOSKToggle returns the current on-screen keyboard toggle button.
//
// Since: 2.9
func GamepadOSKToggle() GamepadButton {
	gamepadBindingsLock.RLock()
	defer gamepadBindingsLock.RUnlock()
	return gamepadOSK.toggle
}

// SetGamepadOSKLanguage selects the keyboard layout ("en", "de").
// Unknown codes fall back to English. Default: "en".
//
// Since: 2.9
func SetGamepadOSKLanguage(code string) {
	gamepadBindingsLock.Lock()
	defer gamepadBindingsLock.Unlock()
	gamepadOSK.lang = code
}

// GamepadOSKLanguage returns the current on-screen keyboard layout code.
//
// Since: 2.9
func GamepadOSKLanguage() string {
	gamepadBindingsLock.RLock()
	defer gamepadBindingsLock.RUnlock()
	return gamepadOSK.lang
}

var gamepadEmulationEnabled = true

// SetGamepadEmulationEnabled turns the whole mouse/keyboard emulation off or on:
// stick cursor, wheel scrolling, button bindings, the on-screen keyboard and the
// help overlay all stop while disabled. Raw state (GamepadStates) keeps updating,
// so games can read the pad directly without the fake mouse interfering.
// Default: true - most apps want the controller to just work.
//
// Since: 2.9
func SetGamepadEmulationEnabled(enabled bool) {
	gamepadBindingsLock.Lock()
	defer gamepadBindingsLock.Unlock()
	gamepadEmulationEnabled = enabled
}

// GamepadEmulationEnabled reports whether mouse/keyboard emulation is active.
//
// Since: 2.9
func GamepadEmulationEnabled() bool {
	gamepadBindingsLock.RLock()
	defer gamepadBindingsLock.RUnlock()
	return gamepadEmulationEnabled
}

// GamepadState is a raw snapshot of one connected controller, updated every frame.
// Axes are unprocessed -1..1 values with no deadzone applied: real pads drift, so
// games should ignore small values themselves (0.15 is a sane threshold).
//
// Since: 2.9
type GamepadState struct {
	Name         string                 // device name, e.g. "Microsoft X-Box 360 pad"
	Buttons      map[GamepadButton]bool // currently pressed; triggers count as pressed past half travel
	LeftX        float32                // left stick horizontal, -1..1 raw
	LeftY        float32                // left stick vertical, -1..1 raw (up is negative)
	RightX       float32                // right stick horizontal, -1..1 raw
	RightY       float32                // right stick vertical, -1..1 raw (up is negative)
	LeftTrigger  float32                // analog trigger travel, 0 at rest to 1 fully pressed
	RightTrigger float32                // analog trigger travel, 0 at rest to 1 fully pressed
}

var (
	gamepadStatesLock sync.RWMutex
	gamepadStates     []GamepadState
)

// GamepadStates returns one snapshot per connected controller, empty when none.
// Safe to call from any thread; the result is a private copy.
//
// Since: 2.9
func GamepadStates() []GamepadState {
	gamepadStatesLock.RLock()
	defer gamepadStatesLock.RUnlock()
	out := make([]GamepadState, len(gamepadStates))
	for i, s := range gamepadStates {
		s.Buttons = make(map[GamepadButton]bool, len(s.Buttons))
		for k, v := range gamepadStates[i].Buttons {
			s.Buttons[k] = v
		}
		out[i] = s
	}
	return out
}

// SetGamepadStates publishes a fresh raw snapshot; for gamepad backends to call
// every frame. Apps only read with GamepadStates, they never call this.
// The data is deep-copied in and out, so neither side can mutate the other's view.
//
// Since: 2.9
func SetGamepadStates(states []GamepadState) {
	gamepadStatesLock.Lock()
	defer gamepadStatesLock.Unlock()
	stored := make([]GamepadState, len(states))
	for i, s := range states {
		s.Buttons = make(map[GamepadButton]bool, len(s.Buttons))
		for k, v := range states[i].Buttons {
			s.Buttons[k] = v
		}
		stored[i] = s
	}
	gamepadStates = stored
}
