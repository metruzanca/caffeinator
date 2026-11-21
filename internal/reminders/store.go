package reminders

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const storageVersion = 1

type storage struct {
	Version   int               `json:"version"`
	Timezones map[string]string `json:"timezones"`
	Reminders []*Reminder       `json:"reminders"`
}

// Store persists reminders in a JSON file.
type Store struct {
	path string
	mu   sync.Mutex
	data storage
}

// NewStore loads or creates a reminder store.
func NewStore(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("reminders path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("cannot create reminders directory: %w", err)
	}

	store := &Store{
		path: path,
		data: storage{
			Version:   storageVersion,
			Timezones: make(map[string]string),
		},
	}

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, store.saveLocked()
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read reminders: %w", err)
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &store.data); err != nil {
			return nil, fmt.Errorf("cannot decode reminders: %w", err)
		}
	}
	if store.data.Version == 0 {
		store.data.Version = storageVersion
	}
	if store.data.Timezones == nil {
		store.data.Timezones = make(map[string]string)
	}
	return store, nil
}

// SetTimezone stores a user's IANA timezone.
func (s *Store) SetTimezone(userID, timezone string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Timezones[userID] = timezone
	return s.saveLocked()
}

// Timezone returns a user's IANA timezone.
func (s *Store) Timezone(userID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Timezones[userID]
}

// Create adds a new reminder.
func (s *Store) Create(r *Reminder) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if r.ID == "" {
		id, err := newID()
		if err != nil {
			return err
		}
		r.ID = id
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	r.Active = true
	s.data.Reminders = append(s.data.Reminders, r)
	return s.saveLocked()
}

// Update replaces a reminder by ID.
func (s *Store) Update(r *Reminder) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, existing := range s.data.Reminders {
		if existing.ID == r.ID {
			s.data.Reminders[i] = r
			return s.saveLocked()
		}
	}
	return errors.New("reminder not found")
}

// Delete removes a reminder by ID.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, existing := range s.data.Reminders {
		if existing.ID == id {
			s.data.Reminders = append(s.data.Reminders[:i], s.data.Reminders[i+1:]...)
			return s.saveLocked()
		}
	}
	return errors.New("reminder not found")
}

// Get returns a reminder by ID.
func (s *Store) Get(id string) (*Reminder, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, reminder := range s.data.Reminders {
		if reminder.ID == id {
			return reminder, true
		}
	}
	return nil, false
}

// FindByIDPrefix finds a reminder by an ID prefix.
func (s *Store) FindByIDPrefix(prefix string) (*Reminder, bool) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, reminder := range s.data.Reminders {
		if strings.HasPrefix(reminder.ID, prefix) {
			return reminder, true
		}
	}
	return nil, false
}

// FindByMessageID finds a reminder by its announcement or last trigger message.
func (s *Store) FindByMessageID(messageID string) (*Reminder, bool) {
	if messageID == "" {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, reminder := range s.data.Reminders {
		if reminder.AnnouncementMessageID == messageID || reminder.LastTriggerMessageID == messageID {
			return reminder, true
		}
	}
	return nil, false
}

// CountCreatedBy returns the number of active reminders created by a user.
func (s *Store) CountCreatedBy(userID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for _, reminder := range s.data.Reminders {
		if reminder.Active && reminder.CreatorID == userID {
			count++
		}
	}
	return count
}

// ListForUser returns private reminders involving the user and all active events.
func (s *Store) ListForUser(userID string) []*Reminder {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]*Reminder, 0)
	for _, reminder := range s.data.Reminders {
		if !reminder.Active {
			continue
		}
		if reminder.Kind == KindEvent || reminder.HasParticipant(userID) {
			result = append(result, reminder)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].NextRunAt.Before(result[j].NextRunAt)
	})
	return result
}

// Due returns active reminders whose next run is at or before now.
func (s *Store) Due(now time.Time) []*Reminder {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]*Reminder, 0)
	for _, reminder := range s.data.Reminders {
		if reminder.Active && !reminder.NextRunAt.After(now) {
			result = append(result, reminder)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].NextRunAt.Before(result[j].NextRunAt)
	})
	return result
}

// AddParticipant adds a user to a reminder.
func (s *Store) AddParticipant(id, userID string) (*Reminder, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	reminder := s.findLocked(id)
	if reminder == nil {
		return nil, false, errors.New("reminder not found")
	}
	if reminder.HasParticipant(userID) {
		return reminder, false, nil
	}
	reminder.Participants = append(reminder.Participants, userID)
	return reminder, true, s.saveLocked()
}

// RemoveParticipant removes a user from a reminder.
func (s *Store) RemoveParticipant(id, userID string) (*Reminder, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	reminder := s.findLocked(id)
	if reminder == nil {
		return nil, false, errors.New("reminder not found")
	}
	if reminder.CreatorID == userID {
		return reminder, false, errors.New("creator cannot leave their own reminder")
	}

	for i, participant := range reminder.Participants {
		if participant == userID {
			reminder.Participants = append(reminder.Participants[:i], reminder.Participants[i+1:]...)
			return reminder, true, s.saveLocked()
		}
	}
	return reminder, false, nil
}

func (s *Store) findLocked(id string) *Reminder {
	for _, reminder := range s.data.Reminders {
		if reminder.ID == id {
			return reminder
		}
	}
	return nil
}

func (s *Store) saveLocked() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot encode reminders: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".reminders-*")
	if err != nil {
		return fmt.Errorf("cannot create temporary reminders file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return fmt.Errorf("cannot set reminders permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("cannot write reminders: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("cannot close reminders file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("cannot replace reminders file: %w", err)
	}
	return nil
}

func newID() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("cannot create reminder ID: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
