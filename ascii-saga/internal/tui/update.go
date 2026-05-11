package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/agent"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.Ready = m.Width >= m.Cfg.TUI.MinTermWidth && m.Height >= m.Cfg.TUI.MinTermHeight
		return m, nil

	case llmHealthMsg:
		m.LLMHealthy = msg.OK
		if msg.OK {
			m.StatusLine = "LM Studio conectado. " + m.StatusLine
		} else {
			m.StatusLine = "Atenção: o LM Studio parece offline. Aldric e Sira dormem."
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "ctrl+q":
			return m, tea.Quit
		case "enter":
			if m.Busy {
				return m, nil
			}
			input := strings.TrimSpace(m.Input.Value())
			if input == "" {
				return m, nil
			}
			// Slash commands
			if strings.HasPrefix(input, "/") {
				m.handleCommand(input)
				m.Input.SetValue("")
				return m, nil
			}
			// Numbered shortcuts pick a suggestion
			if len(input) == 1 && input[0] >= '1' && input[0] <= '9' {
				idx := int(input[0] - '1')
				if idx >= 0 && idx < len(m.Suggestions) {
					input = m.Suggestions[idx]
				}
			}
			m.Input.SetValue("")
			m.NarrativeBuf.Reset()
			m.NarrativeDone = false
			m.CurrentTitle = "..."
			m.CurrentSubtitle = ""
			m.StatusLine = "O Mestre pondera..."
			m.Busy = true
			return m, runScene(m.Eng, input)
		}

	case sceneDoneMsg:
		m.Busy = false
		m.NarrativeDone = true
		m.CurrentTitle = msg.Out.Blueprint.Title
		m.CurrentSubtitle = msg.Out.Blueprint.Subtitle
		m.CurrentArt = msg.Out.Art.Art
		m.ArtSource = msg.Out.Art.Source
		m.NarrativeBuf.Reset()
		m.NarrativeBuf.WriteString(msg.Out.Narrative)
		if len(msg.Out.Suggestions) > 0 {
			m.Suggestions = msg.Out.Suggestions
		}
		m.LastEvents = msg.Out.Events
		m.LastTick = time.Now()
		m.StatusLine = composeStatus(msg.Out.Events, msg.Out.Art.Source)
		return m, nil

	case sceneErrMsg:
		m.Busy = false
		m.NarrativeDone = true
		m.StatusLine = "Erro: " + msg.Err.Error()
		return m, nil
	}

	var cmd tea.Cmd
	m.Input, cmd = m.Input.Update(msg)
	return m, cmd
}

func (m *Model) handleCommand(cmd string) {
	switch cmd {
	case "/quit", "/exit":
		// soft-handled by the next key cycle when input is "/quit" — we
		// just set status; tea.Quit would be cleaner via a returned cmd
		// but keeping commands non-disruptive in this minimal wiring.
		m.StatusLine = "Use Ctrl+C ou Ctrl+Q para sair."
	case "/sheet":
		ch := m.State.GetCharacter("jogador")
		if ch != nil {
			m.StatusLine = "Ficha: " + ch.Name + " (" + ch.Class + ") Nv " + itoa(ch.Level) +
				" — XP " + itoa(ch.XP) + " — HP " + itoa(ch.HP) + "/" + itoa(ch.HPMax)
		}
	case "/save":
		if err := m.Eng.Store.Save(m.State); err != nil {
			m.StatusLine = "Falha ao salvar: " + err.Error()
		} else {
			m.StatusLine = "Estado salvo."
		}
	case "/help":
		m.StatusLine = "Comandos: /sheet /save /help — número (1-3) escolhe sugestão."
	default:
		m.StatusLine = "Comando desconhecido: " + cmd
	}
}

func composeStatus(events []agent.Event, artSource string) string {
	parts := []string{}
	for _, ev := range events {
		parts = append(parts, ev.Text)
	}
	if artSource != "" {
		parts = append(parts, "[arte: "+artSource+"]")
	}
	if len(parts) == 0 {
		return "Pronto."
	}
	return strings.Join(parts, " · ")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
