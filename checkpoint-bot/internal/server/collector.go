package server

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/charmbracelet/log"
	"github.com/metruzanca/checkpoint-bot/internal/memory"
)

type passiveCollectorConfig struct {
	Interval         time.Duration
	MaxBatchMessages int
	MaxTranscript    int
	ExtractTimeout   time.Duration
	IgnoreChannels   map[string]bool
}

type passiveCollector struct {
	store     *memory.Store
	extractor memoryExtractor
	cfg       passiveCollectorConfig

	messages chan *discordgo.MessageCreate
	stop     chan struct{}
	done     chan struct{}

	mu      sync.Mutex
	pending map[string][]*discordgo.MessageCreate
}

func newPassiveCollector(store *memory.Store, extractor memoryExtractor, cfg passiveCollectorConfig) *passiveCollector {
	if cfg.Interval <= 0 {
		cfg.Interval = 60 * time.Second
	}
	if cfg.MaxBatchMessages <= 0 {
		cfg.MaxBatchMessages = 20
	}
	if cfg.MaxTranscript <= 0 {
		cfg.MaxTranscript = maxTranscriptChars
	}
	if cfg.ExtractTimeout <= 0 {
		cfg.ExtractTimeout = 30 * time.Second
	}

	return &passiveCollector{
		store:     store,
		extractor: extractor,
		cfg:       cfg,
		messages:  make(chan *discordgo.MessageCreate, 1000),
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
		pending:   make(map[string][]*discordgo.MessageCreate),
	}
}

func (c *passiveCollector) Start() {
	go c.run()
}

func (c *passiveCollector) Stop() {
	close(c.stop)
	<-c.done
}

func (c *passiveCollector) Handle(s *discordgo.Session, m *discordgo.MessageCreate) {
	if c == nil || s == nil || s.State == nil || s.State.User == nil {
		return
	}
	if m == nil || m.Message == nil || m.Author == nil || m.Author.Bot {
		return
	}
	if m.GuildID == "" || m.Author.ID == s.State.User.ID || !isDigits(m.Author.ID) {
		return
	}
	if c.cfg.IgnoreChannels[m.ChannelID] {
		return
	}
	if messageText(m.Message) == "" {
		return
	}

	select {
	case c.messages <- m:
	default:
		log.Warn("passive memory queue full", "channel", m.ChannelID, "user", m.Author.ID)
	}
}

func (c *passiveCollector) run() {
	defer close(c.done)
	ticker := time.NewTicker(c.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stop:
			c.drain()
			c.flush()
			return
		case m := <-c.messages:
			c.add(m)
		case <-ticker.C:
			c.flush()
		}
	}
}

func (c *passiveCollector) drain() {
	for {
		select {
		case m := <-c.messages:
			c.add(m)
		default:
			return
		}
	}
}

func (c *passiveCollector) add(m *discordgo.MessageCreate) {
	if m == nil || m.Author == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	batch := append(c.pending[m.Author.ID], m)
	if len(batch) > c.cfg.MaxBatchMessages {
		batch = batch[len(batch)-c.cfg.MaxBatchMessages:]
	}
	c.pending[m.Author.ID] = batch
}

func (c *passiveCollector) flush() {
	c.mu.Lock()
	pending := c.pending
	c.pending = make(map[string][]*discordgo.MessageCreate)
	c.mu.Unlock()

	ids := make([]string, 0, len(pending))
	for id := range pending {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		c.process(id, pending[id])
	}
}

func (c *passiveCollector) process(authorID string, messages []*discordgo.MessageCreate) {
	if len(messages) == 0 {
		return
	}

	author := messages[len(messages)-1].Author
	if author == nil {
		return
	}

	transcript := buildPassiveTranscript(messages, c.cfg.MaxTranscript)
	if transcript == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.ExtractTimeout)
	defer cancel()

	facts, err := c.extractor.Extract(ctx, transcript, map[string]string{
		authorID: displayName(author),
	})
	if err != nil {
		log.Error("passive memory extraction failed", "err", err, "user", authorID)
		return
	}

	filtered := facts[:0]
	for _, fact := range facts {
		if fact.SubjectID == authorID {
			filtered = append(filtered, fact)
		}
	}
	if len(filtered) == 0 {
		log.Debug("passive memory found nothing", "user", authorID, "messages", len(messages))
		return
	}
	if err := c.store.AppendFacts(filtered); err != nil {
		log.Error("cannot store passive memory", "err", err, "user", authorID)
		return
	}
	log.Info("passive memory updated", "user", authorID, "facts", len(filtered), "messages", len(messages))
}

func buildPassiveTranscript(messages []*discordgo.MessageCreate, maxChars int) string {
	var builder strings.Builder

	for _, m := range messages {
		if m == nil || m.Message == nil || m.Author == nil {
			continue
		}
		text := messageText(m.Message)
		if text == "" {
			continue
		}
		fmt.Fprintf(&builder, "%s (%s): %s\n", displayName(m.Author), m.Author.ID, truncateRunes(text, 2000))
	}

	return tailRunes(builder.String(), maxChars)
}
