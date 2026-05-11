// Package state owns the persistent game world: characters, inventory,
// relationships, the rolling event log. The Judge mutates GameState; the
// store persists it to SQLite between sessions.
package state

import "time"

type Item struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Character struct {
	ID        string `json:"id"`        // stable key: "jogador" | "aldric" | "sira"
	Name      string `json:"name"`      // display name
	Class     string `json:"class"`
	Level     int    `json:"level"`
	XP        int    `json:"xp"`
	HP        int    `json:"hp"`
	HPMax     int    `json:"hp_max"`
	Inventory []Item `json:"inventory"`
	Mood      string `json:"mood"`      // companion-only: "neutro", "tenso", ...
	Bond      int    `json:"bond"`      // companion-only: -100..+100 vs player
	IsParty   bool   `json:"is_party"`  // true for player + active companions
}

// HPBar returns a 5-cell unicode bar for the TUI sheet panel.
func (c Character) HPBar() string {
	if c.HPMax <= 0 {
		return "▱▱▱▱▱"
	}
	cells := 5
	filled := c.HP * cells / c.HPMax
	if c.HP > 0 && filled == 0 {
		filled = 1
	}
	out := make([]rune, cells)
	for i := 0; i < cells; i++ {
		if i < filled {
			out[i] = '▰'
		} else {
			out[i] = '▱'
		}
	}
	return string(out)
}

// Event is one immutable line in the campaign log.
type Event struct {
	ID        int64
	Timestamp time.Time
	Kind      string
	Text      string
}

// GameState is the in-memory snapshot. Loaded from store at startup,
// flushed back at autosave points.
type GameState struct {
	Campaign   string
	SceneIndex int
	Characters []*Character
	Events     []Event
	WorldFlags map[string]string // tiny KV store for "altar_destroyed" etc.
}

func NewGameState(campaign string) *GameState {
	return &GameState{
		Campaign:   campaign,
		Characters: []*Character{},
		Events:     []Event{},
		WorldFlags: map[string]string{},
	}
}

// SeedDefaultParty creates the player + Aldric + Sira if the campaign is empty.
// Numbers chosen so an early scene matters but isn't lethal.
func (gs *GameState) SeedDefaultParty(playerName, playerClass string) {
	if len(gs.Characters) > 0 {
		return
	}
	gs.Characters = []*Character{
		{ID: "jogador", Name: playerName, Class: playerClass, Level: 1, XP: 0,
			HP: 28, HPMax: 28, IsParty: true, Inventory: []Item{
				{Name: "Espada longa", Description: "Aço comum, fio honesto."},
				{Name: "Manto viajado", Description: "Cheira a chuva e ferro."},
			}},
		{ID: "aldric", Name: "Aldric", Class: "Mago", Level: 5, XP: 6500,
			HP: 31, HPMax: 42, IsParty: true, Mood: "irônico", Bond: 10,
			Inventory: []Item{{Name: "Cajado de freixo", Description: "Marcado por sete runas finas."}}},
		{ID: "sira", Name: "Sira", Class: "Guerreira", Level: 5, XP: 6800,
			HP: 38, HPMax: 48, IsParty: true, Mood: "atenta", Bond: 15,
			Inventory: []Item{{Name: "Espadarte gemini", Description: "Duas lâminas, um cabo."}}},
	}
}

func (gs *GameState) GetCharacter(id string) *Character {
	for _, c := range gs.Characters {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// AppendEvent records a moment in the campaign log. Cap at 500 entries
// in memory — the SQLite copy keeps the full history.
func (gs *GameState) AppendEvent(kind, text string) {
	gs.Events = append(gs.Events, Event{Timestamp: time.Now(), Kind: kind, Text: text})
	if len(gs.Events) > 500 {
		gs.Events = gs.Events[len(gs.Events)-500:]
	}
}
