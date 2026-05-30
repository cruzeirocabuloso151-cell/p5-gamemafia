package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/state"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/tui/theme"
)

// View composes the screen. Modals take precedence — when one is open
// the four-panel main view is replaced by a centered card.
func (m Model) View() string {
	if !m.Ready {
		return m.smallTerminalView()
	}
	if m.Screen != ScreenMain {
		return m.renderModal()
	}
	return m.mainView()
}

func (m Model) mainView() string {
	t := m.Theme

	header := m.renderHeader()
	body := m.renderBody()

	sugStyle := lipgloss.NewStyle().Foreground(t.Faint)
	sugLine := strings.Builder{}
	for i, s := range m.Suggestions {
		if i > 0 {
			sugLine.WriteString("   ")
		}
		fmt.Fprintf(&sugLine, "[%d] %s", i+1, s)
	}

	prompt := m.Input.View()
	if m.Busy {
		prompt = lipgloss.NewStyle().Foreground(t.Faint).
			Render("("+stageLabel(m.PipelineStage)+") ") + m.Input.View()
	}

	footer := m.renderFooter()

	return strings.Join([]string{
		header,
		body,
		sugStyle.Render("  " + sugLine.String()),
		"  " + prompt,
		footer,
	}, "\n")
}

func (m Model) renderHeader() string {
	t := m.Theme
	title := nz(strings.ToUpper(m.CurrentTitle), "ASCII SAGA")
	subtitle := nz(m.CurrentSubtitle, "uma jornada que se desenha enquanto se vive")
	// LM Studio indicator: green dot if healthy, red if not.
	dot := lipgloss.NewStyle().Foreground(t.HPGood).Render("●")
	if !m.LLMHealthy {
		dot = lipgloss.NewStyle().Foreground(t.HPLow).Render("●")
	}
	left := lipgloss.NewStyle().Foreground(t.Title).Bold(true).Render(title) +
		"   " + dim(t, subtitle)
	right := dot + " " + dim(t, "LM Studio") + "   " +
		dim(t, fmt.Sprintf("cena %d", m.State.SceneIndex)) + "   " +
		dim(t, "tema: "+themeStr(m.Theme))

	bar := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(t.Border).
		Width(m.Width - 2).
		Padding(0, 1)

	gap := m.Width - 4 - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return bar.Render(left + strings.Repeat(" ", gap) + right)
}

func (m Model) renderBody() string {
	t := m.Theme
	bodyHeight := m.Height - 7
	if bodyHeight < 12 {
		bodyHeight = 12
	}

	artW := m.Cfg.TUI.ArtWidth + 4
	if artW > m.Width/3 {
		artW = m.Width / 3
	}
	sheetW := 30
	if m.FocusMode {
		sheetW = 0
		artW = m.Cfg.TUI.ArtWidth + 4
	}
	if sheetW > m.Width/4 {
		sheetW = m.Width / 4
	}
	storyW := m.Width - artW - sheetW - 4
	if storyW < 30 {
		storyW = 30
	}

	artBox := lipgloss.NewStyle().
		Width(artW).Height(bodyHeight).
		Border(lipgloss.RoundedBorder()).BorderForeground(t.Border).
		Foreground(t.Accent).Padding(0, 1).
		Render(m.renderArt(artW-4, bodyHeight-2))

	storyBox := lipgloss.NewStyle().
		Width(storyW).Height(bodyHeight).
		Border(lipgloss.RoundedBorder()).BorderForeground(t.Border).
		Foreground(t.Fg).Padding(0, 1).
		Render(m.renderStory(storyW-4, bodyHeight-2))

	if m.FocusMode {
		return lipgloss.JoinHorizontal(lipgloss.Top, artBox, storyBox)
	}

	sheetsBox := lipgloss.NewStyle().
		Width(sheetW).Height(bodyHeight).
		Border(lipgloss.RoundedBorder()).BorderForeground(t.Border).
		Foreground(t.Fg).Padding(0, 1).
		Render(m.renderSheets(sheetW-4, bodyHeight-2))

	return lipgloss.JoinHorizontal(lipgloss.Top, artBox, storyBox, sheetsBox)
}

func (m Model) renderFooter() string {
	t := m.Theme
	statusStyle := lipgloss.NewStyle().Foreground(t.Faint).Italic(true)
	hints := []string{
		"[Enter] agir",
		"[1-3] sugestões",
		"[Ctrl+F] foco",
		"[Ctrl+T] tema",
		"[PgUp/PgDn] rolar",
		"[/help] comandos",
	}
	hintLine := strings.Join(hints, "   ")
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(t.Border).
		Width(m.Width - 2).
		Padding(0, 1).
		Render(statusStyle.Render(m.StatusLine) + "\n  " + dim(t, hintLine))
}

func (m Model) renderArt(maxW, maxH int) string {
	if m.CurrentArt == "" && !m.ArtBusy {
		return placeholder(maxW, maxH, "")
	}
	art := m.CurrentArt
	if m.ArtBusy {
		art = m.busyArt(maxW, maxH)
	}
	lines := strings.Split(art, "\n")
	if len(lines) > maxH {
		lines = lines[:maxH]
	}
	for i, ln := range lines {
		if r := []rune(ln); len(r) > maxW {
			lines[i] = string(r[:maxW])
		}
	}
	// Confetti overlay — paint particles on top of the art lines.
	if m.Confetti.Active() {
		lines = m.overlayConfetti(lines, maxW, maxH)
	}
	return strings.Join(lines, "\n")
}

// busyArt returns the placeholder shown while the Artist is rendering.
// Returns raw runes (no ANSI) so the outer panel's foreground style
// applies uniformly and rune slicing in renderArt stays safe.
func (m Model) busyArt(w, h int) string {
	rows := make([]string, h)
	for i := range rows {
		rows[i] = strings.Repeat(" ", w)
	}
	cx, cy := w/2, h/2
	label := "desenhando..."
	stamp := []rune(string(m.ArtSpinner.Glyph()) + " " + label)
	x := cx - len(stamp)/2
	if x < 0 {
		x = 0
	}
	row := []rune(rows[cy])
	for i, r := range stamp {
		if x+i < w {
			row[x+i] = r
		}
	}
	rows[cy] = string(row)
	return strings.Join(rows, "\n")
}

func (m Model) overlayConfetti(lines []string, w, h int) []string {
	t := m.Theme
	grids := make([][]rune, len(lines))
	for i, ln := range lines {
		grids[i] = []rune(ln)
		for len(grids[i]) < w {
			grids[i] = append(grids[i], ' ')
		}
	}
	colors := []lipgloss.Color{t.Accent, t.HPGood, t.Companion, t.Title}
	for _, p := range m.Confetti.Particles {
		x, y := int(p.X), int(p.Y)
		if y < 0 || y >= h || x < 0 || x >= w {
			continue
		}
		grids[y][x] = p.Glyph
		_ = colors // color application is left to a future pass; per-rune
		// coloring in lipgloss requires segment styling — for now the
		// confetti glyphs ride the panel foreground, which still pops.
	}
	out := make([]string, len(grids))
	for i, g := range grids {
		out[i] = string(g)
	}
	return out
}

func (m Model) renderStory(maxW, maxH int) string {
	t := m.Theme
	body := m.Typewriter.Visible()
	if body == "" {
		body = dim(t, "Pressione Enter ou digite uma ação para começar.")
	}
	// Append a blinking caret while the typewriter is still revealing.
	if m.Typewriter.Active() {
		body += "▍"
	}
	wrapped := softWrap(body, maxW)
	out := strings.Builder{}
	for _, ln := range strings.Split(wrapped, "\n") {
		trimmed := strings.TrimSpace(ln)
		if strings.HasPrefix(trimmed, "— Aldric") || strings.HasPrefix(trimmed, "— Sira") {
			out.WriteString(lipgloss.NewStyle().Foreground(t.Companion).Italic(true).Render(ln))
		} else {
			out.WriteString(ln)
		}
		out.WriteString("\n")
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	// Scroll-back: positive m.NarrativeScroll lifts the visible window up.
	if m.NarrativeScroll > 0 {
		end := len(lines) - m.NarrativeScroll
		if end < maxH {
			end = maxH
		}
		if end > len(lines) {
			end = len(lines)
		}
		start := end - maxH
		if start < 0 {
			start = 0
		}
		lines = lines[start:end]
		marker := dim(t, fmt.Sprintf("↑ rolando (%d linhas atrás) — Esc para retornar", m.NarrativeScroll))
		lines = append([]string{marker}, lines...)
	}
	if len(lines) > maxH {
		lines = lines[len(lines)-maxH:]
	}
	for len(lines) < maxH {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderSheets(maxW, maxH int) string {
	t := m.Theme
	out := strings.Builder{}
	for _, c := range m.State.Characters {
		if !c.IsParty {
			continue
		}
		marker := "◆"
		if c.ID == "aldric" || c.ID == "sira" {
			marker = "◇"
		}
		// Level-up halo: bright accent + ✦ for ~2.5s after the event.
		nameRender := strings.ToUpper(c.Name)
		if lu, ok := m.LevelUpAt[c.ID]; ok && timeRecent(lu, 2500) {
			nameRender = "✦ " + nameRender + " ✦"
		}
		title := lipgloss.NewStyle().Foreground(t.Accent).Bold(true).
			Render(fmt.Sprintf("%s %s", marker, nameRender))

		sub := dim(t, fmt.Sprintf("%s  Nv %d", c.Class, c.Level))

		hpStyle := lipgloss.NewStyle().Foreground(t.HPGood)
		if float64(c.HP)/float64(c.HPMax) < 0.34 {
			hpStyle = lipgloss.NewStyle().Foreground(t.HPLow).Bold(true)
		}
		if f, ok := m.HPFlash[c.ID]; ok && f.Active() {
			hpStyle = lipgloss.NewStyle().Foreground(t.HPLow).Bold(true).Underline(true)
		}
		hpLine := hpStyle.Render(fmt.Sprintf("HP %d/%d  %s", c.HP, c.HPMax, c.HPBar()))
		xpLine := dim(t, fmt.Sprintf("XP %d", c.XP))

		out.WriteString(title + "\n")
		out.WriteString(sub + "\n")
		out.WriteString(hpLine + "\n")
		out.WriteString(xpLine + "\n")
		if c.ID != "jogador" {
			out.WriteString(dim(t, fmt.Sprintf("%s · bond %d", nz(c.Mood, "—"), c.Bond)) + "\n")
		}
		out.WriteString("\n")
	}
	out.WriteString(lipgloss.NewStyle().Foreground(t.Accent).Render("INVENTÁRIO\n"))
	if pl := m.State.GetCharacter("jogador"); pl != nil {
		out.WriteString(formatInventory(pl, t.Faint))
	}

	rendered := strings.TrimRight(out.String(), "\n")
	lines := strings.Split(rendered, "\n")
	if len(lines) > maxH {
		lines = lines[:maxH]
	}
	for len(lines) < maxH {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func formatInventory(c *state.Character, dimColor lipgloss.Color) string {
	if len(c.Inventory) == 0 {
		return lipgloss.NewStyle().Foreground(dimColor).Render("• vazio\n")
	}
	out := strings.Builder{}
	for i, it := range c.Inventory {
		if i >= 6 {
			fmt.Fprintf(&out, "• … e mais %d\n", len(c.Inventory)-6)
			break
		}
		fmt.Fprintf(&out, "• %s\n", it.Name)
	}
	return out.String()
}

func (m Model) smallTerminalView() string {
	return lipgloss.NewStyle().
		Foreground(m.Theme.HPLow).Bold(true).
		Padding(2, 4).
		Render(fmt.Sprintf(
			"Terminal pequeno demais.\n"+
				"Mínimo: %d×%d  Atual: %d×%d\n\n"+
				"Redimensione e a saga continuará.",
			m.Cfg.TUI.MinTermWidth, m.Cfg.TUI.MinTermHeight, m.Width, m.Height))
}

func placeholder(w, h int, c string) string {
	row := strings.Repeat(c, w)
	if c == "" {
		row = strings.Repeat(" ", w)
	}
	rows := make([]string, h)
	for i := range rows {
		rows[i] = row
	}
	return strings.Join(rows, "\n")
}

func dim(t theme.Theme, s string) string {
	return lipgloss.NewStyle().Foreground(t.Faint).Render(s)
}

func nz(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

// softWrap is a unicode-naive word wrapper that respects rune
// boundaries. Good enough for Portuguese narrative prose.
func softWrap(s string, width int) string {
	if width < 1 {
		width = 1
	}
	out := strings.Builder{}
	for li, line := range strings.Split(s, "\n") {
		if li > 0 {
			out.WriteByte('\n')
		}
		words := strings.Fields(line)
		col := 0
		for _, w := range words {
			wn := len([]rune(w))
			if col == 0 {
				out.WriteString(w)
				col = wn
				continue
			}
			if col+1+wn > width {
				out.WriteByte('\n')
				out.WriteString(w)
				col = wn
				continue
			}
			out.WriteByte(' ')
			out.WriteString(w)
			col += 1 + wn
		}
	}
	return out.String()
}

// timeRecent reports whether t happened within the last ms milliseconds.
func timeRecent(t time.Time, ms int) bool {
	if t.IsZero() {
		return false
	}
	return time.Since(t) < time.Duration(ms)*time.Millisecond
}
