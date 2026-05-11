// Package theme holds the lipgloss color schemes the TUI selects between.
// Three options ship by default: dark_amber (warm, default), moonlit
// (cool blues), parchment (high-contrast for accessibility).
package theme

import "github.com/charmbracelet/lipgloss"

type Theme struct {
	Name      string
	Bg        lipgloss.Color
	Fg        lipgloss.Color
	Accent    lipgloss.Color
	Faint     lipgloss.Color
	Border    lipgloss.Color
	HPGood    lipgloss.Color
	HPLow     lipgloss.Color
	Companion lipgloss.Color
	Title     lipgloss.Color
}

func DarkAmber() Theme {
	return Theme{
		Name:      "dark_amber",
		Bg:        lipgloss.Color("#0c0a07"),
		Fg:        lipgloss.Color("#f1d692"),
		Accent:    lipgloss.Color("#ffb347"),
		Faint:     lipgloss.Color("#7a6a4a"),
		Border:    lipgloss.Color("#a07a3a"),
		HPGood:    lipgloss.Color("#7fc97f"),
		HPLow:     lipgloss.Color("#e15554"),
		Companion: lipgloss.Color("#9bc1ff"),
		Title:     lipgloss.Color("#ffd27f"),
	}
}

func Moonlit() Theme {
	return Theme{
		Name:      "moonlit",
		Bg:        lipgloss.Color("#06070d"),
		Fg:        lipgloss.Color("#cfd8ff"),
		Accent:    lipgloss.Color("#7aa9ff"),
		Faint:     lipgloss.Color("#4a557a"),
		Border:    lipgloss.Color("#5a78c0"),
		HPGood:    lipgloss.Color("#88e0a0"),
		HPLow:     lipgloss.Color("#ff7b72"),
		Companion: lipgloss.Color("#c9b8ff"),
		Title:     lipgloss.Color("#a9c8ff"),
	}
}

func Parchment() Theme {
	return Theme{
		Name:      "parchment",
		Bg:        lipgloss.Color("#f4e8c8"),
		Fg:        lipgloss.Color("#2a1f0e"),
		Accent:    lipgloss.Color("#7c2a0a"),
		Faint:     lipgloss.Color("#8a7758"),
		Border:    lipgloss.Color("#5a3a14"),
		HPGood:    lipgloss.Color("#2d6a3a"),
		HPLow:     lipgloss.Color("#a8201a"),
		Companion: lipgloss.Color("#244a7a"),
		Title:     lipgloss.Color("#5a2a0a"),
	}
}

func Pick(name string) Theme {
	switch name {
	case "moonlit":
		return Moonlit()
	case "parchment":
		return Parchment()
	default:
		return DarkAmber()
	}
}
