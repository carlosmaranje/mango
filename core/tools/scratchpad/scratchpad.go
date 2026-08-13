// Package scratchpad provides a durable key-value "scratchpad" tool that exposes
// an agent's memory.Store to the LLM. It is the one concrete tool that ships in
// core — sanctioned because it is engine plumbing over the Store core already
// provides, not domain knowledge. It is never auto-loaded; a host opts in per
// agent (see core.Options.EnableScratchpad / core.AgentSpec.EnableScratchpad).
package scratchpad

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/carlosmaranje/mango/core/memory"
	"github.com/carlosmaranje/mango/core/tools"
)

// keyPrefix isolates scratchpad entries from any other keys in the Store (e.g.
// the runner's "heartbeat/last").
const keyPrefix = "scratch/"

type scratchpad struct {
	store     memory.Store
	namespace string
}

// New returns a scratchpad tool backed by store. Keys are namespaced so multiple
// agents sharing a store (or a future shared store) never collide.
func New(store memory.Store, namespace string) tools.Tool {
	return &scratchpad{store: store, namespace: namespace}
}

func (s *scratchpad) Name() string { return "scratchpad" }

func (s *scratchpad) Description() string {
	return "A durable key-value scratchpad for carrying state across steps and invocations. " +
		"op=set saves a value under a key; op=get reads it back; op=list enumerates keys under an optional prefix; op=delete removes a key. " +
		"Values persist across tool calls and restarts — use it to hand intermediate results (found URLs, parsed data, progress markers) from an earlier step to a later one."
}

func (s *scratchpad) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{Name: "op", Type: "string", Description: "Operation: get | set | list | delete", Required: true},
		{Name: "key", Type: "string", Description: "Key for get/set/delete", Required: false},
		{Name: "value", Type: "string", Description: "Value to store (op=set)", Required: false},
		{Name: "prefix", Type: "string", Description: "Key prefix to filter (op=list); empty lists all", Required: false},
	}
}

type result struct {
	OK    bool              `json:"ok"`
	Op    string            `json:"op"`
	Key   string            `json:"key,omitempty"`
	Value string            `json:"value,omitempty"`
	Found bool              `json:"found"`
	Keys  map[string]string `json:"keys,omitempty"`
}

func (s *scratchpad) Returns() string {
	return tools.DescribeReturnType(result{})
}

type input struct {
	Op     string `json:"op"`
	Key    string `json:"key"`
	Value  string `json:"value"`
	Prefix string `json:"prefix"`
}

func (s *scratchpad) base() string {
	return keyPrefix + s.namespace + "/"
}

func (s *scratchpad) fullKey(k string) string {
	return s.base() + k
}

func (s *scratchpad) Execute(_ context.Context, raw string) (string, error) {
	if s.store == nil {
		return "", fmt.Errorf("scratchpad has no backing store")
	}
	var in input
	if raw != "" && raw != "{}" {
		if err := json.Unmarshal([]byte(raw), &in); err != nil {
			return "", fmt.Errorf("invalid input: %w", err)
		}
	}

	op := strings.ToLower(strings.TrimSpace(in.Op))
	res := result{Op: op}
	switch op {
	case "set":
		if in.Key == "" {
			return "", fmt.Errorf("op=set requires key")
		}
		if err := s.store.Set(s.fullKey(in.Key), in.Value); err != nil {
			return "", err
		}
		res.OK, res.Key, res.Value, res.Found = true, in.Key, in.Value, true
	case "get":
		if in.Key == "" {
			return "", fmt.Errorf("op=get requires key")
		}
		v, err := s.store.Get(s.fullKey(in.Key))
		if err != nil {
			return "", err
		}
		res.OK, res.Key, res.Value, res.Found = true, in.Key, v, v != ""
	case "delete":
		if in.Key == "" {
			return "", fmt.Errorf("op=delete requires key")
		}
		if err := s.store.Delete(s.fullKey(in.Key)); err != nil {
			return "", err
		}
		res.OK, res.Key = true, in.Key
	case "list":
		entries, err := s.store.List(s.fullKey(in.Prefix))
		if err != nil {
			return "", err
		}
		res.Keys = make(map[string]string, len(entries))
		for k, v := range entries {
			res.Keys[strings.TrimPrefix(k, s.base())] = v
		}
		res.OK, res.Found = true, len(entries) > 0
	default:
		return "", fmt.Errorf("unknown op %q (want get|set|list|delete)", in.Op)
	}

	out, err := json.Marshal(res)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
