package agent

import "github.com/carlosmaranje/mango/core/llm"

// estTokens is a rough, provider-agnostic token estimate (~4 chars/token) plus a
// small per-message overhead. It is intentionally cheap, not exact.
func estTokens(m llm.Message) int {
	n := len(m.Content) / 4
	for _, tc := range m.ToolCalls {
		n += (len(tc.Name) + len(tc.Input)) / 4
	}
	return n + 4
}

// windowMessages keeps a single invocation's message list within a
// token-equivalent budget. It always preserves the system prompt (msgs[0]) and
// the most recent turns, dropping the OLDEST turns first.
//
// Critical invariant: an assistant turn that carries tool calls is kept or
// dropped together with its following tool-result messages as one atomic unit,
// so a tool_call_id is never orphaned (providers reject mismatched pairs).
//
// Returns msgs unchanged when already within budget (the common case), so short
// conversations are unaffected.
func windowMessages(msgs []llm.Message, budget int) []llm.Message {
	if budget <= 0 || len(msgs) <= 2 {
		return msgs
	}
	total := 0
	for _, m := range msgs {
		total += estTokens(m)
	}
	if total <= budget {
		return msgs
	}

	system := msgs[0]
	rest := msgs[1:]

	// Group rest into atomic blocks: an assistant-with-tool-calls plus its
	// trailing tool results is one block; every other message is its own block.
	type block struct {
		msgs   []llm.Message
		tokens int
	}
	var blocks []block
	for i := 0; i < len(rest); {
		m := rest[i]
		b := block{msgs: []llm.Message{m}, tokens: estTokens(m)}
		i++
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			for i < len(rest) && rest[i].Role == "tool" {
				b.msgs = append(b.msgs, rest[i])
				b.tokens += estTokens(rest[i])
				i++
			}
		}
		blocks = append(blocks, b)
	}

	// Keep blocks from the most recent backward until the next would exceed the
	// remaining budget. The most recent block is always kept.
	remaining := budget - estTokens(system)
	keepFrom := len(blocks)
	for j := len(blocks) - 1; j >= 0; j-- {
		if j < len(blocks)-1 && blocks[j].tokens > remaining {
			break
		}
		remaining -= blocks[j].tokens
		keepFrom = j
	}

	out := make([]llm.Message, 0, len(msgs))
	out = append(out, system)
	for _, b := range blocks[keepFrom:] {
		out = append(out, b.msgs...)
	}
	return out
}
