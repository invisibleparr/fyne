# Fyne Gamepad API Reference

This demo (`go run ./cmd/gamepad-advanced`) exercises every knob below.
The full API lives in `fyne.io/fyne/v2/driver/desktop` (file `gamepad.go`).
Everything is process-global and callable at any time; changes take effect on the next poll (~16ms).

## Types

### `GamepadButton`

Physical control ids, using the Xbox 360 layout as lowest common denominator:

`GamepadA/B/X/Y`, `GamepadLeftShoulder/GamepadRightShoulder` (LB/RB), `GamepadBack`,
`GamepadStart`, `GamepadLeftStick/GamepadRightStick` (the stick clicks, L3/R3),
`GamepadDPadUp/Down/Left/Right`, `GamepadLeftTrigger/GamepadRightTrigger` (analog LT/RT),
`GamepadGuide` (center Xbox button; no default binding, doubles as the on-screen keyboard toggle).

Pads with fewer controls simply never fire the missing ones.

### `GamepadActionKind`

What a binding does:

| Kind | Effect |
|---|---|
| `GamepadActionNone` | disabled (zero value) |
| `GamepadActionLeftClick` / `RightClick` / `MiddleClick` | mouse button at cursor position, held like a real button: keep it down and move the stick to drag anything implementing `fyne.Draggable` (sliders, scrollbars, custom widgets) |
| `GamepadActionKey` | synthesizes press+release of `Binding.Key` with optional `Modifier` |
| `GamepadActionSnapCenter` | warps the cursor to the center of the focused window |
| `GamepadActionFunc` | calls `Binding.Func(window)` on the main thread, ordered with regular UI events |

### `GamepadBinding`

```go
type GamepadBinding struct {
    Kind        GamepadActionKind
    Key         fyne.KeyName      // used by GamepadActionKey
    Modifier    fyne.KeyModifier  // optional modifier for GamepadActionKey
    Func        func(fyne.Window) // used by GamepadActionFunc; called on the main thread
    FollowFocus bool              // after a key, warp the cursor onto the newly focused widget
}
```

`FollowFocus` combined with `KeyTab` is what makes pure gamepad focus navigation work:
the keyboard event moves focus and the cursor teleports along to wherever focus landed.

## Defaults (no code required)

A = left click · B = right click · X = Enter · Y = Escape · LB/RB = PageUp/PageDown ·
Back/Start = Shift-Tab / Tab with FollowFocus · L3 = middle click · R3 = snap center ·
d-pad = arrow keys. Left stick moves the cursor (1200 px/s), right stick scrolls
(30 wheel notches/s). Guide opens the on-screen keyboard, Back+Start held 2s shows the keymap.

## Cursor image

- `SetGamepadCursor(img image.Image, hotspotX, hotspotY int)` — replaces the default arrow with
  your image for the whole window. It themes the *hardware* cursor itself, so mouse users see it
  too: one cursor for all devices. Hover cursors (text I-beam and friends) still win. Takes effect
  as the pointer moves on.
- `ResetGamepadCursor()` — restore the standard arrow.
- `GamepadCursor() Cursor` — current custom cursor, nil when the standard arrow is in use.

## Button remapping

- `SetGamepadBinding(button GamepadButton, binding GamepadBinding)` — override one button, live.
- `ResetGamepadBindings()` — wipe all overrides, defaults return.
- `GamepadBindingFor(button GamepadButton) GamepadBinding` — the effective binding (override or
  default). Good for drawing your own keymap UI.

## Help overlay

- `SetGamepadHelpCombo(buttons []GamepadButton, hold time.Duration)` — which buttons must be held
  together to show the live keymap diagram; pass an empty slice to disable it entirely.
  Default: Back+Start for two seconds.
- `GamepadHelpCombo() ([]GamepadButton, time.Duration)` — current combo and hold time.

## On-screen keyboard

- `SetGamepadOSKToggle(button GamepadButton)` / `GamepadOSKToggle()` — the button that opens AND
  closes the keyboard. Default: `GamepadGuide`. Pads without a guide button should pick any free
  button (it has no default binding of its own). The toggle is checked before bindings, so it wins
  even if the same button also carries a binding.
- `SetGamepadOSKLanguage(code string)` / `GamepadOSKLanguage() string` — layout `"en"` or `"de"`,
  unknown codes fall back to English. Applies when the keyboard next opens.

## Emulation switch

- `SetGamepadEmulationEnabled(bool)` / `GamepadEmulationEnabled() bool` — turn the whole fake
  mouse/keyboard layer off or on at runtime: stick cursor, wheel scrolling, button bindings,
  the on-screen keyboard and the help overlay all stop. Raw state (`GamepadStates`) keeps
  updating either way. Default: true - most apps want the controller to just work; games turn
  it off and drive themselves (see `cmd/gamepad-pong`).

## Raw state

- `GamepadStates() []GamepadState` — one snapshot per connected controller, empty when none,
  safe from any thread. Each `GamepadState` carries the device `Name`, a `Buttons` map of
  currently pressed buttons (triggers count as pressed past half travel) and raw analog axes:
  `LeftX/LeftY`, `RightX/RightY` (-1..1, up is negative) plus `LeftTrigger/RightTrigger`
  (0 at rest to 1 fully pressed). Axes have NO deadzone applied - real pads drift, so games
  should ignore small values themselves (~0.15 is sane).

## What is NOT public (deliberately)

Stick deadzone, cursor speed and scroll rate are driver constants; OSK layouts are internal data
tables (`oskRowsFor` in `internal/driver/glfw/gamepad_osk_desktop.go`); and there is no press-callback
API — buttons only produce input events or pollable state, exactly like a real mouse or keyboard would.
