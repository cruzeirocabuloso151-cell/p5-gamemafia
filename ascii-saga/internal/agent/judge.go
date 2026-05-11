package agent

import (
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/state"
)

// Judge is deterministic. No LLM. It takes SceneMeta and applies it to
// the GameState, returning a list of human-readable events the TUI can
// surface (level ups, downed allies, item grants).
type Judge struct {
	XPTable []int // index = level, value = XP needed to REACH that level
}

// DefaultXPTable is a classic D&D-flavored progression curve.
func DefaultXPTable() []int {
	return []int{
		0, 0, 300, 900, 2700, 6500, 14000, 23000, 34000,
		48000, 64000, 85000, 100000, 120000, 140000, 165000,
		195000, 225000, 265000, 305000, 355000,
	}
}

func NewJudge() *Judge {
	return &Judge{XPTable: DefaultXPTable()}
}

// Event is something the Judge wants the TUI to know about.
type Event struct {
	Kind string // "level_up" | "downed" | "item_added" | "item_removed" | "hp_change" | "xp_gain"
	Who  string
	Text string
	N    int // numeric payload when relevant (xp gained, hp delta)
}

func (j *Judge) Apply(gs *state.GameState, meta SceneMeta) []Event {
	events := []Event{}

	for who, gain := range meta.XPDelta {
		if gain == 0 {
			continue
		}
		ch := gs.GetCharacter(who)
		if ch == nil {
			continue
		}
		ch.XP += gain
		events = append(events, Event{Kind: "xp_gain", Who: who, N: gain,
			Text: who + " ganhou " + itoa(gain) + " XP"})
		for ch.Level+1 < len(j.XPTable) && ch.XP >= j.XPTable[ch.Level+1] {
			ch.Level++
			ch.HPMax += 6 // small bump per level — keeps numbers feeling
			ch.HP = ch.HPMax
			events = append(events, Event{Kind: "level_up", Who: who, N: ch.Level,
				Text: who + " subiu para o nível " + itoa(ch.Level) + "!"})
		}
	}

	for who, delta := range meta.HPDelta {
		if delta == 0 {
			continue
		}
		ch := gs.GetCharacter(who)
		if ch == nil {
			continue
		}
		ch.HP += delta
		if ch.HP > ch.HPMax {
			ch.HP = ch.HPMax
		}
		if ch.HP < 0 {
			ch.HP = 0
		}
		events = append(events, Event{Kind: "hp_change", Who: who, N: delta,
			Text: who + " " + hpVerb(delta) + " " + itoa(absInt(delta)) + " HP"})
		if ch.HP == 0 {
			events = append(events, Event{Kind: "downed", Who: who,
				Text: who + " caiu inconsciente!"})
		}
	}

	for _, g := range meta.ItemsAdded {
		ch := gs.GetCharacter(g.To)
		if ch == nil {
			continue
		}
		ch.Inventory = append(ch.Inventory, state.Item{Name: g.Name, Description: g.Desc})
		events = append(events, Event{Kind: "item_added", Who: g.To,
			Text: g.To + " recebeu: " + g.Name})
	}

	for _, g := range meta.ItemsRemoved {
		ch := gs.GetCharacter(g.To)
		if ch == nil {
			continue
		}
		for i, it := range ch.Inventory {
			if it.Name == g.Name {
				ch.Inventory = append(ch.Inventory[:i], ch.Inventory[i+1:]...)
				events = append(events, Event{Kind: "item_removed", Who: g.To,
					Text: g.To + " perdeu: " + g.Name})
				break
			}
		}
	}

	return events
}

func hpVerb(d int) string {
	if d < 0 {
		return "perdeu"
	}
	return "recuperou"
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func itoa(n int) string {
	// avoid pulling strconv into a hot path — this is plenty fast
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
