package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// SceneBlueprint is the Director's structured output. The Master uses it
// as a roadmap; the Artist reads the visual hints; the engine schedules
// companions based on the hooks.
type SceneBlueprint struct {
	Title          string            `json:"titulo"`
	Subtitle       string            `json:"subtitulo"`
	Beats          []string          `json:"beats"`
	VisualBrief    string            `json:"visual_brief"`
	VisualTags     []string          `json:"visual_tags"`
	VisualReuse    []string          `json:"visual_reuse"`
	CompanionsHook map[string]string `json:"companions_hook"`
	Difficulty     string            `json:"dificuldade"`
	XPSuggested    int               `json:"xp_sugerido"`
}

// Fallback returns a deterministic blueprint when the LLM is unreachable
// or returns garbage. Better a degraded scene than a crash.
func (b SceneBlueprint) Fallback(playerInput string) SceneBlueprint {
	if b.Title == "" {
		b.Title = "Um Instante de Pausa"
	}
	if b.Subtitle == "" {
		b.Subtitle = "O mundo respira fundo antes do próximo passo"
	}
	if len(b.Beats) == 0 {
		b.Beats = []string{
			fmt.Sprintf("O grupo considera: %q", playerInput),
			"O ambiente reage sutilmente, sem revelar tudo.",
			"Algo se move nas sombras — mas é cedo demais para nomeá-lo.",
		}
	}
	if b.VisualBrief == "" {
		b.VisualBrief = "Cena de transição. Pouca luz, atmosfera contemplativa."
	}
	if len(b.VisualTags) == 0 {
		b.VisualTags = []string{"atmosphere", "mystery", "transition"}
	}
	if b.CompanionsHook == nil {
		b.CompanionsHook = map[string]string{}
	}
	if b.Difficulty == "" {
		b.Difficulty = "média"
	}
	return b
}

type Director struct {
	client *Client
	model  string
}

func NewDirector(c *Client, model string) *Director {
	return &Director{client: c, model: model}
}

const directorSystem = `Você é o DIRETOR de uma cena de RPG narrativo.
Sua única tarefa é PLANEJAR a cena em JSON estruturado. Você não narra,
não descreve em prosa, não conversa. Apenas devolve um objeto JSON.`

const directorTemplate = `Contexto:
- Cena anterior (resumo): %s
- Lore relevante: %s
- Estado dos personagens: %s
- Eventos recentes: %s
- Ação do jogador: %q

Retorne APENAS este JSON, sem texto antes ou depois, sem cercas markdown:

{
  "titulo": "máximo 6 palavras, evocativo",
  "subtitulo": "máximo 12 palavras, complementa o título",
  "beats": [
    "batida 1: o que acontece no início",
    "batida 2: desenvolvimento ou complicação",
    "batida 3: clímax ou cliffhanger"
  ],
  "visual_brief": "2-3 frases densas para o ARTISTA: foco, ambiente, iluminação",
  "visual_tags": ["categoria", "subcategoria", "humor"],
  "visual_reuse": ["nomes_de_assets_existentes_relevantes"],
  "companions_hook": {"aldric": "tom_da_reação_ou_null", "sira": "tom_da_reação_ou_null"},
  "dificuldade": "fácil|média|difícil|chefe",
  "xp_sugerido": 0
}`

// PlanContext is the bag of strings the Director uses to plan.
// Keep them short — the Director is supposed to be cheap.
type PlanContext struct {
	PreviousSceneSummary string
	RelevantLore         string
	CharacterState       string
	RecentEvents         string
	PlayerInput          string
}

func (d *Director) Plan(ctx context.Context, pc PlanContext) (SceneBlueprint, error) {
	prompt := fmt.Sprintf(directorTemplate,
		nz(pc.PreviousSceneSummary, "início da campanha"),
		nz(pc.RelevantLore, "—"),
		nz(pc.CharacterState, "—"),
		nz(pc.RecentEvents, "—"),
		pc.PlayerInput,
	)
	out, err := d.client.Complete(ctx, CallParams{
		Model:       d.model,
		System:      directorSystem,
		User:        prompt,
		Temperature: 0.5,
		MaxTokens:   400,
	})
	bp := SceneBlueprint{}
	if err != nil {
		return bp.Fallback(pc.PlayerInput), err
	}
	if perr := parseJSONLoose(out, &bp); perr != nil {
		return bp.Fallback(pc.PlayerInput), fmt.Errorf("director json: %w", perr)
	}
	return bp.Fallback(pc.PlayerInput), nil
}

// parseJSONLoose tries to extract a JSON object from a noisy LLM response.
// Real LLMs sometimes wrap output in ```json fences or chat-style preamble;
// we trim aggressively rather than failing on the first whitespace surprise.
func parseJSONLoose(s string, into any) error {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < 0 || end <= start {
		return fmt.Errorf("no json object found")
	}
	return json.Unmarshal([]byte(s[start:end+1]), into)
}

func nz(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
