//go:build wasm || test_web_driver

package glfw

import "fyne.io/fyne/v2"

// gamepadOSKPhysicalTarget has no meaning without the desktop on-screen keyboard.
func gamepadOSKPhysicalTarget(w *window) fyne.Focusable { return nil }
