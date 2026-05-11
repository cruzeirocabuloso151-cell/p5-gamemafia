package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderModal dispatches to the active screen renderer and frames it as
// a centered card over the underlying layout.
func (m Model) renderModal() string {
	t := m.Theme

	var title, body string
	switch m.Screen {
	case ScreenSheet:
		title, body = "FICHAS DO GRUPO", m.renderSheetScreen()
	case ScreenJournal:
		title, body = "CRÔNICA", m.renderJournalScreen()
	case ScreenMap:
		title, body = "MAPA — LOCAIS VISITADOS", m.renderMapScreen()
	case ScreenInventory:
		title, body = "INVENTÁRIO COMPLETO", m.renderInventoryScreen()
	case ScreenHelp:
		title, body = "AJUDA", m.renderHelpScreen()
	default:
		return m.View()
	}

	w := m.Width - 6
	if w > 100 {
		w = 100
	}
	h := m.Height - 6
	if h < 14 {
		h = 14
	}

	titleStyle := lipgloss.NewStyle().Foreground(t.Title).Bold(true)
	hint := lipgloss.NewStyle().Foreground(t.Faint).Italic(true).
		Render("[Esc] voltar     [↑/↓ ou PgUp/PgDn] navegar")
	card := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).BorderForeground(t.Accent).
		Width(w).Height(h).
		Padding(1, 2).
		Foreground(t.Fg).
		Render(titleStyle.Render(title) + "\n\n" + body + "\n\n" + hint)

	// Center the card over a dimmed background.
	bg := lipgloss.NewStyle().Foreground(t.Faint)
	bgLine := bg.Render(strings.Repeat("·", m.Width))
	pad := strings.Repeat(bgLine+"\n", (m.Height-h)/2)
	return pad + lipgloss.PlaceHorizontal(m.Width, lipgloss.Center, card)
}

func (m Model) renderSheetScreen() string {
	t := m.Theme
	out := strings.Builder{}
	for _, c := range m.State.Characters {
		if !c.IsParty {
			continue
		}
		header := lipgloss.NewStyle().Foreground(t.Accent).Bold(true).
			Render(strings.ToUpper(c.Name))
		sub := dim(t, fmt.Sprintf("%s · Nível %d · XP %d", c.Class, c.Level, c.XP))
		hpStyle := lipgloss.NewStyle().Foreground(t.HPGood)
		if float64(c.HP)/float64(c.HPMax) < 0.34 {
			hpStyle = lipgloss.NewStyle().Foreground(t.HPLow).Bold(true)
		}
		hp := hpStyle.Render(fmt.Sprintf("HP %d/%d  %s", c.HP, c.HPMax, c.HPBar()))
		mood := ""
		if c.ID != "jogador" {
			mood = dim(t, fmt.Sprintf("Humor: %s   Bond: %s", nzv(c.Mood, "—"), bondLabel(c.Bond)))
		}
		out.WriteString(header + "\n")
		out.WriteString("  " + sub + "\n")
		out.WriteString("  " + hp + "\n")
		if mood != "" {
			out.WriteString("  " + mood + "\n")
		}
		out.WriteString("\n")
	}
	return out.String()
}

func (m Model) renderJournalScreen() string {
	t := m.Theme
	ev := m.State.Events
	if len(ev) == 0 {
		return dim(t, "Sua crônica ainda está em branco. Viva uma cena e ela escreverá a si mesma.")
	}
	// Newest first when offset == 0; PgUp pages further back.
	start := len(ev) - 1 - m.JournalOffset
	if start < 0 {
		start = 0
	}
	pageSize := 12
	end := start - pageSize
	if end < 0 {
		end = -1
	}
	out := strings.Builder{}
	for i := start; i > end; i-- {
		entry := ev[i]
		ts := entry.Timestamp.Format("15:04")
		line := fmt.Sprintf("[%s] %s — %s", ts, kindLabel(entry.Kind), entry.Text)
		out.WriteString(line + "\n")
	}
	out.WriteString("\n")
	out.WriteString(dim(t, fmt.Sprintf("Mostrando %d–%d de %d eventos", end+2, start+1, len(ev))))
	return out.String()
}

func (m Model) renderMapScreen() string {
	t := m.Theme
	// Locations are tracked as world_flags with prefix "location_visited:".
	// Also infer from event log entries tagged with location names.
	visited := map[string]struct{}{}
	for k := range m.State.WorldFlags {
		if strings.HasPrefix(k, "location_visited:") {
			visited[strings.TrimPrefix(k, "location_visited:")] = struct{}{}
		}
	}
	for _, ev := range m.State.Events {
		// crude: any "art: library" event referencing a known location
		// name isn't tracked here; this stays empty until the engine
		// starts writing location flags. Document for the player.
		_ = ev
	}
	if len(visited) == 0 {
		return dim(t, "Nenhum local registrado ainda. Aventure-se e o mapa começará a se desenhar.")
	}
	out := strings.Builder{}
	for name := range visited {
		fmt.Fprintf(&out, "  ◆ %s\n", name)
	}
	return out.String()
}

func (m Model) renderInventoryScreen() string {
	t := m.Theme
	out := strings.Builder{}
	for _, c := range m.State.Characters {
		if !c.IsParty {
			continue
		}
		out.WriteString(lipgloss.NewStyle().Foreground(t.Accent).Bold(true).
			Render(strings.ToUpper(c.Name)) + "\n")
		if len(c.Inventory) == 0 {
			out.WriteString("  " + dim(t, "vazio") + "\n\n")
			continue
		}
		for _, it := range c.Inventory {
			fmt.Fprintf(&out, "  ◆ %s\n", it.Name)
			if it.Description != "" {
				fmt.Fprintf(&out, "    %s\n", dim(t, it.Description))
			}
		}
		out.WriteString("\n")
	}
	return out.String()
}

func (m Model) renderHelpScreen() string {
	t := m.Theme
	rows := [][2]string{
		{"Enter", "executa o comando ou escolhe sugestão (1, 2, 3)"},
		{"Ctrl+F", "alterna modo foco (esconde fichas, expande narrativa)"},
		{"Ctrl+T", "cicla entre temas (dark_amber → moonlit → parchment)"},
		{"PgUp/PgDn", "rola a narrativa para revisitar trechos passados"},
		{"Esc", "fecha modal ou volta ao live"},
		{"Ctrl+C/Q", "encerra a sessão (autosave acontece após cada cena)"},
		{"", ""},
		{"/sheet", "abre fichas detalhadas do grupo"},
		{"/journal", "abre a crônica de eventos (do mais recente ao início)"},
		{"/map", "lista locais visitados nesta campanha"},
		{"/inventory", "inventário completo com descrições"},
		{"/save", "força um save manual no SQLite"},
		{"/theme", "equivalente a Ctrl+T"},
		{"/help", "esta tela"},
	}
	out := strings.Builder{}
	for _, r := range rows {
		if r[0] == "" {
			out.WriteString("\n")
			continue
		}
		fmt.Fprintf(&out, "  %s%s%s\n",
			lipgloss.NewStyle().Foreground(t.Accent).Width(14).Render(r[0]),
			"  ",
			r[1])
	}
	return out.String()
}

func bondLabel(b int) string {
	switch {
	case b >= 70:
		return "ferrenho (+" + itoa(b) + ")"
	case b >= 30:
		return "confiável (+" + itoa(b) + ")"
	case b >= 0:
		return "cordial (+" + itoa(b) + ")"
	case b >= -30:
		return "tenso (" + itoa(b) + ")"
	}
	return "hostil (" + itoa(b) + ")"
}

func kindLabel(k string) string {
	switch k {
	case "xp_gain":
		return "XP"
	case "level_up":
		return "NÍVEL"
	case "hp_change":
		return "HP"
	case "item_added":
		return "ITEM"
	case "item_removed":
		return "PERDA"
	case "downed":
		return "QUEDA"
	}
	return strings.ToUpper(k)
}

func nzv(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

