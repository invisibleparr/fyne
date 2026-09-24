//go:build !wasm && !test_web_driver

package glfw

import (
	"image"
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/go-gl/glfw/v3.4/glfw"
	"github.com/stretchr/testify/assert"
)

var keyCodeMap = map[glfw.Key]fyne.KeyName{
	// non-printable
	glfw.KeyEscape:    fyne.KeyEscape,
	glfw.KeyEnter:     fyne.KeyReturn,
	glfw.KeyTab:       fyne.KeyTab,
	glfw.KeyBackspace: fyne.KeyBackspace,
	glfw.KeyInsert:    fyne.KeyInsert,
	glfw.KeyDelete:    fyne.KeyDelete,
	glfw.KeyRight:     fyne.KeyRight,
	glfw.KeyLeft:      fyne.KeyLeft,
	glfw.KeyDown:      fyne.KeyDown,
	glfw.KeyUp:        fyne.KeyUp,
	glfw.KeyPageUp:    fyne.KeyPageUp,
	glfw.KeyPageDown:  fyne.KeyPageDown,
	glfw.KeyHome:      fyne.KeyHome,
	glfw.KeyEnd:       fyne.KeyEnd,

	glfw.KeySpace:   fyne.KeySpace,
	glfw.KeyKPEnter: fyne.KeyEnter,

	// functions
	glfw.KeyF1:  fyne.KeyF1,
	glfw.KeyF2:  fyne.KeyF2,
	glfw.KeyF3:  fyne.KeyF3,
	glfw.KeyF4:  fyne.KeyF4,
	glfw.KeyF5:  fyne.KeyF5,
	glfw.KeyF6:  fyne.KeyF6,
	glfw.KeyF7:  fyne.KeyF7,
	glfw.KeyF8:  fyne.KeyF8,
	glfw.KeyF9:  fyne.KeyF9,
	glfw.KeyF10: fyne.KeyF10,
	glfw.KeyF11: fyne.KeyF11,
	glfw.KeyF12: fyne.KeyF12,

	// numbers - lookup by code to avoid AZERTY using the symbol name instead of number
	glfw.Key0:   fyne.Key0,
	glfw.KeyKP0: fyne.Key0,
	glfw.Key1:   fyne.Key1,
	glfw.KeyKP1: fyne.Key1,
	glfw.Key2:   fyne.Key2,
	glfw.KeyKP2: fyne.Key2,
	glfw.Key3:   fyne.Key3,
	glfw.KeyKP3: fyne.Key3,
	glfw.Key4:   fyne.Key4,
	glfw.KeyKP4: fyne.Key4,
	glfw.Key5:   fyne.Key5,
	glfw.KeyKP5: fyne.Key5,
	glfw.Key6:   fyne.Key6,
	glfw.KeyKP6: fyne.Key6,
	glfw.Key7:   fyne.Key7,
	glfw.KeyKP7: fyne.Key7,
	glfw.Key8:   fyne.Key8,
	glfw.KeyKP8: fyne.Key8,
	glfw.Key9:   fyne.Key9,
	glfw.KeyKP9: fyne.Key9,

	// desktop
	glfw.KeyLeftShift:    desktop.KeyShiftLeft,
	glfw.KeyRightShift:   desktop.KeyShiftRight,
	glfw.KeyLeftControl:  desktop.KeyControlLeft,
	glfw.KeyRightControl: desktop.KeyControlRight,
	glfw.KeyLeftAlt:      desktop.KeyAltLeft,
	glfw.KeyRightAlt:     desktop.KeyAltRight,
	glfw.KeyLeftSuper:    desktop.KeySuperLeft,
	glfw.KeyRightSuper:   desktop.KeySuperRight,
	glfw.KeyMenu:         desktop.KeyMenu,
	glfw.KeyPrintScreen:  desktop.KeyPrintScreen,
	glfw.KeyCapsLock:     desktop.KeyCapsLock,
}

func TestGlfwKeyToKeyName(t *testing.T) {
	for key, value := range keyCodeMap {
		translated := glfwKeyToKeyName(key)
		assert.Equal(t, value, translated)
	}

	invalid := glfwKeyToKeyName(glfw.Key(-1))
	assert.Equal(t, fyne.KeyUnknown, invalid)
}

func TestConvertASCII(t *testing.T) {
	for i := 0; i <= 'Z'-'A'; i++ {
		translated := convertASCII(glfw.KeyA + glfw.Key(i))
		expected := fyne.KeyName(rune(fyne.KeyA[0] + byte(i)))
		assert.Equal(t, expected, translated)
	}

	invalid := convertASCII(glfw.Key(-1))
	assert.Equal(t, fyne.KeyUnknown, invalid)
}

var keyNameMapSpecialCharacters = map[string]fyne.KeyName{
	"'": fyne.KeyApostrophe,
	",": fyne.KeyComma,
	"-": fyne.KeyMinus,
	".": fyne.KeyPeriod,
	"/": fyne.KeySlash,
	"*": fyne.KeyAsterisk,
	"`": fyne.KeyBackTick,

	";": fyne.KeySemicolon,
	"+": fyne.KeyPlus,
	"=": fyne.KeyEqual,

	"[":  fyne.KeyLeftBracket,
	"\\": fyne.KeyBackslash,
	"]":  fyne.KeyRightBracket,
}

func TestKeyCodeToKeyName(t *testing.T) {
	for key, value := range keyNameMapSpecialCharacters {
		translated := keyCodeToKeyName(key)
		assert.Equal(t, value, translated)
	}

	for i := rune(0); i <= 'z'-'a'; i++ {
		translated := keyCodeToKeyName(string('a' + i))
		expected := fyne.KeyName(rune(fyne.KeyA[0]) + i)
		assert.Equal(t, expected, translated)
	}

	invalid := keyCodeToKeyName("@")
	assert.Equal(t, fyne.KeyUnknown, invalid)

	invalid = keyCodeToKeyName("invalid")
	assert.Equal(t, fyne.KeyUnknown, invalid)
}

func TestScaleImageUpscaleSolid(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	red := color.NRGBA{R: 255, A: 255}
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			src.Set(x, y, red)
		}
	}

	dst := scaleImage(src, 2.0)
	assert.Equal(t, 4, dst.Bounds().Dx())
	assert.Equal(t, 4, dst.Bounds().Dy())
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			r, g, b, a := dst.At(x, y).RGBA()
			assert.InDelta(t, 255, r>>8, 1)
			assert.InDelta(t, 0, g>>8, 1)
			assert.InDelta(t, 0, b>>8, 1)
			assert.InDelta(t, 255, a>>8, 1)
		}
	}
}

func TestScaleImageDownscale(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 8, 6))
	dst := scaleImage(src, 0.5)
	assert.Equal(t, 4, dst.Bounds().Dx())
	assert.Equal(t, 3, dst.Bounds().Dy())

	tiny := scaleImage(src, 0.01) // never zero or negative size
	assert.Equal(t, 1, tiny.Bounds().Dx())
}

func TestScaleImageBlendsCheckerboard(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	src.Set(0, 0, white)
	src.Set(1, 1, white) // other two stay transparent black

	dst := scaleImage(src, 4.0)
	midR, _, _, _ := dst.At(3, 3).RGBA() // between all four cells = gray-ish, not pure white/black
	assert.Greater(t, midR>>8, uint32(60))
	assert.Less(t, midR>>8, uint32(200))
}
