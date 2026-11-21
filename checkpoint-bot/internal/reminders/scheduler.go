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
	store    *Store
	trigger  TriggerFunc
	interval time.Duration
	grace    time.Duration
	stop     chan struct{}
	done     chan struct{}
}

// NewScheduler creates a reminder scheduler.
func NewScheduler(store *Store, trigger TriggerFunc, interval, grace time.Duration) *Scheduler {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	if grace <= 0 {
		grace = 60 * time.Minute
	}
	return &Scheduler{
		store:    store,
		trigger:  trigger,
		interval: interval,
		grace:    grace,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
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
			log.Error("cannot trigger reminder", "err", err, "id", reminder.ID)
		}
		s.advance(reminder, now)
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
