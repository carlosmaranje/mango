package discord

import (
	"sync"

	"github.com/carlosmaranje/goclaw/internal/llm"
)

const DefaultHistorySize = 20

type ChannelHistory struct {
	mu      sync.Mutex
	size    int
	buffers map[string][]llm.Message
}

func NewChannelHistory(size int) *ChannelHistory {
	if size <= 0 {
		size = DefaultHistorySize
	}
	return &ChannelHistory{size: size, buffers: make(map[string][]llm.Message)}
}

func (c *ChannelHistory) Append(channelID string, msg llm.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	buf := c.buffers[channelID]
	buf = append(buf, msg)
	if len(buf) > c.size {
		buf = buf[len(buf)-c.size:]
	}
	c.buffers[channelID] = buf
}

func (c *ChannelHistory) Get(channelID string) []llm.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	buf := c.buffers[channelID]
	out := make([]llm.Message, len(buf))
	copy(out, buf)
	return out
}
