package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/agent"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/assets"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/config"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/engine"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/state"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/tui/effects"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/tui/theme"
)

// Screen enumerates the modal overlays the user can pop up.
// ScreenMain is the normal four-panel layout.
type Screen int

const (
	ScreenMain Screen = iota
	ScreenSheet
	ScreenJournal
	ScreenMap
	ScreenInventory
	ScreenHelp
)

// Sender is the bridge from engine goroutines back into the TUI's
// message loop. main wires this to (*tea.Program).Send.
type Sender func(tea.Msg)

type Model struct {
	Cfg    config.Config
	Theme  theme.Theme
	Themes []theme.Theme // for Ctrl+T cycling
	Eng    *engine.Engine
	State  *state.GameState
	Logger agent.Logger

	// Communication out of background goroutines
	Send Sender

	// Layout
	Width, Height int
	Ready         bool
	FocusMode     bool

	// Screen state
	Screen        Screen
	JournalOffset int
	NarrativeScroll int // 0 = follow live; positive = look back

	// Live scene buffers
	CurrentTitle    string
	CurrentSubtitle string
	CurrentArt      string
	ArtSource       string
	Typewriter      *effects.Typewriter
	NarrativeDone   bool
	Suggestions     []string
	StatusLine      string
	PipelineStage   string // director|master|artist|judge|idle|""

	// Input
	Input textinput.Model
	Busy  bool

	// Health
	LLMHealthy bool

	// Effects
	HPFlash     map[string]effects.Flash // characterID -> flash state
	LevelUpAt   map[string]time.Time
	Confetti    *effects.Confetti
	ArtSpinner  *effects.Spinner
	ArtBusy     bool
	TitlePulse  time.Time

	// Last scene events
	LastEvents []agent.Event

	// HP snapshot for diffing on scene completion
	hpSnapshot map[string]int
}

func NewModel(cfg config.Config, eng *engine.Engine, gs *state.GameState, log agent.Logger, send Sender) Model {
	ti := textinput.New()
	ti.Placeholder = "O que você faz?"
	ti.Prompt = "› "
	ti.Focus()
	ti.CharLimit = 240

	if send == nil {
		send = func(tea.Msg) {}
	}

	themes := []theme.Theme{theme.DarkAmber(), theme.Moonlit(), theme.Parchment()}

	m := Model{
		Cfg:        cfg,
		Theme:      theme.Pick(cfg.TUI.Theme),
		Themes:     themes,
		Eng:        eng,
		State:      gs,
		Logger:     log,
		Send:       send,
		Input:      ti,
		Typewriter: effects.NewTypewriter(rateFromConfig(cfg.TUI.TypewriterMS)),
		Confetti:   effects.NewConfetti(),
		ArtSpinner: effects.NewSpinner(),
		HPFlash:    map[string]effects.Flash{},
		LevelUpAt:  map[string]time.Time{},
		hpSnapshot: snapshotHP(gs),
		StatusLine: "Pressione Enter para começar a sua jornada.",
		Suggestions: []string{
			"Olhar ao redor com atenção",
			"Conversar com Aldric",
			"Avançar pelo caminho",
		},
	}
	return m
}

func rateFromConfig(typewriterMS int) int {
	if typewriterMS <= 0 {
		return 80
	}
	// chars per second ≈ 1000 / ms_per_char
	return 1000 / typewriterMS
}

func snapshotHP(gs *state.GameState) map[string]int {
	out := map[string]int{}
	for _, c := range gs.Characters {
		out[c.ID] = c.HP
	}
	return out
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		checkHealth(m.Eng.Client),
		textinput.Blink,
		frameTick(),
	)
}

// runScene executes a scene in the background and emits StreamEvents
// as tea.Msgs through m.Send. The Cmd itself returns the final
// sceneDoneMsg (or sceneErrMsg).
func runScene(eng *engine.Engine, send Sender, input string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		out, err := eng.RunScene(ctx, input, func(ev engine.StreamEvent) {
			switch ev.Kind {
			case "stage":
				if s, ok := ev.Payload.(string); ok {
					send(pipelineStageMsg{Stage: s})
				}
			case "title":
				if bp, ok := ev.Payload.(agent.SceneBlueprint); ok {
					send(sceneStartedMsg{Blueprint: bp})
				}
			case "art_started":
				send(struct{ artBusyMsg }{artBusyMsg{Busy: true}})
			case "art_ready":
				if r, ok := ev.Payload.(assets.RenderResult); ok {
					send(artReadyMsg{Art: r})
				}
			case "narrative_chunk":
				if s, ok := ev.Payload.(string); ok {
					send(narrativeChunkMsg{Chunk: s})
				}
			case "narrative_done":
				send(narrativeDoneMsg{})
			}
		})
		if err != nil {
			return sceneErrMsg{Err: err}
		}
		return sceneDoneMsg{Out: out}
	}
}

type artBusyMsg struct{ Busy bool }

func checkHealth(c *agent.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := c.Health(ctx)
		return llmHealthMsg{OK: err == nil, Err: err}
	}
}

// frameTick fires the animation/typewriter tick at ~30fps.
func frameTick() tea.Cmd {
	return tea.Tick(33*time.Millisecond, func(time.Time) tea.Msg { return frameTickMsg{} })
}

// startupArt is reserved for future use (title-card before first scene).
// Kept compiled to avoid breaking the public surface if other code linked it.
func startupArt(lib *assets.Library, w, h int) string {
	if p, ok := lib.Match([]string{"atmospheres", "viagem"}); ok {
		return p.Art
	}
	return strings.Repeat(strings.Repeat(" ", w)+"\n", h)
}
