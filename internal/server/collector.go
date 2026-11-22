package server

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/charmbracelet/log"
	"github.com/metruzanca/caffeinator/agent"
)

type passiveCollectorConfig struct {
	Interval         time.Duration
	MaxBatchMessages int
	ExtractTimeout   time.Duration
	IgnoreChannels   map[string]bool
}

type memoryLearner interface {
	Learn(ctx context.Context, turns []agent.Turn, subject agent.User, includeServer bool) ([]agent.Fact, error)
}

type passiveCollector struct {
	learner memoryLearner
	cfg     passiveCollectorConfig

	messages chan agent.Turn
	stop     chan struct{}
	done     chan struct{}

	mu      sync.Mutex
	pending map[string][]agent.Turn
}

func newPassiveCollector(learner memoryLearner, cfg passiveCollectorConfig) *passiveCollector {
	if cfg.Interval <= 0 {
		cfg.Interval = 60 * time.Second
	}
	if cfg.MaxBatchMessages <= 0 {
		cfg.MaxBatchMessages = 20
	}
	if cfg.ExtractTimeout <= 0 {
		cfg.ExtractTimeout = 30 * time.Second
	}

	return &passiveCollector{
		learner:  learner,
		cfg:      cfg,
		messages: make(chan agent.Turn, 1000),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		pending:  make(map[string][]agent.Turn),
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
	if m.WebhookID != "" {
		return
	}
	if m.GuildID == "" || m.Author.ID == s.State.User.ID || !isDigits(m.Author.ID) {
		return
	}
	if c.cfg.IgnoreChannels[m.ChannelID] {
		return
	}
	if strings.TrimSpace(m.Content) == "" {
		return
	}

	select {
	case c.messages <- toAgentTurn(m.Message):
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
		case turn := <-c.messages:
			c.add(turn)
		case <-ticker.C:
			c.flush()
		}
	}
}

func (c *passiveCollector) drain() {
	for {
		select {
		case turn := <-c.messages:
			c.add(turn)
		default:
			return
		}
	}
}

func (c *passiveCollector) add(turn agent.Turn) {
	if turn.Author.ID == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	batch := append(c.pending[turn.Author.ID], turn)
	if len(batch) > c.cfg.MaxBatchMessages {
		batch = batch[len(batch)-c.cfg.MaxBatchMessages:]
	}
	c.pending[turn.Author.ID] = batch
}

func (c *passiveCollector) flush() {
	c.mu.Lock()
	pending := c.pending
	c.pending = make(map[string][]agent.Turn)
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

func (c *passiveCollector) process(authorID string, turns []agent.Turn) {
	if len(turns) == 0 {
		return
	}

	author := turns[len(turns)-1].Author
	if author.ID == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.cfg.ExtractTimeout)
	defer cancel()

	facts, err := c.learner.Learn(ctx, turns, author, false)
	if err != nil {
		log.Error("passive memory extraction failed", "err", err, "user", authorID)
		return
	}
	if len(facts) == 0 {
		log.Debug("passive memory found nothing", "user", authorID, "messages", len(turns))
		return
	}
	log.Info("passive memory updated", "user", authorID, "facts", len(facts), "messages", len(turns))
}
