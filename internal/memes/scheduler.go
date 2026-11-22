package memes

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/robfig/cron/v3"
)

const defaultRefreshTimeout = 5 * time.Minute

// Scheduler runs meme refreshes on a cron expression.
type Scheduler struct {
	runner   *Runner
	expr     string
	location *time.Location
	timeout  time.Duration

	mu       sync.Mutex
	cron     *cron.Cron
	schedule cron.Schedule
	started  bool
}

// NewScheduler creates a cron scheduler. An empty expression creates a
// scheduler that only supports manual refreshes.
func NewScheduler(runner *Runner, expression, timezone string, timeout time.Duration) (*Scheduler, error) {
	expression = strings.TrimSpace(expression)
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		timezone = "UTC"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, errors.New("invalid meme timezone: " + timezone)
	}
	if timeout <= 0 {
		timeout = defaultRefreshTimeout
	}

	scheduler := &Scheduler{
		runner:   runner,
		expr:     expression,
		location: location,
		timeout:  timeout,
	}
	if expression == "" {
		return scheduler, nil
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(expression)
	if err != nil {
		return nil, errors.New("invalid meme cron expression: " + expression)
	}
	scheduler.schedule = schedule

	jobCron := cron.New(
		cron.WithLocation(location),
		cron.WithChain(cron.SkipIfStillRunning(cronLog{})),
	)
	jobCron.Schedule(schedule, cron.FuncJob(scheduler.run))
	scheduler.cron = jobCron
	return scheduler, nil
}

// Expression returns the configured cron expression.
func (s *Scheduler) Expression() string {
	if s == nil {
		return ""
	}
	return s.expr
}

// Timezone returns the configured timezone name.
func (s *Scheduler) Timezone() string {
	if s == nil || s.location == nil {
		return ""
	}
	return s.location.String()
}

// Next returns the next scheduled refresh.
func (s *Scheduler) Next() time.Time {
	if s == nil || s.schedule == nil {
		return time.Time{}
	}
	return s.schedule.Next(time.Now().In(s.location))
}

// Start begins the scheduler.
func (s *Scheduler) Start() {
	if s == nil || s.cron == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return
	}
	s.started = true
	s.cron.Start()
}

// Stop stops the scheduler and waits for a running refresh to finish.
func (s *Scheduler) Stop() {
	if s == nil || s.cron == nil {
		return
	}
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return
	}
	s.started = false
	stopped := s.cron.Stop()
	s.mu.Unlock()

	select {
	case <-stopped.Done():
	case <-time.After(5 * time.Second):
		log.Warn("meme scheduler did not stop in time")
	}
}

func (s *Scheduler) run() {
	if s.runner == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	result, err := s.runner.Refresh(ctx)
	if err != nil {
		log.Error("scheduled meme refresh failed", "err", err)
		return
	}
	log.Info("scheduled meme refresh finished",
		"added", result.Added,
		"failed", result.Failed,
		"pruned", result.Pruned,
	)
}

type cronLog struct{}

func (cronLog) Info(msg string, _ ...any) {
	log.Debug("meme cron", "message", msg)
}

func (cronLog) Error(err error, msg string, _ ...any) {
	log.Error("meme cron", "err", err, "message", msg)
}
