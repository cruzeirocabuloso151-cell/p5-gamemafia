package assets

import (
	"context"
	"fmt"
	"strings"

	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/agent"
)

// Artist is the five-layer pipeline:
//
//  1. Identity reuse  — locked vault assets used verbatim
//  2. Composition     — multiple locked layers merged deterministically
//  3. New generation  — LLM call with strict prompt + few-shots
//  4. Validation      — autofix + retry
//  5. Library fallback — curated piece by tag match
type Artist struct {
	client  *agent.Client
	model   string
	vault   *Vault
	library *Library
	canvas  Canvas
	palette string
	logger  agent.Logger
}

type ArtistOpts struct {
	Model   string
	Width   int
	Height  int
	Palette string
	Logger  agent.Logger
}

func NewArtist(c *agent.Client, vault *Vault, lib *Library, opts ArtistOpts) *Artist {
	if opts.Logger == nil {
		opts.Logger = agent.NopLogger{}
	}
	if opts.Palette == "" {
		opts.Palette = "default"
	}
	return &Artist{
		client:  c,
		model:   opts.Model,
		vault:   vault,
		library: lib,
		canvas:  Canvas{W: opts.Width, H: opts.Height},
		palette: opts.Palette,
		logger:  opts.Logger,
	}
}

type RenderRequest struct {
	Brief        string
	Tags         []string
	ReuseAssets  []string // names of vault assets to compose if found
	AllowLLM     bool     // if false, skip the LLM generation step entirely
}

type RenderResult struct {
	Art    string
	Source string // "vault" | "compose" | "llm" | "library"
	CacheKey string
}

func (a *Artist) Render(ctx context.Context, req RenderRequest) RenderResult {
	// Layer 1 — single locked asset matches the request exactly
	if len(req.ReuseAssets) == 1 {
		for _, cat := range []string{"portraits", "locations", "items"} {
			if art, ok := a.vault.Get(cat, req.ReuseAssets[0]); ok {
				return RenderResult{Art: a.fitToCanvas(art), Source: "vault"}
			}
		}
	}

	// Layer 2 — multiple reuse hints: try compositing
	if len(req.ReuseAssets) > 1 {
		if composed, ok := a.composeFromVault(req.ReuseAssets); ok {
			return RenderResult{Art: composed, Source: "compose"}
		}
	}

	// Cache key only depends on the brief + tags — same prompt, same art.
	key := Key(req.Brief, req.Tags, a.canvas.W, a.canvas.H, "default", a.palette)
	if cached, ok := a.vault.GetCachedScene(key); ok {
		return RenderResult{Art: cached, Source: "vault", CacheKey: key}
	}

	// Layer 3 — generate via LLM
	if req.AllowLLM && a.client != nil && a.model != "" {
		if art, ok := a.generate(ctx, req); ok {
			_ = a.vault.CacheScene(key, art)
			return RenderResult{Art: art, Source: "llm", CacheKey: key}
		}
	}

	// Layer 5 — library fallback
	if p, ok := a.library.Match(req.Tags); ok {
		return RenderResult{Art: a.fitToCanvas(p.Art), Source: "library"}
	}

	return RenderResult{Art: blankCanvas(a.canvas), Source: "library"}
}

func (a *Artist) composeFromVault(names []string) (string, bool) {
	layers := []Layer{}
	z := 0
	for _, n := range names {
		// background candidates first
		if art, ok := a.vault.Get("locations", n); ok {
			layers = append(layers, Layer{Art: art, X: 0, Y: 0, ZOrder: z})
			z++
			continue
		}
	}
	xCursor := 2
	for _, n := range names {
		if art, ok := a.vault.Get("portraits", n); ok {
			layers = append(layers, Layer{Art: art, X: xCursor, Y: 2, ZOrder: z + 10})
			xCursor += widthOf(art) + 2
			z++
			continue
		}
		if art, ok := a.vault.Get("items", n); ok {
			layers = append(layers, Layer{Art: art, X: xCursor, Y: a.canvas.H - heightOf(art) - 1, ZOrder: z + 5})
			xCursor += widthOf(art) + 2
			z++
		}
	}
	if len(layers) == 0 {
		return "", false
	}
	composed := Compose(layers, a.canvas.W, a.canvas.H)
	if err := Validate(composed, a.canvas, a.palette); err != nil {
		composed, _ = AutoFix(composed, a.canvas, a.palette)
	}
	return composed, true
}

const artistSystem = `Você é um ARTISTA ASCII mestre. Gere arte em bloco UNICODE.
Sem emojis. Sem texto fora do bloco. Apenas o desenho.`

const artistTemplate = `Estilo: %s
Paleta permitida (use APENAS estes caracteres + espaço):
%s

Cena: %s
Tags: %s

REGRAS DE OURO:
- O bloco deve ter EXATAMENTE %d linhas.
- Cada linha deve ter EXATAMENTE %d caracteres (preencha com espaços à direita).
- Composição: foco no elemento principal centrado ou no terço áureo.
- Contraste: zonas de █▓▒ contra zonas de espaço ou ·.
- Profundidade: elementos próximos sólidos; distantes pontilhados.
- Retorne APENAS o bloco entre as cercas %sart e %s. Nada mais.

%sart
(seu desenho aqui)
%s`

func (a *Artist) generate(ctx context.Context, req RenderRequest) (string, bool) {
	pal := Palettes[a.palette]
	if pal == "" {
		pal = Palettes["default"]
	}
	tagStr := strings.Join(req.Tags, ", ")
	if tagStr == "" {
		tagStr = "—"
	}
	const fence = "```"
	prompt := fmt.Sprintf(artistTemplate,
		"ascii_lineart_with_shading", pal,
		req.Brief, tagStr,
		a.canvas.H, a.canvas.W,
		fence, fence,
		fence, fence,
	)
	maxTokens := a.canvas.W*a.canvas.H/2 + 200 // generous budget
	if maxTokens > 2000 {
		maxTokens = 2000
	}
	out, err := a.client.Complete(ctx, agent.CallParams{
		Model:       a.model,
		System:      artistSystem,
		User:        prompt,
		Temperature: 0.3,
		MaxTokens:   maxTokens,
	})
	if err != nil {
		a.logger.Error("artist generate failed", "err", err)
		return "", false
	}
	art := extractArtBlock(out)
	if err := Validate(art, a.canvas, a.palette); err != nil {
		fixed, fixErr := AutoFix(art, a.canvas, a.palette)
		if fixErr != nil {
			a.logger.Debug("artist autofix failed", "err", fixErr)
			return "", false
		}
		return fixed, true
	}
	return art, true
}

// extractArtBlock pulls the content between ```art ... ``` fences. If
// fences are missing, returns the raw stripped output.
func extractArtBlock(s string) string {
	const open = "```art"
	const close = "```"
	if i := strings.Index(s, open); i >= 0 {
		s = s[i+len(open):]
	} else if i := strings.Index(s, "```"); i >= 0 {
		// any fence opening at all
		s = s[i+3:]
		// skip optional language tag like "art\n"
		if nl := strings.Index(s, "\n"); nl >= 0 && nl < 12 {
			s = s[nl+1:]
		}
	}
	if i := strings.Index(s, close); i >= 0 {
		s = s[:i]
	}
	return strings.TrimRight(strings.TrimLeft(s, "\n"), " \n")
}

func (a *Artist) fitToCanvas(art string) string {
	out, err := AutoFix(art, a.canvas, a.palette)
	if err != nil {
		// Even autofix can fail if every rune is illegal; return blank.
		return blankCanvas(a.canvas)
	}
	return out
}

func blankCanvas(c Canvas) string {
	row := strings.Repeat(" ", c.W)
	rows := make([]string, c.H)
	for i := range rows {
		rows[i] = row
	}
	return strings.Join(rows, "\n")
}

func widthOf(art string) int {
	max := 0
	for _, ln := range strings.Split(art, "\n") {
		if n := len([]rune(ln)); n > max {
			max = n
		}
	}
	return max
}

func heightOf(art string) int {
	return len(strings.Split(art, "\n"))
}
