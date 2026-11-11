//go:build js && wasm

package main

import (
	"math"
	"strconv"
	"syscall/js"
)

var modeNames = [...]string{
	modeTitle: "title", modePlay: "play", modeName: "name",
	modeScores: "scores", modeList: "list", modeEditor: "editor",
}

// syncMode publishes the current screen as <body data-mode="...">, which the
// stylesheet uses to decide which on-screen controls are visible.
func (a *app) syncMode() {
	if a.mode == a.shownMode {
		return
	}
	a.shownMode = a.mode
	a.doc.Get("body").Get("dataset").Set("mode", modeNames[a.mode])
	if a.mode == modeName {
		a.doc.Call("getElementById", "name-input").Set("value", a.name)
	}
}

// bindTouch wires up the on-screen controls (see #touch in index.html).
// They work by pressing the same virtual keys the keyboard does.
func (a *app) bindTouch() {
	win, doc := js.Global(), a.doc
	enable := func() {
		if !a.touch {
			a.touch = true
			doc.Get("body").Get("classList").Call("add", "touch")
		}
	}
	if mm := win.Get("matchMedia"); mm.Type() == js.TypeFunction && win.Call("matchMedia", "(pointer: coarse)").Get("matches").Bool() {
		enable()
	}
	a.listen(win, "touchstart", func(js.Value) { enable() })
	// iOS only unlocks audio from touchend/click, not from pointerdown
	a.listen(win, "touchend", func(js.Value) { a.audio.init() })
	a.listen(win, "click", func(js.Value) { a.audio.init() })

	// buttons: each holds down the key named in data-key
	btns := doc.Call("querySelectorAll", "#touch button")
	for i := 0; i < btns.Length(); i++ {
		b := btns.Index(i)
		code := b.Get("dataset").Get("key").String()
		a.listen(b, "pointerdown", func(e js.Value) {
			e.Call("preventDefault")
			a.audio.init()
			b.Call("setPointerCapture", e.Get("pointerId"))
			b.Get("classList").Call("add", "down")
			a.keys[code] = true
			a.tapped[code] = true
			a.onKey(code, "")
		})
		release := func(js.Value) {
			b.Get("classList").Call("remove", "down")
			delete(a.keys, code)
		}
		a.listen(b, "pointerup", release)
		a.listen(b, "pointercancel", release)
		a.listen(b, "contextmenu", func(e js.Value) { e.Call("preventDefault") })
	}

	// virtual joystick: an eight-way stick driving the arrow keys
	stick := doc.Call("getElementById", "stick")
	knob := doc.Call("getElementById", "knob")
	active := js.Null()
	setStick := func(nx, ny float64) {
		const dead = 0.28
		set := func(code string, on bool) {
			if on {
				a.keys[code] = true
			} else {
				delete(a.keys, code)
			}
		}
		set("ArrowLeft", nx < -dead)
		set("ArrowRight", nx > dead)
		set("ArrowUp", ny < -dead)
		set("ArrowDown", ny > dead)
		knob.Get("style").Set("transform",
			"translate("+strconv.FormatFloat(nx*38, 'f', 1, 64)+"px,"+strconv.FormatFloat(ny*38, 'f', 1, 64)+"px)")
	}
	track := func(e js.Value) {
		r := stick.Call("getBoundingClientRect")
		rad := r.Get("width").Float() / 2
		if rad == 0 {
			return
		}
		nx := (e.Get("clientX").Float() - r.Get("left").Float() - rad) / rad
		ny := (e.Get("clientY").Float() - r.Get("top").Float() - rad) / rad
		if l := math.Hypot(nx, ny); l > 1 {
			nx, ny = nx/l, ny/l
		}
		setStick(nx, ny)
	}
	a.listen(stick, "pointerdown", func(e js.Value) {
		e.Call("preventDefault")
		a.audio.init()
		active = e.Get("pointerId")
		stick.Call("setPointerCapture", active)
		track(e)
	})
	a.listen(stick, "pointermove", func(e js.Value) {
		if !active.IsNull() && e.Get("pointerId").Equal(active) {
			track(e)
		}
	})
	end := func(e js.Value) {
		if !active.IsNull() && e.Get("pointerId").Equal(active) {
			active = js.Null()
			setStick(0, 0)
		}
	}
	a.listen(stick, "pointerup", end)
	a.listen(stick, "pointercancel", end)

	// name entry: a real text field, so the phone's keyboard appears
	input := doc.Call("getElementById", "name-input")
	a.listen(input, "input", func(js.Value) {
		r := []rune(input.Get("value").String())
		if len(r) > 12 {
			r = r[:12]
		}
		a.name = string(r)
	})
	a.listen(doc.Call("getElementById", "namebox"), "submit", func(e js.Value) {
		e.Call("preventDefault")
		input.Call("blur")
		if a.mode == modeName {
			a.submitScore()
		}
	})
	a.listen(doc.Call("getElementById", "name-skip"), "click", func(js.Value) {
		if a.mode == modeName {
			a.toTitle()
		}
	})
}
