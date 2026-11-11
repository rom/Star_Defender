//go:build js && wasm

package main

import (
	"math"
	"math/rand"
	"strconv"

	"stardefender/internal/game"
	"stardefender/internal/level"
)

const (
	viewW = float64(level.ViewW)
	viewH = float64(level.ViewH)
)

// Vector shapes, as x,y pairs around the sprite centre, facing right.
var (
	shipHull    = []float64{22, 2, 6, -3, -1, -9, -14, -9, -10, -2, -18, -2, -18, 5, 6, 6}
	shipCanopy  = []float64{8, -1, 1, -6, -6, -6, -4, -1}
	shipFlame   = []float64{-18, -1, -30, 1.5, -18, 4}
	snatBody    = []float64{-11, -1, -6, -10, 6, -10, 11, -1, 6, 5, -6, 5}
	reaverBody  = []float64{0, -13, 4, -4, 13, -6, 6, 1, 11, 11, 0, 5, -11, 11, -6, 1, -13, -6, -4, -4}
	bomberBody  = []float64{-13, 0, 0, -11, 13, 0, 0, 11}
	bomberCore  = []float64{-5, 0, 0, -4, 5, 0, 0, 4}
	hiveBody    = []float64{-16, 0, -8, -14, 8, -14, 16, 0, 8, 14, -8, 14}
	waspBody    = []float64{8, 0, -6, -6, -2, 0, -6, 6}
	stalkHull   = []float64{-16, 2, -6, -3, 6, -3, 16, 2, 6, 7, -6, 7}
	stalkDome   = []float64{-6, -3, -3, -9, 3, -9, 6, -3}
	turretBase  = []float64{-13, 9, -8, -1, 8, -1, 13, 9}
	dreadHull   = []float64{-44, 0, -30, -14, -8, -18, 0, -26, 8, -18, 30, -14, 44, 0, 30, 10, 12, 16, -12, 16, -30, 10}
	dreadBridge = []float64{-10, -8, 10, -8, 6, 2, -6, 2}
)

type star struct{ x, y, depth, size float64 }

var stars = func() []star {
	rng := rand.New(rand.NewSource(42))
	out := make([]star, 90)
	for i := range out {
		d := 0.08 + rng.Float64()*0.3
		out[i] = star{rng.Float64() * viewW, 64 + rng.Float64()*(viewH-64), d, 1 + d*4}
	}
	return out
}()

var starColors = []string{"#556", "#778", "#99a", "#ccd"}

func (a *app) drawStars(camX float64) {
	for i, s := range stars {
		x := math.Mod(s.x-camX*s.depth, viewW)
		if x < 0 {
			x += viewW
		}
		a.g.rect(x, s.y, s.size, s.size, starColors[i%len(starColors)])
	}
}

// drawTerrain draws the visible slice of a level's mountain range.
func (a *app) drawTerrain(lv *level.Level, camX float64) {
	c := a.g.ctx
	first := int(math.Floor(camX / level.SegW))
	c.Call("beginPath")
	c.Call("moveTo", -40, viewH+4)
	for i := first; i <= first+level.ViewW/level.SegW+1; i++ {
		wx := float64(i * level.SegW)
		c.Call("lineTo", wx-camX, viewH-lv.HeightAt(wx))
	}
	c.Call("lineTo", viewW+40, viewH+4)
	a.g.fill(hsl(lv.Hue, 70, 9))
	c.Call("fill")
	a.g.stroke(hsl(lv.Hue, 90, 58))
	a.g.width(2)
	c.Call("stroke")
}

func (a *app) drawShip(x, y, dir float64, thrust bool, t float64) {
	g := a.g
	if thrust {
		col := "#fa3"
		if int(t*30)%2 == 0 {
			col = "#ff8"
		}
		g.poly(shipFlame, x, y, dir*(1+0.3*math.Sin(t*40)), 1, col, "")
	}
	g.poly(shipHull, x, y, dir, 1, "#123", "#dff")
	g.poly(shipCanopy, x, y, dir, 1, "#3cf", "")
}

func (a *app) drawColonist(x, y float64, t float64) {
	g := a.g
	g.rect(x-2, y-7, 4, 4, "#fc9") // head
	g.rect(x-3, y-3, 6, 6, "#f6b") // body
	g.rect(x-3, y+3, 2, 4, "#9cf") // legs
	g.rect(x+1, y+3, 2, 4, "#9cf")
}

// drawEnemy draws one enemy sprite. aim is the direction (radians) a turret points.
func (a *app) drawEnemy(k game.Kind, x, y, t, phase float64, flash bool, aim float64) {
	g := a.g
	col := k.Color()
	dark := "#111"
	if flash {
		col, dark = "#fff", "#fff"
	}
	switch k {
	case game.Snatcher:
		g.line(x-8, y+5, x-12, y+13, col, 1.6)
		g.line(x, y+5, x, y+13, col, 1.6)
		g.line(x+8, y+5, x+12, y+13, col, 1.6)
		g.poly(snatBody, x, y, 1, 1, dark, col)
		g.rect(x-5+3*math.Sin(t*4+phase), y-5, 6, 4, "#ff6")
	case game.Reaver:
		s := 1 + 0.12*math.Sin(t*14+phase)
		g.poly(reaverBody, x, y, s, s, "#402", col)
		g.rect(x-3, y-3, 6, 5, "#fff")
	case game.Bomber:
		g.poly(bomberBody, x, y, 1, 1, dark, col)
		c := "#f33"
		if int(t*4+phase)%2 == 0 {
			c = "#fe6"
		}
		g.poly(bomberCore, x, y, 1, 1, c, "")
	case game.Hive:
		g.poly(hiveBody, x, y, 1, 1, "#320", col)
		s := 0.45 + 0.12*math.Sin(t*5+phase)
		g.poly(hiveBody, x, y, s, s, "", "#f84")
		g.rect(x-2, y-2, 4, 4, "#f66")
	case game.Wasp:
		d := 1.0
		if math.Sin(t*9+phase) < 0 {
			d = -1
		}
		g.poly(waspBody, x, y, d, 1, "#400", col)
	case game.Stalker:
		g.poly(stalkDome, x, y, 1, 1, "#036", col)
		g.poly(stalkHull, x, y, 1, 1, dark, col)
		i := float64(int(t*8+phase) % 4)
		g.rect(x-11+i*6, y+1, 4, 3, "#fff")
	case game.Turret:
		g.line(x, y-2, x+math.Cos(aim)*15, y-2+math.Sin(aim)*15, col, 4)
		g.poly(turretBase, x, y, 1, 1, dark, col)
		g.circle(x, y-2, 5, "#520", col)
	case game.Dreadnought:
		g.poly(dreadHull, x, y, 1, 1, "#200", col)
		g.poly(dreadBridge, x, y, 1, 1, "#fa3", "")
		for i := 0; i < 5; i++ {
			c := "#600"
			if (int(t*6)+i)%5 == 0 {
				c = "#ff6"
			}
			g.rect(x-26+float64(i)*12, y+6, 5, 4, c)
		}
	}
}

func (a *app) drawGame() {
	g, gm := a.g, a.game
	g.rect(0, 0, viewW, viewH, "#000008")

	shx, shy := 0.0, 0.0
	if gm.Shake > 0 {
		shx, shy = (rand.Float64()-0.5)*14*gm.Shake, (rand.Float64()-0.5)*14*gm.Shake
		g.offset(shx, shy)
	}
	cam := gm.CamX
	center := cam + viewW/2
	sx := func(x float64) float64 { return gm.Dx(center, x) + viewW/2 }
	vis := func(x, margin float64) bool { return x > -margin && x < viewW+margin }

	a.drawStars(cam)
	if !gm.PlanetDead {
		a.drawTerrain(gm.L, cam)
	}

	for _, c := range gm.Colonists {
		if x := sx(c.X); !c.Dead && vis(x, 10) {
			a.drawColonist(x, c.Y, a.t)
		}
	}
	for _, m := range gm.Mines {
		if x := sx(m.X); vis(x, 10) {
			c := "#fa3"
			if int(m.Life*6)%2 == 0 {
				c = "#f33"
			}
			g.rect(x-5, m.Y-1.5, 10, 3, c)
			g.rect(x-1.5, m.Y-5, 3, 10, c)
		}
	}
	p := &gm.Player
	for _, e := range gm.Enemies {
		x := sx(e.X)
		if !vis(x, 60) {
			continue
		}
		aim := -math.Pi / 2
		if e.Kind == game.Turret && p.Alive {
			aim = math.Atan2(p.Y-e.Y, gm.Dx(e.X, p.X))
			if aim > 0 { // never point into the ground
				aim = -math.Pi / 2
			}
		}
		a.drawEnemy(e.Kind, x, e.Y, a.t, e.Phase, e.Flash > 0, aim)
		if e.MaxHP >= 10 {
			g.rect(x-30, e.Y-36, 60, 4, "#400")
			g.rect(x-30, e.Y-36, 60*float64(e.HP)/float64(e.MaxHP), 4, "#f44")
		}
	}
	for _, s := range gm.Shots {
		if x := sx(s.X); vis(x, 100) {
			d := 1.0
			if s.VX < 0 {
				d = -1
			}
			g.rect(x, s.Y-1, -d*s.Len, 2, "#f6f")
			g.rect(x, s.Y-1.5, -d*math.Min(s.Len, 22), 3, "#fff")
		}
	}
	for _, b := range gm.Bullets {
		if x := sx(b.X); vis(x, 6) {
			g.rect(x-3, b.Y-3, 6, 6, "#f33")
			g.rect(x-1.5, b.Y-1.5, 3, 3, "#ff8")
		}
	}
	for _, pt := range gm.Particles {
		if x := sx(pt.X); vis(x, 4) {
			s := pt.Size * (0.3 + 0.7*pt.Life/pt.Max)
			g.rect(x-s/2, pt.Y-s/2, s, s, pt.Color)
		}
	}
	if p.Alive && (p.Invuln <= 0 || int(a.t*14)%2 == 0) {
		a.drawShip(sx(p.X), p.Y, p.Dir, p.Thrust, a.t)
	}
	for _, pp := range gm.Popups {
		if x := sx(pp.X); vis(x, 40) {
			g.text(pp.Text, x, pp.Y, 14, "#ff8", "center")
		}
	}

	if gm.Shake > 0 {
		g.offset(0, 0)
	}
	if gm.Flash > 0 {
		g.setAlpha(math.Min(1, gm.Flash) * 0.85)
		g.rect(0, 0, viewW, viewH, "#fff")
		g.setAlpha(1)
	}
	a.drawHUD()
}

func (a *app) drawHUD() {
	g, gm := a.g, a.game
	g.rect(0, 0, viewW, 62, "#000")
	g.line(0, 62.5, viewW, 62.5, "#247", 1)

	g.text(pad(gm.Score, 7), 14, 20, 24, "#fff", "left")
	for i := 0; i < gm.Lives-1 && i < 6; i++ {
		g.poly(shipHull, 24+float64(i)*30, 46, 0.6, 0.6, "#123", "#dff")
	}
	for i := 0; i < gm.Bombs; i++ {
		g.circle(viewW-20-float64(i)*18, 46, 6, "#f84", "#ff6")
	}
	title := gm.L.Name
	if !a.single {
		title = "LEVEL " + strconv.Itoa(gm.LevelNum)
	}
	g.text(title, viewW-12, 14, 14, "#8cf", "right")
	g.text("ENEMIES "+strconv.Itoa(gm.EnemiesLeft()), viewW-12, 30, 12, "#f88", "right")
	g.text("COLONISTS "+strconv.Itoa(gm.AliveColonists()), 220, 46, 12, "#f9c", "right")

	// radar: the whole world, centred on the camera
	const rx, ry, rw, rh = 240.0, 6.0, 480.0, 50.0
	g.rect(rx, ry, rw, rh, "#00101c")
	center := gm.CamX + viewW/2
	mx := func(x float64) float64 { return rx + rw/2 + gm.Dx(center, x)/gm.W*rw }
	my := func(y float64) float64 {
		return ry + 2 + (y-game.TopY)/(viewH-game.TopY)*(rh-4)
	}
	if !gm.PlanetDead {
		col := hsl(gm.L.Hue, 70, 40)
		n := len(gm.L.Terrain)
		step := n/100 + 1
		for i := 0; i < n; i += step {
			wx := float64(i * level.SegW)
			g.rect(mx(wx), my(gm.L.GroundY(wx)), 2, 1.5, col)
		}
	}
	for _, c := range gm.Colonists {
		if !c.Dead {
			g.rect(mx(c.X)-1, my(c.Y)-2, 2, 3, "#f9c")
		}
	}
	for _, e := range gm.Enemies {
		s := 3.0
		if e.Kind == game.Dreadnought {
			s = 6
		}
		g.rect(mx(e.X)-s/2, my(e.Y)-s/2, s, s, e.Kind.Color())
	}
	if gm.Player.Alive {
		g.rect(mx(gm.Player.X)-2, my(gm.Player.Y)-1.5, 5, 3, "#fff")
	}
	vw := viewW / gm.W * rw
	g.box(rx+rw/2-vw/2, ry, vw, rh, "#69c", 1)
	g.box(rx, ry, rw, rh, "#247", 1)
}

func pad(n, width int) string {
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// banner draws big centred text with an optional subtitle.
func (a *app) banner(title, sub, col string) {
	a.g.text(title, viewW/2+2, 232, 44, "#000", "center")
	a.g.text(title, viewW/2, 230, 44, col, "center")
	if sub != "" {
		a.g.text(sub, viewW/2, 276, 18, "#fff", "center")
	}
}
