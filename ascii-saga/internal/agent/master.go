package agent

import (
	"context"
	"fmt"
	"strings"
)

// SceneMeta is the hidden bookkeeping block the Master returns at the
// end of a scene, inside <scene_meta>...</scene_meta> tags. The Judge
// turns it into deterministic state changes.
type SceneMeta struct {
	XPDelta            map[string]int    `json:"xp_delta"`
	HPDelta            map[string]int    `json:"hp_delta"`
	ItemsAdded         []ItemGrant       `json:"items_added"`
	ItemsRemoved       []ItemGrant       `json:"items_removed"`
	NPCsIntroduced     []string          `json:"npcs_introduced"`
	WorldStateChanges  []string          `json:"world_state_changes"`
	EmotionalTone      string            `json:"emotional_tone"`
}

type ItemGrant struct {
	To   string `json:"to"`
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// CleanMeta returns a meta with all maps non-nil, easier to consume.
func (m SceneMeta) CleanMeta() SceneMeta {
	if m.XPDelta == nil {
		m.XPDelta = map[string]int{}
	}
	if m.HPDelta == nil {
		m.HPDelta = map[string]int{}
	}
	return m
}

type Master struct {
	client *Client
	model  string
}

func NewMaster(c *Client, model string) *Master {
	return &Master{client: c, model: model}
}

const masterSystem = `Você é o MESTRE de um RPG narrativo solo. Narre em terceira pessoa,
com densidade sensorial e peso emocional. Tom: épico e íntimo —
The Witcher encontra O Senhor dos Anéis. Use português brasileiro.`

const masterTemplate = `Personagens em cena:
%s

Cena planejada pelo Diretor:
Título: %s
Subtítulo: %s
Beats:
%s
Hooks de companheiros: %s

DIRETRIZES:
1. Narre os beats em sequência. Não pule nenhum.
2. Quando um beat envolver Aldric ou Sira reagindo, escreva o marcador
   {{companion:aldric}} ou {{companion:sira}} em uma linha própria — NÃO
   invente fala deles. Outro agente vai preencher.
3. Inclua 2 ou 3 detalhes sensoriais por beat (som, cheiro, textura, luz).
4. Termine com tensão, descoberta ou um silêncio carregado. Nunca com
   uma conclusão neutra.
5. NÃO faça aritmética. NÃO declare ganho de XP no corpo da narrativa.
6. Comprimento alvo: 180–300 palavras de narrativa.

Após a narrativa, e SOMENTE após, inclua este bloco (que será oculto ao
jogador). O JSON deve ser válido:

<scene_meta>
{
  "xp_delta": {"jogador": 0, "aldric": 0, "sira": 0},
  "hp_delta": {"jogador": 0, "aldric": 0, "sira": 0},
  "items_added": [],
  "items_removed": [],
  "npcs_introduced": [],
  "world_state_changes": [],
  "emotional_tone": "uma palavra ou expressão curta"
}
</scene_meta>`

type NarrateInput struct {
	CharacterDossier string
	Blueprint        SceneBlueprint
}

// Narrate streams the scene narration. onChunk receives raw delta text
// (including the eventual <scene_meta> block — it's the caller's job to
// hide it from the user before display).
func (m *Master) Narrate(ctx context.Context, in NarrateInput, onChunk func(string)) (string, SceneMeta, error) {
	beats := strings.Builder{}
	for i, b := range in.Blueprint.Beats {
		fmt.Fprintf(&beats, "  %d) %s\n", i+1, b)
	}
	hookParts := []string{}
	for name, tone := range in.Blueprint.CompanionsHook {
		if tone == "" || tone == "null" {
			continue
		}
		hookParts = append(hookParts, fmt.Sprintf("%s=%s", name, tone))
	}
	hooks := strings.Join(hookParts, ", ")
	if hooks == "" {
		hooks = "nenhum"
	}

	prompt := fmt.Sprintf(masterTemplate,
		in.CharacterDossier,
		in.Blueprint.Title,
		in.Blueprint.Subtitle,
		beats.String(),
		hooks,
	)

	full, err := m.client.Stream(ctx, CallParams{
		Model:       m.model,
		System:      masterSystem,
		User:        prompt,
		Temperature: 0.85,
		MaxTokens:   1200,
	}, onChunk)
	if err != nil {
		return full, SceneMeta{}.CleanMeta(), err
	}
	meta := ExtractSceneMeta(full).CleanMeta()
	return full, meta, nil
}

// ExtractSceneMeta finds and parses the <scene_meta>...</scene_meta>
// block. Returns zero value if missing or malformed — callers should
// always treat zero-value meta as "no changes".
func ExtractSceneMeta(narr string) SceneMeta {
	const open = "<scene_meta>"
	const close = "</scene_meta>"
	start := strings.Index(narr, open)
	end := strings.Index(narr, close)
	if start < 0 || end < 0 || end <= start {
		return SceneMeta{}
	}
	body := narr[start+len(open) : end]
	meta := SceneMeta{}
	_ = parseJSONLoose(body, &meta)
	return meta
}

// StripSceneMeta removes the bookkeeping block from the narrative so the
// player never sees it.
func StripSceneMeta(narr string) string {
	const open = "<scene_meta>"
	const close = "</scene_meta>"
	start := strings.Index(narr, open)
	if start < 0 {
		return narr
	}
	end := strings.Index(narr, close)
	if end < 0 {
		return strings.TrimSpace(narr[:start])
	}
	return strings.TrimSpace(narr[:start]) + "\n" + strings.TrimSpace(narr[end+len(close):])
}
