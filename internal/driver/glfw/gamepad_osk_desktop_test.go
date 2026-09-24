//go:build !wasm && !test_web_driver

package glfw

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"github.com/stretchr/testify/assert"
)

func TestOSKRowsFor(t *testing.T) {
	en := oskRowsFor("en", false)
	assert.Len(t, en, 4)
	labels := oskLabelSet(en)
	for _, want := range []string{"q", "a", "z", "space", "enter", "paste"} {
		assert.Contains(t, labels, want)
	}

	de := oskRowsFor("de", false)
	deLabels := oskLabelSet(de)
	for _, want := range []string{"ä", "ö", "ü", "ß"} {
		assert.Contains(t, deLabels, want, "german layout must carry %s", want)
	}

	sym := oskRowsFor("en", true)
	symbols := oskLabelSet(sym)
	assert.Contains(t, symbols, "?")
	assert.Contains(t, symbols, "@")
}

func oskLabelSet(rows [][]oskKey) map[string]bool {
	set := map[string]bool{}
	for _, row := range rows {
		for _, k := range row {
			set[k.label] = true
		}
	}
	return set
}

func TestOSKHitTest(t *testing.T) {
	hits, kb := layoutOSK(fyne.NewSize(1280, 720), oskRowsFor("en", false))
	assert.NotEmpty(t, hits)

	for _, h := range hits { // center of every key must hit exactly that key
		center := fyne.NewPos((h.rect.Min.X+h.rect.Max.X)/2, (h.rect.Min.Y+h.rect.Max.Y)/2)
		got := oskKeyAt(hits, center)
		if assert.NotNil(t, got) {
			assert.Equal(t, h.key.label, got.label)
		}
	}

	assert.Nil(t, oskKeyAt(hits, fyne.NewPos(kb.Min.X+kb.Dx()/2, kb.Min.Y-kb.Dy()))) // above keyboard: nothing
}

func TestOSKMoveStaysOnKeyboard(t *testing.T) {
	kb := oskRect{Min: fyne.NewPos(100, 400), Max: fyne.NewPos(900, 700)}
	oneSecond := time.Second

	right := oskMove(fyne.NewPos(500, 550), 1, 0, kb, oneSecond)
	assert.InDelta(t, kb.Max.X, right.X, 0.01) // full tilt crosses the whole keyboard in a second (clamped at edge)

	left := oskMove(fyne.NewPos(500, 550), -2, 0, kb, oneSecond)
	assert.InDelta(t, kb.Min.X, left.X, 0.01)

	still := oskMove(fyne.NewPos(300, 450), 0, 0, kb, oneSecond) // deadzone slop = no drift
	assert.Equal(t, float32(300), still.X)
	assert.Equal(t, float32(450), still.Y)
}

func TestOSKTypedShift(t *testing.T) {
	a := oskRune('a')
	assert.Equal(t, 'A', oskTyped(a, true))
	assert.Equal(t, 'a', oskTyped(a, false))
	assert.Equal(t, '5', oskTyped(oskRune('5'), true)) // digits ignore shift here: symbols page owns them

	assert.Equal(t, 'Ä', oskTyped(oskRune('ä'), true))
	assert.Zero(t, oskTyped(oskKey{key: fyne.KeyReturn}, true), "special keys type no rune")
}

func TestOSKRepeatCount(t *testing.T) {
	assert.Equal(t, 0, oskRepeatCount(399*time.Millisecond))
	assert.Equal(t, 1, oskRepeatCount(oskRepeatDelay))
	assert.Equal(t, 1, oskRepeatCount(oskRepeatDelay+49*time.Millisecond))
	assert.Equal(t, 2, oskRepeatCount(oskRepeatDelay+oskRepeatRate))
}

func TestGamepadOSKPresentDetectsStackEviction(t *testing.T) {
	c := test.NewCanvas()
	dialog := container.NewWithoutLayout()
	keyboard := container.NewWithoutLayout()
	c.Overlays().Add(dialog)   // file-save dialog opens first...
	c.Overlays().Add(keyboard) // ...OSK on top of it

	if !gamepadOSKPresent(c, keyboard) {
		t.Fatal("keyboard should be present while both overlays are up")
	}

	c.Overlays().Remove(dialog) // Enter confirms the name: fyne evicts the dialog AND everything above it

	if gamepadOSKPresent(c, keyboard) {
		t.Error("stack must have silently evicted our keyboard - detection failed")
	}
	if gamepadOSKPresent(c, nil) {
		t.Error("nil content is never present")
	}
}
