package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/agent"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/tui/effects"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/tui/theme"
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

	case frameTickMsg:
		m.advanceFrame()
		return m, frameTick()

	case pipelineStageMsg:
		m.PipelineStage = msg.Stage
		return m, nil

	case sceneStartedMsg:
		m.CurrentTitle = msg.Blueprint.Title
		m.CurrentSubtitle = msg.Blueprint.Subtitle
		m.TitlePulse = time.Now()
		m.NarrativeDone = false
		m.NarrativeScroll = 0
		m.Typewriter.Reset()
		return m, nil

	case artBusyMsg:
		m.ArtBusy = msg.Busy
		return m, nil

	case artReadyMsg:
		m.CurrentArt = msg.Art.Art
		m.ArtSource = msg.Art.Source
		m.ArtBusy = false
		return m, nil

	case narrativeChunkMsg:
		m.Typewriter.Append(msg.Chunk)
		return m, nil

	case narrativeDoneMsg:
		m.NarrativeDone = true
		return m, nil

	case sceneDoneMsg:
		m.Busy = false
		m.NarrativeDone = true
		m.PipelineStage = "idle"
		m.CurrentTitle = msg.Out.Blueprint.Title
		m.CurrentSubtitle = msg.Out.Blueprint.Subtitle
		m.CurrentArt = msg.Out.Art.Art
		m.ArtSource = msg.Out.Art.Source
		m.ArtBusy = false
		// If streamed chunks ended early or scene_meta leaked, take the
		// engine's canonical narrative as the source of truth — but only
		// if it differs from what we already revealed.
		if msg.Out.Narrative != "" {
			m.Typewriter.Reset()
			m.Typewriter.Append(msg.Out.Narrative)
		}
		if len(msg.Out.Suggestions) > 0 {
			m.Suggestions = msg.Out.Suggestions
		}
		m.LastEvents = msg.Out.Events
		m.captureEffectsFrom(msg.Out.Events)
		m.StatusLine = composeStatus(msg.Out.Events, msg.Out.Art.Source)
		m.hpSnapshot = snapshotHP(m.State)
		return m, nil

	case sceneErrMsg:
		m.Busy = false
		m.NarrativeDone = true
		m.PipelineStage = "idle"
		m.StatusLine = "Erro: " + msg.Err.Error()
		return m, nil

	case themeCycleMsg:
		m.cycleTheme()
		return m, nil

	case tea.KeyMsg:
		// Modal screens have their own key handling first.
		if m.Screen != ScreenMain {
			return m.updateModal(msg)
		}
		return m.updateMain(msg)
	}

	var cmd tea.Cmd
	m.Input, cmd = m.Input.Update(msg)
	return m, cmd
}

// updateMain handles key events while the main four-panel view is showing.
func (m Model) updateMain(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "ctrl+q":
		return m, tea.Quit
	case "ctrl+f":
		m.FocusMode = !m.FocusMode
		return m, nil
	case "ctrl+t":
		m.cycleTheme()
		return m, nil
	case "pgup":
		m.NarrativeScroll++
		return m, nil
	case "pgdown":
		if m.NarrativeScroll > 0 {
			m.NarrativeScroll--
		}
		return m, nil
	case "esc":
		// Cancel scroll-back / clear status hint
		m.NarrativeScroll = 0
		return m, nil
	case "enter":
		if m.Busy {
			return m, nil
		}
		input := strings.TrimSpace(m.Input.Value())
		if input == "" {
			return m, nil
		}
		if strings.HasPrefix(input, "/") {
			cmd := m.handleCommand(input)
			m.Input.SetValue("")
			return m, cmd
		}
		if len(input) == 1 && input[0] >= '1' && input[0] <= '9' {
			idx := int(input[0] - '1')
			if idx >= 0 && idx < len(m.Suggestions) {
				input = m.Suggestions[idx]
			}
		}
		m.Input.SetValue("")
		m.Typewriter.Reset()
		m.NarrativeDone = false
		m.CurrentTitle = "..."
		m.CurrentSubtitle = ""
		m.StatusLine = "O Mestre pondera..."
		m.Busy = true
		m.PipelineStage = "director"
		m.hpSnapshot = snapshotHP(m.State)
		return m, runScene(m.Eng, m.Send, input)
	}
	var cmd tea.Cmd
	m.Input, cmd = m.Input.Update(msg)
	return m, cmd
}

// updateModal routes key events while a screen overlay is open.
func (m Model) updateModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "ctrl+c":
		m.Screen = ScreenMain
		m.JournalOffset = 0
		return m, nil
	case "pgup", "k", "up":
		if m.Screen == ScreenJournal {
			m.JournalOffset += 5
		}
		return m, nil
	case "pgdown", "j", "down":
		if m.Screen == ScreenJournal && m.JournalOffset > 0 {
			m.JournalOffset -= 5
			if m.JournalOffset < 0 {
				m.JournalOffset = 0
			}
		}
		return m, nil
	}
	return m, nil
}

// handleCommand routes slash commands. Returns a Cmd when the command
// triggers async work (e.g. save), otherwise nil.
func (m *Model) handleCommand(cmd string) tea.Cmd {
	switch cmd {
	case "/quit", "/exit":
		m.StatusLine = "Use Ctrl+C ou Ctrl+Q para sair."
		return nil
	case "/sheet":
		m.Screen = ScreenSheet
		return nil
	case "/journal":
		m.Screen = ScreenJournal
		m.JournalOffset = 0
		return nil
	case "/map":
		m.Screen = ScreenMap
		return nil
	case "/inventory":
		m.Screen = ScreenInventory
		return nil
	case "/help":
		m.Screen = ScreenHelp
		return nil
	case "/save":
		if err := m.Eng.Store.Save(m.State); err != nil {
			m.StatusLine = "Falha ao salvar: " + err.Error()
		} else {
			m.StatusLine = "Estado salvo."
		}
		return nil
	case "/theme":
		m.cycleTheme()
		return nil
	default:
		m.StatusLine = "Comando desconhecido: " + cmd + "  (tente /help)"
		return nil
	}
}

// advanceFrame is called by the frame tick. It advances every active
// animation; doesn't re-emit a tick (the Update handler does that).
func (m *Model) advanceFrame() {
	now := time.Now()
	m.Typewriter.Tick(now)
	if m.ArtBusy {
		m.ArtSpinner.Tick()
	}
	m.Confetti.Tick()
	// Decay HP flashes — Active() check happens in view; no explicit GC needed.
}

// captureEffectsFrom inspects judge events and spawns the matching
// animation: HP loss/gain triggers a per-character flash; level-ups
// trigger a confetti burst.
func (m *Model) captureEffectsFrom(events []agent.Event) {
	for _, ev := range events {
		switch ev.Kind {
		case "hp_change":
			m.HPFlash[ev.Who] = effects.NewFlash(700 * time.Millisecond)
		case "level_up":
			m.LevelUpAt[ev.Who] = time.Now()
			// burst from the center of the art panel area
			m.Confetti.Burst(float64(m.Cfg.TUI.ArtWidth)/2, float64(m.Cfg.TUI.ArtHeight)/2, 18)
		case "downed":
			m.HPFlash[ev.Who] = effects.NewFlash(1500 * time.Millisecond)
		}
	}
}

// cycleTheme advances to the next theme in m.Themes.
func (m *Model) cycleTheme() {
	if len(m.Themes) == 0 {
		return
	}
	idx := 0
	for i, t := range m.Themes {
		if t.Name == m.Theme.Name {
			idx = i
			break
		}
	}
	m.Theme = m.Themes[(idx+1)%len(m.Themes)]
	m.StatusLine = "Tema: " + m.Theme.Name
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

// stageLabel returns a human Portuguese label for the pipeline stage.
func stageLabel(s string) string {
	switch s {
	case "director":
		return "Diretor planejando..."
	case "master":
		return "Mestre narrando..."
	case "artist":
		return "Artista desenhando..."
	case "judge":
		return "Aplicando consequências..."
	case "idle", "":
		return ""
	}
	return s
}

// themeStr is a small helper since model.go and view.go both reach for
// the theme name from a foreign package.
func themeStr(t theme.Theme) string { return t.Name }

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
