package assets

import (
	"embed"
	"path/filepath"
	"strings"
)

// Library is the curated fallback bank: hand-crafted ASCII pieces tagged
// by category that cover ~90% of common scenes without any LLM call.
//
// Pieces are embedded so the binary ships standalone — no external file
// dependencies at runtime.
//
//go:embed library/*/*.txt
var libraryFS embed.FS

type Piece struct {
	Category string
	Name     string
	Tags     []string
	Art      string
}

type Library struct {
	pieces []Piece
}

func LoadLibrary() (*Library, error) {
	lib := &Library{}
	dirs, err := libraryFS.ReadDir("library")
	if err != nil {
		return lib, err
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		files, err := libraryFS.ReadDir(filepath.Join("library", d.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".txt") {
				continue
			}
			b, err := libraryFS.ReadFile(filepath.Join("library", d.Name(), f.Name()))
			if err != nil {
				continue
			}
			name := strings.TrimSuffix(f.Name(), ".txt")
			tags := append([]string{d.Name()}, strings.Split(name, "_")...)
			lib.pieces = append(lib.pieces, Piece{
				Category: d.Name(),
				Name:     name,
				Tags:     tags,
				Art:      strings.TrimRight(string(b), "\n"),
			})
		}
	}
	return lib, nil
}

// Match returns the best library piece for the given tags. Scoring is
// simple tag overlap — good enough for fallback selection.
func (l *Library) Match(tags []string) (Piece, bool) {
	if len(l.pieces) == 0 {
		return Piece{}, false
	}
	want := map[string]struct{}{}
	for _, t := range tags {
		want[strings.ToLower(t)] = struct{}{}
	}
	bestScore := -1
	bestIdx := 0
	for i, p := range l.pieces {
		score := 0
		for _, t := range p.Tags {
			if _, ok := want[strings.ToLower(t)]; ok {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	return l.pieces[bestIdx], true
}

// ByCategory returns every piece in a category. Useful for UIs that
// want to let the player browse the bestiary, etc.
func (l *Library) ByCategory(cat string) []Piece {
	out := []Piece{}
	for _, p := range l.pieces {
		if p.Category == cat {
			out = append(out, p)
		}
	}
	return out
}

// All returns every loaded piece.
func (l *Library) All() []Piece { return l.pieces }
