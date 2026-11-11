//go:build js && wasm

package main

import (
	"math"
	"strconv"
	"syscall/js"
)

// gfx is a thin, state-caching wrapper around CanvasRenderingContext2D.
// Every call into JavaScript costs time, so redundant state changes are skipped.
type gfx struct {
	ctx     js.Value
	fillC   string
	strokeC string
	font    string
	align   string
	lw      float64
	alpha   float64
	fonts   map[int]string
}

func newGfx(canvas js.Value) *gfx {
	g := &gfx{ctx: canvas.Call("getContext", "2d"), alpha: 1, lw: 1, fonts: map[int]string{}}
	g.ctx.Set("lineJoin", "round")
	g.ctx.Set("textBaseline", "middle")
	return g
}

func (g *gfx) fill(c string) {
	if c != g.fillC {
		g.fillC = c
		g.ctx.Set("fillStyle", c)
	}
}

func (g *gfx) stroke(c string) {
	if c != g.strokeC {
		g.strokeC = c
		g.ctx.Set("strokeStyle", c)
	}
}

func (g *gfx) width(w float64) {
	if w != g.lw {
		g.lw = w
		g.ctx.Set("lineWidth", w)
	}
}

func (g *gfx) setAlpha(a float64) {
	if a != g.alpha {
		g.alpha = a
		g.ctx.Set("globalAlpha", a)
	}
}

func (g *gfx) offset(x, y float64) { g.ctx.Call("setTransform", 1, 0, 0, 1, x, y) }

func (g *gfx) rect(x, y, w, h float64, c string) {
	g.fill(c)
	g.ctx.Call("fillRect", x, y, w, h)
}

func (g *gfx) box(x, y, w, h float64, c string, lw float64) {
	g.stroke(c)
	g.width(lw)
	g.ctx.Call("strokeRect", x, y, w, h)
}

func (g *gfx) line(x1, y1, x2, y2 float64, c string, lw float64) {
	g.stroke(c)
	g.width(lw)
	g.ctx.Call("beginPath")
	g.ctx.Call("moveTo", x1, y1)
	g.ctx.Call("lineTo", x2, y2)
	g.ctx.Call("stroke")
}

// poly draws a closed polygon. pts holds x,y pairs relative to (x,y), scaled by sx,sy.
func (g *gfx) poly(pts []float64, x, y, sx, sy float64, fillC, strokeC string) {
	c := g.ctx
	c.Call("beginPath")
	c.Call("moveTo", x+pts[0]*sx, y+pts[1]*sy)
	for i := 2; i < len(pts); i += 2 {
		c.Call("lineTo", x+pts[i]*sx, y+pts[i+1]*sy)
	}
	c.Call("closePath")
	if fillC != "" {
		g.fill(fillC)
		c.Call("fill")
	}
	if strokeC != "" {
		g.stroke(strokeC)
		g.width(1.6)
		c.Call("stroke")
	}
}

func (g *gfx) circle(x, y, r float64, fillC, strokeC string) {
	c := g.ctx
	c.Call("beginPath")
	c.Call("arc", x, y, r, 0, 2*math.Pi)
	if fillC != "" {
		g.fill(fillC)
		c.Call("fill")
	}
	if strokeC != "" {
		g.stroke(strokeC)
		g.width(1.6)
		c.Call("stroke")
	}
}

func (g *gfx) text(s string, x, y float64, size int, c, align string) {
	f, ok := g.fonts[size]
	if !ok {
		f = "bold " + strconv.Itoa(size) + "px Consolas, Menlo, 'DejaVu Sans Mono', 'Courier New', monospace"
		g.fonts[size] = f
	}
	if f != g.font {
		g.font = f
		g.ctx.Set("font", f)
	}
	if align != g.align {
		g.align = align
		g.ctx.Set("textAlign", align)
	}
	g.fill(c)
	g.ctx.Call("fillText", s, x, y)
}

func hsl(h int, s, l int) string {
	return "hsl(" + strconv.Itoa(h) + "," + strconv.Itoa(s) + "%," + strconv.Itoa(l) + "%)"
}
