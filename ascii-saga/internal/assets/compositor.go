package assets

import "strings"

// Layer is one stamp to mix into a composite scene. Transparent runes
// (space, ·, .) let the layer below show through.
type Layer struct {
	Art    string
	X, Y   int
	ZOrder int
}

func isTransparent(r rune) bool {
	return r == ' ' || r == '·' || r == '.' || r == 0
}

// Compose merges layers onto a blank canvas of (w, h). Layers are drawn
// in ascending ZOrder. Out-of-bounds runes are clipped silently.
func Compose(layers []Layer, w, h int) string {
	grid := make([][]rune, h)
	for i := range grid {
		grid[i] = make([]rune, w)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}
	// Stable z sort without pulling sort package every call.
	ord := append([]Layer(nil), layers...)
	for i := 1; i < len(ord); i++ {
		for j := i; j > 0 && ord[j].ZOrder < ord[j-1].ZOrder; j-- {
			ord[j], ord[j-1] = ord[j-1], ord[j]
		}
	}
	for _, ly := range ord {
		paintLayer(grid, ly, w, h)
	}
	out := strings.Builder{}
	for i, row := range grid {
		if i > 0 {
			out.WriteByte('\n')
		}
		for _, r := range row {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func paintLayer(grid [][]rune, ly Layer, w, h int) {
	lines := strings.Split(ly.Art, "\n")
	for li, line := range lines {
		y := ly.Y + li
		if y < 0 || y >= h {
			continue
		}
		x := ly.X
		for _, r := range line {
			if x >= 0 && x < w && !isTransparent(r) {
				grid[y][x] = r
			}
			x++
		}
	}
}
