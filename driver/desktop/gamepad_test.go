package desktop_test

import (
	"image"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

func TestGamepadBindingForDefaults(t *testing.T) {
	desktop.ResetGamepadBindings()
	defer desktop.ResetGamepadBindings()

	assert.Equal(t, desktop.GamepadActionLeftClick, desktop.GamepadBindingFor(desktop.GamepadA).Kind)
	start := desktop.GamepadBindingFor(desktop.GamepadStart)
	assert.Equal(t, fyne.KeyTab, start.Key)
	assert.True(t, start.FollowFocus)

	// unlisted buttons (triggers) default to the zero value: disabled
	assert.Equal(t, desktop.GamepadActionNone, desktop.GamepadBindingFor(desktop.GamepadLeftTrigger).Kind)
}

func TestGamepadBindingOverrideAndReset(t *testing.T) {
	desktop.ResetGamepadBindings()
	defer desktop.ResetGamepadBindings()

	undo := desktop.GamepadBinding{Kind: desktop.GamepadActionKey, Key: fyne.KeyZ, Modifier: fyne.KeyModifierControl}
	desktop.SetGamepadBinding(desktop.GamepadY, undo)
	assert.Equal(t, undo, desktop.GamepadBindingFor(desktop.GamepadY))

	// other buttons untouched
	assert.Equal(t, desktop.GamepadActionRightClick, desktop.GamepadBindingFor(desktop.GamepadB).Kind)

	desktop.ResetGamepadBindings()
	assert.Equal(t, fyne.KeyEscape, desktop.GamepadBindingFor(desktop.GamepadY).Key)
}

func TestGamepadCursorRegistry(t *testing.T) {
	desktop.ResetGamepadCursor()
	defer desktop.ResetGamepadCursor()

	assert.Nil(t, desktop.GamepadCursor())

	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	desktop.SetGamepadCursor(img, 2, 3)
	cur := desktop.GamepadCursor()
	if assert.NotNil(t, cur) {
		got, x, y := cur.Image()
		assert.Equal(t, img, got)
		assert.Equal(t, 2, x)
		assert.Equal(t, 3, y)
	}

	desktop.SetGamepadCursor(nil, 0, 0) // nil resets too
	assert.Nil(t, desktop.GamepadCursor())
}

func TestGamepadHelpComboRegistry(t *testing.T) {
	defer desktop.SetGamepadHelpCombo([]desktop.GamepadButton{desktop.GamepadBack, desktop.GamepadStart}, 2*time.Second)

	buttons, hold := desktop.GamepadHelpCombo()
	assert.Equal(t, []desktop.GamepadButton{desktop.GamepadBack, desktop.GamepadStart}, buttons)
	assert.Equal(t, 2*time.Second, hold)

	desktop.SetGamepadHelpCombo([]desktop.GamepadButton{desktop.GamepadLeftShoulder, desktop.GamepadRightShoulder}, 500*time.Millisecond)
	buttons, hold = desktop.GamepadHelpCombo()
	assert.Equal(t, []desktop.GamepadButton{desktop.GamepadLeftShoulder, desktop.GamepadRightShoulder}, buttons)
	assert.Equal(t, 500*time.Millisecond, hold)

	// registry must not share slices with callers (defensive copies both ways)
	buttons[0] = desktop.GamepadA
	got, _ := desktop.GamepadHelpCombo()
	assert.Equal(t, desktop.GamepadLeftShoulder, got[0])

	input := []desktop.GamepadButton{desktop.GamepadX}
	desktop.SetGamepadHelpCombo(input, time.Second)
	input[0] = desktop.GamepadY
	got, _ = desktop.GamepadHelpCombo()
	assert.Equal(t, desktop.GamepadX, got[0])

	// empty slice disables the overlay
	desktop.SetGamepadHelpCombo(nil, 0)
	buttons, hold = desktop.GamepadHelpCombo()
	assert.Empty(t, buttons)
	assert.Equal(t, time.Second, hold) // zero hold keeps previous value, does not poison it
}

func TestGamepadOSKSettings(t *testing.T) {
	assert.Equal(t, desktop.GamepadGuide, desktop.GamepadOSKToggle()) // defaults
	assert.Equal(t, "en", desktop.GamepadOSKLanguage())

	desktop.SetGamepadOSKToggle(desktop.GamepadRightStick)
	desktop.SetGamepadOSKLanguage("de")
	assert.Equal(t, desktop.GamepadRightStick, desktop.GamepadOSKToggle())
	assert.Equal(t, "de", desktop.GamepadOSKLanguage())

	desktop.SetGamepadOSKToggle(desktop.GamepadGuide) // restore for other tests
	desktop.SetGamepadOSKLanguage("en")
}

func TestGamepadGuideHasNoDefaultBinding(t *testing.T) {
	assert.Equal(t, desktop.GamepadActionNone, desktop.GamepadBindingFor(desktop.GamepadGuide).Kind)
}

func TestGamepadEmulationSwitch(t *testing.T) {
	if !desktop.GamepadEmulationEnabled() {
		t.Error("emulation must default to on: apps should just work")
	}
	desktop.SetGamepadEmulationEnabled(false)
	if desktop.GamepadEmulationEnabled() {
		t.Error("off did not stick")
	}
	desktop.SetGamepadEmulationEnabled(true)
}

func TestGamepadStatesSnapshot(t *testing.T) {
	states := []desktop.GamepadState{
		{Name: "test pad", Buttons: map[desktop.GamepadButton]bool{desktop.GamepadA: true}, LeftY: -1, RightTrigger: 0.7},
	}
	desktop.SetGamepadStates(states)

	got := desktop.GamepadStates()
	if len(got) != 1 || got[0].Name != "test pad" || !got[0].Buttons[desktop.GamepadA] {
		t.Fatalf("snapshot lost data: %+v", got)
	}
	got[0].Buttons[desktop.GamepadB] = true // map mutation must not reach the store either
	got[0].Name = "mutated"
	states[0].LeftY = 42 // caller mutating its own input must not corrupt the store
	if desktop.GamepadStates()[0].LeftY == 42 {
		t.Error("store shares mutable state with caller")
	}

	desktop.SetGamepadStates(nil)
	if len(desktop.GamepadStates()) != 0 {
		t.Error("nil should clear all pads")
	}
}
