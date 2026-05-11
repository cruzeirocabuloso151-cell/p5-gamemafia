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
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/tui/theme"
)

// Model is Bubble Tea's central state holder. We keep the entire
// engine + game state inline rather than splitting via interface — this
// is a single-binary game, not a library.
type Model struct {
	Cfg    config.Config
	Theme  theme.Theme
	Eng    *engine.Engine
	State  *state.GameState
	Logger agent.Logger

	// Layout
	Width, Height int
	Ready         bool

	// Live scene buffers
	CurrentTitle    string
	CurrentSubtitle string
	CurrentArt      string
	ArtSource       string
	NarrativeBuf    strings.Builder
	NarrativeDone   bool
	Suggestions    []string
	StatusLine     string

	// Input
	Input textinput.Model
	Busy  bool

	// Health
	LLMHealthy bool

	// Last scene events (for flash effects)
	LastEvents []agent.Event
	LastTick   time.Time
}

func NewModel(cfg config.Config, eng *engine.Engine, gs *state.GameState, log agent.Logger) Model {
	ti := textinput.New()
	ti.Placeholder = "O que você faz?"
	ti.Prompt = "› "
	ti.Focus()
	ti.CharLimit = 240

	return Model{
		Cfg:        cfg,
		Theme:      theme.Pick(cfg.TUI.Theme),
		Eng:        eng,
		State:      gs,
		Logger:     log,
		Input:      ti,
		StatusLine: "Pressione Enter para começar a sua jornada.",
		Suggestions: []string{
			"Olhar ao redor com atenção",
			"Conversar com Aldric",
			"Avançar pelo caminho",
		},
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(checkHealth(m.Eng.Client), textinput.Blink)
}

// runScene returns a Cmd that executes one scene end-to-end and emits
// progress messages back into the Bubble Tea event loop.
func runScene(eng *engine.Engine, input string) tea.Cmd {
	return func() tea.Msg {
		// We need to bridge engine's emit() callbacks into the Bubble Tea
		// program's message loop. The simplest robust way is to push them
		// onto a channel that we drain into a single batch — for this
		// build we keep things simple and run synchronously, sending
		// chunks through a side channel via tea's Send mechanism.
		//
		// However tea Cmds return one Msg each. So we collect everything
		// and emit as a single sceneDoneMsg. The streaming visual feel
		// comes from the typewriter effect in the view, not from chunked
		// network arrival in this MVP wiring. (Hooking up tea.Program.Send
		// from within the engine is a clean follow-up.)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		out, err := eng.RunScene(ctx, input, func(_ engine.StreamEvent) {})
		if err != nil {
			return sceneErrMsg{Err: err}
		}
		return sceneDoneMsg{Out: out}
	}
}

func checkHealth(c *agent.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := c.Health(ctx)
		return llmHealthMsg{OK: err == nil, Err: err}
	}
}

// startupArt returns a deterministic title-card ASCII drawn from the
// curated library so the game shows something meaningful before any
// LLM call lands.
func startupArt(lib *assets.Library, w, h int) string {
	if p, ok := lib.Match([]string{"atmospheres", "viagem"}); ok {
		return p.Art
	}
	return strings.Repeat(strings.Repeat(" ", w)+"\n", h)
}
