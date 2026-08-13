package tools

import (
	"context"
	"errors"
	"testing"
	"time"
)

// flakyTool fails its first failsLeft calls, then succeeds.
type flakyTool struct{ failsLeft int }

func (f *flakyTool) Name() string            { return "flaky" }
func (f *flakyTool) Description() string     { return "fails then succeeds" }
func (f *flakyTool) Returns() string         { return "" }
func (f *flakyTool) Parameters() []Parameter { return nil }
func (f *flakyTool) Execute(_ context.Context, _ string) (string, error) {
	if f.failsLeft > 0 {
		f.failsLeft--
		return "", errors.New("transient")
	}
	return "ok", nil
}

// blockTool blocks until the context is cancelled.
type blockTool struct{}

func (blockTool) Name() string            { return "block" }
func (blockTool) Description() string     { return "blocks until ctx done" }
func (blockTool) Returns() string         { return "" }
func (blockTool) Parameters() []Parameter { return nil }
func (blockTool) Execute(ctx context.Context, _ string) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func TestExecuteWithPolicy_RetrySucceeds(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&flakyTool{failsLeft: 1})

	res := reg.ExecuteWithPolicy(context.Background(), "flaky", "{}", ExecPolicy{MaxAttempts: 3})
	if res.Err != nil {
		t.Fatalf("expected success after retry, got %v", res.Err)
	}
	if res.Attempts != 2 {
		t.Errorf("attempts = %d, want 2", res.Attempts)
	}
	if res.Output != "ok" {
		t.Errorf("output = %q, want ok", res.Output)
	}
}

func TestExecuteWithPolicy_DefaultNoRetry(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&flakyTool{failsLeft: 1})

	res := reg.ExecuteWithPolicy(context.Background(), "flaky", "{}", ExecPolicy{})
	if res.Err == nil {
		t.Fatal("expected failure with default (no retry) policy")
	}
	if res.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", res.Attempts)
	}
}

func TestExecuteWithPolicy_Timeout(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(blockTool{})

	res := reg.ExecuteWithPolicy(context.Background(), "block", "{}", ExecPolicy{MaxAttempts: 1, Timeout: 20 * time.Millisecond})
	if res.Err == nil {
		t.Fatal("expected timeout error")
	}
	if !res.TimedOut {
		t.Error("expected TimedOut = true")
	}
}
