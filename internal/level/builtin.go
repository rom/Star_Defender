package level

import (
	"fmt"
	"math"
	"math/rand"
)

type group struct {
	t string
	n int
}

type spec struct {
	name      string
	segs      int
	colonists int
	rough     float64 // terrain amplitude in pixels
	hue       int
	speed     float64
	aggr      float64
	initial   []group
	waves     []Wave
}

var specs = []spec{
	{"First Contact", 120, 10, 30, 140, 0.80, 0.60,
		[]group{{"snatcher", 4}},
		[]Wave{{12, "snatcher", 3}, {26, "snatcher", 3}}},
	{"Dust Plains", 120, 10, 22, 35, 0.85, 0.70,
		[]group{{"snatcher", 5}, {"bomber", 1}},
		[]Wave{{12, "snatcher", 4}, {28, "snatcher", 3}, {28, "bomber", 1}}},
	{"Minefield", 130, 10, 40, 190, 0.90, 0.80,
		[]group{{"snatcher", 4}, {"bomber", 3}},
		[]Wave{{14, "snatcher", 4}, {30, "bomber", 2}, {30, "snatcher", 2}}},
	{"The Hives", 130, 10, 45, 285, 0.95, 0.85,
		[]group{{"snatcher", 5}, {"hive", 2}},
		[]Wave{{15, "snatcher", 4}, {30, "hive", 1}, {30, "snatcher", 3}}},
	{"Dreadnought", 120, 8, 35, 0, 1.00, 0.90,
		[]group{{"dreadnought", 1}, {"snatcher", 4}},
		[]Wave{{20, "snatcher", 4}}},
	{"Night Raid", 140, 10, 50, 220, 1.05, 1.00,
		[]group{{"snatcher", 6}, {"bomber", 2}, {"hive", 1}},
		[]Wave{{12, "snatcher", 4}, {25, "stalker", 1}, {25, "snatcher", 3}, {40, "snatcher", 3}}},
	{"Canyon Guns", 140, 10, 85, 20, 1.10, 1.00,
		[]group{{"snatcher", 5}, {"turret", 5}, {"bomber", 1}},
		[]Wave{{15, "snatcher", 4}, {30, "snatcher", 4}, {30, "bomber", 1}}},
	{"Swarm Season", 150, 10, 40, 60, 1.15, 1.05,
		[]group{{"snatcher", 4}, {"hive", 4}},
		[]Wave{{12, "hive", 2}, {26, "snatcher", 5}, {40, "wasp", 6}}},
	{"Stalker Moon", 150, 10, 55, 250, 1.20, 1.10,
		[]group{{"snatcher", 6}, {"stalker", 2}, {"bomber", 2}},
		[]Wave{{14, "snatcher", 4}, {14, "stalker", 1}, {30, "snatcher", 4}, {30, "stalker", 2}}},
	{"Twin Titans", 140, 8, 45, 320, 1.20, 1.10,
		[]group{{"dreadnought", 2}, {"snatcher", 4}, {"turret", 3}},
		[]Wave{{20, "snatcher", 4}, {45, "snatcher", 3}}},
	{"Crimson Ridge", 160, 10, 90, 350, 1.30, 1.20,
		[]group{{"snatcher", 7}, {"turret", 6}, {"hive", 2}, {"bomber", 2}},
		[]Wave{{14, "snatcher", 5}, {30, "reaver", 4}, {44, "snatcher", 4}}},
	{"No Shelter", 160, 8, 18, 170, 1.35, 1.25,
		[]group{{"snatcher", 8}, {"reaver", 3}, {"stalker", 1}, {"bomber", 2}},
		[]Wave{{12, "snatcher", 5}, {26, "stalker", 2}, {26, "snatcher", 4}, {40, "reaver", 4}}},
	{"Iron Rain", 170, 10, 60, 205, 1.40, 1.30,
		[]group{{"snatcher", 6}, {"bomber", 5}, {"turret", 4}, {"hive", 2}},
		[]Wave{{12, "bomber", 3}, {12, "snatcher", 4}, {28, "snatcher", 5}, {28, "stalker", 2}, {44, "hive", 2}}},
	{"Last Colony", 170, 6, 50, 100, 1.50, 1.40,
		[]group{{"snatcher", 10}, {"reaver", 4}, {"hive", 3}, {"stalker", 2}},
		[]Wave{{12, "snatcher", 6}, {26, "reaver", 5}, {40, "snatcher", 5}, {40, "stalker", 2}}},
	{"Mothership Down", 180, 10, 70, 300, 1.50, 1.50,
		[]group{{"dreadnought", 3}, {"snatcher", 6}, {"turret", 4}, {"stalker", 2}, {"hive", 2}},
		[]Wave{{20, "snatcher", 5}, {40, "reaver", 4}, {40, "stalker", 2}, {60, "snatcher", 5}}},

	// ---- second campaign: the counter-attack
	{"Aftermath", 180, 10, 35, 125, 1.50, 1.45,
		[]group{{"snatcher", 9}, {"bomber", 3}, {"hive", 2}},
		[]Wave{{12, "snatcher", 5}, {26, "reaver", 3}, {26, "stalker", 1}, {40, "snatcher", 5}}},
	{"Glass Desert", 190, 10, 14, 45, 1.55, 1.50,
		[]group{{"snatcher", 8}, {"stalker", 3}, {"reaver", 3}},
		[]Wave{{12, "snatcher", 6}, {24, "stalker", 2}, {36, "wasp", 8}, {48, "snatcher", 5}}},
	{"Mine Belt", 190, 10, 55, 180, 1.55, 1.55,
		[]group{{"snatcher", 7}, {"bomber", 7}, {"turret", 3}},
		[]Wave{{12, "bomber", 4}, {12, "snatcher", 4}, {28, "bomber", 3}, {28, "snatcher", 5}, {44, "stalker", 2}}},
	{"Hornet Nest", 200, 10, 60, 55, 1.60, 1.55,
		[]group{{"snatcher", 6}, {"hive", 7}, {"wasp", 4}},
		[]Wave{{12, "hive", 3}, {26, "snatcher", 6}, {40, "wasp", 10}, {54, "hive", 2}}},
	{"Siege Engine", 180, 8, 65, 5, 1.60, 1.60,
		[]group{{"dreadnought", 2}, {"snatcher", 6}, {"turret", 6}, {"bomber", 2}},
		[]Wave{{18, "snatcher", 5}, {36, "stalker", 2}, {36, "snatcher", 4}, {54, "reaver", 4}}},
	{"Blackout", 200, 10, 50, 235, 1.65, 1.65,
		[]group{{"snatcher", 9}, {"stalker", 4}, {"hive", 2}},
		[]Wave{{12, "snatcher", 5}, {24, "stalker", 3}, {36, "snatcher", 5}, {48, "reaver", 5}}},
	{"Razorback", 200, 10, 100, 15, 1.70, 1.65,
		[]group{{"snatcher", 8}, {"turret", 9}, {"bomber", 3}, {"stalker", 1}},
		[]Wave{{12, "snatcher", 5}, {26, "bomber", 3}, {26, "snatcher", 4}, {40, "stalker", 3}}},
	{"Reaver Tide", 210, 8, 30, 310, 1.70, 1.70,
		[]group{{"snatcher", 6}, {"reaver", 8}, {"hive", 2}},
		[]Wave{{10, "reaver", 4}, {22, "snatcher", 6}, {34, "reaver", 6}, {46, "snatcher", 5}, {46, "stalker", 2}}},
	{"Crossfire", 210, 10, 75, 270, 1.75, 1.80,
		[]group{{"snatcher", 8}, {"turret", 8}, {"stalker", 4}, {"bomber", 2}},
		[]Wave{{12, "snatcher", 6}, {26, "stalker", 3}, {40, "snatcher", 5}, {40, "bomber", 3}}},
	{"Triad", 200, 8, 55, 335, 1.75, 1.75,
		[]group{{"dreadnought", 3}, {"snatcher", 7}, {"hive", 3}, {"stalker", 2}},
		[]Wave{{18, "snatcher", 5}, {36, "reaver", 5}, {54, "snatcher", 5}, {54, "stalker", 2}}},
	{"Long Night", 220, 10, 45, 215, 1.80, 1.85,
		[]group{{"snatcher", 10}, {"bomber", 4}, {"hive", 4}, {"stalker", 3}},
		[]Wave{{12, "snatcher", 6}, {24, "wasp", 8}, {36, "snatcher", 6}, {48, "reaver", 5}, {60, "stalker", 3}}},
	{"Hell's Teeth", 220, 8, 110, 355, 1.85, 1.90,
		[]group{{"snatcher", 9}, {"turret", 10}, {"reaver", 5}, {"bomber", 4}},
		[]Wave{{12, "snatcher", 6}, {26, "reaver", 5}, {40, "snatcher", 6}, {40, "stalker", 3}}},
	{"Extinction Line", 230, 6, 40, 85, 1.90, 2.00,
		[]group{{"snatcher", 12}, {"reaver", 6}, {"stalker", 4}, {"hive", 3}},
		[]Wave{{10, "snatcher", 6}, {22, "reaver", 6}, {34, "snatcher", 6}, {46, "stalker", 4}, {58, "wasp", 10}}},
	{"Armada", 240, 10, 60, 195, 1.95, 2.05,
		[]group{{"snatcher", 10}, {"bomber", 6}, {"hive", 5}, {"stalker", 4}, {"turret", 6}},
		[]Wave{{12, "snatcher", 7}, {24, "bomber", 4}, {36, "reaver", 6}, {48, "snatcher", 6}, {60, "stalker", 4}}},
	{"Final Stand", 240, 10, 80, 290, 2.00, 2.10,
		[]group{{"dreadnought", 4}, {"snatcher", 8}, {"turret", 6}, {"stalker", 3}, {"hive", 3}},
		[]Wave{{18, "snatcher", 6}, {36, "reaver", 6}, {36, "stalker", 3}, {54, "snatcher", 6}, {72, "dreadnought", 1}, {72, "reaver", 4}}},
}

// GenTerrain builds a seamless (wrapping) mountain range of n segments.
func GenTerrain(rng *rand.Rand, n int, rough float64) []int {
	type wave struct{ k, amp, ph float64 }
	var ws []wave
	for _, k := range []float64{1, 2, 3, 5, 8, 13, 21} {
		ws = append(ws, wave{k, rough / math.Pow(k, 0.75) * (0.6 + rng.Float64()*0.8), rng.Float64() * 2 * math.Pi})
	}
	base := 40 + rough*1.1
	out := make([]int, n)
	for i := range out {
		h := base
		for _, w := range ws {
			h += w.amp * math.Sin(2*math.Pi*w.k*float64(i)/float64(n)+w.ph)
		}
		h += (rng.Float64() - 0.5) * rough * 0.25
		out[i] = int(math.Max(GroundMin, math.Min(GroundMax, h)))
	}
	return out
}

var builtin []Level

// Builtin returns the campaign levels, in order. The returned slice
// is shared; callers must not modify it (use Clone).
func Builtin() []Level {
	if builtin != nil {
		return builtin
	}
	for i, s := range specs {
		rng := rand.New(rand.NewSource(int64(1000 + i*77)))
		l := Level{
			ID:         fmt.Sprintf("level-%02d", i+1),
			Name:       s.name,
			Author:     "Star Defender",
			Builtin:    true,
			Terrain:    GenTerrain(rng, s.segs, s.rough),
			Speed:      s.speed,
			Aggression: s.aggr,
			Hue:        s.hue,
			Waves:      append([]Wave{}, s.waves...),
			Colonists:  []float64{},
			Entities:   []Entity{},
		}
		w := l.Width()
		for c := 0; c < s.colonists; c++ {
			// spread evenly with some jitter
			x := (float64(c) + 0.2 + rng.Float64()*0.6) / float64(s.colonists) * w
			l.Colonists = append(l.Colonists, math.Floor(x))
		}
		start := w / 2 // the player starts mid-world
		for _, g := range s.initial {
			for k := 0; k < g.n; k++ {
				var x float64
				for try := 0; try < 20; try++ {
					x = math.Floor(rng.Float64() * w)
					if math.Abs(x-start) > 600 {
						break
					}
				}
				y := math.Floor(100 + rng.Float64()*160)
				l.Entities = append(l.Entities, Entity{Type: g.t, X: x, Y: y})
			}
		}
		builtin = append(builtin, l)
	}
	return builtin
}
