package game

import (
	"math"
	"math/rand"
	"testing"

	"stardefender/internal/level"
)

// Every campaign level must survive a long session of random play with all
// state staying finite and inside the world.
func TestCampaignSmoke(t *testing.T) {
	for i := range level.Builtin() {
		lv := level.Builtin()[i].Clone()
		g := New(int64(i + 1))
		g.Lives = 1000
		g.LoadLevel(&lv, i+1)
		rng := rand.New(rand.NewSource(int64(i)))
		var in Input
		for f := 0; f < 60*240 && g.State != Clear; f++ {
			if f%20 == 0 {
				in = Input{
					Left: rng.Intn(3) == 0, Right: rng.Intn(3) == 0,
					Up: rng.Intn(3) == 0, Down: rng.Intn(3) == 0,
					Fire: true, Bomb: rng.Intn(40) == 0, Warp: rng.Intn(60) == 0,
				}
			}
			g.Step(in)
			g.DrainEvents()
			p := g.Player
			if math.IsNaN(p.X+p.Y) || p.X < 0 || p.X >= g.W || p.Y < TopY || p.Y > ViewH {
				t.Fatalf("level %d frame %d: bad player pos %v,%v", i+1, f, p.X, p.Y)
			}
			for _, e := range g.Enemies {
				if math.IsNaN(e.X+e.Y) || e.X < 0 || e.X >= g.W || e.Y < TopY-1 || e.Y > ViewH+1 {
					t.Fatalf("level %d frame %d: bad enemy %d pos %v,%v", i+1, f, e.Kind, e.X, e.Y)
				}
			}
		}
	}
}

func oneEnemyLevel() level.Level {
	lv := level.Blank()
	lv.Entities = []level.Entity{{Type: "snatcher", X: 100, Y: 200}}
	lv.Colonists = []float64{120}
	return lv
}

func TestSnatcherAbductsAndMutates(t *testing.T) {
	lv := oneEnemyLevel()
	g := New(1)
	g.LoadLevel(&lv, 1)
	g.Player.Invuln = 1e9 // spectate
	grabbed := false
	for f := 0; f < 60*60; f++ {
		g.Step(Input{})
		if g.Colonists[0].State == Grabbed {
			grabbed = true
		}
		if g.PlanetDead {
			break
		}
	}
	if !grabbed {
		t.Fatal("snatcher never grabbed the colonist")
	}
	if !g.Colonists[0].Dead || !g.PlanetDead {
		t.Fatal("colonist should have been carried off and the planet lost")
	}
	if len(g.Enemies) != 1 || g.Enemies[0].Kind != Reaver {
		t.Fatalf("snatcher should have mutated into a reaver: %+v", g.Enemies)
	}
}

func TestShootingClearsLevel(t *testing.T) {
	lv := oneEnemyLevel()
	lv.Colonists = nil
	g := New(1)
	g.LoadLevel(&lv, 3)
	e := g.Enemies[0]
	g.Player.X, g.Player.Y = e.X-200, e.Y
	for f := 0; f < 120 && g.State == Playing; f++ {
		g.Player.Y = e.Y
		g.Step(Input{Fire: true})
	}
	if g.State != Clear {
		t.Fatalf("level should be clear, state=%v enemies=%d", g.State, len(g.Enemies))
	}
	if g.Score != 150 {
		t.Errorf("score = %d, want 150", g.Score)
	}
}

func TestRescueFallingColonist(t *testing.T) {
	lv := oneEnemyLevel()
	g := New(1)
	g.LoadLevel(&lv, 1)
	g.Player.Invuln = 1e9
	c := g.Colonists[0]
	for f := 0; f < 60*60 && c.State != Grabbed; f++ {
		g.Step(Input{})
	}
	for f := 0; f < 60*2; f++ { // let it climb a while
		g.Step(Input{})
	}
	g.kill(g.Enemies[0], true)
	if c.State != Falling {
		t.Fatalf("colonist should be falling, state=%v", c.State)
	}
	g.Player.X, g.Player.Y, g.Player.VX = c.X, c.Y, 0
	g.Step(Input{})
	if g.Player.Carry != c {
		t.Fatal("player should have caught the colonist")
	}
	for f := 0; f < 60*5 && c.State != OnGround; f++ {
		g.Step(Input{Down: true})
	}
	if c.State != OnGround || c.Dead {
		t.Fatal("colonist should be safely back on the ground")
	}
	if g.Score != 150+500+500+100 { // kill + catch + landing + level bonus
		t.Errorf("score = %d, want 1250", g.Score)
	}
}

func TestGameOverAfterLastLife(t *testing.T) {
	lv := oneEnemyLevel()
	g := New(1)
	g.LoadLevel(&lv, 1)
	for g.State != GameOver {
		if g.StateTime > 60 {
			t.Fatal("stuck")
		}
		g.Player.Invuln = 0
		g.killPlayer()
		g.Step(Input{})
	}
	if g.Lives != 0 {
		t.Errorf("lives = %d", g.Lives)
	}
}
