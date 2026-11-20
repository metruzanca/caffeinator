// Package memory stores and retrieves simple markdown memory.
package memory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const defaultMaxFactChars = 300

// Fact is a durable fact about a user or the server.
type Fact struct {
	SubjectID   string
	SubjectName string
	Text        string
	Category    string
	Confidence  float64
}

// Store keeps markdown memory files on disk.
type Store struct {
	dir             string
	maxContextChars int
	maxFactChars    int
	now             func() time.Time
	mu              sync.Mutex
}

// NewStore creates a markdown memory store in dir.
func NewStore(dir string, maxContextChars int) (*Store, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("memory directory is empty")
	}
	if maxContextChars <= 0 {
		maxContextChars = 3000
	}
	if err := os.MkdirAll(filepath.Join(dir, "users"), 0700); err != nil {
		return nil, fmt.Errorf("cannot create memory directory: %w", err)
	}

	return &Store{
		dir:             dir,
		maxContextChars: maxContextChars,
		maxFactChars:    defaultMaxFactChars,
		now:             time.Now,
	}, nil
}

// Dir returns the memory directory.
func (s *Store) Dir() string {
	return s.dir
}

// ForSubjects returns a combined memory block for the server and the given
// subject IDs. It always includes server memory.
func (s *Store) ForSubjects(subjects map[string]string) string {
	if s == nil {
		return ""
	}

	parts := make([]string, 0, len(subjects)+1)
	if text := s.read("server"); text != "" {
		parts = append(parts, "Server memory:\n"+text)
	}

	ids := make([]string, 0, len(subjects))
	for id := range subjects {
		if id == "server" {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		text := s.read(id)
		if text == "" {
			continue
		}
		name := strings.TrimSpace(subjects[id])
		if name == "" || name == id {
			parts = append(parts, fmt.Sprintf("Memory for user %s:\n%s", id, text))
			continue
		}
		parts = append(parts, fmt.Sprintf("Memory for %s (%s):\n%s", name, id, text))
	}

	combined := strings.TrimSpace(strings.Join(parts, "\n\n"))
	return tailRunes(combined, s.maxContextChars)
}

// Get returns the raw markdown for a subject.
func (s *Store) Get(subjectID string) (string, error) {
	if s == nil {
		return "", errors.New("memory is disabled")
	}
	path, err := s.subjectPath(subjectID)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("cannot read memory: %w", err)
	}
	return string(data), nil
}

// Forget deletes the markdown file for a subject.
func (s *Store) Forget(subjectID string) error {
	if s == nil {
		return errors.New("memory is disabled")
	}
	path, err := s.subjectPath(subjectID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cannot delete memory: %w", err)
	}
	return nil
}

// ListUsers returns user IDs that have memory files.
func (s *Store) ListUsers() ([]string, error) {
	if s == nil {
		return nil, errors.New("memory is disabled")
	}
	entries, err := os.ReadDir(filepath.Join(s.dir, "users"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot list memory: %w", err)
	}

	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(name, ".md"))
	}
	sort.Strings(ids)
	return ids, nil
}

// AppendFacts adds facts to the matching markdown files.
func (s *Store) AppendFacts(facts []Fact) error {
	if s == nil {
		return errors.New("memory is disabled")
	}
	if len(facts) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	grouped := make(map[string][]Fact)
	for _, fact := range facts {
		if strings.TrimSpace(fact.SubjectID) == "" {
			continue
		}
		grouped[fact.SubjectID] = append(grouped[fact.SubjectID], fact)
	}

	for subjectID, subjectFacts := range grouped {
		if err := s.appendFactsToSubject(subjectID, subjectFacts); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) appendFactsToSubject(subjectID string, facts []Fact) error {
	path, err := s.subjectPath(subjectID)
	if err != nil {
		return err
	}

	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cannot read memory: %w", err)
	}
	existingText := string(existing)
	normalizedExisting := normalize(existingText)

	var builder strings.Builder
	builder.WriteString(existingText)
	if strings.TrimSpace(existingText) == "" {
		if subjectID == "server" {
			builder.WriteString("# Server\n\n")
		} else {
			name := ""
			for _, fact := range facts {
				if strings.TrimSpace(fact.SubjectName) != "" {
					name = strings.TrimSpace(fact.SubjectName)
					break
				}
			}
			if name != "" {
				fmt.Fprintf(&builder, "# %s (%s)\n\n", name, subjectID)
			} else {
				fmt.Fprintf(&builder, "# User %s\n\n", subjectID)
			}
		}
	} else if !strings.HasSuffix(existingText, "\n") {
		builder.WriteString("\n")
	}

	added := 0
	date := s.now().UTC().Format("2006-01-02")
	for _, fact := range facts {
		text := cleanFact(fact.Text, s.maxFactChars)
		if text == "" {
			continue
		}
		normalized := normalize(text)
		if normalized == "" || strings.Contains(normalizedExisting, normalized) {
			continue
		}
		category := cleanCategory(fact.Category)
		fmt.Fprintf(&builder, "- %s [%s]: %s\n", date, category, text)
		normalizedExisting += " " + normalized
		added++
	}

	if added == 0 {
		return nil
	}
	return writeAtomic(path, []byte(builder.String()))
}

func (s *Store) read(subjectID string) string {
	path, err := s.subjectPath(subjectID)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (s *Store) subjectPath(subjectID string) (string, error) {
	subjectID = strings.TrimSpace(subjectID)
	if subjectID == "server" {
		return filepath.Join(s.dir, "server.md"), nil
	}
	if !isDigits(subjectID) {
		return "", fmt.Errorf("invalid memory subject: %q", subjectID)
	}
	return filepath.Join(s.dir, "users", subjectID+".md"), nil
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".memory-*")
	if err != nil {
		return fmt.Errorf("cannot create temporary memory file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return fmt.Errorf("cannot set memory permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("cannot write memory: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("cannot close memory file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("cannot replace memory file: %w", err)
	}
	return nil
}

func cleanFact(text string, max int) string {
	text = strings.Join(strings.Fields(text), " ")
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return truncateRunes(text, max)
}

func cleanCategory(category string) string {
	category = strings.ToLower(strings.TrimSpace(category))
	switch category {
	case "identity", "work", "preference", "project", "server", "other":
		return category
	default:
		return "other"
	}
}

func normalize(text string) string {
	return strings.Join(strings.Fields(strings.ToLower(text)), " ")
}

func isDigits(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func tailRunes(text string, max int) string {
	runes := []rune(strings.TrimSpace(text))
	if max <= 0 || len(runes) <= max {
		return string(runes)
	}
	return string(runes[len(runes)-max:])
}

func truncateRunes(text string, max int) string {
	runes := []rune(text)
	if max <= 0 || len(runes) <= max {
		return text
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}
