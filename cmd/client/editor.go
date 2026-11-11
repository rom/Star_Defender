//go:build js && wasm

package main

import (
	"encoding/json"
	"math"
	"math/rand"
	"strconv"
	"syscall/js"
	"time"

	"stardefender/internal/game"
	"stardefender/internal/level"
)

// editor is the level editor: the level is drawn on the game canvas and the
// settings live in an HTML side panel (see web/index.html).
type editor struct {
	a      *app
	lv     level.Level
	cam    float64 // world x of the left screen edge
	tool   string
	mx, my float64
	inside bool
	drag   bool
	last   int // last terrain segment edited during a drag
	dirty  bool
	rng    *rand.Rand
}

const minimapH = 62.0

func newEditor(a *app) *editor {
	e := &editor{a: a, tool: "terrain", rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
	e.lv = level.Blank()
	e.lv.Terrain = level.GenTerrain(e.rng, len(e.lv.Terrain), 40)
	e.cam = e.lv.Width()/2 - viewW/2
	e.bind()
	return e
}

func (e *editor) el(id string) js.Value { return e.a.doc.Call("getElementById", id) }

func (e *editor) status(msg string, isErr bool) {
	s := e.el("ed-status")
	s.Set("textContent", msg)
	cls := ""
	if isErr {
		cls = "err"
	}
	s.Set("className", cls)
}

func (e *editor) open() {
	e.a.mode = modeEditor
	e.a.audio.setSong(-1, 0)
	e.a.doc.Get("body").Get("classList").Call("add", "editing")
	e.syncForm()
	e.refreshList()
	e.status("Pick a tool, then click or drag in the level.", false)
}

// resume returns to the editor after a test play.
func (e *editor) resume() {
	e.a.mode = modeEditor
	e.a.audio.setSong(-1, 0)
	e.a.doc.Get("body").Get("classList").Call("add", "editing")
}

func (e *editor) hide() { e.a.doc.Get("body").Get("classList").Call("remove", "editing") }

func (e *editor) close() {
	if e.dirty && !js.Global().Call("confirm", "Leave the editor? Unsaved changes stay in the editor only until you reload the page.").Bool() {
		return
	}
	e.hide()
	e.a.toTitle()
}

// ---------------------------------------------------------------- form

func (e *editor) syncForm() {
	e.el("ed-name").Set("value", e.lv.Name)
	e.el("ed-author").Set("value", e.lv.Author)
	e.el("ed-screens").Set("value", len(e.lv.Terrain)*level.SegW/level.ViewW)
	e.el("ed-speed").Set("value", e.lv.Speed)
	e.el("ed-aggr").Set("value", e.lv.Aggression)
	e.el("ed-hue").Set("value", e.lv.Hue)
	e.rebuildWaves()
	e.markTool()
}

func (e *editor) markTool() {
	btns := e.el("ed-tools").Call("querySelectorAll", "button")
	for i := 0; i < btns.Length(); i++ {
		b := btns.Index(i)
		b.Get("classList").Call("toggle", "on", b.Get("dataset").Get("tool").String() == e.tool)
	}
}

func (e *editor) rebuildWaves() {
	doc := e.a.doc
	body := e.el("ed-waves")
	body.Set("textContent", "")
	for i, w := range e.lv.Waves {
		idx := strconv.Itoa(i)
		tr := doc.Call("createElement", "tr")
		cell := func(child js.Value) {
			td := doc.Call("createElement", "td")
			td.Call("appendChild", child)
			tr.Call("appendChild", td)
		}
		num := func(field string, v float64, min, max int) js.Value {
			in := doc.Call("createElement", "input")
			in.Set("type", "number")
			in.Set("min", min)
			in.Set("max", max)
			in.Set("value", v)
			in.Get("dataset").Set("idx", idx)
			in.Get("dataset").Set("field", field)
			return in
		}
		cell(num("time", w.Time, 0, level.MaxWaveTime))
		sel := doc.Call("createElement", "select")
		sel.Get("dataset").Set("idx", idx)
		sel.Get("dataset").Set("field", "type")
		for _, t := range level.EnemyTypes {
			if t == "turret" {
				continue
			}
			o := doc.Call("createElement", "option")
			o.Set("value", t)
			o.Set("textContent", t)
			sel.Call("appendChild", o)
		}
		sel.Set("value", w.Type)
		cell(sel)
		cell(num("count", float64(w.Count), 1, level.MaxWaveCount))
		del := doc.Call("createElement", "button")
		del.Set("textContent", "✕")
		del.Set("title", "Remove this wave")
		del.Get("dataset").Set("del", idx)
		cell(del)
		body.Call("appendChild", tr)
	}
}

func (e *editor) refreshList() {
	request("GET", "/api/levels", nil, func(status int, data []byte) {
		var list []level.Summary
		if status != 200 || json.Unmarshal(data, &list) != nil {
			e.status("Could not fetch the level list from the server.", true)
			return
		}
		sel := e.el("ed-load")
		sel.Set("textContent", "")
		for i, s := range list {
			o := e.a.doc.Call("createElement", "option")
			o.Set("value", s.ID)
			label := s.Name
			if s.Builtin {
				label = "Campaign " + strconv.Itoa(i+1) + ": " + label
			} else if s.Author != "" {
				label += " — " + s.Author
			}
			o.Set("textContent", label)
			sel.Call("appendChild", o)
		}
		if e.lv.ID != "" {
			sel.Set("value", e.lv.ID)
		}
	})
}

func fieldFloat(v js.Value, lo, hi float64) float64 {
	f, err := strconv.ParseFloat(v.Get("value").String(), 64)
	if err != nil || math.IsNaN(f) {
		f = lo
	}
	return math.Max(lo, math.Min(hi, f))
}

func (e *editor) bind() {
	a := e.a
	click := func(id string, fn func()) {
		a.listen(e.el(id), "click", func(js.Value) { fn() })
	}
	input := func(id string, fn func(v js.Value)) {
		a.listen(e.el(id), "input", func(ev js.Value) { fn(ev.Get("target")); e.dirty = true })
	}
	input("ed-name", func(v js.Value) { e.lv.Name = level.CleanText(v.Get("value").String(), 32) })
	input("ed-author", func(v js.Value) { e.lv.Author = level.CleanText(v.Get("value").String(), 24) })
	input("ed-speed", func(v js.Value) { e.lv.Speed = fieldFloat(v, 0.5, 2.5) })
	input("ed-aggr", func(v js.Value) { e.lv.Aggression = fieldFloat(v, 0.2, 2.5) })
	input("ed-hue", func(v js.Value) { e.lv.Hue = int(fieldFloat(v, 0, 359)) })
	a.listen(e.el("ed-screens"), "change", func(ev js.Value) {
		e.resize(int(fieldFloat(ev.Get("target"), 2, 12)))
		e.syncForm()
	})

	a.listen(e.el("ed-tools"), "click", func(ev js.Value) {
		if t := ev.Get("target").Get("dataset").Get("tool"); t.Type() == js.TypeString {
			e.tool = t.String()
			e.markTool()
		}
	})

	a.listen(e.el("ed-waves"), "change", func(ev js.Value) {
		t := ev.Get("target")
		ds := t.Get("dataset")
		if ds.Get("field").Type() != js.TypeString {
			return
		}
		i, err := strconv.Atoi(ds.Get("idx").String())
		if err != nil || i < 0 || i >= len(e.lv.Waves) {
			return
		}
		w := &e.lv.Waves[i]
		switch ds.Get("field").String() {
		case "time":
			w.Time = math.Round(fieldFloat(t, 0, level.MaxWaveTime))
			t.Set("value", w.Time)
		case "count":
			w.Count = int(fieldFloat(t, 1, level.MaxWaveCount))
			t.Set("value", w.Count)
		case "type":
			if v := t.Get("value").String(); level.ValidType(v) && v != "turret" {
				w.Type = v
			}
		}
		e.dirty = true
	})
	a.listen(e.el("ed-waves"), "click", func(ev js.Value) {
		d := ev.Get("target").Get("dataset").Get("del")
		if d.Type() != js.TypeString {
			return
		}
		if i, err := strconv.Atoi(d.String()); err == nil && i >= 0 && i < len(e.lv.Waves) {
			e.lv.Waves = append(e.lv.Waves[:i], e.lv.Waves[i+1:]...)
			e.dirty = true
			e.rebuildWaves()
		}
	})
	click("ed-addwave", func() {
		if len(e.lv.Waves) >= level.MaxWaves {
			e.status("That is the maximum number of waves.", true)
			return
		}
		t := 10.0
		if n := len(e.lv.Waves); n > 0 {
			t = math.Min(e.lv.Waves[n-1].Time+15, level.MaxWaveTime)
		}
		e.lv.Waves = append(e.lv.Waves, level.Wave{Time: t, Type: "snatcher", Count: 3})
		e.dirty = true
		e.rebuildWaves()
	})

	click("ed-random", func() {
		e.lv.Terrain = level.GenTerrain(e.rng, len(e.lv.Terrain), 15+e.rng.Float64()*75)
		e.dirty = true
	})
	click("ed-smooth", func() {
		n := len(e.lv.Terrain)
		out := make([]int, n)
		for i := range out {
			out[i] = (e.lv.Terrain[(i+n-1)%n] + 2*e.lv.Terrain[i] + e.lv.Terrain[(i+1)%n] + 2) / 4
		}
		e.lv.Terrain = out
		e.dirty = true
	})
	click("ed-flat", func() {
		for i := range e.lv.Terrain {
			e.lv.Terrain[i] = 60
		}
		e.dirty = true
	})

	click("ed-new", func() {
		if e.dirty && !js.Global().Call("confirm", "Discard the current level?").Bool() {
			return
		}
		author := e.lv.Author
		e.lv = level.Blank()
		e.lv.Author = author
		e.lv.Terrain = level.GenTerrain(e.rng, len(e.lv.Terrain), 40)
		e.cam = e.lv.Width()/2 - viewW/2
		e.dirty = false
		e.syncForm()
		e.status("New level.", false)
	})
	click("ed-loadbtn", e.load)
	click("ed-save", e.save)
	click("ed-delete", e.remove)
	click("ed-test", func() {
		if err := e.lv.Validate(); err != nil {
			e.status("Cannot test: "+err.Error()+".", true)
			return
		}
		e.hide()
		a.audio.play("start")
		a.startSingle(e.lv.Clone(), modeEditor)
	})
	click("ed-exit", e.close)
}

// resize changes the world width, resampling the terrain to fit.
func (e *editor) resize(screens int) {
	n := screens * level.ViewW / level.SegW
	old := e.lv.Clone()
	if n == len(old.Terrain) {
		return
	}
	e.lv.Terrain = make([]int, n)
	for i := range e.lv.Terrain {
		e.lv.Terrain[i] = int(math.Round(old.HeightAt(float64(i) / float64(n) * old.Width())))
	}
	w := e.lv.Width()
	e.lv.Colonists = e.lv.Colonists[:0]
	for _, x := range old.Colonists {
		if x < w {
			e.lv.Colonists = append(e.lv.Colonists, x)
		}
	}
	e.lv.Entities = e.lv.Entities[:0]
	for _, en := range old.Entities {
		if en.X < w {
			e.lv.Entities = append(e.lv.Entities, en)
		}
	}
	e.cam = e.wrap(e.cam)
	e.dirty = true
}

// ---------------------------------------------------------------- server

func apiError(data []byte, fallback string) string {
	var r struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &r) == nil && r.Error != "" {
		return r.Error
	}
	return fallback
}

func (e *editor) load() {
	id := e.el("ed-load").Get("value").String()
	if id == "" {
		return
	}
	if e.dirty && !js.Global().Call("confirm", "Discard the current level and load another?").Bool() {
		return
	}
	request("GET", "/api/levels/"+id, nil, func(status int, data []byte) {
		var lv level.Level
		if status != 200 || json.Unmarshal(data, &lv) != nil || lv.Validate() != nil {
			e.status("Load failed: "+apiError(data, "server unreachable")+".", true)
			return
		}
		msg := "Loaded “" + lv.Name + "”."
		if lv.Builtin {
			// campaign levels are templates: saving creates your own copy
			lv.ID, lv.Builtin, lv.Author = "", false, e.lv.Author
			msg += " Saving will create your own copy."
		}
		if lv.Colonists == nil {
			lv.Colonists = []float64{}
		}
		if lv.Entities == nil {
			lv.Entities = []level.Entity{}
		}
		if lv.Waves == nil {
			lv.Waves = []level.Wave{}
		}
		e.lv = lv
		e.cam = lv.Width()/2 - viewW/2
		e.dirty = false
		e.syncForm()
		e.status(msg, false)
	})
}

func (e *editor) save() {
	if err := e.lv.Validate(); err != nil {
		e.status("Cannot save: "+err.Error()+".", true)
		return
	}
	body, _ := json.Marshal(e.lv)
	e.status("Saving...", false)
	request("POST", "/api/levels", body, func(status int, data []byte) {
		var saved level.Level
		if status != 200 || json.Unmarshal(data, &saved) != nil || saved.ID == "" {
			e.status("Save failed: "+apiError(data, "server unreachable")+".", true)
			return
		}
		e.lv.ID = saved.ID
		e.dirty = false
		e.status("Saved. Play it from SINGLE LEVEL on the title screen.", false)
		e.refreshList()
	})
}

func (e *editor) remove() {
	if e.lv.ID == "" {
		e.status("This level has not been saved, so there is nothing to delete.", true)
		return
	}
	if !js.Global().Call("confirm", "Delete “"+e.lv.Name+"” from the server?").Bool() {
		return
	}
	request("DELETE", "/api/levels/"+e.lv.ID, nil, func(status int, data []byte) {
		if status != 200 {
			e.status("Delete failed: "+apiError(data, "server unreachable")+".", true)
			return
		}
		e.lv.ID = ""
		e.dirty = true
		e.status("Deleted from the server. The level is still open here.", false)
		e.refreshList()
	})
}

// ---------------------------------------------------------------- canvas

func (e *editor) wrap(x float64) float64 {
	w := e.lv.Width()
	x = math.Mod(x, w)
	if x < 0 {
		x += w
	}
	if x >= w { // a tiny negative x rounds up to exactly the width
		x = 0
	}
	return x
}

func (e *editor) dx(a, b float64) float64 {
	w := e.lv.Width()
	d := math.Mod(b-a, w)
	if d > w/2 {
		d -= w
	} else if d < -w/2 {
		d += w
	}
	return d
}

func (e *editor) sx(x float64) float64 { return e.dx(e.cam+viewW/2, x) + viewW/2 }

func (e *editor) scroll(d float64) { e.cam = e.wrap(e.cam + d) }

func (e *editor) update(dt float64) {
	a := e.a
	v := 0.0
	if a.down("ArrowLeft", "KeyA") {
		v -= 900
	}
	if a.down("ArrowRight", "KeyD") {
		v += 900
	}
	e.scroll(v * dt)
	clear(a.tapped)
}

func (e *editor) setTerrain(x, y float64) {
	n := len(e.lv.Terrain)
	seg := int(math.Round(e.wrap(e.cam+x)/level.SegW)) % n
	h := int(math.Max(level.GroundMin, math.Min(level.GroundMax, viewH-y)))
	if !e.drag {
		e.last = seg
	}
	// fill every segment between the previous one and this one (shortest way round)
	d := seg - e.last
	if d > n/2 {
		d -= n
	} else if d < -n/2 {
		d += n
	}
	step := 1
	if d < 0 {
		step = -1
	}
	for i := e.last; ; i = ((i+step)%n + n) % n {
		e.lv.Terrain[i] = h
		if i == seg {
			break
		}
	}
	e.last = seg
	e.dirty = true
}

func (e *editor) erase(x, y float64) {
	wx := e.wrap(e.cam + x)
	best, bi, isCol := 28.0, -1, false
	for i, c := range e.lv.Colonists {
		if d := math.Hypot(e.dx(wx, c), y-(e.lv.GroundY(c)-7)); d < best {
			best, bi, isCol = d, i, true
		}
	}
	for i, en := range e.lv.Entities {
		if d := math.Hypot(e.dx(wx, en.X), y-e.entityY(en)); d < best {
			best, bi, isCol = d, i, false
		}
	}
	if bi < 0 {
		return
	}
	if isCol {
		e.lv.Colonists = append(e.lv.Colonists[:bi], e.lv.Colonists[bi+1:]...)
	} else {
		e.lv.Entities = append(e.lv.Entities[:bi], e.lv.Entities[bi+1:]...)
	}
	e.dirty = true
}

func (e *editor) entityY(en level.Entity) float64 {
	if en.Type == "turret" {
		return e.lv.GroundY(en.X) - 9
	}
	return en.Y
}

func (e *editor) mouseDown(x, y float64, button int) {
	e.mx, e.my, e.inside = x, y, true
	if y < minimapH {
		e.cam = e.wrap(x/viewW*e.lv.Width() - viewW/2)
		return
	}
	if button == 2 {
		e.erase(x, y)
		return
	}
	if button != 0 {
		return
	}
	wx := math.Floor(e.wrap(e.cam + x))
	switch e.tool {
	case "terrain":
		e.setTerrain(x, y)
		e.drag = true
	case "erase":
		e.erase(x, y)
	case "colonist":
		if len(e.lv.Colonists) >= level.MaxColonists {
			e.status("That is the maximum number of colonists.", true)
			return
		}
		e.lv.Colonists = append(e.lv.Colonists, wx)
		e.dirty = true
	default:
		if !level.ValidType(e.tool) {
			return
		}
		if len(e.lv.Entities) >= level.MaxEntities {
			e.status("That is the maximum number of placed enemies.", true)
			return
		}
		ey := math.Floor(math.Max(level.SkyTop+20, math.Min(e.lv.GroundY(wx)-20, y)))
		e.lv.Entities = append(e.lv.Entities, level.Entity{Type: e.tool, X: wx, Y: ey})
		e.dirty = true
	}
}

func (e *editor) mouseMove(x, y float64) {
	e.mx, e.my, e.inside = x, y, true
	if e.drag && e.tool == "terrain" && y >= minimapH {
		e.setTerrain(x, y)
	}
}

func (e *editor) draw() {
	a, g := e.a, e.a.g
	lv := &e.lv
	w := lv.Width()
	g.rect(0, 0, viewW, viewH, "#000008")
	a.drawStars(e.cam)
	a.drawTerrain(lv, e.cam)
	vis := func(x float64) bool { return x > -60 && x < viewW+60 }

	// where the player will appear
	if x := e.sx(w / 2); vis(x) {
		g.setAlpha(0.55)
		a.drawShip(x, 230, 1, false, 0)
		g.text("START", x, 252, 11, "#dff", "center")
		g.setAlpha(1)
	}
	for _, c := range lv.Colonists {
		if x := e.sx(c); vis(x) {
			a.drawColonist(x, lv.GroundY(c)-7, 0)
		}
	}
	for _, en := range lv.Entities {
		if x := e.sx(en.X); vis(x) {
			if k, ok := game.KindOf(en.Type); ok {
				a.drawEnemy(k, x, e.entityY(en), a.t, 0, false, -math.Pi/2)
			}
		}
	}

	// tool preview under the cursor
	if e.inside && e.my >= minimapH {
		g.setAlpha(0.5)
		wx := e.wrap(e.cam + e.mx)
		switch e.tool {
		case "terrain":
			g.line(e.mx, e.my, e.mx, viewH, "#fff", 1)
			g.rect(e.mx-4, e.my-1, 8, 2, "#fff")
		case "colonist":
			a.drawColonist(e.mx, lv.GroundY(wx)-7, 0)
		case "erase":
			g.circle(e.mx, e.my, 14, "", "#f44")
			g.line(e.mx-9, e.my-9, e.mx+9, e.my+9, "#f44", 2)
		default:
			if k, ok := game.KindOf(e.tool); ok {
				y := math.Max(level.SkyTop+20, math.Min(lv.GroundY(wx)-20, e.my))
				if k == game.Turret {
					y = lv.GroundY(wx) - 9
				}
				a.drawEnemy(k, e.mx, y, a.t, 0, false, -math.Pi/2)
			}
		}
		g.setAlpha(1)
	}

	// minimap of the whole level
	g.rect(0, 0, viewW, minimapH, "#00101c")
	col := hsl(lv.Hue, 70, 45)
	n := len(lv.Terrain)
	for i := 0; i < n; i += n/240 + 1 {
		h := float64(lv.Terrain[i]) / viewH * minimapH
		g.rect(float64(i)/float64(n)*viewW, minimapH-h, viewW/float64(n)*float64(n/240+1)+0.5, h, col)
	}
	my := func(y float64) float64 { return y / viewH * minimapH }
	for _, c := range lv.Colonists {
		g.rect(c/w*viewW-1, my(lv.GroundY(c))-4, 2, 4, "#f9c")
	}
	for _, en := range lv.Entities {
		if k, ok := game.KindOf(en.Type); ok {
			g.rect(en.X/w*viewW-2, my(e.entityY(en))-2, 4, 4, k.Color())
		}
	}
	g.rect(w/2/w*viewW-1, my(230)-1, 3, 3, "#fff")
	vx, vw := e.cam/w*viewW, viewW/w*viewW
	g.box(vx, 1, vw, minimapH-2, "#fff", 1.5)
	if vx+vw > viewW { // the view straddles the wrap-around seam
		g.box(vx-viewW, 1, vw, minimapH-2, "#fff", 1.5)
	}
	g.line(0, minimapH+0.5, viewW, minimapH+0.5, "#247", 1)

	title := "LEVEL EDITOR · " + lv.Name
	if e.dirty {
		title += " *"
	}
	g.text(title, 10, minimapH+16, 13, "#8cf", "left")
	g.text(strconv.Itoa(len(lv.Colonists))+" COLONISTS · "+strconv.Itoa(len(lv.Entities))+" ENEMIES · "+strconv.Itoa(len(lv.Waves))+" WAVES",
		viewW-10, minimapH+16, 13, "#8cf", "right")
	g.text("A/D OR WHEEL: SCROLL · CLICK MINIMAP: JUMP · RIGHT-CLICK: ERASE", viewW/2, viewH-10, 11, "#678", "center")
}
