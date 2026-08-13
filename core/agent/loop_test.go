package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/carlosmaranje/mango/core/llm"
	"github.com/carlosmaranje/mango/core/tools"
)

// loopLLM returns a scripted response per call index (1-based).
type loopLLM struct {
	mu    sync.Mutex
	calls int
	fn    func(call int, req llm.CompletionRequest) llm.CompletionResponse
}

func (l *loopLLM) Complete(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	l.mu.Lock()
	l.calls++
	n := l.calls
	l.mu.Unlock()
	return l.fn(n, req), nil
}

func (l *loopLLM) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

// blockingLLM blocks until the context is cancelled.
type blockingLLM struct{}

func (blockingLLM) Complete(ctx context.Context, _ llm.CompletionRequest) (llm.CompletionResponse, error) {
	<-ctx.Done()
	return llm.CompletionResponse{}, ctx.Err()
}

type echoTool struct{}

func (echoTool) Name() string                  { return "echo" }
func (echoTool) Description() string           { return "echoes its input" }
func (echoTool) Returns() string               { return "" }
func (echoTool) Parameters() []tools.Parameter { return nil }
func (echoTool) Execute(_ context.Context, in string) (string, error) {
	return in, nil
}

func echoRegistry() *tools.Registry {
	r := tools.NewRegistry()
	_ = r.Register(echoTool{})
	return r
}

func TestRunner_MaxStepsStopsGracefully(t *testing.T) {
	llmc := &loopLLM{fn: func(call int, _ llm.CompletionRequest) llm.CompletionResponse {
		// Always call a tool (never finishes), with a unique input so the
		// no-progress detector doesn't fire first.
		return llm.CompletionResponse{
			Content:   "working",
			ToolCalls: []llm.ToolCall{{ID: "x", Name: "echo", Input: fmt.Sprintf(`{"n":%d}`, call)}},
		}
	}}
	a := &Agent{Name: "x", LLM: llmc, SystemPrompt: "sp", Limits: Limits{MaxSteps: 3}}
	r := NewRunner(a, echoRegistry(), time.Second)

	out, reason, err := r.runSteps(context.Background(), "t1", "go", nil, false)
	if err != nil {
		t.Fatalf("max-steps should stop gracefully, got err: %v", err)
	}
	if reason != StopMaxSteps {
		t.Errorf("reason = %q, want %q", reason, StopMaxSteps)
	}
	if out != "working" {
		t.Errorf("expected best-effort content %q, got %q", "working", out)
	}
	if llmc.count() != 3 {
		t.Errorf("calls = %d, want 3", llmc.count())
	}
}

func TestRunner_NoProgressStops(t *testing.T) {
	llmc := &loopLLM{fn: func(_ int, _ llm.CompletionRequest) llm.CompletionResponse {
		// Identical content + identical tool call every time.
		return llm.CompletionResponse{
			Content:   "same",
			ToolCalls: []llm.ToolCall{{ID: "x", Name: "echo", Input: `{"same":true}`}},
		}
	}}
	a := &Agent{Name: "x", LLM: llmc, SystemPrompt: "sp", Limits: Limits{MaxSteps: 20, NoProgressN: 2}}
	r := NewRunner(a, echoRegistry(), time.Second)

	_, reason, err := r.runSteps(context.Background(), "t", "go", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if reason != StopNoProgress {
		t.Errorf("reason = %q, want %q", reason, StopNoProgress)
	}
	if llmc.count() > 4 {
		t.Errorf("expected early no-progress stop, got %d calls", llmc.count())
	}
}

func TestRunner_DeadlineStops(t *testing.T) {
	a := &Agent{Name: "x", LLM: blockingLLM{}, SystemPrompt: "sp", Limits: Limits{Deadline: 30 * time.Millisecond}}
	r := NewRunner(a, nil, time.Second)

	_, reason, err := r.runSteps(context.Background(), "t", "go", nil, false)
	if err != nil {
		t.Fatalf("deadline should stop gracefully, got err: %v", err)
	}
	if reason != StopDeadline {
		t.Errorf("reason = %q, want %q", reason, StopDeadline)
	}
}

func TestRunner_ParentCancelStops(t *testing.T) {
	a := &Agent{Name: "x", LLM: blockingLLM{}, SystemPrompt: "sp"}
	r := NewRunner(a, nil, time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, reason, err := r.runSteps(ctx, "t", "go", nil, false)
	if err != nil {
		t.Fatalf("parent cancel should stop gracefully, got err: %v", err)
	}
	if reason != StopContextCanceled {
		t.Errorf("reason = %q, want %q", reason, StopContextCanceled)
	}
}

func TestWindowMessages_NoOpUnderBudget(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "s"},
		{Role: "user", Content: "u"},
		{Role: "assistant", Content: "a"},
	}
	out := windowMessages(msgs, 100_000)
	if len(out) != len(msgs) {
		t.Fatalf("expected unchanged, got %d of %d", len(out), len(msgs))
	}
}

func TestWindowMessages_KeepsSystemAndRecent(t *testing.T) {
	big := strings.Repeat("x", 4000)
	msgs := []llm.Message{{Role: "system", Content: "sys"}}
	for range 8 {
		msgs = append(msgs, llm.Message{Role: "user", Content: big})
		msgs = append(msgs, llm.Message{Role: "assistant", Content: big})
	}
	out := windowMessages(msgs, 1500)

	if out[0].Role != "system" || out[0].Content != "sys" {
		t.Error("system prompt not preserved")
	}
	if len(out) >= len(msgs) {
		t.Errorf("expected trimming, got %d of %d", len(out), len(msgs))
	}
	if out[len(out)-1].Content != msgs[len(msgs)-1].Content {
		t.Error("most recent message not preserved")
	}
}

func TestWindowMessages_NeverOrphansToolResults(t *testing.T) {
	big := strings.Repeat("x", 4000)
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "old"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "a", Name: "echo", Input: "{}"}}},
		{Role: "tool", ToolCallID: "a", Name: "echo", Content: big},
		{Role: "user", Content: "recent"},
	}
	for _, budget := range []int{10, 100, 500, 1010, 1018, 1100, 100_000} {
		out := windowMessages(msgs, budget)
		if out[0].Role != "system" {
			t.Errorf("budget %d: system dropped", budget)
		}
		for i, m := range out {
			if m.Role != "tool" {
				continue
			}
			ok := false
			for j := i - 1; j >= 0; j-- {
				if out[j].Role == "assistant" {
					for _, tc := range out[j].ToolCalls {
						if tc.ID == m.ToolCallID {
							ok = true
						}
					}
					break
				}
			}
			if !ok {
				t.Errorf("budget %d: orphaned tool result at index %d", budget, i)
			}
		}
	}
}
