// Command checkboxxer is a tiny survival game: a 10x10 grid of unlabeled checkboxes,
// the computer ticks them faster and faster, you untick them. Survive as long as you can.
//
// It needs no gamepad code at all - the driver already turns your controller into a
// mouse, so left stick aims, A unticks. That is the point of this fork.
package main

import (
	"math/rand"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

const (
	gridSize   = 10
	totalBoxes = gridSize * gridSize
	startDelay = 900 * time.Millisecond // computer's first tick interval
	minDelay   = 120 * time.Millisecond // floor: mercy has limits
	rampFactor = 0.985                  // interval shrinks a bit with every box the computer ticks
)

type game struct {
	window    fyne.Window
	checks    []*widget.Check
	hud       *widget.Label
	speed     *widget.ProgressBar
	stopCh    chan struct{}
	running   bool
	resetting bool
	unticks   int
	started   time.Time
}

// pace is the computer's interval after it has ticked this many boxes total.
func pace(ticked int) time.Duration {
	d := float64(startDelay)
	for i := 0; i < ticked; i++ {
		d *= rampFactor
		if d <= float64(minDelay) {
			return minDelay
		}
	}
	return time.Duration(d)
}

func (g *game) build() {
	g.checks = make([]*widget.Check, totalBoxes)
	cells := make([]fyne.CanvasObject, totalBoxes)
	for i := range g.checks {
		g.checks[i] = widget.NewCheck("", nil)
		g.checks[i].OnChanged = func(checked bool) {
			if !g.resetting && !checked && g.running {
				g.unticks++
				g.updateHUD()
			}
		}
		cells[i] = g.checks[i]
	}

	grid := container.NewCenter(container.NewGridWithColumns(gridSize, cells...))
	g.hud = widget.NewLabel("")
	g.speed = widget.NewProgressBar() // default range 0..1 is exactly our speed meter
	top := container.NewBorder(nil, nil, g.hud, g.speed, nil)
	g.window.SetContent(container.NewBorder(top, nil, nil, nil, grid))
}

// updateHUD and everything it reads runs on the main thread only.
func (g *game) updateHUD() {
	ticked := 0
	for _, c := range g.checks {
		if c.Checked {
			ticked++
		}
	}
	elapsed := time.Since(g.started).Round(time.Second)
	g.hud.SetText("survived " + elapsed.String() + "  ·  unticked " + strconv.Itoa(g.unticks))
	fullness := float64(ticked) / totalBoxes // grid full = speed meter pegged
	g.speed.SetValue(fullness)
}

func (g *game) start() {
	g.running = true
	g.started = time.Now()
	stop := make(chan struct{})
	g.stopCh = stop
	go g.loop(stop)
}

// loop alternates: one computer move on the main thread, then a sleep.
// All widget access lives inside fyne.DoAndWait; the goroutine only holds channels.
func (g *game) loop(stop <-chan struct{}) {
	for delay := startDelay; ; { // first tick lands a full interval after start: grace period
		select {
		case <-stop:
			return
		case <-time.After(delay):
		}
		var lost bool
		fyne.DoAndWait(func() { // synchronous: we need the result before sleeping again
			var open []int
			for i, c := range g.checks {
				if !c.Checked {
					open = append(open, i)
				}
			}
			if len(open) == 0 {
				g.running = false
				elapsed := time.Since(g.started).Round(time.Second)
				msg := "survived " + elapsed.String() + " · unticked " + strconv.Itoa(g.unticks) + " boxes"
				loss := dialog.NewInformation("You lost", msg, g.window)
				loss.SetOnClosed(func() { g.reset() }) // OK closes the dialog and restarts
				loss.Show()
				lost = true
				return
			}
			g.checks[open[rand.Intn(len(open))]].SetChecked(true)
			g.updateHUD()
			delay = pace(totalBoxes - len(open) + 1) // next interval, derived from ticks done
		})
		if lost {
			return
		}
	}
}

func (g *game) reset() {
	close(g.stopCh) // stop the old loop for good before starting a new one
	g.resetting = true
	for _, c := range g.checks {
		c.SetChecked(false) // fires OnChanged: do not score our own cleanup
	}
	g.resetting = false
	g.unticks = 0
	g.start()
}

func main() {
	a := app.New()
	w := a.NewWindow("checkboxxer")

	g := &game{window: w}
	g.build()
	g.start()

	w.Resize(fyne.NewSize(520, 640))
	w.ShowAndRun()
}
