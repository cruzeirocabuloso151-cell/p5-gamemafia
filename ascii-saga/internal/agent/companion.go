package agent

import (
	"context"
	"fmt"
	"strings"
)

// Companion is one playable / non-playable ally with a persistent voice.
// We keep the prompt template per-instance because each companion's
// personality is dense enough that sharing a base template would be
// counter-productive — Aldric and Sira talk in genuinely different ways.
type Companion struct {
	Name         string
	SystemPrompt string
	client       *Client
	model        string
}

func NewCompanion(name, system string, c *Client, model string) *Companion {
	return &Companion{Name: name, SystemPrompt: system, client: c, model: model}
}

type ReactInput struct {
	HP, HPMax    int
	Mood         string
	Bond         int
	RecentMemory string
	SceneSummary string
	Hook         string
}

const companionTemplate = `Estado atual: HP %d/%d, humor %q, bond com jogador %d.
Últimos eventos vividos: %s
Contexto da cena: %s
Gatilho: %s

Responda com 1 ou 2 frases. Apenas a fala — sem narração externa, sem
aspas, sem prefixar o nome.`

// React produces the companion's line for a beat. Short, voiced.
func (c *Companion) React(ctx context.Context, in ReactInput) (string, error) {
	prompt := fmt.Sprintf(companionTemplate,
		in.HP, in.HPMax, nz(in.Mood, "neutro"), in.Bond,
		nz(in.RecentMemory, "—"),
		nz(in.SceneSummary, "—"),
		nz(in.Hook, "reagir naturalmente ao momento"),
	)
	out, err := c.client.Complete(ctx, CallParams{
		Model:       c.model,
		System:      c.SystemPrompt,
		User:        prompt,
		Temperature: 0.9,
		MaxTokens:   90,
	})
	if err != nil {
		return c.fallback(in), err
	}
	out = strings.Trim(out, "\"' \n\t")
	if out == "" {
		return c.fallback(in), nil
	}
	return out, nil
}

func (c *Companion) fallback(in ReactInput) string {
	switch strings.ToLower(c.Name) {
	case "aldric":
		return "Tolice... mas faça como quiser, rapaz."
	case "sira":
		return "Decida logo. A noite não vai esperar."
	}
	return "..."
}
