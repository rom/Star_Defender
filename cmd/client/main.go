//go:build js && wasm

// Command client is the Star Defender game, compiled to WebAssembly and run
// in the browser. It draws on a <canvas>, synthesises audio with Web Audio
// and talks to the server's JSON API for high scores and custom levels.
package main

import (
	"encoding/json"
	"math"
	"strconv"
	"syscall/js"
	"time"

	"stardefender/internal/game"
	"stardefender/internal/level"
)

const (
	modeTitle = iota
	modePlay
	modeName
	modeScores
	modeList
	modeEditor
)

type scoreRow struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
	Level int    `json:"level"`
}

type app struct {
	g      *gfx
	audio  *audio
	doc    js.Value
	canvas js.Value

	mode      int
	shownMode int  // last mode published to the page (see syncMode)
	touch     bool // a touch screen is in use: show on-screen controls
	t, last   float64
	acc       float64
	keys      map[string]bool
	tapped    map[string]bool // pressed since the last simulation step, even if already released

	game     *game.Game
	levels   []level.Level // the campaign
	levelIdx int
	single   bool // playing one level (practice or editor test), no high score
	singleLv level.Level
	returnTo int
	paused   bool
	victory  bool

	menuSel  int
	titleCam float64

	name       string
	submitting bool
	finalScore int
	finalLevel int

	scores    []scoreRow
	rank      int
	scoresMsg string

	list    []level.Summary
	listSel int
	listMsg string

	ed *editor
}

var menuItems = []string{"START GAME", "SINGLE LEVEL", "LEVEL EDITOR", "HIGH SCORES"}

func main() {
	doc := js.Global().Get("document")
	canvas := doc.Call("getElementById", "game")
	a := &app{
		g: newGfx(canvas), audio: newAudio(), doc: doc, canvas: canvas,
		keys: map[string]bool{}, tapped: map[string]bool{}, levels: level.Builtin(),
	}
	a.ed = newEditor(a)
	a.name = loadName()
	a.shownMode = -1
	a.bindInput()
	a.bindTouch()
	a.audio.setSong(0, 0)

	var frame js.Func
	frame = js.FuncOf(func(this js.Value, args []js.Value) any {
		a.frame(args[0].Float() / 1000)
		js.Global().Call("requestAnimationFrame", frame)
		return nil
	})
	js.Global().Call("requestAnimationFrame", frame)
	if l := doc.Call("getElementById", "loading"); !l.IsNull() {
		l.Call("remove")
	}
	select {}
}

// ---------------------------------------------------------------- input

func (a *app) listen(target js.Value, event string, fn func(e js.Value)) {
	target.Call("addEventListener", event, js.FuncOf(func(this js.Value, args []js.Value) any {
		fn(args[0])
		return nil
	}))
}

func isFormField(e js.Value) bool {
	t := e.Get("target")
	if t.IsNull() || t.IsUndefined() {
		return false
	}
	switch t.Get("tagName").String() {
	case "INPUT", "SELECT", "TEXTAREA", "BUTTON":
		return true
	}
	return false
}

var gameKeys = map[string]bool{
	"ArrowUp": true, "ArrowDown": true, "ArrowLeft": true, "ArrowRight": true,
	"Space": true, "Enter": true, "Backspace": true, "Tab": true,
}

func (a *app) mousePos(e js.Value) (float64, float64) {
	r := a.canvas.Call("getBoundingClientRect")
	w := r.Get("width").Float()
	if w == 0 {
		return 0, 0
	}
	s := viewW / w
	return (e.Get("clientX").Float() - r.Get("left").Float()) * s, (e.Get("clientY").Float() - r.Get("top").Float()) * s
}

func (a *app) bindInput() {
	win := js.Global()
	a.listen(win, "keydown", func(e js.Value) {
		a.audio.init()
		if isFormField(e) || e.Get("ctrlKey").Bool() || e.Get("metaKey").Bool() || e.Get("altKey").Bool() {
			return
		}
		code := e.Get("code").String()
		if gameKeys[code] {
			e.Call("preventDefault")
		}
		a.keys[code] = true
		a.tapped[code] = true
		if !e.Get("repeat").Bool() || a.mode == modeName || a.mode == modeList || a.mode == modeTitle {
			a.onKey(code, e.Get("key").String())
		}
	})
	a.listen(win, "keyup", func(e js.Value) { delete(a.keys, e.Get("code").String()) })
	a.listen(win, "blur", func(e js.Value) {
		a.keys = map[string]bool{}
		if a.mode == modePlay && a.game.State == game.Playing {
			a.paused = true
		}
	})
	a.listen(a.canvas, "pointerdown", func(e js.Value) {
		a.audio.init()
		// take focus away from any editor form field
		if ae := a.doc.Get("activeElement"); !ae.IsNull() && !ae.IsUndefined() {
			ae.Call("blur")
		}
		x, y := a.mousePos(e)
		switch a.mode {
		case modeTitle:
			if i := menuHit(x, y); i >= 0 {
				a.menuSel = i
				a.menuActivate()
			}
		case modeScores:
			a.toTitle()
		case modeList:
			if i := a.listTop() + int(math.Floor((y-159)/26)); y >= 159 && i < a.listTop()+listRows && i < len(a.list) {
				a.listSel = i
				a.playListed()
			}
		case modePlay:
			if a.paused {
				a.paused = false
			}
		case modeEditor:
			e.Call("preventDefault")
			a.ed.mouseDown(x, y, e.Get("button").Int())
		}
	})
	a.listen(a.canvas, "pointermove", func(e js.Value) {
		x, y := a.mousePos(e)
		switch a.mode {
		case modeTitle:
			if i := menuHit(x, y); i >= 0 && i != a.menuSel {
				a.menuSel = i
			}
		case modeEditor:
			a.ed.mouseMove(x, y)
		}
	})
	a.listen(win, "pointerup", func(e js.Value) { a.ed.drag = false })
	a.listen(win, "pointercancel", func(e js.Value) { a.ed.drag = false })
	a.listen(a.canvas, "pointerleave", func(e js.Value) { a.ed.inside = false })
	a.listen(a.canvas, "contextmenu", func(e js.Value) { e.Call("preventDefault") })
	a.canvas.Call("addEventListener", "wheel", js.FuncOf(func(this js.Value, args []js.Value) any {
		if a.mode == modeEditor {
			args[0].Call("preventDefault")
			a.ed.scroll(args[0].Get("deltaY").Float() + args[0].Get("deltaX").Float())
		}
		return nil
	}), map[string]any{"passive": false})
}

func menuHit(x, y float64) int {
	for i := range menuItems {
		if my := 300 + float64(i)*40; y > my-19 && y < my+19 && x > 300 && x < 660 {
			return i
		}
	}
	return -1
}

func (a *app) down(codes ...string) bool {
	for _, c := range codes {
		if a.keys[c] || a.tapped[c] {
			return true
		}
	}
	return false
}

func (a *app) onKey(code, key string) {
	if a.mode != modeName {
		switch code {
		case "KeyM":
			a.audio.toggleMute()
			return
		case "KeyN":
			a.audio.toggleMusic()
			return
		}
	}
	switch a.mode {
	case modeTitle:
		switch code {
		case "ArrowUp", "KeyW":
			a.menuSel = (a.menuSel + len(menuItems) - 1) % len(menuItems)
			a.audio.play("select")
		case "ArrowDown", "KeyS":
			a.menuSel = (a.menuSel + 1) % len(menuItems)
			a.audio.play("select")
		case "Enter", "Space":
			a.menuActivate()
		}
	case modePlay:
		switch code {
		case "KeyP":
			a.paused = !a.paused
		case "Escape":
			if a.single {
				a.finishSingle()
			} else {
				a.toTitle()
			}
		}
	case modeName:
		switch {
		case code == "Enter":
			a.submitScore()
		case code == "Backspace":
			if r := []rune(a.name); len(r) > 0 {
				a.name = string(r[:len(r)-1])
			}
		case code == "Escape":
			a.toTitle()
		default:
			if r := []rune(key); len(r) == 1 && r[0] >= 32 && r[0] != 127 && len([]rune(a.name)) < 12 {
				a.name += string(r)
			}
		}
	case modeScores:
		if code == "Enter" || code == "Escape" || code == "Space" {
			a.toTitle()
		}
	case modeList:
		switch code {
		case "ArrowUp", "KeyW":
			if a.listSel > 0 {
				a.listSel--
				a.audio.play("select")
			}
		case "ArrowDown", "KeyS":
			if a.listSel < len(a.list)-1 {
				a.listSel++
				a.audio.play("select")
			}
		case "PageUp":
			a.listSel = max(0, a.listSel-listRows)
		case "PageDown":
			a.listSel = max(0, min(len(a.list)-1, a.listSel+listRows))
		case "Enter", "Space":
			a.playListed()
		case "Escape":
			a.toTitle()
		}
	}
}

func (a *app) menuActivate() {
	a.audio.play("start")
	switch a.menuSel {
	case 0:
		a.startCampaign()
	case 1:
		a.openList()
	case 2:
		a.ed.open()
	case 3:
		a.showScores()
	}
}

// ---------------------------------------------------------------- flow

func (a *app) toTitle() {
	a.mode = modeTitle
	a.audio.setSong(0, 0)
}

func hasBoss(lv *level.Level) bool {
	for _, e := range lv.Entities {
		if e.Type == "dreadnought" {
			return true
		}
	}
	for _, w := range lv.Waves {
		if w.Type == "dreadnought" {
			return true
		}
	}
	return false
}

func (a *app) beginLevel(lv *level.Level, num int) {
	a.game.LoadLevel(lv, num)
	a.acc, a.paused = 0, false
	a.mode = modePlay
	if hasBoss(lv) {
		a.audio.setSong(3, float64(num))
	} else {
		a.audio.setSong(1+(num-1)%2, float64(num)*1.5)
	}
}

func (a *app) startCampaign() {
	a.single, a.victory = false, false
	a.levelIdx = 0
	a.game = game.New(time.Now().UnixNano())
	a.beginLevel(&a.levels[0], 1)
}

// startSingle plays one level on its own and then returns to mode returnTo.
func (a *app) startSingle(lv level.Level, returnTo int) {
	a.single, a.victory = true, false
	a.singleLv = lv
	a.returnTo = returnTo
	a.game = game.New(time.Now().UnixNano())
	a.beginLevel(&a.singleLv, 1)
}

func (a *app) finishSingle() {
	if a.returnTo == modeEditor {
		a.ed.resume()
		return
	}
	a.mode = modeList
	a.audio.setSong(0, 0)
}

func (a *app) updatePlay(dt float64) {
	gm := a.game
	if !a.paused {
		a.acc += dt
		in := game.Input{
			Left:  a.down("ArrowLeft", "KeyA"),
			Right: a.down("ArrowRight", "KeyD"),
			Up:    a.down("ArrowUp", "KeyW"),
			Down:  a.down("ArrowDown", "KeyS"),
			Fire:  a.down("Space", "KeyZ", "KeyJ"),
			Bomb:  a.down("KeyB", "KeyX", "ShiftLeft", "ShiftRight"),
			Warp:  a.down("KeyH", "KeyC"),
		}
		for a.acc >= game.DT {
			a.acc -= game.DT
			gm.Step(in)
			for _, ev := range gm.DrainEvents() {
				a.audio.play(ev)
			}
			clear(a.tapped)
		}
	} else {
		clear(a.tapped)
	}
	switch gm.State {
	case game.Clear:
		if gm.StateTime > 4.5 {
			switch {
			case a.single:
				a.finishSingle()
			case a.levelIdx+1 >= len(a.levels):
				a.victory = true
				a.toNameEntry()
			default:
				a.levelIdx++
				a.beginLevel(&a.levels[a.levelIdx], a.levelIdx+1)
			}
		}
	case game.GameOver:
		if gm.StateTime > 3.5 {
			switch {
			case a.single:
				a.finishSingle()
			case gm.Score > 0:
				a.toNameEntry()
			default:
				a.toTitle()
			}
		}
	}
}

func (a *app) toNameEntry() {
	a.finalScore, a.finalLevel = a.game.Score, a.levelIdx+1
	a.submitting = false
	a.mode = modeName
	a.audio.setSong(0, 0)
}

func (a *app) submitScore() {
	name := level.CleanText(a.name, 12)
	if name == "" || a.submitting {
		return
	}
	a.submitting = true
	saveName(name)
	body, _ := json.Marshal(map[string]any{"name": name, "score": a.finalScore, "level": a.finalLevel})
	request("POST", "/api/scores", body, func(status int, data []byte) {
		a.submitting = false
		if a.mode != modeName {
			return
		}
		var resp struct {
			Rank   int        `json:"rank"`
			Scores []scoreRow `json:"scores"`
		}
		if status != 200 || json.Unmarshal(data, &resp) != nil {
			a.showScores()
			a.scoresMsg = "COULD NOT SAVE YOUR SCORE"
			return
		}
		a.scores, a.rank, a.scoresMsg = resp.Scores, resp.Rank, ""
		a.mode = modeScores
	})
}

func (a *app) showScores() {
	a.mode = modeScores
	a.rank, a.scoresMsg, a.scores = 0, "LOADING...", nil
	request("GET", "/api/scores", nil, func(status int, data []byte) {
		if status != 200 || json.Unmarshal(data, &a.scores) != nil {
			if a.scoresMsg == "LOADING..." {
				a.scoresMsg = "HIGH SCORES UNAVAILABLE"
			}
			return
		}
		if a.scoresMsg == "LOADING..." {
			a.scoresMsg = ""
		}
	})
}

func (a *app) openList() {
	a.mode = modeList
	a.listMsg = "LOADING..."
	request("GET", "/api/levels", nil, func(status int, data []byte) {
		var l []level.Summary
		if status != 200 || json.Unmarshal(data, &l) != nil {
			// offline fallback: the campaign levels are compiled in
			for i := range a.levels {
				l = append(l, a.levels[i].Summary())
			}
		}
		a.list, a.listMsg = l, ""
		if a.listSel >= len(l) {
			a.listSel = 0
		}
	})
}

func (a *app) playListed() {
	if a.listSel >= len(a.list) || a.listMsg != "" {
		return
	}
	s := a.list[a.listSel]
	if s.Builtin {
		for i := range a.levels {
			if a.levels[i].ID == s.ID {
				a.audio.play("start")
				a.startSingle(a.levels[i].Clone(), modeList)
				return
			}
		}
	}
	a.listMsg = "LOADING..."
	request("GET", "/api/levels/"+s.ID, nil, func(status int, data []byte) {
		a.listMsg = ""
		var lv level.Level
		if status != 200 || json.Unmarshal(data, &lv) != nil || lv.Validate() != nil {
			a.listMsg = "COULD NOT LOAD THAT LEVEL"
			return
		}
		if a.mode == modeList {
			a.audio.play("start")
			a.startSingle(lv, modeList)
		}
	})
}

// ---------------------------------------------------------------- frame

func (a *app) frame(now float64) {
	dt := now - a.last
	a.last = now
	if dt > 0.1 || dt < 0 {
		dt = 0.1
	}
	a.t += dt
	a.audio.tick()
	a.syncMode()

	switch a.mode {
	case modePlay:
		a.updatePlay(dt)
		if a.mode == modePlay {
			a.drawPlay()
		}
	case modeEditor:
		a.ed.update(dt)
		a.ed.draw()
	default:
		a.titleCam += 40 * dt
		a.drawBackdrop()
		switch a.mode {
		case modeTitle:
			a.drawTitle()
		case modeName:
			a.drawName()
		case modeScores:
			a.drawScores()
		case modeList:
			a.drawList()
		}
	}
	if a.audio.muted {
		a.g.text("MUTED", viewW-10, viewH-12, 12, "#f66", "right")
	}
}

func (a *app) drawPlay() {
	a.drawGame()
	gm := a.game
	switch {
	case a.paused:
		if a.touch {
			a.banner("PAUSED", "TAP TO RESUME", "#8cf")
		} else {
			a.banner("PAUSED", "P TO RESUME · ESC TO QUIT", "#8cf")
		}
	case gm.State == game.Clear:
		sub := "COLONIST BONUS " + strconv.Itoa(gm.Bonus)
		if !a.single && a.levelIdx+1 >= len(a.levels) {
			a.banner("GALAXY SAVED!", sub, "#ff6")
		} else {
			a.banner("LEVEL COMPLETE", sub, "#6f8")
		}
	case gm.State == game.GameOver:
		a.banner("GAME OVER", "", "#f44")
	case gm.State == game.Playing && gm.Time < 2.6 && gm.Lives > 0:
		title := gm.L.Name
		if !a.single {
			title = "LEVEL " + strconv.Itoa(gm.LevelNum)
		}
		sub := gm.L.Name
		if a.single {
			sub = "PRACTICE"
			if a.returnTo == modeEditor {
				sub = "TEST PLAY · ESC RETURNS TO THE EDITOR"
				if a.touch {
					sub = "TEST PLAY · ✕ RETURNS TO THE EDITOR"
				}
			}
		}
		a.banner(title, sub, "#8cf")
	}
	if gm.PlanetDead && gm.State == game.Playing && int(a.t*2)%2 == 0 {
		a.g.text("PLANET LOST", viewW/2, 82, 14, "#f44", "center")
	}
}

func (a *app) drawBackdrop() {
	a.g.rect(0, 0, viewW, viewH, "#000008")
	a.drawStars(a.titleCam)
	a.drawTerrain(&a.levels[0], a.titleCam)
}

func (a *app) drawTitle() {
	g := a.g
	g.text("STAR DEFENDER", viewW/2+4, 154, 84, "#a0f", "center")
	g.text("STAR DEFENDER", viewW/2, 150, 84, "#6ff", "center")
	g.text("PROTECT THE COLONISTS · DESTROY THE INVADERS", viewW/2, 212, 16, "#f9c", "center")
	for i, it := range menuItems {
		y := 300 + float64(i)*40
		col := "#89a"
		if i == a.menuSel {
			col = "#fff"
			a.drawShip(viewW/2-150, y, 1, true, a.t)
		}
		g.text(it, viewW/2, y, 26, col, "center")
	}
	if a.touch {
		g.text("TAP TO CHOOSE", viewW/2, 478, 13, "#8cf", "center")
		g.text("IN FLIGHT: STICK ON THE LEFT, FIRE · BOMB · WARP ON THE RIGHT", viewW/2, 498, 13, "#8cf", "center")
	} else {
		g.text("ARROWS / WASD  FLY     SPACE  FIRE     B  SMART BOMB     H  WARP", viewW/2, 478, 13, "#8cf", "center")
		g.text("P  PAUSE     M  MUTE     N  MUSIC ON/OFF     ESC  QUIT", viewW/2, 498, 13, "#8cf", "center")
	}
}

func (a *app) drawName() {
	g := a.g
	if a.victory {
		g.text("VICTORY!", viewW/2, 120, 64, "#ff6", "center")
		g.text("ALL "+strconv.Itoa(len(a.levels))+" LEVELS CLEARED", viewW/2, 172, 18, "#fff", "center")
	} else {
		g.text("GAME OVER", viewW/2, 120, 64, "#f44", "center")
		g.text("YOU REACHED LEVEL "+strconv.Itoa(a.finalLevel), viewW/2, 172, 18, "#fff", "center")
	}
	g.text("SCORE  "+strconv.Itoa(a.finalScore), viewW/2, 230, 34, "#6ff", "center")
	g.text("ENTER YOUR NAME", viewW/2, 300, 18, "#8cf", "center")
	cursor := "_"
	if int(a.t*3)%2 == 0 || a.submitting {
		cursor = " "
	}
	if a.touch { // the HTML name field is shown instead
		if a.submitting {
			g.text("SAVING...", viewW/2, 470, 14, "#89a", "center")
		}
		return
	}
	g.text(a.name+cursor, viewW/2, 340, 34, "#fff", "center")
	msg := "ENTER TO SAVE · ESC TO SKIP"
	if a.submitting {
		msg = "SAVING..."
	}
	g.text(msg, viewW/2, 400, 14, "#89a", "center")
}

func (a *app) drawScores() {
	g := a.g
	g.text("HIGH SCORES", viewW/2, 90, 48, "#6ff", "center")
	if a.scoresMsg != "" {
		g.text(a.scoresMsg, viewW/2, 140, 16, "#f88", "center")
	} else if len(a.scores) == 0 {
		g.text("NO SCORES YET - BE THE FIRST!", viewW/2, 250, 18, "#fff", "center")
	}
	for i, s := range a.scores {
		if i >= 10 {
			break
		}
		y := 170 + float64(i)*30
		col := "#cde"
		if i+1 == a.rank {
			col = "#ff6"
			if int(a.t*4)%2 == 0 {
				col = "#fff"
			}
		}
		g.text(strconv.Itoa(i+1)+".", 290, y, 20, col, "right")
		g.text(s.Name, 310, y, 20, col, "left")
		g.text(strconv.Itoa(s.Score), 600, y, 20, col, "right")
		g.text("LV "+strconv.Itoa(s.Level), 680, y, 16, col, "right")
	}
	if a.rank > 10 {
		g.text("YOUR RANK: "+strconv.Itoa(a.rank), viewW/2, 478, 16, "#ff6", "center")
	}
	if a.touch {
		g.text("TAP TO CONTINUE", viewW/2, 506, 14, "#89a", "center")
	} else {
		g.text("PRESS ENTER", viewW/2, 506, 14, "#89a", "center")
	}
}

func (a *app) drawList() {
	g := a.g
	g.text("SINGLE LEVEL", viewW/2, 80, 44, "#6ff", "center")
	g.text("PRACTICE ANY LEVEL - SCORES ARE NOT RECORDED", viewW/2, 118, 13, "#89a", "center")
	if a.listMsg != "" {
		g.text(a.listMsg, viewW/2, 146, 15, "#f88", "center")
	}
	top := a.listTop()
	for i := top; i < len(a.list) && i < top+listRows; i++ {
		s := a.list[i]
		y := 172 + float64(i-top)*26
		col := "#9ab"
		if i == a.listSel {
			col = "#fff"
			g.rect(200, y-13, 560, 26, "#135")
		}
		label := s.Name
		if s.Builtin {
			label = strconv.Itoa(i+1) + ". " + label
		}
		g.text(label, 216, y, 17, col, "left")
		by := "CUSTOM"
		if s.Builtin {
			by = "CAMPAIGN"
		} else if s.Author != "" {
			by = "BY " + s.Author
		}
		g.text(by, 746, y, 13, col, "right")
	}
	if a.touch {
		g.text("TAP A LEVEL TO PLAY · ▲▼ SCROLL · ✕ BACK", viewW/2, 506, 14, "#89a", "center")
	} else {
		g.text("↑↓ SELECT · ENTER PLAY · ESC BACK", viewW/2, 506, 14, "#89a", "center")
	}
}

const listRows = 12

// listTop is the index of the first visible row of the level list.
func (a *app) listTop() int {
	top := a.listSel - listRows/2
	if top > len(a.list)-listRows {
		top = len(a.list) - listRows
	}
	if top < 0 {
		top = 0
	}
	return top
}

// ---------------------------------------------------------------- storage

// The player's name is remembered between games. Storage may be blocked
// (private mode, sandboxed frames), in which case this quietly does nothing.
func loadName() (name string) {
	defer func() { recover() }()
	v := js.Global().Get("localStorage").Call("getItem", "stardefender.name")
	if v.Type() == js.TypeString {
		return level.CleanText(v.String(), 12)
	}
	return ""
}

func saveName(name string) {
	defer func() { recover() }()
	js.Global().Get("localStorage").Call("setItem", "stardefender.name", name)
}
