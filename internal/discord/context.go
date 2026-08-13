package discord

import (
	"sync"

	"github.com/carlosmaranje/mango/core"
)

const DefaultHistorySize = 100

type ChannelHistory struct {
	mu      sync.Mutex
	size    int
	buffers map[string][]core.Message
}

func NewChannelHistory(size int) *ChannelHistory {
	if size <= 0 {
		size = DefaultHistorySize
	}
	return &ChannelHistory{size: size, buffers: make(map[string][]core.Message)}
}

func (c *ChannelHistory) Append(channelID string, msg core.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	buf := c.buffers[channelID]
	buf = append(buf, msg)
	if len(buf) > c.size {
		buf = buf[len(buf)-c.size:]
	}
	c.buffers[channelID] = buf
}

func (c *ChannelHistory) Get(channelID string) []core.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	buf := c.buffers[channelID]
	out := make([]core.Message, len(buf))
	copy(out, buf)
	return out
}
