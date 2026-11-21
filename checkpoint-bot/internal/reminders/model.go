// Package reminders stores and schedules Discord reminders and events.
package reminders

import "time"

// Kind distinguishes a private reminder from a public event.
type Kind string

const (
	KindReminder Kind = "reminder"
	KindEvent    Kind = "event"
)

// Recurrence describes a simple repeating schedule.
type Recurrence struct {
	Freq     string `json:"freq"` // daily, weekly, monthly
	Interval int    `json:"interval"`
	Weekdays []int  `json:"weekdays,omitempty"` // time.Weekday values, 0 is Sunday
	MonthDay int    `json:"month_day,omitempty"`
}

// Reminder is a scheduled reminder or event.
type Reminder struct {
	ID                    string      `json:"id"`
	Kind                  Kind        `json:"kind"`
	CreatorID             string      `json:"creator_id"`
	GuildID               string      `json:"guild_id"`
	ChannelID             string      `json:"channel_id"`
	AnnouncementMessageID string      `json:"announcement_message_id,omitempty"`
	LastTriggerMessageID  string      `json:"last_trigger_message_id,omitempty"`
	Text                  string      `json:"text,omitempty"`
	Timezone              string      `json:"timezone"`
	StartAt               time.Time   `json:"start_at"`
	NextRunAt             time.Time   `json:"next_run_at"`
	Recurrence            *Recurrence `json:"recurrence,omitempty"`
	Participants          []string    `json:"participants,omitempty"`
	Active                bool        `json:"active"`
	CreatedAt             time.Time   `json:"created_at"`
}

// HasParticipant reports whether a user is the creator or a participant.
func (r *Reminder) HasParticipant(userID string) bool {
	if r.CreatorID == userID {
		return true
	}
	for _, participant := range r.Participants {
		if participant == userID {
			return true
		}
	}
	return false
}

// MentionIDs returns the creator and participants without duplicates.
func (r *Reminder) MentionIDs() []string {
	seen := make(map[string]bool, len(r.Participants)+1)
	ids := make([]string, 0, len(r.Participants)+1)
	for _, id := range append([]string{r.CreatorID}, r.Participants...) {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}
