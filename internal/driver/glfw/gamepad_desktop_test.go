//go:build !wasm && !test_web_driver

package glfw

import (
	"math"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/stretchr/testify/assert"
)

func TestGamepadAxisDelta(t *testing.T) {
	tests := map[string]struct {
		axis   float32
		dt     float64
		expect float64
	}{
		"zero axis no move":       {0, 1.0 / 60, 0},
		"drift inside deadzone":   {0.07, 1.0 / 60, 0},
		"negative drift deadzone": {-0.07, 1.0 / 60, 0},
		"full tilt one second":    {1.0, 1.0, gamepadCursorSpeed},
		"full tilt negative flip": {-1.0, 1.0, -gamepadCursorSpeed},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := gamepadAxisDelta(tt.axis, tt.dt)
			assert.InDelta(t, tt.expect, got, 0.001)
		})
	}
}

func TestGamepadAxisDeltaRampGrowsMonotonically(t *testing.T) {
	prev := -1.0
	for v := float32(gamepadDeadzone); v <= 1.0; v += 0.05 {
		got := math.Abs(gamepadAxisDelta(v, 1.0))
		assert.GreaterOrEqual(t, got, prev)
		prev = got
	}
}

func TestGamepadOwnershipLastMoverWins(t *testing.T) {
	gamepad = gamepadState{} // fresh state, stick at rest (zero value must work!)

	// drift inside deadzone must never grab the cursor
	gamepadUpdateOwnership(100, 100, []float32{0.07, -0.05})
	assert.False(t, gamepad.ownsCursor)

	// FIRST touch ever grabs immediately (zero-value bug regression guard)
	gamepadUpdateOwnership(100, 100, []float32{0.8, 0})
	assert.True(t, gamepad.ownsCursor)
	assert.Equal(t, 100.0, gamepad.lastWarpX)

	// our own warp to new position must NOT look like a takeover
	gamepad.lastWarpX = 150
	gamepadUpdateOwnership(150, 100, []float32{0.8, 0})
	assert.True(t, gamepad.ownsCursor)

	// real mouse jumps cursor far away -> ownership released
	gamepadUpdateOwnership(400, 300, []float32{0.8, 0})
	assert.False(t, gamepad.ownsCursor)

	// holding stick does not re-grab; release and touch again does
	gamepadUpdateOwnership(400, 300, []float32{0.8, 0})
	assert.False(t, gamepad.ownsCursor)
	gamepadUpdateOwnership(400, 300, []float32{0, 0}) // release to center
	gamepadUpdateOwnership(400, 300, []float32{-0.5, 0.5})
	assert.True(t, gamepad.ownsCursor)
}

func TestMovedPast(t *testing.T) {
	assert.False(t, movedPast(100, 100, 101, 99)) // sub-pixel jitter = our own warp rounding
	assert.True(t, movedPast(100, 100, 120, 100)) // real mouse yank
	assert.True(t, movedPast(100, 100, 100, 80))  // vertical too
}

func TestGamepadOwnershipNilAxes(t *testing.T) {
	gamepad = gamepadState{}

	// multi-pad poll passes nil when no pad pushes its stick - must not panic, must not grab
	gamepadUpdateOwnership(100, 100, nil)
	assert.False(t, gamepad.ownsCursor)

	// and it must not poison the next real touch
	gamepadUpdateOwnership(100, 100, []float32{0.9, 0})
	assert.True(t, gamepad.ownsCursor)
}

func TestGamepadHelpTimer(t *testing.T) {
	hold := 2 * time.Second
	step := 500 * time.Millisecond

	// below threshold: accumulate silently
	held, visible := gamepadHelpTimer(true, 0, step, hold)
	assert.False(t, visible)
	assert.Equal(t, step, held)

	// four half-second ticks of holding reach the two second threshold
	for i := 0; i < 2; i++ { // ticks 2 and 3 still below the hold time
		held, visible = gamepadHelpTimer(true, held, step, hold)
		assert.False(t, visible)
	}
	held, visible = gamepadHelpTimer(true, held, step, hold) // tick 4 hits 2s
	assert.True(t, visible)

	// keep holding: stays visible while fingers stay down
	_, visible = gamepadHelpTimer(true, held, step, hold)
	assert.True(t, visible)

	// release hides immediately and resets the accumulator
	held, visible = gamepadHelpTimer(false, held, step, hold)
	assert.Equal(t, time.Duration(0), held)
	assert.False(t, visible)
}

func TestDescribeGamepadBinding(t *testing.T) {
	tests := map[string]struct {
		binding desktop.GamepadBinding
		expect  string
	}{
		"left click":     {desktop.GamepadBinding{Kind: desktop.GamepadActionLeftClick}, "left click"},
		"snap center":    {desktop.GamepadBinding{Kind: desktop.GamepadActionSnapCenter}, "snap cursor to center"},
		"func":           {desktop.GamepadBinding{Kind: desktop.GamepadActionFunc, Func: func(fyne.Window) {}}, "custom function"},
		"plain key":      {desktop.GamepadBinding{Kind: desktop.GamepadActionKey, Key: fyne.KeyReturn}, "Enter"},
		"page up":        {desktop.GamepadBinding{Kind: desktop.GamepadActionKey, Key: fyne.KeyPageUp}, "Page Up"},
		"page down":      {desktop.GamepadBinding{Kind: desktop.GamepadActionKey, Key: fyne.KeyPageDown}, "Page Down"},
		"ctrl z":         {desktop.GamepadBinding{Kind: desktop.GamepadActionKey, Key: fyne.KeyZ, Modifier: fyne.KeyModifierControl}, "Ctrl+Z"},
		"tab follow":     {desktop.GamepadBinding{Kind: desktop.GamepadActionKey, Key: fyne.KeyTab, FollowFocus: true}, "Tab (cursor follows focus)"},
		"shift tab both": {desktop.GamepadBinding{Kind: desktop.GamepadActionKey, Key: fyne.KeyTab, Modifier: fyne.KeyModifierShift, FollowFocus: true}, "Shift+Tab (cursor follows focus)"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.expect, describeGamepadBinding(tt.binding))
		})
	}
}

func TestGamepadHelpLabelTableComplete(t *testing.T) {
	seen := map[desktop.GamepadButton]int{}
	for _, lbl := range gamepadHelpLabels {
		seen[lbl.button]++
	}
	all := []desktop.GamepadButton{
		desktop.GamepadA, desktop.GamepadB, desktop.GamepadX, desktop.GamepadY,
		desktop.GamepadLeftShoulder, desktop.GamepadRightShoulder,
		desktop.GamepadBack, desktop.GamepadStart,
		desktop.GamepadLeftStick, desktop.GamepadRightStick,
		desktop.GamepadDPadUp, desktop.GamepadDPadDown, desktop.GamepadDPadLeft, desktop.GamepadDPadRight,
		desktop.GamepadLeftTrigger, desktop.GamepadRightTrigger,
	}
	for _, b := range all {
		assert.Equal(t, 1, seen[b], "every button needs exactly one label anchor")
	}
	assert.Len(t, gamepadHelpLabels, len(all))
}

func TestBuildGamepadHelpCounts(t *testing.T) {
	none := func(desktop.GamepadButton) desktop.GamepadBinding {
		return desktop.GamepadBinding{Kind: desktop.GamepadActionNone}
	}
	root := buildGamepadHelp(fyne.NewSize(900, 500), none)
	lines, texts := countHelpObjects(root)
	assert.Equal(t, len(gamepadHelpNotes), lines, "only the axis note lines remain when nothing is bound")

	two := func(b desktop.GamepadButton) desktop.GamepadBinding {
		if b == desktop.GamepadA || b == desktop.GamepadB {
			return desktop.GamepadBinding{Kind: desktop.GamepadActionLeftClick}
		}
		return desktop.GamepadBinding{Kind: desktop.GamepadActionNone}
	}
	root = buildGamepadHelp(fyne.NewSize(900, 500), two)
	lines2, texts2 := countHelpObjects(root)
	assert.Equal(t, len(gamepadHelpNotes)+2, lines2)
	assert.Equal(t, texts+2, texts2, "one description text per bound button")
}

func countHelpObjects(obj fyne.CanvasObject) (lines, texts int) {
	for _, o := range obj.(*fyne.Container).Objects { // overlay builder keeps one flat layer
		switch o.(type) {
		case *canvas.Line:
			lines++
		case *canvas.Text:
			texts++
		}
	}
	return lines, texts
}
