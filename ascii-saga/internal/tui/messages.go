package tui

import (
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/agent"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/assets"
	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/engine"
)

// Bubble Tea message types. Streaming events from the engine and
// internal animation/timing events.

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

// pipelineStageMsg signals which agent is currently doing work.
// Values: "director" | "master" | "artist" | "judge" | "idle".
type pipelineStageMsg struct {
	Stage string
}

// frameTickMsg drives animations at ~33ms cadence (≈30fps).
type frameTickMsg struct{}

type llmHealthMsg struct {
	OK  bool
	Err error
}

// themeCycleMsg cycles to the next theme.
type themeCycleMsg struct{}
