//go:build js && wasm

package main

import (
	"encoding/binary"
	"math"
	"math/rand"
	"syscall/js"
)

// audio synthesises every sound effect and all music with the Web Audio
// API - there are no sample files.
type audio struct {
	ctx    js.Value
	sfx    js.Value // gain bus for effects
	music  js.Value // gain bus for music
	master js.Value
	bassF  js.Value // low-pass for the bass voice
	leadF  js.Value // low-pass for the lead voice
	noise  js.Value // one second of white noise
	ok     bool

	muted    bool
	musicOff bool

	song     *song
	wantSong int
	wantBPM  float64
	step     int
	nextT    float64
	lastSfx  map[string]float64
}

func newAudio() *audio { return &audio{lastSfx: map[string]float64{}, wantSong: -1} }

// init must be called from a user gesture (browsers block audio before one).
func (a *audio) init() {
	if a.ok {
		if a.ctx.Get("state").String() == "suspended" {
			a.ctx.Call("resume")
		}
		return
	}
	ctor := js.Global().Get("AudioContext")
	if ctor.IsUndefined() {
		ctor = js.Global().Get("webkitAudioContext")
	}
	if ctor.IsUndefined() {
		return
	}
	a.ctx = ctor.New()
	dest := a.ctx.Get("destination")

	comp := a.ctx.Call("createDynamicsCompressor")
	comp.Call("connect", dest)
	a.master = a.gainNode(0.9, comp)
	a.sfx = a.gainNode(0.55, a.master)
	a.music = a.gainNode(0.5, a.master)
	a.bassF = a.filter("lowpass", 700, a.music)
	a.leadF = a.filter("lowpass", 2600, a.music)

	// white-noise buffer for explosions and drums
	rate := a.ctx.Get("sampleRate").Int()
	buf := a.ctx.Call("createBuffer", 1, rate, rate)
	raw := make([]byte, rate*4)
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < rate; i++ {
		binary.LittleEndian.PutUint32(raw[i*4:], math.Float32bits(rng.Float32()*2-1))
	}
	data := buf.Call("getChannelData", 0)
	u8 := js.Global().Get("Uint8Array").New(data.Get("buffer"), data.Get("byteOffset"), rate*4)
	js.CopyBytesToJS(u8, raw)
	a.noise = buf

	a.ok = true
	a.applyVolume()
	if a.wantSong >= 0 {
		a.setSong(a.wantSong, a.wantBPM)
	}
}

func (a *audio) gainNode(v float64, dest js.Value) js.Value {
	g := a.ctx.Call("createGain")
	g.Get("gain").Set("value", v)
	g.Call("connect", dest)
	return g
}

func (a *audio) filter(kind string, freq float64, dest js.Value) js.Value {
	f := a.ctx.Call("createBiquadFilter")
	f.Set("type", kind)
	f.Get("frequency").Set("value", freq)
	f.Call("connect", dest)
	return f
}

func (a *audio) now() float64 { return a.ctx.Get("currentTime").Float() }

func (a *audio) applyVolume() {
	if !a.ok {
		return
	}
	m, mu := 0.9, 0.5
	if a.muted {
		m = 0
	}
	if a.musicOff {
		mu = 0
	}
	a.master.Get("gain").Set("value", m)
	a.music.Get("gain").Set("value", mu)
}

func (a *audio) toggleMute()  { a.muted = !a.muted; a.applyVolume() }
func (a *audio) toggleMusic() { a.musicOff = !a.musicOff; a.applyVolume() }

// env shapes a gain node: quick attack, exponential decay to silence.
func env(g js.Value, t, vol, dur float64) {
	p := g.Get("gain")
	p.Call("setValueAtTime", 0.0001, t)
	p.Call("linearRampToValueAtTime", vol, t+0.006)
	p.Call("exponentialRampToValueAtTime", 0.0001, t+dur)
}

// tone plays an oscillator sweeping from f0 to f1 Hz.
func (a *audio) tone(wave string, f0, f1, dur, vol, at float64, dest js.Value) {
	o := a.ctx.Call("createOscillator")
	o.Set("type", wave)
	fp := o.Get("frequency")
	fp.Call("setValueAtTime", f0, at)
	if f1 != f0 {
		fp.Call("exponentialRampToValueAtTime", math.Max(f1, 1), at+dur)
	}
	g := a.ctx.Call("createGain")
	env(g, at, vol, dur)
	o.Call("connect", g)
	g.Call("connect", dest)
	o.Call("start", at)
	o.Call("stop", at+dur+0.03)
}

// hiss plays filtered noise, sweeping the filter from f0 to f1 Hz.
func (a *audio) hiss(kind string, f0, f1, dur, vol, at float64, dest js.Value) {
	s := a.ctx.Call("createBufferSource")
	s.Set("buffer", a.noise)
	f := a.ctx.Call("createBiquadFilter")
	f.Set("type", kind)
	fp := f.Get("frequency")
	fp.Call("setValueAtTime", f0, at)
	if f1 != f0 {
		fp.Call("exponentialRampToValueAtTime", math.Max(f1, 1), at+dur)
	}
	g := a.ctx.Call("createGain")
	env(g, at, vol, dur)
	s.Call("connect", f)
	f.Call("connect", g)
	g.Call("connect", dest)
	s.Call("start", at, rand.Float64()*0.5)
	s.Call("stop", at+dur+0.03)
}

// play triggers the sound effect for a game event.
func (a *audio) play(name string) {
	if !a.ok || a.muted {
		return
	}
	t := a.now()
	if t-a.lastSfx[name] < 0.045 {
		return
	}
	a.lastSfx[name] = t
	d := a.sfx
	switch name {
	case "laser":
		a.tone("sawtooth", 1500, 220, 0.13, 0.16, t, d)
		a.tone("square", 2200, 500, 0.07, 0.07, t, d)
	case "enemyshot":
		a.tone("square", 520, 180, 0.16, 0.09, t, d)
	case "hit":
		a.tone("square", 900, 300, 0.06, 0.12, t, d)
		a.hiss("highpass", 3000, 1500, 0.06, 0.15, t, d)
	case "explode":
		a.hiss("lowpass", 2600, 120, 0.42, 0.5, t, d)
		a.tone("sawtooth", 260, 40, 0.3, 0.2, t, d)
	case "bigexplode":
		a.hiss("lowpass", 3200, 60, 1.6, 0.9, t, d)
		a.tone("sawtooth", 180, 25, 1.2, 0.4, t, d)
		a.hiss("lowpass", 1200, 80, 0.9, 0.6, t+0.25, d)
	case "playerdie":
		a.hiss("lowpass", 4000, 80, 1.3, 0.8, t, d)
		a.tone("sawtooth", 700, 30, 1.1, 0.35, t, d)
		a.tone("square", 350, 20, 1.3, 0.2, t+0.05, d)
	case "bomb":
		a.hiss("lowpass", 6000, 60, 1.1, 0.9, t, d)
		a.tone("sine", 120, 28, 1.0, 0.7, t, d)
	case "planet":
		a.hiss("lowpass", 5000, 40, 2.6, 1.0, t, d)
		a.tone("sawtooth", 110, 20, 2.4, 0.5, t, d)
	case "warp":
		a.tone("sine", 200, 2400, 0.22, 0.2, t, d)
		a.tone("sine", 2400, 200, 0.25, 0.2, t+0.2, d)
	case "warpin":
		a.tone("triangle", 90, 900, 0.4, 0.14, t, d)
	case "grab":
		for i := 0; i < 4; i++ {
			a.tone("square", 1100-float64(i%2)*350, 1100-float64(i%2)*350, 0.09, 0.14, t+float64(i)*0.1, d)
		}
	case "grabfar":
		for i := 0; i < 4; i++ {
			a.tone("square", 1100-float64(i%2)*350, 1100-float64(i%2)*350, 0.09, 0.05, t+float64(i)*0.1, d)
		}
	case "mutate":
		a.tone("sawtooth", 150, 1300, 0.5, 0.2, t, d)
		a.tone("square", 157, 1340, 0.5, 0.12, t, d)
	case "rescue":
		for i, f := range []float64{660, 880, 1320} {
			a.tone("triangle", f, f, 0.12, 0.25, t+float64(i)*0.07, d)
		}
	case "land":
		for i, f := range []float64{520, 780} {
			a.tone("triangle", f, f, 0.14, 0.22, t+float64(i)*0.09, d)
		}
	case "colonistdie":
		a.tone("triangle", 900, 110, 0.5, 0.28, t, d)
	case "extralife":
		for i, f := range []float64{523, 659, 784, 1047, 1319} {
			a.tone("square", f, f, 0.14, 0.14, t+float64(i)*0.09, d)
		}
	case "levelclear":
		for i, f := range []float64{392, 523, 659, 784, 1047} {
			a.tone("square", f, f, 0.22, 0.14, t+float64(i)*0.13, d)
			a.tone("triangle", f/2, f/2, 0.22, 0.2, t+float64(i)*0.13, d)
		}
	case "gameover":
		for i, f := range []float64{440, 415, 370, 330, 220} {
			a.tone("sawtooth", f, f*0.97, 0.4, 0.14, t+float64(i)*0.3, d)
		}
	case "select":
		a.tone("square", 660, 660, 0.05, 0.1, t, d)
	case "start":
		a.tone("square", 440, 880, 0.18, 0.14, t, d)
		a.tone("square", 660, 1320, 0.2, 0.1, t+0.1, d)
	}
}

// ---------------------------------------------------------------- music

const rest = -99

// song is a looping 16th-note step-sequencer pattern.
type song struct {
	bpm    float64
	root   int      // MIDI note of the tonic
	chords [][3]int // one triad per bar, in semitones from the tonic
	bass   []int    // 16 steps: semitones above the chord root, or rest
	arp    []int    // 16 steps: index into the chord (3+ = an octave up), or rest
	lead   [][]int  // phrases of 16 steps, semitones from the tonic (an octave up)
	plan   []int    // which phrase the lead plays in each bar (-1 = silent)
	kick   string
	snare  string
	hat    string
}

var (
	chI   = [3]int{0, 3, 7}
	chIII = [3]int{3, 7, 10}
	chIV  = [3]int{5, 8, 12}
	chV   = [3]int{-5, -1, 2}
	chVI  = [3]int{-4, 0, 3}
	chVII = [3]int{-2, 2, 5}
	chbII = [3]int{1, 5, 8}
)

const r = rest

var songs = []*song{
	{ // 0: title - slow and brooding
		bpm: 100, root: 45,
		chords: [][3]int{chI, chI, chVI, chVI, chIII, chIII, chVII, chVII},
		bass:   []int{0, r, r, r, 0, r, r, 12, 0, r, r, r, 7, r, 12, r},
		arp:    []int{0, 1, 2, 3, 2, 1, 0, 1, 2, 3, 4, 3, 2, 1, 2, 1},
		lead: [][]int{
			{12, r, r, r, r, r, 10, r, 7, r, r, r, r, r, r, r},
			{3, r, r, r, 5, r, 7, r, r, r, r, r, 10, r, r, r},
			{12, r, r, 15, r, r, 12, r, 10, r, r, r, 7, r, 5, r},
			{7, r, r, r, r, r, r, r, r, r, r, r, r, r, r, r},
		},
		plan: []int{-1, -1, -1, -1, -1, -1, -1, -1, 0, 1, 0, 2, 0, 1, 2, 3},
		kick: "x.......x.......", snare: "....x.......x...", hat: "..x...x...x...x.",
	},
	{ // 1: patrol - driving
		bpm: 132, root: 40,
		chords: [][3]int{chI, chI, chVI, chVII, chI, chI, chIII, chVII},
		bass:   []int{0, r, 0, 0, r, 0, r, 0, 0, r, 0, 0, r, 0, 12, 7},
		arp:    []int{0, r, 2, r, 1, r, 3, r, 2, r, 4, r, 3, r, 2, 1},
		lead: [][]int{
			{12, r, 12, r, 15, r, 12, r, 10, r, r, 7, r, r, 10, r},
			{12, r, r, r, 7, r, 10, r, 12, r, 15, r, 17, r, r, r},
			{19, r, 17, r, 15, r, 12, r, 15, r, r, r, 10, r, 12, r},
			{7, r, r, r, r, r, 10, 12, r, r, r, r, r, r, r, r},
		},
		plan: []int{-1, -1, -1, -1, 0, 1, 0, 3, 0, 1, 2, 3, 2, 1, 2, 3},
		kick: "x...x...x...x.x.", snare: "....x.......x...", hat: "x.xxx.xxx.xxx.xx",
	},
	{ // 2: pursuit - faster, darker
		bpm: 140, root: 38,
		chords: [][3]int{chI, chIII, chVII, chVI, chI, chIII, chIV, chV},
		bass:   []int{0, 0, 12, 0, 0, 12, 0, 0, 0, 0, 12, 0, 7, 0, 12, 0},
		arp:    []int{3, 2, 1, 0, 3, 2, 1, 0, 4, 3, 2, 1, 4, 3, 2, 1},
		lead: [][]int{
			{12, r, r, 10, r, r, 7, r, 5, r, 7, r, 10, r, r, r},
			{15, r, r, 12, r, r, 10, r, 12, r, r, r, r, r, 10, 12},
			{17, r, 15, r, 12, r, 15, r, 17, r, 19, r, 22, r, r, r},
			{12, r, r, r, r, r, r, r, 7, r, r, r, r, r, r, r},
		},
		plan: []int{-1, -1, -1, -1, 0, 1, 0, 3, 2, 1, 2, 3, 0, 1, 2, 3},
		kick: "x..x..x.x..x..x.", snare: "....x.......x..x", hat: "xxxxxxxxxxxxxxxx",
	},
	{ // 3: boss - menacing
		bpm: 150, root: 36,
		chords: [][3]int{chI, chI, chbII, chI, chI, chI, chbII, chV},
		bass:   []int{0, 0, r, 0, 0, r, 0, 0, r, 0, 0, r, 12, r, 1, r},
		arp:    []int{0, 2, 1, 2, 0, 2, 1, 2, 3, 2, 1, 2, 3, 2, 4, 2},
		lead: [][]int{
			{12, r, r, r, 13, r, 12, r, r, r, 7, r, 8, r, 7, r},
			{12, r, 15, r, 13, r, 12, r, 19, r, r, r, 18, r, r, r},
			{24, r, r, 19, r, r, 20, r, 19, r, 15, r, 13, r, 12, r},
			{12, 12, r, 12, r, r, r, r, 13, 13, r, 13, r, r, r, r},
		},
		plan: []int{3, -1, 3, -1, 0, 1, 0, 2, 3, 1, 3, 2, 0, 1, 2, 2},
		kick: "x.x...x.x.x...x.", snare: "....x.......x.xx", hat: "x.x.x.x.x.x.x.x.",
	},
}

func mtof(n int) float64 { return 440 * math.Pow(2, float64(n-69)/12) }

// setSong switches the music. bpmBoost speeds the song up (later levels).
func (a *audio) setSong(id int, bpmBoost float64) {
	a.wantSong, a.wantBPM = id, bpmBoost
	if !a.ok {
		return
	}
	if id < 0 {
		a.song = nil
		return
	}
	s := *songs[id%len(songs)]
	s.bpm += bpmBoost
	a.song = &s
	a.step = 0
	a.nextT = a.now() + 0.1
}

// tick schedules upcoming notes a little ahead of the audio clock.
func (a *audio) tick() {
	if !a.ok || a.song == nil {
		return
	}
	now := a.now()
	if a.nextT < now-0.2 { // the tab was in the background
		a.nextT = now + 0.05
	}
	stepLen := 60 / a.song.bpm / 4
	for a.nextT < now+0.15 {
		if !a.musicOff && !a.muted {
			a.scheduleStep(a.step, a.nextT, stepLen)
		}
		a.nextT += stepLen
		a.step++
	}
}

func (a *audio) scheduleStep(n int, t, stepLen float64) {
	s := a.song
	i := n % 16
	bar := n / 16
	ch := s.chords[bar%len(s.chords)]

	if b := s.bass[i]; b != rest {
		a.tone("sawtooth", mtof(s.root+ch[0]+b-12), mtof(s.root+ch[0]+b-12), stepLen*1.8, 0.3, t, a.bassF)
	}
	if v := s.arp[i]; v != rest {
		note := s.root + ch[v%3] + 12*(v/3) + 12
		a.tone("square", mtof(note), mtof(note), stepLen*1.2, 0.045, t, a.music)
	}
	if p := s.plan[bar%len(s.plan)]; p >= 0 {
		if v := s.lead[p][i]; v != rest {
			f := mtof(s.root + v + 12)
			a.tone("sawtooth", f, f, stepLen*2.6, 0.1, t, a.leadF)
			a.tone("triangle", f*1.005, f*1.005, stepLen*2.6, 0.1, t, a.leadF)
		}
	}
	if s.kick[i] == 'x' {
		a.tone("sine", 150, 42, 0.14, 0.7, t, a.music)
	}
	if s.snare[i] == 'x' {
		a.hiss("bandpass", 1900, 1900, 0.11, 0.35, t, a.music)
		a.tone("triangle", 220, 140, 0.07, 0.2, t, a.music)
	}
	if s.hat[i] == 'x' {
		v := 0.07
		if i%4 == 2 {
			v = 0.12
		}
		a.hiss("highpass", 7500, 7500, 0.035, v, t, a.music)
	}
}
