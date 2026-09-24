// Command gamepad-minimal shows how little a Fyne app must do to support a game controller.
//
// Answer: nothing at all. The desktop driver turns any connected pad into a mouse and
// keyboard before your widgets ever see an event, so every existing widget just works.
//
//	Left stick   moves the cursor        Right stick  scrolls
//	A / B        left / right click      X            Enter
//	Start/Back   Tab / Shift-Tab focus navigation (cursor follows focus)
//	LB / RB      PageUp / PageDown       LT / RT      keys too, see the keymap below
//	D-pad        arrow keys              R3           snap cursor to window center
//	Guide        on-screen keyboard      hold Back+Start 2s: show the full keymap
package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func main() {
	a := app.New()
	w := a.NewWindow("gamepad minimal")

	name := widget.NewEntry()
	name.SetPlaceHolder("try the on-screen keyboard: press Guide...")

	w.SetContent(container.NewVBox(
		widget.NewLabelWithStyle("No gamepad code here. Grab your controller.", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewButton("A button", func() { name.SetText("clicked with A (or a real mouse)") }),
		name,
	))

	w.Resize(fyne.NewSize(420, 240))
	w.ShowAndRun()
}
