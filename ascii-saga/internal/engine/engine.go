// Package engine orchestrates a single scene end-to-end:
// Director plans, Master narrates (streaming), Artist renders in parallel,
// Companions react when hooked, Judge applies state changes.
package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/agent"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/agent/companions"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/assets"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/state"
)

type Engine struct {
	Client   *agent.Client
	Dir      *agent.Director
	Mas      *agent.Master
	Judge    *agent.Judge
	Aldric   *agent.Companion
	Sira     *agent.Companion
	Artist   *assets.Artist
	Store    *state.Store
	GameState *state.GameState
	Logger   agent.Logger
}

type EngineOpts struct {
	ModelDirector  string
	ModelMaster    string
	ModelArtist    string
	ModelCompanion string
	Vault          *assets.Vault
	Library        *assets.Library
	ArtWidth       int
	ArtHeight      int
	Palette        string
}

func New(client *agent.Client, store *state.Store, gs *state.GameState, opts EngineOpts, log agent.Logger) *Engine {
	if log == nil {
		log = agent.NopLogger{}
	}
	return &Engine{
		Client:    client,
		Dir:       agent.NewDirector(client, opts.ModelDirector),
		Mas:       agent.NewMaster(client, opts.ModelMaster),
		Judge:     agent.NewJudge(),
		Aldric:    agent.NewCompanion("Aldric", companions.Aldric, client, opts.ModelCompanion),
		Sira:      agent.NewCompanion("Sira", companions.Sira, client, opts.ModelCompanion),
		Artist:    assets.NewArtist(client, opts.Vault, opts.Library, assets.ArtistOpts{Model: opts.ModelArtist, Width: opts.ArtWidth, Height: opts.ArtHeight, Palette: opts.Palette, Logger: log}),
		Store:     store,
		GameState: gs,
		Logger:    log,
	}
}

// SceneOutput is the bundle the TUI consumes after a scene resolves.
type SceneOutput struct {
	Blueprint  agent.SceneBlueprint
	Art        assets.RenderResult
	Narrative  string // with companion lines spliced in, scene_meta stripped
	Meta       agent.SceneMeta
	Events     []agent.Event
	Suggestions []string
}

// StreamEvent is one of: "art_ready", "title", "narrative_chunk",
// "narrative_done", "companions_done", "scene_done".
type StreamEvent struct {
	Kind    string
	Payload any
}

// RunScene executes the full pipeline for one player input.
//
// Streaming events emitted via emit():
//   - "stage" + payload string in {"director","master","artist","judge","idle"}
//   - "title" + SceneBlueprint
//   - "art_started" (signal for spinner) and "art_ready" + RenderResult
//   - "narrative_chunk" + string, then "narrative_done"
//   - "scene_done" + SceneOutput
func (e *Engine) RunScene(ctx context.Context, playerInput string, emit func(StreamEvent)) (SceneOutput, error) {
	out := SceneOutput{}

	emit(StreamEvent{Kind: "stage", Payload: "director"})

	// Director — quick, blocking. Without it, the rest has no rails.
	planCtx := e.buildPlanContext(playerInput)
	bp, err := e.Dir.Plan(ctx, planCtx)
	if err != nil {
		e.Logger.Error("director plan", "err", err)
	}
	out.Blueprint = bp
	emit(StreamEvent{Kind: "title", Payload: bp})

	emit(StreamEvent{Kind: "art_started", Payload: nil})
	emit(StreamEvent{Kind: "stage", Payload: "artist"})

	// Kick off Artist in parallel.
	artCh := make(chan assets.RenderResult, 1)
	go func() {
		artCh <- e.Artist.Render(ctx, assets.RenderRequest{
			Brief:       bp.VisualBrief,
			Tags:        bp.VisualTags,
			ReuseAssets: bp.VisualReuse,
			AllowLLM:    true,
		})
	}()

	emit(StreamEvent{Kind: "stage", Payload: "master"})

	// Master — streams.
	dossier := e.buildCharacterDossier()
	var rawNarr strings.Builder
	rawAll, meta, err := e.Mas.Narrate(ctx, agent.NarrateInput{
		CharacterDossier: dossier,
		Blueprint:        bp,
	}, func(chunk string) {
		rawNarr.WriteString(chunk)
		// Hide scene_meta delta from the player as it streams.
		if visible := hideMetaDelta(chunk); visible != "" {
			emit(StreamEvent{Kind: "narrative_chunk", Payload: visible})
		}
	})
	if err != nil {
		e.Logger.Error("master narrate", "err", err)
	}
	emit(StreamEvent{Kind: "narrative_done", Payload: nil})

	// Wait for art (it usually arrived first; this is safety).
	art := <-artCh
	out.Art = art
	emit(StreamEvent{Kind: "art_ready", Payload: art})

	// Strip the bookkeeping block and splice companion lines.
	narr := agent.StripSceneMeta(rawAll)
	if narr == "" {
		narr = rawNarr.String()
	}
	narr = e.spliceCompanionReactions(ctx, narr, bp)
	out.Narrative = strings.TrimSpace(narr)
	out.Meta = meta

	emit(StreamEvent{Kind: "stage", Payload: "judge"})

	// Judge — deterministic state mutation.
	out.Events = e.Judge.Apply(e.GameState, meta)
	for _, ev := range out.Events {
		e.GameState.AppendEvent(ev.Kind, ev.Text)
		_ = e.Store.AppendEventDurable(state.Event{Kind: ev.Kind, Text: ev.Text})
	}
	e.GameState.SceneIndex++

	// Suggestions for the next turn — just heuristics for now; can become
	// a tiny LLM call later without changing this contract.
	out.Suggestions = defaultSuggestions(bp)

	if err := e.Store.Save(e.GameState); err != nil {
		e.Logger.Error("autosave", "err", err)
	}

	emit(StreamEvent{Kind: "stage", Payload: "idle"})
	emit(StreamEvent{Kind: "scene_done", Payload: out})
	return out, nil
}

// buildPlanContext crafts a compact, RAG-flavored context for the Director.
// In a full RAG-enabled build this is where chromem-go retrieve would land.
func (e *Engine) buildPlanContext(input string) agent.PlanContext {
	prev := ""
	for i := len(e.GameState.Events) - 1; i >= 0 && len(prev) < 240; i-- {
		ev := e.GameState.Events[i]
		prev = ev.Text + ". " + prev
	}

	state := strings.Builder{}
	for _, c := range e.GameState.Characters {
		fmt.Fprintf(&state, "%s (Nv %d, HP %d/%d) ", c.Name, c.Level, c.HP, c.HPMax)
	}

	recent := strings.Builder{}
	start := len(e.GameState.Events) - 5
	if start < 0 {
		start = 0
	}
	for _, ev := range e.GameState.Events[start:] {
		fmt.Fprintf(&recent, "[%s] %s; ", ev.Kind, ev.Text)
	}

	return agent.PlanContext{
		PreviousSceneSummary: prev,
		CharacterState:       state.String(),
		RecentEvents:         recent.String(),
		PlayerInput:          input,
	}
}

func (e *Engine) buildCharacterDossier() string {
	b := strings.Builder{}
	for _, c := range e.GameState.Characters {
		fmt.Fprintf(&b, "- %s (%s, Nv %d, HP %d/%d", c.Name, c.Class, c.Level, c.HP, c.HPMax)
		if c.Mood != "" {
			fmt.Fprintf(&b, ", humor %s", c.Mood)
		}
		fmt.Fprintln(&b, ")")
	}
	return b.String()
}

// spliceCompanionReactions finds {{companion:name}} markers and replaces
// them with the companion's actual line. Markers without a backing
// companion are dropped silently.
func (e *Engine) spliceCompanionReactions(ctx context.Context, narr string, bp agent.SceneBlueprint) string {
	scene := bp.Title + " — " + bp.Subtitle
	for _, name := range []string{"aldric", "sira"} {
		marker := "{{companion:" + name + "}}"
		if !strings.Contains(narr, marker) {
			continue
		}
		var c *agent.Companion
		var ch *state.Character
		switch name {
		case "aldric":
			c, ch = e.Aldric, e.GameState.GetCharacter("aldric")
		case "sira":
			c, ch = e.Sira, e.GameState.GetCharacter("sira")
		}
		if c == nil {
			narr = strings.ReplaceAll(narr, marker, "")
			continue
		}
		hp, hpmax, mood, bond := 0, 0, "", 0
		if ch != nil {
			hp, hpmax, mood, bond = ch.HP, ch.HPMax, ch.Mood, ch.Bond
		}
		line, _ := c.React(ctx, agent.ReactInput{
			HP: hp, HPMax: hpmax, Mood: mood, Bond: bond,
			SceneSummary: scene,
			Hook:         bp.CompanionsHook[name],
		})
		quoted := "— " + capitalizeName(name) + ": “" + line + "”"
		narr = strings.Replace(narr, marker, quoted, 1)
	}
	return narr
}

func capitalizeName(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = upper(r[0])
	return string(r)
}

func upper(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 32
	}
	return r
}

// hideMetaDelta strips text once we cross into the <scene_meta> block so
// the player never sees the JSON streaming. Conservative: if we see "<"
// at the tail, withhold until we know whether it's the marker or content.
func hideMetaDelta(chunk string) string {
	if i := strings.Index(chunk, "<scene_meta>"); i >= 0 {
		return chunk[:i]
	}
	return chunk
}

func defaultSuggestions(bp agent.SceneBlueprint) []string {
	switch bp.Difficulty {
	case "fácil":
		return []string{"Avançar com confiança", "Examinar o ambiente", "Falar com seus companheiros"}
	case "difícil", "chefe":
		return []string{"Atacar primeiro", "Recuar para se reagrupar", "Tentar negociar"}
	}
	return []string{"Investigar mais de perto", "Conversar antes de agir", "Manter a guarda alta"}
}
