package reminders

import (
	"errors"
	"strings"
	"time"
)

// ValidateRecurrence checks a recurrence rule.
func ValidateRecurrence(rec *Recurrence) error {
	if rec == nil {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(rec.Freq)) {
	case "daily", "weekly", "monthly":
	default:
		return errors.New("recurrence frequency must be daily, weekly, or monthly")
	}
	if rec.Interval < 1 {
		return errors.New("recurrence interval must be at least 1")
	}
	for _, weekday := range rec.Weekdays {
		if weekday < 0 || weekday > 6 {
			return errors.New("recurrence weekdays must be between 0 and 6")
		}
	}
	if rec.MonthDay < 0 || rec.MonthDay > 31 {
		return errors.New("recurrence month day must be between 1 and 31")
	}
	return nil
}

// NextRun returns the next occurrence at or after now.
func NextRun(now, start time.Time, rec *Recurrence, loc *time.Location) (time.Time, bool) {
	if loc == nil {
		loc = start.Location()
	}
	if loc == nil {
		loc = time.UTC
	}

	if rec == nil {
		if start.After(now) {
			return start.UTC(), true
		}
		return time.Time{}, false
	}

	startLocal := start.In(loc)
	nowLocal := now.In(loc)
	cursor := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)

	for i := 0; i < 366*8; i++ {
		candidate := time.Date(
			cursor.Year(),
			cursor.Month(),
			cursor.Day(),
			startLocal.Hour(),
			startLocal.Minute(),
			0,
			0,
			loc,
		)
		if !candidate.Before(now) && matchesRecurrence(startLocal, candidate, rec) {
			return candidate.UTC(), true
		}
		cursor = cursor.AddDate(0, 0, 1)
	}

	return time.Time{}, false
}

func matchesRecurrence(start, candidate time.Time, rec *Recurrence) bool {
	if candidate.Before(start) {
		return false
	}

	interval := rec.Interval
	if interval < 1 {
		interval = 1
	}

	switch strings.ToLower(strings.TrimSpace(rec.Freq)) {
	case "daily":
		days := daysBetween(start, candidate)
		return days >= 0 && days%interval == 0
	case "weekly":
		weekdays := rec.Weekdays
		if len(weekdays) == 0 {
			weekdays = []int{int(start.Weekday())}
		}
		if !containsWeekday(weekdays, int(candidate.Weekday())) {
			return false
		}
		weeks := daysBetween(startOfWeek(start), startOfWeek(candidate)) / 7
		return weeks >= 0 && weeks%interval == 0
	case "monthly":
		monthDay := rec.MonthDay
		if monthDay == 0 {
			monthDay = start.Day()
		}
		if candidate.Day() != monthDay {
			return false
		}
		months := (candidate.Year()-start.Year())*12 + int(candidate.Month()-start.Month())
		return months >= 0 && months%interval == 0
	default:
		return false
	}
}

func daysBetween(a, b time.Time) int {
	aDate := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	bDate := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	return int(bDate.Sub(aDate).Hours() / 24)
}

func startOfWeek(t time.Time) time.Time {
	date := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return date.AddDate(0, 0, -int(t.Weekday()))
}

func containsWeekday(weekdays []int, day int) bool {
	for _, weekday := range weekdays {
		if weekday == day {
			return true
		}
	}
	return false
}
