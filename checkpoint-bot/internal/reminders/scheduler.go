package reminders

import (
	"context"
	"time"

	"github.com/charmbracelet/log"
)

// TriggerFunc sends one reminder.
type TriggerFunc func(ctx context.Context, reminder *Reminder) error

// Scheduler fires due reminders.
type Scheduler struct {
	store       *Store
	trigger     TriggerFunc
	interval    time.Duration
	grace       time.Duration
	retryDelay  time.Duration
	maxAttempts int
	stop        chan struct{}
	done        chan struct{}
}

// NewScheduler creates a reminder scheduler.
func NewScheduler(store *Store, trigger TriggerFunc, interval, grace, retryDelay time.Duration, maxAttempts int) *Scheduler {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	if grace <= 0 {
		grace = 60 * time.Minute
	}
	if retryDelay <= 0 {
		retryDelay = 2 * time.Minute
	}
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	return &Scheduler{
		store:       store,
		trigger:     trigger,
		interval:    interval,
		grace:       grace,
		retryDelay:  retryDelay,
		maxAttempts: maxAttempts,
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
	}
}

// Start begins the scheduler loop.
func (s *Scheduler) Start() {
	go s.run()
}

// Stop stops the scheduler loop.
func (s *Scheduler) Stop() {
	close(s.stop)
	<-s.done
}

func (s *Scheduler) run() {
	defer close(s.done)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.tick(time.Now())
	for {
		select {
		case <-s.stop:
			return
		case now := <-ticker.C:
			s.tick(now)
		}
	}
}

func (s *Scheduler) tick(now time.Time) {
	for _, reminder := range s.store.Due(now) {
		if now.Sub(reminder.NextRunAt) > s.grace {
			log.Warn("skipping missed reminder",
				"id", reminder.ID,
				"next_run_at", reminder.NextRunAt,
				"late_by", now.Sub(reminder.NextRunAt),
			)
			s.advance(reminder, now)
			continue
		}

		if err := s.trigger(context.Background(), reminder); err != nil {
			log.Error("cannot trigger reminder", "err", err, "id", reminder.ID, "failures", reminder.Failures+1)
			s.handleFailure(reminder, now)
			continue
		}
		reminder.Failures = 0
		s.advance(reminder, now)
	}
}

func (s *Scheduler) handleFailure(reminder *Reminder, now time.Time) {
	reminder.Failures++
	if reminder.Failures >= s.maxAttempts {
		log.Error("reminder retry limit reached",
			"id", reminder.ID,
			"failures", reminder.Failures,
			"recurring", reminder.Recurrence != nil,
		)
		reminder.Failures = 0
		if reminder.Recurrence != nil {
			s.advance(reminder, now)
			return
		}
		reminder.Active = false
		if err := s.store.Update(reminder); err != nil {
			log.Error("cannot deactivate reminder after failures", "err", err, "id", reminder.ID)
		}
		return
	}

	reminder.NextRunAt = now.Add(s.retryDelay).UTC()
	if err := s.store.Update(reminder); err != nil {
		log.Error("cannot schedule reminder retry", "err", err, "id", reminder.ID)
	}
}

func (s *Scheduler) advance(reminder *Reminder, now time.Time) {
	if reminder.Recurrence == nil {
		reminder.Active = false
	} else {
		loc, err := time.LoadLocation(reminder.Timezone)
		if err != nil {
			log.Error("cannot load reminder timezone", "err", err, "id", reminder.ID, "timezone", reminder.Timezone)
			loc = time.UTC
		}
		if next, ok := NextRun(now, reminder.StartAt, reminder.Recurrence, loc); ok {
			reminder.NextRunAt = next
		} else {
			reminder.Active = false
		}
	}

	if err := s.store.Update(reminder); err != nil {
		log.Error("cannot update reminder after trigger", "err", err, "id", reminder.ID)
	}
}
