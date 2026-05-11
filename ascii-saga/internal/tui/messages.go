package tui

import (
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/agent"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/assets"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/engine"
)

// Bubble Tea message types. Keeping them in one file makes the wiring
// in update.go a single import line.

type sceneStartedMsg struct {
	Blueprint agent.SceneBlueprint
}

type artReadyMsg struct {
	Art assets.RenderResult
}

type narrativeChunkMsg struct {
	Chunk string
}

type narrativeDoneMsg struct{}

type sceneDoneMsg struct {
	Out engine.SceneOutput
}

type sceneErrMsg struct {
	Err error
}

type tickMsg struct{}

type llmHealthMsg struct {
	OK  bool
	Err error
}
