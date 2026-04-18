package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/carlosmaranje/goclaw/internal/llm"
)

type captureLLM struct {
	mu       sync.Mutex
	calls    []llm.CompletionRequest
	response string
	err      error
}

func (c *captureLLM) Complete(ctx context.Context, req llm.CompletionRequest) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, req)
	return c.response, c.err
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
	r := NewRunner(&Agent{Name: "x", LLM: &captureLLM{response: "ok"}}, time.Second)
	if _, err := r.invokeLLM(context.Background(), "hi", nil, false); err == nil {
		t.Fatal("expected error for empty system prompt")
	}
}

func TestRunner_InvokeLLM_RequiresLLM(t *testing.T) {
	r := NewRunner(&Agent{Name: "x", SystemPrompt: "sp"}, time.Second)
	if _, err := r.invokeLLM(context.Background(), "hi", nil, false); err == nil {
		t.Fatal("expected error for missing LLM client")
	}
}

func TestRunner_InvokeLLM_UsesSystemPromptAndGoal(t *testing.T) {
	llmc := &captureLLM{response: "reply"}
	r := NewRunner(&Agent{Name: "x", LLM: llmc, SystemPrompt: "I am x"}, time.Second)

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

func TestRunner_InvokeLLM_HistoryPreferredOverSession(t *testing.T) {
	llmc := &captureLLM{response: "ok"}
	sess := NewSessionStore()
	sess.Append(llm.Message{Role: "user", Content: "from session"})

	r := NewRunner(&Agent{Name: "x", LLM: llmc, SystemPrompt: "sp", Session: sess}, time.Second)

	history := []llm.Message{{Role: "user", Content: "from history"}}
	if _, err := r.invokeLLM(context.Background(), "goal", history, false); err != nil {
		t.Fatal(err)
	}

	msgs := llmc.lastMessages()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages (system+history+goal), got %d: %+v", len(msgs), msgs)
	}
	if msgs[1].Content != "from history" {
		t.Errorf("expected history msg, got %q", msgs[1].Content)
	}
	if len(sess.Snapshot()) != 1 {
		t.Error("Session was written despite explicit history — should skip Session writes")
	}
}

func TestRunner_InvokeLLM_FallsBackToSession(t *testing.T) {
	llmc := &captureLLM{response: "reply"}
	sess := NewSessionStore()
	sess.Append(llm.Message{Role: "user", Content: "prior"})

	r := NewRunner(&Agent{Name: "x", LLM: llmc, SystemPrompt: "sp", Session: sess}, time.Second)

	if _, err := r.invokeLLM(context.Background(), "next", nil, false); err != nil {
		t.Fatal(err)
	}

	snap := sess.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("expected 3 session msgs (prior+next+reply), got %d", len(snap))
	}
	if snap[1].Content != "next" || snap[2].Content != "reply" {
		t.Errorf("session update wrong: %+v", snap)
	}
}

func TestRunner_InvokeLLM_LLMError(t *testing.T) {
	r := NewRunner(&Agent{Name: "x", LLM: &captureLLM{err: errors.New("boom")}, SystemPrompt: "sp"}, time.Second)
	if _, err := r.invokeLLM(context.Background(), "goal", nil, false); err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestRunner_SubmitAndReply(t *testing.T) {
	llmc := &captureLLM{response: "done"}
	r := NewRunner(&Agent{Name: "x", LLM: llmc, SystemPrompt: "sp"}, 50*time.Millisecond)

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
	r := NewRunner(&Agent{Name: "x", LLM: &captureLLM{}, SystemPrompt: "sp"}, time.Second)
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
	r := NewRunner(&Agent{Name: "x", LLM: &captureLLM{}, SystemPrompt: "sp"}, time.Second)
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
