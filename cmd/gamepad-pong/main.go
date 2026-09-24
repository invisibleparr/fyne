// Command gamepad-pong is the "raw gamepad" demo: two paddles, one ball, no winners.
// Left analog stick drives the left paddle, right analog stick the right paddle -
// one pad for both players or two pads with one stick each, it does not care.
//
// It shows the game developer's setup in three lines:
//
//	desktop.SetGamepadEmulationEnabled(false) // no fake mouse/keyboard/OSK/help overlay
//	states := desktop.GamepadStates()         // raw sticks, buttons and triggers every frame
//	... steer paddles by LeftY/RightY (own deadzone), smash via LeftTrigger/RightTrigger ...
package main

import (
	"fmt"
	"image/color"
	"math"
	"math/rand"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
)

const (
	ballSize    = 14.0
	paddleW     = 12.0
	paddleSpeed = 700.0 // px/s at full stick tilt
	deadzone    = 0.15  // our own: raw axes drift, the driver does not filter for games
	ballStart   = 340.0 // px/s serve speed
	ballMax     = 900.0
	ballFloor   = 180.0 // lazy rallies slow toward this, never stall
)

type pong struct {
	root           *fyne.Container
	field          *canvas.Rectangle // own dark background: white shapes stay visible in any theme
	left, right    *canvas.Rectangle
	ball           *canvas.Rectangle
	scoreL, scoreR *canvas.Text
	smashL, smashR *canvas.Text // trigger squeeze readout, 1.0 = untouched
	hint           *canvas.Text
	bx, by, vx, vy float64 // ball position and velocity in canvas pixels
	ly, ry         float64 // paddle top-edge positions
	lScore, rScore int
	lt, rt         float32 // trigger squeeze 0..1, kept for the readout
	placed         bool    // first serve waits for a real window size
}

func (g *pong) step(dt float64, size fyne.Size) {
	if size.Width < 100 || size.Height < 100 {
		return
	}
	if !g.placed {
		g.serve(size, rand.Intn(2)*2-1)
		g.placed = true
	}
	ph := float64(size.Height) / 5
	margin := paddleW * 2.5

	lv, rv, lt, rt := stickAxes()
	g.lt, g.rt = lt, rt
	g.ly = clamp(g.ly+float64(lv)*paddleSpeed*dt, 0, float64(size.Height)-ph)
	g.ry = clamp(g.ry+float64(rv)*paddleSpeed*dt, 0, float64(size.Height)-ph)

	g.bx += g.vx * dt
	g.by += g.vy * dt
	if g.by < 0 {
		g.by, g.vy = 0, -g.vy
	} else if g.by > float64(size.Height)-ballSize {
		g.by, g.vy = float64(size.Height)-ballSize, -g.vy
	}

	bcx := g.bx + ballSize/2
	if g.vx < 0 && bcx <= margin+paddleW && bcx >= margin-4 && g.touches(g.ly, ph) {
		g.bounce(margin+paddleW, 1, hitOffset(g.by, g.ly, ph), float64(lt)) // edge hits fly steep; LT charges the smash
	} else if g.vx > 0 && bcx >= float64(size.Width)-margin-paddleW && bcx <= float64(size.Width)-margin+4 && g.touches(g.ry, ph) {
		g.bounce(float64(size.Width)-margin-paddleW-ballSize, -1, hitOffset(g.by, g.ry, ph), float64(rt)) // RT charges the smash
	}

	if g.bx < -ballSize {
		g.rScore++
		g.serve(size, -1) // serve back at the loser: they get to swing first
	} else if g.bx > float64(size.Width) {
		g.lScore++
		g.serve(size, 1)
	}

	g.layout(size, ph, margin)
}

// touches reports whether the ball overlaps a paddle vertically.
func (g *pong) touches(paddleY, ph float64) bool {
	return g.by+ballSize >= paddleY && g.by <= paddleY+ph
}

// hitOffset: where on the paddle the ball landed, -1 at its top edge to +1 at its bottom.
func hitOffset(ballY, paddleY, ph float64) float64 {
	return clamp((ballY+ballSize/2-(paddleY+ph/2))/(ph/2), -1, 1)
}

// bounce: boost is trigger travel 0..1 - no squeeze returns at the usual pace,
// a full smash doubles the speed (ping pong players know the wrist trick).
// bounce: boost is trigger travel 0..1. No squeeze bleeds a little speed off
// (ping pong without wrist action dies away); full squeeze doubles it.
func (g *pong) bounce(x, dir, off, boost float64) { // x = where the ball's left edge belongs after the hit
	g.bx = x
	speed := clamp(math.Hypot(g.vx, g.vy)*(0.9+1.1*boost), ballFloor, ballMax)
	g.vy = off * speed * 0.8                      // steep at the paddle edges (~53 deg), flat in the middle
	g.vx = dir * math.Sqrt(speed*speed-g.vy*g.vy) // |vy| <= 0.8*speed keeps vx >= 60% of speed: never vertical
}

func (g *pong) serve(size fyne.Size, dir int) {
	g.bx, g.by = float64(size.Width)/2, float64(size.Height)/2
	g.vx = float64(dir) * ballStart
	g.vy = (rand.Float64()*2 - 1) * ballStart / 3
	g.scoreL.Text = strconv.Itoa(g.lScore)
	g.scoreR.Text = strconv.Itoa(g.rScore)
	g.scoreL.Refresh()
	g.scoreR.Refresh()
}

// stickAxes: any pad's left stick drives the left paddle, any right stick the right;
// triggers report how far either side is squeezed (max across pads).
func stickAxes() (lv, rv, lt, rt float32) {
	for _, s := range desktop.GamepadStates() {
		if abs32(s.LeftY) > deadzone {
			lv = s.LeftY
		}
		if abs32(s.RightY) > deadzone {
			rv = s.RightY
		}
		lt = max(lt, min(s.LeftTrigger, 1))
		rt = max(rt, min(s.RightTrigger, 1))
	}
	return lv, rv, lt, rt
}

func (g *pong) layout(size fyne.Size, ph float64, margin float64) {
	bleed := float32(4) // field must overhang the viewport: its texture seam bleeds white lines along top/left when flush with the border (GL atlas sampling, seen at fractional scale)
	g.field.Move(fyne.NewPos(-bleed, -bleed))
	g.field.Resize(fyne.NewSize(size.Width+2*bleed, size.Height+2*bleed))
	g.left.Resize(fyne.NewSize(paddleW, float32(ph)))
	g.right.Resize(fyne.NewSize(paddleW, float32(ph)))
	g.ball.Resize(fyne.NewSize(ballSize, ballSize))
	g.left.Move(fyne.NewPos(float32(margin), float32(g.ly)))
	g.right.Move(fyne.NewPos(float32(float64(size.Width)-margin-paddleW), float32(g.ry)))
	g.ball.Move(fyne.NewPos(float32(g.bx), float32(g.by)))

	g.scoreL.Resize(fyne.NewSize(size.Width/2, 70)) // centered text needs the full half-width box
	g.scoreR.Resize(fyne.NewSize(size.Width/2, 70))
	g.scoreL.Move(fyne.NewPos(0, 26)) // pushed down to make room for the smash readouts above
	g.scoreR.Move(fyne.NewPos(size.Width/2, 26))
	g.hint.Resize(fyne.NewSize(size.Width, 24))
	g.hint.Move(fyne.NewPos(0, size.Height-30))

	setSmash(g.smashL, g.lt) // 1.00 = trigger untouched, 2.00 = full squeeze; refresh only on change
	setSmash(g.smashR, g.rt)
	g.smashL.Resize(fyne.NewSize(size.Width/2, 18)) // centered above each score, clear of the window border
	g.smashR.Resize(fyne.NewSize(size.Width/2, 18))
	g.smashL.Move(fyne.NewPos(0, 4))
	g.smashR.Move(fyne.NewPos(size.Width/2, 4))

	g.root.Refresh()
}

func main() {
	a := app.New()
	w := a.NewWindow("gamepad pong")
	desktop.SetGamepadEmulationEnabled(false) // the sticks belong to the paddles now

	white := color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	g := &pong{
		field:  canvas.NewRectangle(color.NRGBA{R: 0x12, G: 0x14, B: 0x1a, A: 0xff}), // dark fill, any theme
		left:   canvas.NewRectangle(white),
		right:  canvas.NewRectangle(white),
		ball:   canvas.NewRectangle(white),
		scoreL: scoreText(),
		scoreR: scoreText(),
		smashL: hintText(""),
		smashR: hintText(""),
		hint:   hintText("left stick = left paddle   right stick = right paddle   triggers = smash power"),
	}
	g.root = container.NewWithoutLayout(g.field, g.left, g.right, g.ball, g.scoreL, g.scoreR, g.smashL, g.smashR, g.hint)
	w.SetContent(g.root)

	go func() { // 60 fps game loop; all UI touching happens inside DoAndWait on the main thread
		t := time.NewTicker(16 * time.Millisecond)
		defer t.Stop()
		last := time.Now()
		for range t.C {
			now := time.Now()
			dt := min(now.Sub(last).Seconds(), 0.05) // a stall must not teleport the ball
			last = now
			fyne.DoAndWait(func() { g.step(dt, w.Canvas().Size()) })
		}
	}()

	w.Resize(fyne.NewSize(800, 500))
	w.ShowAndRun()
}

// setSmash updates a readout text without hammering the painter every frame.
func setSmash(t *canvas.Text, v float32) {
	if txt := fmt.Sprintf("smash x%.2f", 1+float64(v)); t.Text != txt {
		t.Text = txt
		t.Refresh()
	}
}

func scoreText() *canvas.Text {
	t := canvas.NewText("0", color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xcc})
	t.TextSize = 56
	t.Alignment = fyne.TextAlignCenter
	return t
}

func hintText(s string) *canvas.Text {
	t := canvas.NewText(s, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xaa})
	t.TextSize = 14
	t.Alignment = fyne.TextAlignCenter
	return t
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func clamp(v, lo, hi float64) float64 {
	return min(max(v, lo), hi)
}
