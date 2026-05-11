// Package agent contains the LLM-facing components: Director, Master,
// Artist, Companion, plus the deterministic Judge.
//
// All LLM calls funnel through Client, which wraps go-openai pointed at
// an OpenAI-compatible endpoint (LM Studio, llama.cpp server, vLLM, etc.).
package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"github.com/cruzeirocabuloso151-cell/ascii-saga/internal/config"
)

// Client is a thin wrapper that exposes the small slice of go-openai
// the rest of the project actually uses.
type Client struct {
	api    *openai.Client
	cfg    config.LLMConfig
	logger Logger
}

// Logger is intentionally tiny so callers can pass any structured logger
// (or io.Discard) without dragging zerolog into every package.
type Logger interface {
	Debug(msg string, kv ...any)
	Info(msg string, kv ...any)
	Error(msg string, kv ...any)
}

// NopLogger throws everything away. Useful in tests.
type NopLogger struct{}

func (NopLogger) Debug(string, ...any) {}
func (NopLogger) Info(string, ...any)  {}
func (NopLogger) Error(string, ...any) {}

func NewClient(cfg config.LLMConfig, log Logger) *Client {
	if log == nil {
		log = NopLogger{}
	}
	oc := openai.DefaultConfig(cfg.APIKey)
	oc.BaseURL = cfg.BaseURL
	return &Client{api: openai.NewClientWithConfig(oc), cfg: cfg, logger: log}
}

// Health pings the endpoint with a one-token completion.
// We avoid /models because some llama.cpp builds don't expose it.
func (c *Client) Health(ctx context.Context) error {
	hctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := c.api.CreateChatCompletion(hctx, openai.ChatCompletionRequest{
		Model:     c.cfg.ModelDirector,
		MaxTokens: 1,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: "ping"},
		},
	})
	return err
}

// CallParams collects the knobs each agent role needs.
type CallParams struct {
	Model       string
	System      string
	User        string
	Temperature float32
	MaxTokens   int
	Stop        []string
}

// Complete runs a non-streaming chat completion and returns the full text.
func (c *Client) Complete(ctx context.Context, p CallParams) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, time.Duration(c.cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	resp, err := c.api.CreateChatCompletion(cctx, openai.ChatCompletionRequest{
		Model:       p.Model,
		Temperature: p.Temperature,
		MaxTokens:   p.MaxTokens,
		Stop:        p.Stop,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: p.System},
			{Role: openai.ChatMessageRoleUser, Content: p.User},
		},
	})
	if err != nil {
		c.logger.Error("complete failed", "model", p.Model, "err", err)
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("no choices returned")
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

// Stream runs a streaming chat completion. Each chunk of new text is
// pushed to onChunk. The final concatenated text is returned, regardless
// of whether onChunk did anything with it.
func (c *Client) Stream(ctx context.Context, p CallParams, onChunk func(string)) (string, error) {
	sctx, cancel := context.WithTimeout(ctx, time.Duration(c.cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	stream, err := c.api.CreateChatCompletionStream(sctx, openai.ChatCompletionRequest{
		Model:       p.Model,
		Temperature: p.Temperature,
		MaxTokens:   p.MaxTokens,
		Stop:        p.Stop,
		Stream:      true,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: p.System},
			{Role: openai.ChatMessageRoleUser, Content: p.User},
		},
	})
	if err != nil {
		return "", fmt.Errorf("open stream: %w", err)
	}
	defer stream.Close()

	var b strings.Builder
	for {
		ev, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return b.String(), fmt.Errorf("stream recv: %w", err)
		}
		if len(ev.Choices) == 0 {
			continue
		}
		delta := ev.Choices[0].Delta.Content
		if delta == "" {
			continue
		}
		b.WriteString(delta)
		if onChunk != nil {
			onChunk(delta)
		}
	}
	return b.String(), nil
}
