// Package level defines the level format shared by the game client, the
// level editor and the server, plus the fifteen built-in campaign levels.
package level

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

const (
	ViewW = 960 // logical screen width in pixels
	ViewH = 540 // logical screen height in pixels
	SegW  = 32  // width of one terrain segment in pixels

	MinSegs = 60  // two screens
	MaxSegs = 360 // twelve screens

	GroundMin = 16  // lowest terrain height (pixels above the screen bottom)
	GroundMax = 240 // highest terrain height

	SkyTop = 70 // top of the playfield (the HUD lives above it)

	MaxColonists = 30
	MaxEntities  = 200
	MaxWaves     = 40
	MaxWaveCount = 20
	MaxWaveTime  = 600
)

// Enemy type names as they appear in level files.
var EnemyTypes = []string{
	"snatcher", "reaver", "bomber", "hive", "wasp", "stalker", "turret", "dreadnought",
}

// Entity is an enemy placed in the level from the start.
type Entity struct {
	Type string  `json:"type"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

// Wave is a group of enemies that warps in after Time seconds.
type Wave struct {
	Time  float64 `json:"time"`
	Type  string  `json:"type"`
	Count int     `json:"count"`
}

// Level is a complete, self-contained level description.
type Level struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Author     string    `json:"author,omitempty"`
	Builtin    bool      `json:"builtin,omitempty"`
	Terrain    []int     `json:"terrain"`    // one height per segment, wraps around
	Colonists  []float64 `json:"colonists"`  // x positions
	Entities   []Entity  `json:"entities"`   // enemies present at the start
	Waves      []Wave    `json:"waves"`      // timed reinforcements
	Speed      float64   `json:"speed"`      // enemy speed multiplier
	Aggression float64   `json:"aggression"` // enemy fire-rate multiplier
	Hue        int       `json:"hue"`        // terrain colour, 0..359
	Updated    int64     `json:"updated,omitempty"`
}

// Summary is the short form used in level listings.
type Summary struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Author  string `json:"author,omitempty"`
	Builtin bool   `json:"builtin,omitempty"`
}

func (l *Level) Summary() Summary {
	return Summary{ID: l.ID, Name: l.Name, Author: l.Author, Builtin: l.Builtin}
}

// Width is the world width in pixels. The world wraps horizontally.
func (l *Level) Width() float64 { return float64(len(l.Terrain) * SegW) }

// HeightAt returns the interpolated terrain height at world x.
func (l *Level) HeightAt(x float64) float64 {
	n := len(l.Terrain)
	if n == 0 {
		return GroundMin
	}
	f := x / SegW
	i := int(math.Floor(f))
	t := f - float64(i)
	a := l.Terrain[((i%n)+n)%n]
	b := l.Terrain[(((i+1)%n)+n)%n]
	return float64(a) + (float64(b)-float64(a))*t
}

// GroundY returns the screen y of the ground surface at world x.
func (l *Level) GroundY(x float64) float64 { return ViewH - l.HeightAt(x) }

func ValidType(t string) bool {
	for _, e := range EnemyTypes {
		if e == t {
			return true
		}
	}
	return false
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// CleanText trims s, drops control characters and limits it to max runes.
func CleanText(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > max {
		r = r[:max]
	}
	return strings.TrimSpace(string(r))
}

// Validate checks every field against the format limits. It never modifies
// the level, so a level that validates is safe to hand to the game as is.
func (l *Level) Validate() error {
	if l.Name == "" || l.Name != CleanText(l.Name, 32) {
		return errors.New("name must be 1-32 printable characters")
	}
	if l.Author != CleanText(l.Author, 24) {
		return errors.New("author must be at most 24 printable characters")
	}
	if n := len(l.Terrain); n < MinSegs || n > MaxSegs {
		return fmt.Errorf("terrain must have %d-%d segments", MinSegs, MaxSegs)
	}
	for _, h := range l.Terrain {
		if h < GroundMin || h > GroundMax {
			return fmt.Errorf("terrain heights must be %d-%d", GroundMin, GroundMax)
		}
	}
	w := l.Width()
	if len(l.Colonists) > MaxColonists {
		return fmt.Errorf("at most %d colonists", MaxColonists)
	}
	for _, x := range l.Colonists {
		if !finite(x) || x < 0 || x >= w {
			return errors.New("colonist outside the world")
		}
	}
	if len(l.Entities) > MaxEntities {
		return fmt.Errorf("at most %d placed enemies", MaxEntities)
	}
	for _, e := range l.Entities {
		if !ValidType(e.Type) {
			return fmt.Errorf("unknown enemy type %q", e.Type)
		}
		if !finite(e.X) || e.X < 0 || e.X >= w || !finite(e.Y) || e.Y < 0 || e.Y > ViewH {
			return errors.New("enemy outside the world")
		}
	}
	if len(l.Waves) > MaxWaves {
		return fmt.Errorf("at most %d waves", MaxWaves)
	}
	for _, wv := range l.Waves {
		if !ValidType(wv.Type) || wv.Type == "turret" {
			return fmt.Errorf("enemy type %q cannot be used in a wave", wv.Type)
		}
		if wv.Count < 1 || wv.Count > MaxWaveCount {
			return fmt.Errorf("wave count must be 1-%d", MaxWaveCount)
		}
		if !finite(wv.Time) || wv.Time < 0 || wv.Time > MaxWaveTime {
			return fmt.Errorf("wave time must be 0-%d seconds", MaxWaveTime)
		}
	}
	if len(l.Entities) == 0 && len(l.Waves) == 0 {
		return errors.New("a level needs at least one enemy or wave")
	}
	if !finite(l.Speed) || l.Speed < 0.5 || l.Speed > 2.5 {
		return errors.New("speed must be 0.5-2.5")
	}
	if !finite(l.Aggression) || l.Aggression < 0.2 || l.Aggression > 2.5 {
		return errors.New("aggression must be 0.2-2.5")
	}
	if l.Hue < 0 || l.Hue > 359 {
		return errors.New("hue must be 0-359")
	}
	return nil
}

// Clone returns a deep copy.
func (l *Level) Clone() Level {
	c := *l
	c.Terrain = append([]int(nil), l.Terrain...)
	c.Colonists = append([]float64(nil), l.Colonists...)
	c.Entities = append([]Entity(nil), l.Entities...)
	c.Waves = append([]Wave(nil), l.Waves...)
	return c
}

// Blank returns a small empty level, the editor's starting point.
func Blank() Level {
	l := Level{Name: "Untitled", Speed: 1, Aggression: 1, Hue: 140}
	l.Terrain = make([]int, 120)
	for i := range l.Terrain {
		l.Terrain[i] = 60
	}
	l.Colonists = []float64{}
	l.Entities = []Entity{}
	l.Waves = []Wave{}
	return l
}
