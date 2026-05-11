package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/state"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/tui/theme"
)

// View composes the four-panel layout. Sizes are calculated from the
// terminal dimensions so we degrade gracefully on narrow terminals
// (down to the minimum specified in config).
func (m Model) View() string {
	if !m.Ready {
		return m.smallTerminalView()
	}

	t := m.Theme

	// Header band — title + subtitle + sessão
	titleStyle := lipgloss.NewStyle().
		Foreground(t.Title).Bold(true).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(t.Border).
		Width(m.Width - 2).
		Padding(0, 1)
	header := titleStyle.Render(fmt.Sprintf("%s   %s",
		nz(strings.ToUpper(m.CurrentTitle), "ASCII SAGA"),
		dim(t, nz(m.CurrentSubtitle, "uma jornada que se desenha enquanto se vive"))))

	// Body row: art | story | sheets
	bodyHeight := m.Height - 7
	if bodyHeight < 12 {
		bodyHeight = 12
	}
	artW := m.Cfg.TUI.ArtWidth + 2
	if artW > m.Width/3 {
		artW = m.Width / 3
	}
	sheetW := 28
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

	sheetsBox := lipgloss.NewStyle().
		Width(sheetW).Height(bodyHeight).
		Border(lipgloss.RoundedBorder()).BorderForeground(t.Border).
		Foreground(t.Fg).Padding(0, 1).
		Render(m.renderSheets(sheetW-4, bodyHeight-2))

	body := lipgloss.JoinHorizontal(lipgloss.Top, artBox, storyBox, sheetsBox)

	// Suggestions row
	sugStyle := lipgloss.NewStyle().Foreground(t.Faint)
	sugLine := strings.Builder{}
	for i, s := range m.Suggestions {
		if i > 0 {
			sugLine.WriteString("   ")
		}
		fmt.Fprintf(&sugLine, "[%d] %s", i+1, s)
	}

	// Input
	prompt := m.Input.View()
	if m.Busy {
		prompt = lipgloss.NewStyle().Foreground(t.Faint).Render("(aguardando o Mestre...) ") + m.Input.View()
	}

	// Status / footer
	statusStyle := lipgloss.NewStyle().Foreground(t.Faint).Italic(true)
	footer := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(t.Border).
		Width(m.Width - 2).
		Padding(0, 1).
		Render(statusStyle.Render(m.StatusLine) + "\n  /save  /sheet  /help")

	return strings.Join([]string{
		header,
		body,
		sugStyle.Render("  " + sugLine.String()),
		"  " + prompt,
		footer,
	}, "\n")
}

func (m Model) renderArt(maxW, maxH int) string {
	if m.CurrentArt == "" {
		return placeholder(maxW, maxH, "...")
	}
	lines := strings.Split(m.CurrentArt, "\n")
	if len(lines) > maxH {
		lines = lines[:maxH]
	}
	for i, ln := range lines {
		if r := []rune(ln); len(r) > maxW {
			lines[i] = string(r[:maxW])
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderStory(maxW, maxH int) string {
	t := m.Theme
	body := m.NarrativeBuf.String()
	if body == "" {
		body = dim(t, "Pressione Enter ou digite uma ação para começar.")
	}
	wrapped := softWrap(body, maxW)
	// Highlight companion lines.
	out := strings.Builder{}
	for _, ln := range strings.Split(wrapped, "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), "— Aldric") ||
			strings.HasPrefix(strings.TrimSpace(ln), "— Sira") {
			out.WriteString(lipgloss.NewStyle().Foreground(t.Companion).Italic(true).Render(ln))
		} else {
			out.WriteString(ln)
		}
		out.WriteString("\n")
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
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
		title := lipgloss.NewStyle().Foreground(t.Accent).Bold(true).
			Render(fmt.Sprintf("%s %s", marker, strings.ToUpper(c.Name)))
		sub := dim(t, fmt.Sprintf("%s  Nv %d", c.Class, c.Level))
		hpStyle := lipgloss.NewStyle().Foreground(t.HPGood)
		if float64(c.HP)/float64(c.HPMax) < 0.34 {
			hpStyle = lipgloss.NewStyle().Foreground(t.HPLow).Bold(true)
		}
		hpLine := hpStyle.Render(fmt.Sprintf("HP %d/%d  %s", c.HP, c.HPMax, c.HPBar()))
		xpLine := dim(t, fmt.Sprintf("XP %d", c.XP))
		out.WriteString(title + "\n")
		out.WriteString(sub + "\n")
		out.WriteString(hpLine + "\n")
		out.WriteString(xpLine + "\n\n")
	}
	out.WriteString(lipgloss.NewStyle().Foreground(t.Accent).Render("INVENTÁRIO\n"))
	if pl := m.State.GetCharacter("jogador"); pl != nil {
		out.WriteString(formatInventory(pl, t.Faint))
	}
	rendered := strings.TrimRight(out.String(), "\n")
	lines := strings.Split(rendered, "\n")
	for i, ln := range lines {
		if r := []rune(ln); len(r) > maxW+12 { // account for ANSI escape width
			lines[i] = string(r[:maxW+12])
		}
	}
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

// softWrap is a unicode-naive word wrapper. Good enough for narrative
// prose; we'll swap in muesli/reflow if we need anything fancier.
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
		for i, w := range words {
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
			_ = i
		}
	}
	return out.String()
}
