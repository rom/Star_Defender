// Package game is the platform-independent simulation: it knows nothing
// about canvases, audio or the network. The browser client feeds it input
// once per fixed step and then draws whatever state it finds.
package game

import (
	"math"
	"math/rand"
	"sort"

	"stardefender/internal/level"
)

const (
	DT    = 1.0 / 60 // fixed simulation step
	ViewW = float64(level.ViewW)
	ViewH = float64(level.ViewH)
	TopY  = float64(level.SkyTop)

	StartLives = 3
	StartBombs = 3
	MaxBombs   = 5
	ExtraEvery = 10000 // points per bonus ship
)

type Kind int

const (
	Snatcher Kind = iota
	Reaver
	Bomber
	Hive
	Wasp
	Stalker
	Turret
	Dreadnought
)

var kindByName = map[string]Kind{
	"snatcher": Snatcher, "reaver": Reaver, "bomber": Bomber, "hive": Hive,
	"wasp": Wasp, "stalker": Stalker, "turret": Turret, "dreadnought": Dreadnought,
}

// KindOf maps a level-file type name to a Kind.
func KindOf(name string) (Kind, bool) { k, ok := kindByName[name]; return k, ok }

type kindInfo struct {
	r     float64
	hp    int
	score int
	color string
}

var kinds = [...]kindInfo{
	Snatcher:    {13, 1, 150, "#5f5"},
	Reaver:      {13, 1, 200, "#f5f"},
	Bomber:      {14, 1, 250, "#fa3"},
	Hive:        {16, 1, 1000, "#fe4"},
	Wasp:        {7, 1, 150, "#f66"},
	Stalker:     {13, 2, 400, "#6cf"},
	Turret:      {13, 3, 300, "#f84"},
	Dreadnought: {40, 30, 5000, "#f44"},
}

// Color is the signature colour of an enemy kind (radar, explosions).
func (k Kind) Color() string { return kinds[k].color }

type State int

const (
	Playing State = iota
	Dying
	Clear
	GameOver
)

type Input struct {
	Left, Right, Up, Down bool
	Fire, Bomb, Warp      bool
}

type Player struct {
	X, Y, VX, VY float64
	Dir          float64 // +1 facing right, -1 facing left
	Alive        bool
	Invuln       float64
	Thrust       bool
	Carry        *Colonist
	fireCD       float64
	warpCD       float64
}

type Enemy struct {
	Kind         Kind
	X, Y, VX, VY float64
	R            float64
	HP, MaxHP    int
	Phase        float64
	Flash        float64 // >0 just after being hit
	Carrying     *Colonist
	Dead         bool
	target       *Colonist
	state        int
	wait         float64
	fire         float64
	aux          float64
	y0           float64
	dir          float64
}

type ColState int

const (
	OnGround ColState = iota
	Grabbed
	Falling
	Carried
)

type Colonist struct {
	X, Y     float64
	VY       float64
	State    ColState
	Dead     bool
	dir      float64
	fallFrom float64
	claimed  *Enemy
}

type Shot struct {
	X, Y, VX, Life float64
	Len            float64
}

type Bullet struct{ X, Y, VX, VY, Life float64 }

type Mine struct{ X, Y, Life float64 }

type Particle struct {
	X, Y, VX, VY float64
	Life, Max    float64
	Size         float64
	Color        string
}

type Popup struct {
	X, Y, Life float64
	Text       string
}

type Game struct {
	L        *level.Level
	LevelNum int
	W        float64

	Player    Player
	Enemies   []*Enemy
	Colonists []*Colonist
	Shots     []Shot
	Bullets   []Bullet
	Mines     []Mine
	Particles []Particle
	Popups    []Popup

	Score, Lives, Bombs int
	Bonus               int // level-clear bonus, valid in State Clear
	State               State
	StateTime           float64
	Time                float64
	CamX                float64
	Flash               float64 // white screen flash, 1 → 0
	Shake               float64
	PlanetDead          bool
	Events              []string // sound cues since the last DrainEvents

	rng        *rand.Rand
	waves      []level.Wave
	waveIdx    int
	hadColony  bool
	prev       Input
	camOff     float64
	hurry      float64
	nextExtra  int
	lastWaveAt float64
}

func New(seed int64) *Game {
	return &Game{
		rng:       rand.New(rand.NewSource(seed)),
		Lives:     StartLives,
		Bombs:     StartBombs,
		nextExtra: ExtraEvery,
	}
}

// LoadLevel resets the world for lv, keeping score, lives and bombs.
func (g *Game) LoadLevel(lv *level.Level, num int) {
	g.L, g.LevelNum, g.W = lv, num, lv.Width()
	g.Enemies, g.Colonists = nil, nil
	g.Shots, g.Bullets, g.Mines = g.Shots[:0], g.Bullets[:0], g.Mines[:0]
	g.Particles, g.Popups = g.Particles[:0], g.Popups[:0]
	g.State, g.StateTime, g.Time = Playing, 0, 0
	g.PlanetDead, g.Flash, g.Shake, g.Bonus = false, 0, 0, 0
	g.hurry = 0

	g.Player = Player{X: g.W / 2, Y: 230, Dir: 1, Alive: true, Invuln: 3}
	g.camOff = ViewW * 0.35
	g.CamX = g.Player.X - g.camOff

	for _, x := range lv.Colonists {
		d := 1.0
		if g.rng.Intn(2) == 0 {
			d = -1
		}
		g.Colonists = append(g.Colonists, &Colonist{X: x, Y: lv.GroundY(x) - 7, dir: d})
	}
	g.hadColony = len(g.Colonists) > 0

	for _, e := range lv.Entities {
		if k, ok := KindOf(e.Type); ok {
			g.spawn(k, e.X, e.Y)
		}
	}
	g.waves = append(g.waves[:0], lv.Waves...)
	sort.SliceStable(g.waves, func(i, j int) bool { return g.waves[i].Time < g.waves[j].Time })
	g.waveIdx = 0
	g.lastWaveAt = 0
	if n := len(g.waves); n > 0 {
		g.lastWaveAt = g.waves[n-1].Time
	}
}

// DrainEvents returns and clears the pending sound cues.
func (g *Game) DrainEvents() []string {
	ev := g.Events
	g.Events = nil
	return ev
}

func (g *Game) emit(name string) { g.Events = append(g.Events, name) }

// Wrap brings x into [0, W).
func (g *Game) Wrap(x float64) float64 {
	x = math.Mod(x, g.W)
	if x < 0 {
		x += g.W
	}
	if x >= g.W { // a tiny negative x rounds up to exactly the width
		x = 0
	}
	return x
}

// Dx is the shortest signed horizontal distance from a to b in the wrapping world.
func (g *Game) Dx(a, b float64) float64 {
	d := math.Mod(b-a, g.W)
	if d > g.W/2 {
		d -= g.W
	} else if d < -g.W/2 {
		d += g.W
	}
	return d
}

// GroundY is the screen y of the surface at x (far below the screen once the planet is gone).
func (g *Game) GroundY(x float64) float64 {
	if g.PlanetDead {
		return ViewH + 40
	}
	return g.L.GroundY(x)
}

func (g *Game) floorFor(x, margin float64) float64 {
	return math.Min(g.GroundY(x), ViewH) - margin
}

// AliveColonists counts the colonists that are still alive.
func (g *Game) AliveColonists() int {
	n := 0
	for _, c := range g.Colonists {
		if !c.Dead {
			n++
		}
	}
	return n
}

// EnemiesLeft counts living enemies plus those still due to warp in.
func (g *Game) EnemiesLeft() int {
	n := len(g.Enemies)
	for _, w := range g.waves[g.waveIdx:] {
		n += w.Count
	}
	return n
}

func (g *Game) rnd(a, b float64) float64 { return a + g.rng.Float64()*(b-a) }

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

// approach moves v toward target by at most step.
func approach(v, target, step float64) float64 {
	if v < target {
		return math.Min(v+step, target)
	}
	return math.Max(v-step, target)
}

// ---------------------------------------------------------------- effects

const maxParticles = 700

func (g *Game) burst(x, y float64, color string, n int, speed float64) {
	for i := 0; i < n && len(g.Particles) < maxParticles; i++ {
		a := g.rng.Float64() * 2 * math.Pi
		s := speed * (0.25 + g.rng.Float64())
		life := g.rnd(0.3, 0.9)
		g.Particles = append(g.Particles, Particle{
			X: x, Y: y, VX: math.Cos(a) * s, VY: math.Sin(a) * s,
			Life: life, Max: life, Size: g.rnd(1.5, 3.5), Color: color,
		})
	}
}

func (g *Game) addScore(n int, x, y float64, popup bool) {
	if g.State == GameOver {
		return
	}
	g.Score += n
	if popup {
		g.Popups = append(g.Popups, Popup{X: x, Y: y, Life: 1.2, Text: itoa(n)})
	}
	for g.Score >= g.nextExtra {
		g.nextExtra += ExtraEvery
		g.Lives++
		if g.Bombs < MaxBombs {
			g.Bombs++
		}
		g.emit("extralife")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// ---------------------------------------------------------------- spawning

func (g *Game) spawn(k Kind, x, y float64) *Enemy {
	info := kinds[k]
	e := &Enemy{
		Kind: k, X: g.Wrap(x), Y: y, R: info.r, HP: info.hp, MaxHP: info.hp,
		Phase: g.rng.Float64() * 2 * math.Pi,
		fire:  g.rnd(1.5, 4),
		dir:   1,
	}
	if g.rng.Intn(2) == 0 {
		e.dir = -1
	}
	switch k {
	case Snatcher:
		e.wait = g.rnd(1.5, 9)
	case Bomber:
		e.aux = g.rnd(1, 2.5)
	case Hive:
		e.VX, e.VY = e.dir*g.rnd(15, 35), g.rnd(-20, 20)
	case Dreadnought:
		e.aux = 6
	case Turret:
		e.Y = g.GroundY(e.X) - 9
	}
	if k != Turret {
		e.Y = clamp(e.Y, TopY+20, g.floorFor(e.X, 30))
	}
	e.y0 = e.Y
	g.Enemies = append(g.Enemies, e)
	return e
}

// spawnAway warps an enemy in somewhere the player is not looking.
func (g *Game) spawnAway(k Kind) {
	var x float64
	for try := 0; try < 12; try++ {
		x = g.rng.Float64() * g.W
		if math.Abs(g.Dx(g.Player.X, x)) > 560 {
			break
		}
	}
	e := g.spawn(k, x, g.rnd(TopY+30, 280))
	g.burst(e.X, e.Y, k.Color(), 14, 120)
}

func (g *Game) updateWaves() {
	if g.State != Playing {
		return
	}
	// Don't make the player wait around in an empty sky.
	if len(g.Enemies) == 0 && g.waveIdx < len(g.waves) && g.waves[g.waveIdx].Time > g.Time+1.5 {
		shift := g.waves[g.waveIdx].Time - (g.Time + 1.5)
		for i := g.waveIdx; i < len(g.waves); i++ {
			g.waves[i].Time -= shift
		}
		g.lastWaveAt -= shift
	}
	spawned := false
	for g.waveIdx < len(g.waves) && g.waves[g.waveIdx].Time <= g.Time {
		w := g.waves[g.waveIdx]
		g.waveIdx++
		k, ok := KindOf(w.Type)
		if !ok || k == Turret {
			continue
		}
		for i := 0; i < w.Count; i++ {
			g.spawnAway(k)
		}
		spawned = true
	}
	if spawned {
		g.emit("warpin")
	}
	// Dawdlers get hunted.
	if g.waveIdx >= len(g.waves) && g.Time > g.lastWaveAt+70 {
		g.hurry -= DT
		if g.hurry <= 0 {
			g.hurry = 14
			g.spawnAway(Stalker)
			g.emit("warpin")
		}
	}
}

// ---------------------------------------------------------------- step

// Step advances the simulation by DT.
func (g *Game) Step(in Input) {
	g.StateTime += DT
	if g.Flash > 0 {
		g.Flash = math.Max(0, g.Flash-DT*1.8)
	}
	if g.Shake > 0 {
		g.Shake = math.Max(0, g.Shake-DT*2)
	}
	if g.State == Playing {
		g.Time += DT
	}

	g.updatePlayer(in)
	g.updateWaves()
	g.updateEnemies()
	g.updateColonists()
	g.updateShots()
	g.updateBullets()
	g.updateParticles()
	g.checkPlanet()
	g.updateCamera()

	switch g.State {
	case Playing:
		if g.waveIdx >= len(g.waves) && len(g.Enemies) == 0 {
			g.State, g.StateTime = Clear, 0
			g.Bonus = g.AliveColonists() * 100 * g.LevelNum
			g.addScore(g.Bonus, 0, 0, false)
			if g.Bombs < MaxBombs {
				g.Bombs++
			}
			g.Bullets, g.Mines = g.Bullets[:0], g.Mines[:0]
			g.emit("levelclear")
		}
	case Dying:
		if g.StateTime > 2.2 {
			if g.Lives <= 0 {
				g.State, g.StateTime = GameOver, 0
				g.emit("gameover")
			} else {
				g.respawn()
			}
		}
	}
	g.prev = in
}

func (g *Game) respawn() {
	p := &g.Player
	p.Alive, p.Invuln, p.VX, p.VY = true, 3, 0, 0
	p.Y = clamp(p.Y, TopY+40, g.floorFor(p.X, 60))
	g.Bullets = g.Bullets[:0]
	g.State, g.StateTime = Playing, 0
}

func (g *Game) updateCamera() {
	p := &g.Player
	target := ViewW * 0.35
	if p.Dir < 0 {
		target = ViewW * 0.65
	}
	g.camOff += (target - g.camOff) * math.Min(1, 2.5*DT)
	g.CamX = p.X - g.camOff
}

// ---------------------------------------------------------------- player

func (g *Game) updatePlayer(in Input) {
	p := &g.Player
	p.Thrust = false
	if !p.Alive || g.State == GameOver {
		return
	}
	if p.Invuln > 0 {
		p.Invuln -= DT
	}
	p.fireCD -= DT
	p.warpCD -= DT

	const accel, maxV = 1500.0, 580.0
	switch {
	case in.Left && !in.Right:
		p.Dir, p.Thrust = -1, true
		p.VX -= accel * DT
	case in.Right && !in.Left:
		p.Dir, p.Thrust = 1, true
		p.VX += accel * DT
	default:
		p.VX *= 0.982
	}
	p.VX = clamp(p.VX, -maxV, maxV)

	vyT := 0.0
	if in.Up {
		vyT -= 340
	}
	if in.Down {
		vyT += 340
	}
	p.VY += (vyT - p.VY) * math.Min(1, 14*DT)

	p.X = g.Wrap(p.X + p.VX*DT)
	p.Y = clamp(p.Y+p.VY*DT, TopY+14, g.floorFor(p.X, 14))

	if g.State == Clear {
		return
	}

	if in.Fire && p.fireCD <= 0 && len(g.Shots) < 7 {
		p.fireCD = 0.13
		g.Shots = append(g.Shots, Shot{X: g.Wrap(p.X + p.Dir*20), Y: p.Y + 1, VX: p.Dir*1500 + p.VX, Life: 0.5})
		g.emit("laser")
	}
	if in.Bomb && !g.prev.Bomb && g.Bombs > 0 {
		g.Bombs--
		g.smartBomb()
	}
	if in.Warp && !g.prev.Warp && p.warpCD <= 0 {
		g.burst(p.X, p.Y, "#8ff", 24, 220)
		p.X = g.rng.Float64() * g.W
		p.Y = clamp(g.rnd(TopY+40, 300), TopY+14, g.floorFor(p.X, 30))
		p.VX, p.VY = 0, 0
		p.warpCD = 2.5
		p.Invuln = math.Max(p.Invuln, 1.2)
		g.CamX = p.X - g.camOff
		g.burst(p.X, p.Y, "#8ff", 24, 220)
		g.emit("warp")
	}
}

func (g *Game) onScreen(x float64) bool {
	return math.Abs(g.Dx(g.CamX+ViewW/2, x)) < ViewW/2+30
}

func (g *Game) smartBomb() {
	g.Flash, g.Shake = 1, 0.6
	g.emit("bomb")
	for _, e := range g.Enemies {
		if !e.Dead && g.onScreen(e.X) {
			g.damage(e, 8)
		}
	}
	n := 0
	for _, b := range g.Bullets {
		if !g.onScreen(b.X) {
			g.Bullets[n] = b
			n++
		}
	}
	g.Bullets = g.Bullets[:n]
	n = 0
	for _, m := range g.Mines {
		if !g.onScreen(m.X) {
			g.Mines[n] = m
			n++
		} else {
			g.burst(m.X, m.Y, "#fa3", 6, 100)
		}
	}
	g.Mines = g.Mines[:n]
}

func (g *Game) killPlayer() {
	p := &g.Player
	if !p.Alive || p.Invuln > 0 || g.State != Playing {
		return
	}
	p.Alive = false
	if c := p.Carry; c != nil {
		c.State, c.VY, c.fallFrom = Falling, 0, c.Y
		p.Carry = nil
	}
	g.burst(p.X, p.Y, "#fff", 50, 380)
	g.burst(p.X, p.Y, "#6cf", 40, 260)
	g.burst(p.X, p.Y, "#fa3", 30, 160)
	g.Shake = 0.8
	g.Lives--
	g.State, g.StateTime = Dying, 0
	g.emit("playerdie")
}

// ---------------------------------------------------------------- shots

func (g *Game) updateShots() {
	n := 0
	for _, s := range g.Shots {
		step := s.VX * DT
		lo, hi := math.Min(0, step), math.Max(0, step)
		hit := false
		for _, e := range g.Enemies {
			if e.Dead || math.Abs(e.Y-s.Y) > e.R+3 {
				continue
			}
			if d := g.Dx(s.X, e.X); d >= lo-e.R && d <= hi+e.R {
				g.damage(e, 1)
				hit = true
				break
			}
		}
		if !hit {
			for i := range g.Mines {
				m := &g.Mines[i]
				if m.Life > 0 && math.Abs(m.Y-s.Y) < 9 {
					if d := g.Dx(s.X, m.X); d >= lo-7 && d <= hi+7 {
						m.Life = 0
						g.burst(m.X, m.Y, "#fa3", 8, 120)
						g.addScore(25, m.X, m.Y, false)
						g.emit("hit")
						hit = true
						break
					}
				}
			}
		}
		s.X = g.Wrap(s.X + step)
		s.Life -= DT
		s.Len = math.Min(s.Len+math.Abs(step), 90)
		if !hit && s.Life > 0 {
			g.Shots[n] = s
			n++
		}
	}
	g.Shots = g.Shots[:n]
}

func (g *Game) damage(e *Enemy, dmg int) {
	if e.Dead {
		return
	}
	e.HP -= dmg
	e.Flash = 0.12
	if e.HP > 0 {
		g.burst(e.X, e.Y, "#fff", 5, 140)
		g.emit("hit")
		return
	}
	g.kill(e, true)
}

func (g *Game) kill(e *Enemy, score bool) {
	e.Dead = true
	info := kinds[e.Kind]
	if c := e.Carrying; c != nil {
		c.State, c.VY, c.fallFrom = Falling, 0, c.Y
		e.Carrying = nil
	}
	if e.target != nil {
		e.target.claimed = nil
		e.target = nil
	}
	n, sp := 22, 230.0
	if e.Kind == Dreadnought {
		n, sp = 120, 420
		g.Shake = 1
		g.Flash = math.Max(g.Flash, 0.5)
		g.emit("bigexplode")
	} else {
		g.emit("explode")
	}
	g.burst(e.X, e.Y, info.color, n, sp)
	g.burst(e.X, e.Y, "#fff", n/3, sp*0.6)
	if score {
		g.addScore(info.score, e.X, e.Y-e.R, info.score >= 400)
	}
	if e.Kind == Hive {
		for i := 0; i < 5; i++ {
			w := g.spawn(Wasp, e.X+g.rnd(-12, 12), e.Y+g.rnd(-12, 12))
			w.VX, w.VY = g.rnd(-200, 200), g.rnd(-150, 150)
		}
	}
}

func (g *Game) updateBullets() {
	p := &g.Player
	n := 0
	for _, b := range g.Bullets {
		b.X = g.Wrap(b.X + b.VX*DT)
		b.Y += b.VY * DT
		b.Life -= DT
		if b.Life <= 0 || b.Y < TopY || b.Y > g.GroundY(b.X) || b.Y > ViewH {
			continue
		}
		if p.Alive && math.Abs(g.Dx(b.X, p.X)) < 13 && math.Abs(b.Y-p.Y) < 8 {
			if p.Invuln <= 0 && g.State == Playing {
				g.killPlayer()
				continue
			}
		}
		g.Bullets[n] = b
		n++
	}
	g.Bullets = g.Bullets[:n]

	n = 0
	for _, m := range g.Mines {
		m.Life -= DT
		if m.Life <= 0 {
			continue
		}
		if p.Alive && p.Invuln <= 0 && g.State == Playing &&
			math.Abs(g.Dx(m.X, p.X)) < 16 && math.Abs(m.Y-p.Y) < 12 {
			g.burst(m.X, m.Y, "#fa3", 10, 140)
			g.killPlayer()
			continue
		}
		g.Mines[n] = m
		n++
	}
	g.Mines = g.Mines[:n]
}

func (g *Game) updateParticles() {
	n := 0
	for _, p := range g.Particles {
		p.Life -= DT
		if p.Life <= 0 {
			continue
		}
		p.X += p.VX * DT
		p.Y += p.VY * DT
		p.VX *= 0.985
		p.VY = p.VY*0.985 + 40*DT
		g.Particles[n] = p
		n++
	}
	g.Particles = g.Particles[:n]

	n = 0
	for _, p := range g.Popups {
		p.Life -= DT
		p.Y -= 26 * DT
		if p.Life > 0 {
			g.Popups[n] = p
			n++
		}
	}
	g.Popups = g.Popups[:n]
}

// ---------------------------------------------------------------- colonists

func (g *Game) updateColonists() {
	p := &g.Player
	for _, c := range g.Colonists {
		if c.Dead {
			continue
		}
		switch c.State {
		case OnGround:
			if g.rng.Float64() < 0.004 {
				c.dir = -c.dir
			}
			c.X = g.Wrap(c.X + c.dir*9*DT)
			c.Y = g.GroundY(c.X) - 7
		case Falling:
			c.VY = math.Min(c.VY+240*DT, 210)
			c.Y += c.VY * DT
			if p.Alive && p.Carry == nil && g.State == Playing &&
				math.Abs(g.Dx(c.X, p.X)) < 24 && math.Abs(c.Y-p.Y) < 24 {
				c.State, p.Carry = Carried, c
				g.addScore(500, c.X, c.Y-14, true)
				g.emit("rescue")
				continue
			}
			if gy := g.GroundY(c.X) - 7; c.Y >= gy {
				c.Y = gy
				if gy-c.fallFrom > 170 {
					g.killColonist(c)
				} else {
					c.State = OnGround
					g.addScore(250, c.X, c.Y-14, true)
					g.emit("land")
				}
			}
		case Carried:
			c.X, c.Y = p.X, p.Y+17
			if gy := g.GroundY(c.X) - 7; c.Y >= gy-6 {
				c.Y, c.State, p.Carry = gy, OnGround, nil
				g.addScore(500, c.X, c.Y-14, true)
				g.emit("land")
			}
		}
	}
}

func (g *Game) killColonist(c *Colonist) {
	if c.Dead {
		return
	}
	c.Dead = true
	if g.Player.Carry == c {
		g.Player.Carry = nil
	}
	if c.claimed != nil {
		c.claimed.target = nil
		c.claimed = nil
	}
	g.burst(c.X, c.Y, "#f9c", 16, 150)
	g.emit("colonistdie")
}

// checkPlanet blows the planet up once the last colonist is lost.
func (g *Game) checkPlanet() {
	if g.PlanetDead || !g.hadColony || g.AliveColonists() > 0 {
		return
	}
	// explode the visible terrain before it disappears
	for x := 0.0; x < ViewW; x += 24 {
		wx := g.Wrap(g.CamX + x)
		g.burst(wx, g.L.GroundY(wx), "#fff", 5, 320)
	}
	g.PlanetDead = true
	g.Flash, g.Shake = 1, 1
	g.emit("planet")
	for _, e := range g.Enemies {
		switch e.Kind {
		case Snatcher:
			g.mutate(e)
		case Turret:
			g.kill(e, false)
		}
	}
}

func (g *Game) mutate(e *Enemy) {
	e.Kind, e.state, e.Carrying, e.target = Reaver, 0, nil, nil
	e.fire = g.rnd(0.5, 1.5)
	g.burst(e.X, e.Y, Reaver.Color(), 14, 160)
}

// ---------------------------------------------------------------- enemies

func (g *Game) fireAt(e *Enemy, speed, spread float64) {
	p := &g.Player
	dx, dy := g.Dx(e.X, p.X), p.Y-e.Y
	a := math.Atan2(dy, dx) + g.rnd(-spread, spread)
	g.Bullets = append(g.Bullets, Bullet{X: e.X, Y: e.Y, VX: math.Cos(a) * speed, VY: math.Sin(a) * speed, Life: 3.2})
}

// tryFire counts the enemy's cooldown down and fires when the player is in range.
func (g *Game) tryFire(e *Enemy, rng, cdLo, cdHi, speed, spread float64) bool {
	e.fire -= DT * g.L.Aggression
	if e.fire > 0 {
		return false
	}
	p := &g.Player
	if !p.Alive || g.State != Playing || math.Abs(g.Dx(e.X, p.X)) > rng || len(g.Bullets) > 80 {
		e.fire = 0.3
		return false
	}
	e.fire = g.rnd(cdLo, cdHi)
	g.fireAt(e, speed, spread)
	if g.onScreen(e.X) {
		g.emit("enemyshot")
	}
	return true
}

func (g *Game) pickColonist(e *Enemy) *Colonist {
	var best *Colonist
	bd := math.MaxFloat64
	for _, c := range g.Colonists {
		if c.Dead || c.State != OnGround || c.claimed != nil {
			continue
		}
		if d := math.Abs(g.Dx(e.X, c.X)); d < bd {
			best, bd = c, d
		}
	}
	return best
}

func (g *Game) updateEnemies() {
	p := &g.Player
	sp := g.L.Speed
	hunting := p.Alive && g.State == Playing

	for _, e := range g.Enemies {
		if e.Dead {
			continue
		}
		if e.Flash > 0 {
			e.Flash -= DT
		}
		dxp := g.Dx(e.X, p.X)
		dyp := p.Y - e.Y

		switch e.Kind {
		case Snatcher:
			g.updateSnatcher(e, sp)
			g.tryFire(e, 480, 2.2, 4.5, 230, 0.25)

		case Reaver:
			tx, ty := e.dir*120, 60*math.Sin(g.Time*2+e.Phase)
			if hunting {
				d := math.Hypot(dxp, dyp) + 1
				tx = dxp/d*195*sp + 110*math.Sin(g.Time*5+e.Phase)
				ty = dyp/d*195*sp + 110*math.Cos(g.Time*4+e.Phase)
			}
			e.VX += (tx - e.VX) * math.Min(1, 3*DT)
			e.VY += (ty - e.VY) * math.Min(1, 3*DT)
			g.tryFire(e, 520, 1.3, 2.8, 260, 0.2)

		case Bomber:
			e.VX = e.dir * 62 * sp
			e.VY = 0
			e.Y = e.y0 + 46*math.Sin(g.Time*1.3+e.Phase)
			e.aux -= DT * g.L.Aggression
			if e.aux <= 0 {
				e.aux = g.rnd(1.2, 2.2)
				if len(g.Mines) < 60 {
					g.Mines = append(g.Mines, Mine{X: e.X, Y: e.Y + 10, Life: 11})
				}
			}

		case Hive:
			if e.Y < TopY+40 && e.VY < 0 || e.Y > 330 && e.VY > 0 {
				e.VY = -e.VY
			}

		case Wasp:
			tx := e.dir * 200
			ty := 80 * math.Sin(g.Time*3+e.Phase)
			if hunting {
				tx = sign(dxp) * 270 * sp
				ty = clamp((dyp+70*math.Sin(g.Time*3+e.Phase))*3, -220, 220)
			}
			e.VX = approach(e.VX, tx, 520*DT)
			e.VY += (ty - e.VY) * math.Min(1, 4*DT)
			g.tryFire(e, 420, 2.2, 4.5, 250, 0.3)

		case Stalker:
			tx, ty := e.dir*150, 0.0
			if hunting {
				// hold station to one side of the player and strafe
				side := sign(-dxp)
				want := dxp + side*270
				tx = clamp(want*3, -340*sp, 340*sp) + p.VX*0.5
				ty = clamp((dyp+50*math.Sin(g.Time*2+e.Phase))*3, -240, 240)
			}
			e.VX += (tx - e.VX) * math.Min(1, 2.5*DT)
			e.VY += (ty - e.VY) * math.Min(1, 3*DT)
			g.tryFire(e, 460, 1.2, 2.2, 340, 0.08)

		case Turret:
			e.VX, e.VY = 0, 0
			e.Y = g.GroundY(e.X) - 9
			if dyp < -10 {
				g.tryFire(e, 520, 1.6, 2.8, 290, 0.1)
			}

		case Dreadnought:
			tx := e.dir * 40
			if hunting {
				want := dxp - sign(dxp)*300
				tx = clamp(want, -95*sp, 95*sp)
			}
			e.VX += (tx - e.VX) * math.Min(1, 1.5*DT)
			e.VY = 0
			e.Y = clamp(e.y0+70*math.Sin(g.Time*0.7+e.Phase), TopY+50, g.floorFor(e.X, 60))
			e.fire -= DT * g.L.Aggression
			if e.fire <= 0 {
				e.fire = 2.3
				if hunting && math.Abs(dxp) < 640 {
					base := math.Atan2(dyp, dxp)
					for i := -2; i <= 2; i++ {
						a := base + float64(i)*0.16
						g.Bullets = append(g.Bullets, Bullet{X: e.X, Y: e.Y, VX: math.Cos(a) * 250, VY: math.Sin(a) * 250, Life: 3.5})
					}
					g.emit("enemyshot")
				}
			}
			e.aux -= DT
			if e.aux <= 0 {
				e.aux = 9
				if hunting && g.count(Wasp) < 8 {
					for i := 0; i < 2; i++ {
						w := g.spawn(Wasp, e.X, e.Y+20)
						w.VX, w.VY = g.rnd(-150, 150), 120
					}
				}
			}
		}

		if e.Kind != Turret {
			e.X = g.Wrap(e.X + e.VX*DT)
			e.Y += e.VY * DT
			lo, hi := TopY+e.R+4, g.floorFor(e.X, e.R+6)
			if e.Kind == Snatcher && e.state != 0 {
				lo, hi = TopY, ViewH // free to dive to the surface and climb away
			}
			if e.Y < lo {
				e.Y = lo
				e.VY = math.Abs(e.VY) * 0.5
			} else if e.Y > hi {
				e.Y = hi
				e.VY = -math.Abs(e.VY) * 0.5
			}
		}
		if e.Carrying != nil {
			e.Carrying.X, e.Carrying.Y = e.X, e.Y+19
		}

		if hunting && p.Invuln <= 0 && !e.Dead {
			dxp = g.Dx(e.X, p.X)
			if math.Abs(dxp) < e.R+14 && math.Abs(p.Y-e.Y) < e.R+7 {
				if e.Kind != Dreadnought && e.Kind != Turret {
					g.kill(e, true)
				}
				g.killPlayer()
			}
		}
	}

	n := 0
	for _, e := range g.Enemies {
		if !e.Dead {
			g.Enemies[n] = e
			n++
		}
	}
	for i := n; i < len(g.Enemies); i++ {
		g.Enemies[i] = nil
	}
	g.Enemies = g.Enemies[:n]
}

func (g *Game) count(k Kind) int {
	n := 0
	for _, e := range g.Enemies {
		if !e.Dead && e.Kind == k {
			n++
		}
	}
	return n
}

// Snatcher states.
const (
	snCruise = iota
	snDive
	snCarry
)

func (g *Game) updateSnatcher(e *Enemy, sp float64) {
	cruiseY := e.y0 + 40*math.Sin(g.Time*0.9+e.Phase)
	switch e.state {
	case snCruise:
		e.wait -= DT
		if t := e.target; t != nil && (t.Dead || t.State != OnGround) {
			t.claimed, e.target = nil, nil
		}
		if e.target == nil && e.wait <= 0 {
			if c := g.pickColonist(e); c != nil {
				e.target, c.claimed = c, e
			} else {
				e.wait = 2
			}
		}
		e.VY = clamp((cruiseY-e.Y)*2, -70*sp, 70*sp)
		if t := e.target; t != nil {
			d := g.Dx(e.X, t.X)
			e.VX = clamp(d*3, -85*sp, 85*sp)
			if math.Abs(d) < 10 {
				e.state = snDive
			}
		} else {
			e.VX = e.dir * 45 * sp
		}
	case snDive:
		t := e.target
		if t == nil || t.Dead || t.State != OnGround {
			if t != nil {
				t.claimed = nil
			}
			e.target, e.state, e.wait = nil, snCruise, 1
			return
		}
		e.VX = clamp(g.Dx(e.X, t.X)*4, -60*sp, 60*sp)
		e.VY = 85 * sp
		if e.Y >= t.Y-20 {
			e.Y = t.Y - 19
			t.State, t.claimed = Grabbed, nil
			e.target, e.Carrying, e.state = nil, t, snCarry
			if g.onScreen(e.X) {
				g.emit("grab")
			} else {
				g.emit("grabfar")
			}
		}
	case snCarry:
		e.VX = 0
		e.VY = -48 * sp
		if e.Y <= TopY+16 {
			if c := e.Carrying; c != nil {
				e.Carrying = nil
				g.killColonist(c)
			}
			g.mutate(e)
			g.emit("mutate")
		}
	}
}
