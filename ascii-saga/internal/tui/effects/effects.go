// Package effects contains the small state machines that drive the TUI
// animations: typewriter reveal, HP flash, level-up confetti, art spinner.
//
// Everything here is intentionally pure data — no Bubble Tea, no
// lipgloss. The TUI layer reads these states from the model and renders
// them; the model advances them on a 50ms frame tick.
package effects

import (
	"math/rand"
	"time"
)

// Typewriter buffers received text and reveals it character by character.
// The reveal rate is per-rune (not per-byte) so accented characters in
// Portuguese behave correctly.
type Typewriter struct {
	Received   []rune // total text received from the LLM so far
	Revealed   int    // how many runes are currently visible
	RatePerSec int    // baseline reveal speed, e.g. 80 chars/sec
	lastTick   time.Time
}

func NewTypewriter(ratePerSec int) *Typewriter {
	if ratePerSec <= 0 {
		ratePerSec = 80
	}
	return &Typewriter{RatePerSec: ratePerSec}
}

func (t *Typewriter) Reset() {
	t.Received = t.Received[:0]
	t.Revealed = 0
	t.lastTick = time.Time{}
}

// Append adds a fresh chunk of text from the streaming source.
func (t *Typewriter) Append(s string) {
	for _, r := range s {
		t.Received = append(t.Received, r)
	}
}

// Tick advances the reveal cursor based on elapsed wall-clock time.
// When the unrevealed buffer is large (the LLM bursted ahead), reveal
// accelerates so the player doesn't fall too far behind.
func (t *Typewriter) Tick(now time.Time) {
	if t.lastTick.IsZero() {
		t.lastTick = now
		return
	}
	elapsed := now.Sub(t.lastTick).Seconds()
	t.lastTick = now
	rate := float64(t.RatePerSec)
	backlog := len(t.Received) - t.Revealed
	switch {
	case backlog > 200:
		rate *= 3
	case backlog > 80:
		rate *= 1.8
	case backlog < 10:
		rate *= 0.6 // slow down on natural pauses for emphasis
	}
	advance := int(rate * elapsed)
	if advance < 1 && backlog > 0 {
		advance = 1
	}
	t.Revealed += advance
	if t.Revealed > len(t.Received) {
		t.Revealed = len(t.Received)
	}
}

// Visible returns the currently-revealed text.
func (t *Typewriter) Visible() string {
	if t.Revealed > len(t.Received) {
		t.Revealed = len(t.Received)
	}
	return string(t.Received[:t.Revealed])
}

// Done reports whether the buffer is fully revealed.
func (t *Typewriter) Done() bool {
	return t.Revealed >= len(t.Received)
}

// Active reports whether the typewriter still has work to do.
func (t *Typewriter) Active() bool { return !t.Done() }

// Flash is a short-lived "this value just changed" highlight.
// Used for HP loss/gain on the sheet panel.
type Flash struct {
	StartedAt time.Time
	Duration  time.Duration
}

func NewFlash(d time.Duration) Flash {
	return Flash{StartedAt: time.Now(), Duration: d}
}

func (f Flash) Active() bool {
	return !f.StartedAt.IsZero() && time.Since(f.StartedAt) < f.Duration
}

// Spinner cycles through braille frames at one frame per Tick.
type Spinner struct {
	Frame int
	Glyphs []rune
}

func NewSpinner() *Spinner {
	return &Spinner{Glyphs: []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}}
}

func (s *Spinner) Tick() { s.Frame++ }
func (s *Spinner) Glyph() rune {
	if len(s.Glyphs) == 0 {
		return '*'
	}
	return s.Glyphs[s.Frame%len(s.Glyphs)]
}

// Confetti is a tiny particle system for level-up celebration.
// Particles fall with simple gravity until they leave the canvas.
type Confetti struct {
	Particles []Particle
	rng       *rand.Rand
}

type Particle struct {
	X, Y   float64
	VX, VY float64
	Glyph  rune
	Color  int // theme palette index
	Life   int // frames remaining
}

func NewConfetti() *Confetti {
	return &Confetti{rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

var confettiGlyphs = []rune{'✦', '✧', '✶', '✷', '◆', '◇'}

// Burst spawns n particles emitted from (x,y) with a fan-out.
func (c *Confetti) Burst(x, y float64, n int) {
	for i := 0; i < n; i++ {
		c.Particles = append(c.Particles, Particle{
			X:     x + c.rng.Float64()*4 - 2,
			Y:     y,
			VX:    c.rng.Float64()*1.4 - 0.7,
			VY:    -1.0 - c.rng.Float64()*0.8,
			Glyph: confettiGlyphs[c.rng.Intn(len(confettiGlyphs))],
			Color: c.rng.Intn(4),
			Life:  30 + c.rng.Intn(10),
		})
	}
}

// Tick advances every particle. Dead particles are pruned in place.
func (c *Confetti) Tick() {
	live := c.Particles[:0]
	for _, p := range c.Particles {
		p.X += p.VX
		p.Y += p.VY
		p.VY += 0.18 // gravity
		p.Life--
		if p.Life > 0 {
			live = append(live, p)
		}
	}
	c.Particles = live
}

// Active reports whether any particle is still alive.
func (c *Confetti) Active() bool { return len(c.Particles) > 0 }
