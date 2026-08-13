package core_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/carlosmaranje/mango/core"
	"github.com/carlosmaranje/mango/core/llm"
	"github.com/carlosmaranje/mango/core/tools"
)

// echoLLM is a stand-in chat client that replies with "echo: <last user msg>".
type echoLLM struct{}

func (echoLLM) Complete(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	var last string
	for _, m := range req.Messages {
		if m.Role == "user" {
			last = m.Content
		}
	}
	return llm.CompletionResponse{Content: "echo: " + last}, nil
}

// upperTool is a custom host-provided tool: it uppercases its "text" argument.
type upperTool struct{}

func (upperTool) Name() string        { return "upper" }
func (upperTool) Description() string { return "Uppercase the provided text." }
func (upperTool) Returns() string     { return "the uppercased string" }
func (upperTool) Parameters() []tools.Parameter {
	return []tools.Parameter{{Name: "text", Type: "string", Description: "text to uppercase", Required: true}}
}

func (upperTool) Execute(_ context.Context, input string) (string, error) {
	var in struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(input), &in); err != nil {
		return "", err
	}
	return strings.ToUpper(in.Text), nil
}

// toolLLM emits one tool call, then answers using the tool's result.
type toolLLM struct{ calls int }

func (t *toolLLM) Complete(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	t.calls++
	if t.calls == 1 {
		return llm.CompletionResponse{ToolCalls: []llm.ToolCall{
			{ID: "call-1", Name: "upper", Input: `{"text":"hi there"}`},
		}}, nil
	}
	var toolResult string
	for _, m := range req.Messages {
		if m.Role == "tool" {
			toolResult = m.Content
		}
	}
	return llm.CompletionResponse{Content: "result=" + toolResult}, nil
}

func newStarted(t *testing.T, opts core.Options) (*core.Engine, context.Context) {
	t.Helper()
	eng, err := core.New(opts)
	if err != nil {
		t.Fatalf("core.New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		eng.Stop()
	})
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return eng, ctx
}

// A host embeds the engine, registers a worker backed by a custom LLM, and
// drives it entirely through the public Request/Result contract.
func TestEngine_DirectAgent(t *testing.T) {
	eng, ctx := newStarted(t, core.Options{
		Agents: []core.AgentSpec{{
			Name:         "worker",
			SystemPrompt: "You are a worker.",
			LLM:          echoLLM{},
		}},
	})

	res, err := eng.SubmitAndWait(ctx, core.Request{Goal: "hello", Agent: "worker"})
	if err != nil {
		t.Fatalf("SubmitAndWait: %v", err)
	}
	if res.Status != core.StatusDone {
		t.Fatalf("status = %q, want done (err=%q)", res.Status, res.Error)
	}
	if res.Output != "echo: hello" {
		t.Errorf("output = %q, want %q", res.Output, "echo: hello")
	}
}

// The full agent tool loop: LLM -> tool call -> tool registry -> LLM -> answer.
func TestEngine_ToolLoop(t *testing.T) {
	eng, ctx := newStarted(t, core.Options{
		Tools: []tools.Tool{upperTool{}},
		Agents: []core.AgentSpec{{
			Name:         "worker",
			SystemPrompt: "You are a worker with tools.",
			LLM:          &toolLLM{},
		}},
	})

	res, err := eng.SubmitAndWait(ctx, core.Request{Goal: "go", Agent: "worker"})
	if err != nil {
		t.Fatalf("SubmitAndWait: %v", err)
	}
	if res.Output != "result=HI THERE" {
		t.Errorf("output = %q, want %q", res.Output, "result=HI THERE")
	}
}

// The streaming OUTPUT contract: task.created then task.updated(done).
func TestEngine_Events(t *testing.T) {
	eng, ctx := newStarted(t, core.Options{
		Agents: []core.AgentSpec{{
			Name:         "worker",
			SystemPrompt: "You are a worker.",
			LLM:          echoLLM{},
		}},
	})

	unsub, events := eng.Subscribe()
	defer unsub()

	if _, err := eng.Submit(ctx, core.Request{Goal: "hi", Agent: "worker"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	var sawCreated, sawDone bool
	deadline := time.After(3 * time.Second)
	for !sawDone {
		select {
		case ev := <-events:
			switch ev.Type {
			case core.EventTaskCreated:
				sawCreated = true
			case core.EventTaskUpdated:
				if ev.Task != nil && ev.Task.Status == core.StatusDone {
					sawDone = true
				}
			}
		case <-deadline:
			t.Fatalf("timed out waiting for events (created=%v done=%v)", sawCreated, sawDone)
		}
	}
	if !sawCreated {
		t.Error("never observed task.created event")
	}
}

// Agents with empty personas must be rejected, not silently run.
func TestEngine_RejectsEmptyPrompt(t *testing.T) {
	eng, err := core.New(core.Options{
		Agents: []core.AgentSpec{{Name: "bad", LLM: echoLLM{}}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := eng.Start(context.Background()); err == nil {
		eng.Stop()
		t.Fatal("expected Start to fail for empty system prompt")
	}
}

// Leaving Agent empty routes through the orchestrator role.
func TestEngine_OrchestratorRouting(t *testing.T) {
	// orchestrator that immediately finishes with a fixed answer.
	orch := &scriptLLM{reply: `{"action":"finish","tasks":[],"final":"orchestrated answer"}`}
	eng, ctx := newStarted(t, core.Options{
		Agents: []core.AgentSpec{
			{Name: "orchestrator", Role: "orchestrator", SystemPrompt: "Decompose.", LLM: orch},
			{Name: "worker", SystemPrompt: "Work.", LLM: echoLLM{}},
		},
	})

	res, err := eng.SubmitAndWait(ctx, core.Request{Goal: "do it"}) // no Agent => orchestrator
	if err != nil {
		t.Fatalf("SubmitAndWait: %v", err)
	}
	if res.Output != "orchestrated answer" {
		t.Errorf("output = %q, want %q", res.Output, "orchestrated answer")
	}
}

// scriptLLM always returns the same fixed reply.
type scriptLLM struct{ reply string }

func (s *scriptLLM) Complete(_ context.Context, _ llm.CompletionRequest) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{Content: s.reply}, nil
}
