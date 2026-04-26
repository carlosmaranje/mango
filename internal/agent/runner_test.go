package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/carlosmaranje/mango/internal/llm"
)

type captureLLM struct {
	mu       sync.Mutex
	calls    []llm.CompletionRequest
	response string
	err      error
}

func (c *captureLLM) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, req)
	return llm.CompletionResponse{Content: c.response}, c.err
}

func (c *captureLLM) lastMessages() []llm.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.calls) == 0 {
		return nil
	}
	return append([]llm.Message(nil), c.calls[len(c.calls)-1].Messages...)
}

func TestRunner_InvokeLLM_RequiresSystemPrompt(t *testing.T) {
	r := NewRunner(&Agent{Name: "x", LLM: &captureLLM{response: "ok"}}, nil, time.Second)
	if _, err := r.invokeLLM(context.Background(), "hi", nil, false); err == nil {
		t.Fatal("expected error for empty system prompt")
	}
}

func TestRunner_InvokeLLM_RequiresLLM(t *testing.T) {
	r := NewRunner(&Agent{Name: "x", SystemPrompt: "sp"}, nil, time.Second)
	if _, err := r.invokeLLM(context.Background(), "hi", nil, false); err == nil {
		t.Fatal("expected error for missing LLM client")
	}
}

func TestRunner_InvokeLLM_UsesSystemPromptAndGoal(t *testing.T) {
	llmc := &captureLLM{response: "reply"}
	r := NewRunner(&Agent{Name: "x", LLM: llmc, SystemPrompt: "I am x"}, nil, time.Second)

	out, err := r.invokeLLM(context.Background(), "hello", nil, false)
	if err != nil {
		t.Fatalf("invokeLLM: %v", err)
	}
	if out != "reply" {
		t.Errorf("got %q, want reply", out)
	}

	msgs := llmc.lastMessages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (system+user), got %d", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[0].Content != "I am x" {
		t.Errorf("system wrong: %+v", msgs[0])
	}
	if msgs[1].Role != "user" || msgs[1].Content != "hello" {
		t.Errorf("user wrong: %+v", msgs[1])
	}
}

func TestRunner_InvokeLLM_InjectsHistory(t *testing.T) {
	llmc := &captureLLM{response: "ok"}
	r := NewRunner(&Agent{Name: "x", LLM: llmc, SystemPrompt: "sp"}, nil, time.Second)

	history := []llm.Message{
		{Role: "user", Content: "prior"},
		{Role: "assistant", Content: "yes"},
	}
	if _, err := r.invokeLLM(context.Background(), "goal", history, false); err != nil {
		t.Fatal(err)
	}

	msgs := llmc.lastMessages()
	// system + prior + yes + goal
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d: %+v", len(msgs), msgs)
	}
	if msgs[1].Content != "prior" || msgs[2].Content != "yes" {
		t.Errorf("history not injected correctly: %+v", msgs[1:3])
	}
}

func TestRunner_InvokeLLM_LLMError(t *testing.T) {
	r := NewRunner(&Agent{Name: "x", LLM: &captureLLM{err: errors.New("boom")}, SystemPrompt: "sp"}, nil, time.Second)
	if _, err := r.invokeLLM(context.Background(), "goal", nil, false); err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestRunner_SubmitAndReply(t *testing.T) {
	llmc := &captureLLM{response: "done"}
	r := NewRunner(&Agent{Name: "x", LLM: llmc, SystemPrompt: "sp"}, nil, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer r.Stop()

	reply := make(chan TaskResult, 1)
	r.Submit(TaskEnvelope{ID: "1", Goal: "ping", Reply: reply})

	select {
	case res := <-reply:
		if res.Err != nil {
			t.Fatal(res.Err)
		}
		if res.Result != "done" {
			t.Errorf("got %q, want done", res.Result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runner reply")
	}
}

func TestRunner_StartTwiceErrors(t *testing.T) {
	r := NewRunner(&Agent{Name: "x", LLM: &captureLLM{}, SystemPrompt: "sp"}, nil, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	if err := r.Start(ctx); err == nil {
		t.Fatal("starting twice should error")
	}
}

func TestRunner_IsRunningAfterStop(t *testing.T) {
	r := NewRunner(&Agent{Name: "x", LLM: &captureLLM{}, SystemPrompt: "sp"}, nil, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = r.Start(ctx)
	if !r.IsRunning() {
		t.Fatal("expected IsRunning=true after Start")
	}
	r.Stop()
	if r.IsRunning() {
		t.Fatal("expected IsRunning=false after Stop")
	}
}
