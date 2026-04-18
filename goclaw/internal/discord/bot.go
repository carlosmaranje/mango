package discord

import (
	"context"
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"

	"github.com/carlosmaranje/goclaw/internal/llm"
	"github.com/carlosmaranje/goclaw/internal/orchestrator"
)

type Bot struct {
	session    *discordgo.Session
	router     *Router
	history    *ChannelHistory
	dispatcher *orchestrator.Dispatcher
}

func NewBot(token string, router *Router, history *ChannelHistory, dispatcher *orchestrator.Dispatcher) (*Bot, error) {
	sess, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("discord session: %w", err)
	}
	sess.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages | discordgo.IntentsMessageContent

	b := &Bot{
		session:    sess,
		router:     router,
		history:    history,
		dispatcher: dispatcher,
	}
	sess.AddHandler(b.onMessage)
	return b, nil
}

func (b *Bot) Start(ctx context.Context) error {
	if err := b.session.Open(); err != nil {
		return fmt.Errorf("discord open: %w", err)
	}
	go func() {
		<-ctx.Done()
		_ = b.session.Close()
	}()
	return nil
}

func (b *Bot) Close() error {
	return b.session.Close()
}

func (b *Bot) onMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if s.State.User != nil && m.Author.ID == s.State.User.ID {
		return
	}
	if m.Author.Bot {
		return
	}

	agentName := b.router.Resolve(m.ChannelID)
	b.history.Append(m.ChannelID, llm.Message{Role: "user", Content: m.Content})

	ctx := context.Background()
	task, err := b.dispatcher.Submit(ctx, m.Content, agentName)
	if err != nil {
		log.Printf("discord: dispatch: %v", err)
		_, _ = s.ChannelMessageSendReply(m.ChannelID, "error: "+err.Error(), m.Reference())
		return
	}

	go b.waitAndReply(s, m, task.ID)
}

func (b *Bot) waitAndReply(s *discordgo.Session, m *discordgo.MessageCreate, taskID string) {
	for i := 0; i < 300; i++ {
		t, ok := b.dispatcher.Get(taskID)
		if !ok {
			return
		}
		if t.Status == orchestrator.StatusDone || t.Status == orchestrator.StatusFailed {
			reply := t.Result
			if t.Status == orchestrator.StatusFailed {
				reply = "error: " + t.Error
			}
			b.history.Append(m.ChannelID, llm.Message{Role: "assistant", Content: reply})
			if _, err := s.ChannelMessageSendReply(m.ChannelID, reply, m.Reference()); err != nil {
				log.Printf("discord: send reply: %v", err)
			}
			return
		}
		waitOneSecond()
	}
}
