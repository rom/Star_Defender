package level

import "testing"

func TestBuiltinLevelsValid(t *testing.T) {
	ls := Builtin()
	if len(ls) != 30 {
		t.Fatalf("want 30 levels, got %d", len(ls))
	}
	seen := map[string]bool{}
	for _, l := range ls {
		if err := l.Validate(); err != nil {
			t.Errorf("%s (%s): %v", l.ID, l.Name, err)
		}
		if seen[l.ID] {
			t.Errorf("duplicate id %s", l.ID)
		}
		seen[l.ID] = true
	}
}

func TestValidateRejects(t *testing.T) {
	ok := Builtin()[0].Clone()
	cases := map[string]func(l *Level){
		"empty name":    func(l *Level) { l.Name = "" },
		"control chars": func(l *Level) { l.Name = "a\x00b" },
		"short terrain": func(l *Level) { l.Terrain = l.Terrain[:10] },
		"tall terrain":  func(l *Level) { l.Terrain[3] = 9999 },
		"bad type":      func(l *Level) { l.Entities[0].Type = "dragon" },
		"off world":     func(l *Level) { l.Entities[0].X = -5 },
		"turret wave":   func(l *Level) { l.Waves[0].Type = "turret" },
		"huge wave":     func(l *Level) { l.Waves[0].Count = 1000 },
		"speed":         func(l *Level) { l.Speed = 99 },
		"no enemies":    func(l *Level) { l.Entities = nil; l.Waves = nil },
	}
	for name, mut := range cases {
		l := ok.Clone()
		mut(&l)
		if l.Validate() == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestHeightAtWraps(t *testing.T) {
	l := Blank()
	l.Terrain[0] = 100
	if h := l.HeightAt(l.Width()); h != 100 {
		t.Errorf("wrap: got %v", h)
	}
	if h := l.HeightAt(-SegW / 2); h != 80 {
		t.Errorf("negative interpolation: got %v", h)
	}
}
