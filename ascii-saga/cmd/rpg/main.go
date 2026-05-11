// Command rpg is the entry point for ascii-saga, a TUI RPG with
// generative ASCII art powered by a local LLM via LM Studio.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/agent"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/assets"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/config"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/engine"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/state"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/tui"
)

func main() {
	var (
		cfgPath  = flag.String("config", "configs/config.toml", "path to config TOML")
		campaign = flag.String("campaign", "", "campaign name override")
		player   = flag.String("player", "Kael", "player name (used if campaign is fresh)")
		class    = flag.String("class", "Patrulheiro", "player class (used if campaign is fresh)")
		smoke    = flag.Bool("smoke", false, "run a non-interactive smoke test of the engine wiring")
	)
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if *campaign != "" {
		cfg.Storage.Campaign = *campaign
	}
	if err := cfg.EnsureDirs(); err != nil {
		log.Fatalf("dirs: %v", err)
	}

	logger := newFileLogger(cfg.Debug.LogFile)

	store, err := state.OpenStore(cfg.CampaignDir())
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer store.Close()

	gs, loaded, err := store.Load(cfg.Storage.Campaign)
	if err != nil {
		log.Fatalf("load state: %v", err)
	}
	if !loaded {
		gs.SeedDefaultParty(*player, *class)
		if err := store.Save(gs); err != nil {
			log.Fatalf("seed save: %v", err)
		}
	}

	vault, err := assets.OpenVault(filepath.Join(cfg.CampaignDir(), "vault"))
	if err != nil {
		log.Fatalf("vault: %v", err)
	}
	library, err := assets.LoadLibrary()
	if err != nil {
		logger.Error("library load", "err", err)
	}

	client := agent.NewClient(cfg.LLM, logger)
	eng := engine.New(client, store, gs, engine.EngineOpts{
		ModelDirector:  cfg.LLM.ModelDirector,
		ModelMaster:    cfg.LLM.ModelMaster,
		ModelArtist:    cfg.LLM.ModelArtist,
		ModelCompanion: cfg.LLM.ModelCompanion,
		Vault:          vault,
		Library:        library,
		ArtWidth:       cfg.TUI.ArtWidth,
		ArtHeight:      cfg.TUI.ArtHeight,
		Palette:        cfg.Artist.AllowPalette,
	}, logger)

	if *smoke {
		runSmoke(eng, gs)
		return
	}

	// Bridge engine streaming events back into the TUI via Program.Send.
	// The closure captures the pointer lazily so it can be created before
	// the program exists.
	var program *tea.Program
	sender := func(msg tea.Msg) {
		if program != nil {
			program.Send(msg)
		}
	}
	model := tui.NewModel(cfg, eng, gs, logger, sender)
	program = tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := program.Run(); err != nil {
		log.Fatalf("tui: %v", err)
	}
}

// runSmoke exercises the wiring without an LM Studio: the engine will
// fail-soft on the LLM calls, exercising the fallbacks. Useful for CI
// and for confirming the build is intact end-to-end.
func runSmoke(eng *engine.Engine, gs *state.GameState) {
	fmt.Println("ascii-saga smoke test")
	fmt.Println("---------------------")
	fmt.Printf("Campaign: %s   Scene index: %d\n", gs.Campaign, gs.SceneIndex)
	fmt.Printf("Party: ")
	for i, c := range gs.Characters {
		if i > 0 {
			fmt.Printf(", ")
		}
		fmt.Printf("%s (Nv %d, HP %d/%d)", c.Name, c.Level, c.HP, c.HPMax)
	}
	fmt.Println()

	// Test the Artist library fallback path (no LLM needed).
	res := eng.Artist.Render(context.Background(), assets.RenderRequest{
		Brief:    "smoke test",
		Tags:     []string{"locations", "santuario_caido"},
		AllowLLM: false,
	})
	fmt.Printf("Artist source: %s   art lines: %d\n",
		res.Source, len(splitLines(res.Art)))
	fmt.Println("Sample art (first 5 lines):")
	for i, ln := range splitLines(res.Art) {
		if i >= 5 {
			break
		}
		fmt.Println(" ", ln)
	}
	fmt.Println("OK")
}

func splitLines(s string) []string {
	out := []string{""}
	for _, r := range s {
		if r == '\n' {
			out = append(out, "")
			continue
		}
		out[len(out)-1] += string(r)
	}
	return out
}

// newFileLogger is intentionally minimal: log lines to file only, never
// to stdout, since stdout belongs to Bubble Tea's renderer.
func newFileLogger(path string) agent.Logger {
	if path == "" {
		return agent.NopLogger{}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return agent.NopLogger{}
	}
	return &fileLogger{f: f}
}

type fileLogger struct{ f *os.File }

func (l *fileLogger) line(level, msg string, kv []any) {
	fmt.Fprintf(l.f, "[%s] %s", level, msg)
	for i := 0; i+1 < len(kv); i += 2 {
		fmt.Fprintf(l.f, " %v=%v", kv[i], kv[i+1])
	}
	fmt.Fprintln(l.f)
}
func (l *fileLogger) Debug(msg string, kv ...any) { l.line("DBG", msg, kv) }
func (l *fileLogger) Info(msg string, kv ...any)  { l.line("INF", msg, kv) }
func (l *fileLogger) Error(msg string, kv ...any) { l.line("ERR", msg, kv) }
