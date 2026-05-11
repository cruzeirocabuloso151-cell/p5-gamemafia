package assets

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

var (
	ErrWrongHeight = errors.New("art has wrong number of lines")
	ErrWrongWidth  = errors.New("art line has wrong width")
	ErrIllegalRune = errors.New("art contains a rune outside the palette")
	ErrBadDensity  = errors.New("art density is too sparse or too saturated")
)

// Palette presets. The Director picks one via tags; the Artist enforces it.
var Palettes = map[string]string{
	"default": " ·.,:;-_+*=#%@▪▫░▒▓█◆◇◈◉○●▲▼◀▶◢◣◤◥┌┐└┘├┤┬┴┼─│✦✧✶✷",
	"lineart": " ─│┌┐└┘├┤┬┴┼╭╮╯╰═║╔╗╚╝╠╣╦╩╬·.",
	"shaded":  " ░▒▓█·.,:;",
}

type Canvas struct{ W, H int }

// Validate enforces hard contracts so renders never break the TUI layout.
func Validate(art string, c Canvas, paletteName string) error {
	pal, ok := Palettes[paletteName]
	if !ok {
		pal = Palettes["default"]
	}
	allowed := map[rune]struct{}{}
	for _, r := range pal {
		allowed[r] = struct{}{}
	}
	allowed['\n'] = struct{}{}

	lines := strings.Split(strings.TrimRight(art, "\n"), "\n")
	if len(lines) != c.H {
		return fmt.Errorf("%w: got %d, want %d", ErrWrongHeight, len(lines), c.H)
	}
	for i, ln := range lines {
		if utf8.RuneCountInString(ln) != c.W {
			return fmt.Errorf("%w: line %d got %d, want %d",
				ErrWrongWidth, i, utf8.RuneCountInString(ln), c.W)
		}
		for _, r := range ln {
			if _, ok := allowed[r]; !ok {
				return fmt.Errorf("%w: line %d rune %q", ErrIllegalRune, i, r)
			}
		}
	}
	d := density(lines)
	if d < 0.05 || d > 0.95 {
		return fmt.Errorf("%w: %.2f", ErrBadDensity, d)
	}
	return nil
}

// AutoFix tries to coerce slightly-wrong art into shape: pad/trim line widths,
// pad/trim line counts, replace illegal runes with their nearest palette equivalent.
// Returns the fixed art and whether anything still violates the contract.
func AutoFix(art string, c Canvas, paletteName string) (string, error) {
	pal, ok := Palettes[paletteName]
	if !ok {
		pal = Palettes["default"]
	}
	allowed := map[rune]struct{}{}
	for _, r := range pal {
		allowed[r] = struct{}{}
	}

	lines := strings.Split(strings.TrimRight(art, "\n"), "\n")

	for len(lines) < c.H {
		lines = append(lines, strings.Repeat(" ", c.W))
	}
	if len(lines) > c.H {
		lines = lines[:c.H]
	}

	for i, ln := range lines {
		fixed := make([]rune, 0, c.W)
		for _, r := range ln {
			if _, ok := allowed[r]; !ok {
				r = nearestSafeRune(r)
			}
			fixed = append(fixed, r)
			if len(fixed) >= c.W {
				break
			}
		}
		for len(fixed) < c.W {
			fixed = append(fixed, ' ')
		}
		lines[i] = string(fixed)
	}
	out := strings.Join(lines, "\n")
	return out, Validate(out, c, paletteName)
}

func nearestSafeRune(r rune) rune {
	switch {
	case r == '\t':
		return ' '
	case r >= 'A' && r <= 'Z':
		return '#'
	case r >= 'a' && r <= 'z':
		return '·'
	case r >= '0' && r <= '9':
		return '◆'
	}
	return ' '
}

func density(lines []string) float64 {
	total, filled := 0, 0
	for _, ln := range lines {
		for _, r := range ln {
			total++
			if r != ' ' && r != '·' && r != '.' {
				filled++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(filled) / float64(total)
}
