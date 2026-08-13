package core_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/carlosmaranje/mango/core"
	"github.com/carlosmaranje/mango/core/llm"
	"github.com/carlosmaranje/mango/core/tools"
)

// scratchpadLLM drives a 3-step chain: set a value, read it back, then answer
// with whatever the get returned — proving state survives across steps.
type scratchpadLLM struct {
	mu    sync.Mutex
	calls int
}

func (s *scratchpadLLM) Complete(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	s.mu.Lock()
	s.calls++
	n := s.calls
	s.mu.Unlock()

	switch n {
	case 1:
		return llm.CompletionResponse{ToolCalls: []llm.ToolCall{
			{ID: "1", Name: "scratchpad", Input: `{"op":"set","key":"found","value":"http://x"}`},
		}}, nil
	case 2:
		return llm.CompletionResponse{ToolCalls: []llm.ToolCall{
			{ID: "2", Name: "scratchpad", Input: `{"op":"get","key":"found"}`},
		}}, nil
	default:
		var last string
		for _, m := range req.Messages {
			if m.Role == "tool" {
				last = m.Content
			}
		}
		return llm.CompletionResponse{Content: "final:" + last}, nil
	}
}

func TestEngine_ScratchpadRoundTrip(t *testing.T) {
	eng, ctx := newStarted(t, core.Options{
		EnableScratchpad: true,
		Agents: []core.AgentSpec{{
			Name:         "worker",
			SystemPrompt: "sp",
			LLM:          &scratchpadLLM{},
			WorkDir:      t.TempDir(),
		}},
	})

	res, err := eng.SubmitAndWait(ctx, core.Request{Goal: "go", Agent: "worker"})
	if err != nil {
		t.Fatalf("SubmitAndWait: %v", err)
	}
	if !strings.Contains(res.Output, "http://x") {
		t.Errorf("expected scratchpad value to round-trip into output, got %q", res.Output)
	}
}

func TestEngine_ScratchpadDisabledByDefault(t *testing.T) {
	eng, ctx := newStarted(t, core.Options{
		Agents: []core.AgentSpec{{
			Name:         "worker",
			SystemPrompt: "sp",
			LLM:          &scratchpadLLM{},
			WorkDir:      t.TempDir(),
		}},
	})

	res, err := eng.SubmitAndWait(ctx, core.Request{Goal: "go", Agent: "worker"})
	if err != nil {
		t.Fatalf("SubmitAndWait: %v", err)
	}
	if strings.Contains(res.Output, "http://x") {
		t.Errorf("scratchpad should be unavailable by default, but value round-tripped: %q", res.Output)
	}
}

func TestEngine_StepEventsWhenEnabled(t *testing.T) {
	eng, ctx := newStarted(t, core.Options{
		EmitStepEvents: true,
		Tools:          []tools.Tool{upperTool{}},
		Agents: []core.AgentSpec{{
			Name:         "worker",
			SystemPrompt: "sp",
			LLM:          &toolLLM{},
		}},
	})

	unsub, events := eng.Subscribe()
	defer unsub()

	if _, err := eng.Submit(ctx, core.Request{Goal: "go", Agent: "worker"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	var sawStep, sawTool bool
	deadline := time.After(3 * time.Second)
	for !(sawStep && sawTool) {
		select {
		case ev := <-events:
			switch ev.Type {
			case core.EventStepStarted, core.EventStepCompleted:
				sawStep = true
			case core.EventToolCalled, core.EventToolCompleted:
				sawTool = true
			}
		case <-deadline:
			t.Fatalf("missing fine-grained events (step=%v tool=%v)", sawStep, sawTool)
		}
	}
}

func TestEngine_NoStepEventsByDefault(t *testing.T) {
	eng, ctx := newStarted(t, core.Options{
		Tools: []tools.Tool{upperTool{}},
		Agents: []core.AgentSpec{{
			Name:         "worker",
			SystemPrompt: "sp",
			LLM:          &toolLLM{},
		}},
	})

	unsub, events := eng.Subscribe()
	defer unsub()

	if _, err := eng.SubmitAndWait(ctx, core.Request{Goal: "go", Agent: "worker"}); err != nil {
		t.Fatalf("SubmitAndWait: %v", err)
	}

	// Drain briefly; with EmitStepEvents off, no step/tool events must appear.
	timeout := time.After(300 * time.Millisecond)
	for {
		select {
		case ev := <-events:
			switch ev.Type {
			case core.EventStepStarted, core.EventStepCompleted,
				core.EventToolCalled, core.EventToolCompleted, core.EventInvocationStopped:
				t.Fatalf("unexpected fine-grained event with EmitStepEvents off: %q", ev.Type)
			}
		case <-timeout:
			return
		}
	}
}
