package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultMemeLimit       = 3
	memeRelevanceThreshold = 3
	memeShareHistory       = 5
	memeContextLineChars   = 400
)

var memeMarkerPattern = regexp.MustCompile(`\[meme:([^\]\s]+)\]`)

// MemeShare records one Discord message that carried a meme image.
type MemeShare struct {
	MessageID string    `json:"message_id"`
	ChannelID string    `json:"channel_id"`
	SharedAt  time.Time `json:"shared_at"`
}

// Meme is one understood meme kept in memory.
type Meme struct {
	ID             string      `json:"id"`
	Subreddit      string      `json:"subreddit,omitempty"`
	Title          string      `json:"title,omitempty"`
	Caption        string      `json:"caption,omitempty"`
	Context        string      `json:"context,omitempty"`
	Tags           []string    `json:"tags,omitempty"`
	UseWhen        []string    `json:"use_when,omitempty"`
	Score          int         `json:"score,omitempty"`
	Link           string      `json:"link,omitempty"`
	ImageFile      string      `json:"image_file,omitempty"`
	AddedAt        time.Time   `json:"added_at"`
	LastUsed       time.Time   `json:"last_used,omitempty"`
	Uses           int         `json:"uses,omitempty"`
	Reactions      int         `json:"reactions,omitempty"`
	ReactorIDs     []string    `json:"reactor_ids,omitempty"`
	LastReactionAt time.Time   `json:"last_reaction_at,omitempty"`
	Shares         []MemeShare `json:"shares,omitempty"`
}

// MemeStatus summarizes stored meme memory.
type MemeStatus struct {
	LastRun   time.Time
	LastError string
	Stored    int
	Reactions int
}

type memeDocument struct {
	LastRun   time.Time `json:"last_run,omitempty"`
	LastError string    `json:"last_error,omitempty"`
	Memes     []Meme    `json:"memes"`
}

// MemeStore keeps meme memory in one JSON file.
type MemeStore struct {
	path string
	now  func() time.Time

	mu  sync.Mutex
	doc memeDocument
}

// NewMemeStore loads meme memory from path, creating the parent directory.
func NewMemeStore(path string) (*MemeStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("meme memory path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("cannot create meme memory directory: %w", err)
	}

	store := &MemeStore{path: path, now: time.Now}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if len(bytes.TrimSpace(data)) > 0 {
			if err := json.Unmarshal(data, &store.doc); err != nil {
				return nil, fmt.Errorf("cannot read meme memory: %w", err)
			}
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return nil, fmt.Errorf("cannot read meme memory: %w", err)
	}

	store.cleanLocked()
	return store, nil
}

// Path returns the backing file path.
func (s *MemeStore) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// Status returns a summary of the stored memes.
func (s *MemeStore) Status() MemeStatus {
	if s == nil {
		return MemeStatus{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	status := MemeStatus{
		LastRun:   s.doc.LastRun,
		LastError: s.doc.LastError,
		Stored:    len(s.doc.Memes),
	}
	for _, meme := range s.doc.Memes {
		status.Reactions += meme.Reactions
	}
	return status
}

// Add stores one meme. It reports whether the meme was new.
func (s *MemeStore) Add(meme Meme) (bool, error) {
	added, err := s.AddAll([]Meme{meme})
	return added == 1, err
}

// AddAll stores several memes in one write. Known IDs are skipped.
func (s *MemeStore) AddAll(memes []Meme) (int, error) {
	if s == nil {
		return 0, errors.New("meme memory is disabled")
	}
	if len(memes) == 0 {
		return 0, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	known := make(map[string]bool, len(s.doc.Memes))
	for _, meme := range s.doc.Memes {
		known[meme.ID] = true
	}

	previous := s.doc.Memes
	added := 0
	for _, meme := range memes {
		meme = s.cleanMeme(meme)
		if meme.ID == "" || known[meme.ID] {
			continue
		}
		known[meme.ID] = true
		s.doc.Memes = append(s.doc.Memes, meme)
		added++
	}
	if added == 0 {
		return 0, nil
	}
	if err := s.writeLocked(); err != nil {
		s.doc.Memes = previous
		return 0, err
	}
	return added, nil
}

// Contains reports whether a meme ID is stored.
func (s *MemeStore) Contains(id string) bool {
	if s == nil {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, meme := range s.doc.Memes {
		if meme.ID == id {
			return true
		}
	}
	return false
}

// List returns a copy of the stored memes in insertion order.
func (s *MemeStore) List() []Meme {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]Meme(nil), s.doc.Memes...)
}

// ListNewestFirst returns a copy of the stored memes ordered newest first.
func (s *MemeStore) ListNewestFirst() []Meme {
	memes := s.List()
	sort.SliceStable(memes, func(i, j int) bool {
		return memes[i].AddedAt.After(memes[j].AddedAt)
	})
	return memes
}

// RecordRun stores the outcome of the latest scrape.
func (s *MemeStore) RecordRun(errText string) error {
	if s == nil {
		return errors.New("meme memory is disabled")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.doc.LastRun = s.now().UTC()
	s.doc.LastError = strings.TrimSpace(errText)
	return s.writeLocked()
}

// MarkUsed increments the use count for the given meme IDs.
func (s *MemeStore) MarkUsed(ids []string) error {
	if s == nil || len(ids) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}

	now := s.now().UTC()
	changed := false
	for i := range s.doc.Memes {
		if !wanted[s.doc.Memes[i].ID] {
			continue
		}
		s.doc.Memes[i].Uses++
		s.doc.Memes[i].LastUsed = now
		changed = true
	}
	if !changed {
		return nil
	}
	return s.writeLocked()
}

// MarkShared records the Discord message that carried a meme image.
func (s *MemeStore) MarkShared(id, channelID, messageID string) error {
	if s == nil || strings.TrimSpace(messageID) == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.doc.Memes {
		meme := &s.doc.Memes[i]
		if meme.ID != id {
			continue
		}
		for _, share := range meme.Shares {
			if share.MessageID == messageID {
				return nil
			}
		}
		meme.Shares = append(meme.Shares, MemeShare{
			MessageID: messageID,
			ChannelID: channelID,
			SharedAt:  s.now().UTC(),
		})
		if len(meme.Shares) > memeShareHistory {
			meme.Shares = meme.Shares[len(meme.Shares)-memeShareHistory:]
		}
		return s.writeLocked()
	}
	return nil
}

// FindByMessage returns the meme attached to a Discord message.
func (s *MemeStore) FindByMessage(messageID string) (Meme, bool) {
	if s == nil || strings.TrimSpace(messageID) == "" {
		return Meme{}, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, meme := range s.doc.Memes {
		for _, share := range meme.Shares {
			if share.MessageID == messageID {
				return meme, true
			}
		}
	}
	return Meme{}, false
}

// AddReaction records one unique positive reaction. It returns the updated
// meme and whether the reaction was new.
func (s *MemeStore) AddReaction(id, userID string) (Meme, bool, error) {
	if s == nil || strings.TrimSpace(userID) == "" {
		return Meme{}, false, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.doc.Memes {
		meme := &s.doc.Memes[i]
		if meme.ID != id {
			continue
		}
		for _, reactor := range meme.ReactorIDs {
			if reactor == userID {
				return *meme, false, nil
			}
		}
		meme.ReactorIDs = append(meme.ReactorIDs, userID)
		meme.Reactions = len(meme.ReactorIDs)
		meme.LastReactionAt = s.now().UTC()
		if err := s.writeLocked(); err != nil {
			return *meme, false, err
		}
		return *meme, true, nil
	}
	return Meme{}, false, nil
}

// Prune removes unused old memes and enforces the stored cap. Reacted or used
// memes survive age pruning and are evicted last when the cap is reached.
func (s *MemeStore) Prune(retentionDays, maxStored int) ([]Meme, error) {
	if s == nil {
		return nil, errors.New("meme memory is disabled")
	}
	if retentionDays < 0 {
		retentionDays = 0
	}
	if maxStored < 0 {
		maxStored = 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := s.now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	kept := make([]Meme, 0, len(s.doc.Memes))
	var removed []Meme
	for _, meme := range s.doc.Memes {
		if retentionDays > 0 && meme.Reactions == 0 && meme.Uses == 0 &&
			!meme.AddedAt.IsZero() && meme.AddedAt.Before(cutoff) {
			removed = append(removed, meme)
			continue
		}
		kept = append(kept, meme)
	}

	if maxStored > 0 && len(kept) > maxStored {
		sort.SliceStable(kept, func(i, j int) bool {
			left := kept[i].Reactions*100 + kept[i].Uses*3
			right := kept[j].Reactions*100 + kept[j].Uses*3
			if left != right {
				return left < right
			}
			return kept[i].AddedAt.Before(kept[j].AddedAt)
		})
		for len(kept) > maxStored {
			removed = append(removed, kept[0])
			kept = kept[1:]
		}
		sort.SliceStable(kept, func(i, j int) bool {
			return kept[i].AddedAt.After(kept[j].AddedAt)
		})
	}

	if len(removed) == 0 {
		return nil, nil
	}

	previous := s.doc.Memes
	s.doc.Memes = kept
	if err := s.writeLocked(); err != nil {
		s.doc.Memes = previous
		return nil, err
	}
	return removed, nil
}

// Relevant returns memes that match the query, newest and most reacted first.
func (s *MemeStore) Relevant(query string, limit int) []Meme {
	if s == nil {
		return nil
	}
	if limit <= 0 {
		limit = defaultMemeLimit
	}
	tokens := uniqueMemeTokens(query)
	if len(tokens) == 0 {
		return nil
	}
	wanted := make(map[string]bool, len(tokens))
	for _, token := range tokens {
		wanted[token] = true
	}

	s.mu.Lock()
	memes := append([]Meme(nil), s.doc.Memes...)
	s.mu.Unlock()

	type scoredMeme struct {
		meme  Meme
		score int
	}
	now := s.now().UTC()
	scored := make([]scoredMeme, 0, len(memes))
	for _, meme := range memes {
		score := memeMatchScore(meme, wanted, now)
		if score < memeRelevanceThreshold {
			continue
		}
		scored = append(scored, scoredMeme{meme: meme, score: score})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].meme.AddedAt.After(scored[j].meme.AddedAt)
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}

	result := make([]Meme, len(scored))
	for i, item := range scored {
		result[i] = item.meme
	}
	return result
}

// ExtractMemeMarkers removes [meme:<id>] markers from text. Only IDs in the
// selected set are returned, and at most one meme is used per reply.
func ExtractMemeMarkers(text string, selected []Meme) (string, []Meme) {
	allowed := make(map[string]Meme, len(selected))
	for _, meme := range selected {
		allowed[meme.ID] = meme
	}

	var used []Meme
	cleaned := memeMarkerPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := memeMarkerPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return ""
		}
		meme, ok := allowed[parts[1]]
		if !ok || len(used) > 0 {
			return ""
		}
		used = append(used, meme)
		return ""
	})

	return strings.TrimSpace(cleaned), used
}

// MemeQueryText builds a compact retrieval query from a prompt and history.
func MemeQueryText(prompt string, history []Turn) string {
	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(prompt))

	limit := len(history)
	if limit > 2 {
		limit = 2
	}
	if limit > 0 {
		builder.WriteString(" ")
	}
	for _, turn := range history[len(history)-limit:] {
		text := strings.TrimSpace(turn.Content)
		if text == "" {
			continue
		}
		builder.WriteString(truncateRunes(text, 240))
		builder.WriteString(" ")
	}
	return strings.TrimSpace(builder.String())
}

func (s *MemeStore) cleanLocked() {
	if s.doc.Memes == nil {
		s.doc.Memes = []Meme{}
	}
	for i := range s.doc.Memes {
		s.doc.Memes[i] = s.cleanMeme(s.doc.Memes[i])
	}
}

func (s *MemeStore) cleanMeme(meme Meme) Meme {
	meme.ID = strings.TrimSpace(meme.ID)
	meme.Subreddit = strings.TrimSpace(meme.Subreddit)
	meme.Title = truncateRunes(strings.TrimSpace(sanitizeMemeText(meme.Title)), 200)
	meme.Caption = truncateRunes(strings.TrimSpace(sanitizeMemeText(meme.Caption)), 240)
	meme.Context = truncateRunes(strings.TrimSpace(sanitizeMemeText(meme.Context)), 480)
	meme.Link = strings.TrimSpace(meme.Link)
	if name := strings.TrimSpace(meme.ImageFile); name != "" {
		meme.ImageFile = filepath.Base(name)
	} else {
		meme.ImageFile = ""
	}
	meme.Tags = cleanMemeList(meme.Tags, 8, 32)
	meme.UseWhen = cleanMemeList(meme.UseWhen, 4, 80)
	if meme.AddedAt.IsZero() {
		meme.AddedAt = s.now().UTC()
	} else {
		meme.AddedAt = meme.AddedAt.UTC()
	}
	if meme.LastUsed.IsZero() {
		meme.LastUsed = time.Time{}
	} else {
		meme.LastUsed = meme.LastUsed.UTC()
	}
	if meme.LastReactionAt.IsZero() {
		meme.LastReactionAt = time.Time{}
	} else {
		meme.LastReactionAt = meme.LastReactionAt.UTC()
	}
	if len(meme.ReactorIDs) > 0 {
		meme.Reactions = len(meme.ReactorIDs)
	}
	if len(meme.Shares) > memeShareHistory {
		meme.Shares = meme.Shares[len(meme.Shares)-memeShareHistory:]
	}
	return meme
}

func (s *MemeStore) writeLocked() error {
	if len(s.doc.Memes) == 0 {
		s.doc.Memes = []Meme{}
	}
	data, err := json.MarshalIndent(s.doc, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot encode meme memory: %w", err)
	}
	data = append(data, '\n')
	return writeAtomic(s.path, data)
}

func memeMatchScore(meme Meme, wanted map[string]bool, now time.Time) int {
	score := 0

	for _, tag := range meme.Tags {
		for _, token := range uniqueMemeTokens(tag) {
			if wanted[token] {
				score += 4
			}
		}
	}

	for _, phrase := range meme.UseWhen {
		matches := 0
		for _, token := range uniqueMemeTokens(phrase) {
			if wanted[token] {
				matches++
			}
		}
		if matches >= 2 {
			score += 5
		} else if matches == 1 {
			score++
		}
	}

	textMatches := 0
	for _, text := range []string{meme.Title, meme.Caption, meme.Context} {
		for _, token := range uniqueMemeTokens(text) {
			if wanted[token] {
				textMatches++
			}
		}
	}
	if textMatches > 6 {
		textMatches = 6
	}
	score += textMatches

	age := now.Sub(meme.AddedAt)
	switch {
	case age < 7*24*time.Hour:
		score += 2
	case age < 14*24*time.Hour:
		score++
	}

	score += minInt(meme.Reactions, 3)
	if !meme.LastUsed.IsZero() && now.Sub(meme.LastUsed) < 24*time.Hour {
		score -= 3
	}
	return score
}

func cleanMemeList(values []string, maxItems, maxChars int) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(sanitizeMemeText(value)))
		if value == "" || seen[value] || len(result) >= maxItems {
			continue
		}
		seen[value] = true
		result = append(result, truncateRunes(value, maxChars))
	}
	return result
}

func sanitizeMemeText(text string) string {
	text = strings.ReplaceAll(text, "\u2014", ", ")
	text = strings.ReplaceAll(text, "\u2013", ", ")
	return strings.Join(strings.Fields(text), " ")
}

func uniqueMemeTokens(text string) []string {
	seen := make(map[string]bool)
	var tokens []string
	for _, token := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '+' && r != '#'
	}) {
		if len(token) < 2 || seen[token] || memeStopwords[token] {
			continue
		}
		seen[token] = true
		tokens = append(tokens, token)
	}
	return tokens
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

var memeStopwords = map[string]bool{
	"a": true, "about": true, "after": true, "again": true, "all": true,
	"also": true, "am": true, "an": true, "and": true, "any": true,
	"are": true, "as": true, "at": true, "be": true, "because": true,
	"been": true, "before": true, "being": true, "both": true, "but": true,
	"by": true, "can": true, "could": true, "did": true, "do": true,
	"does": true, "doing": true, "down": true, "each": true, "few": true,
	"for": true, "from": true, "get": true, "got": true, "had": true,
	"has": true, "have": true, "he": true, "her": true, "here": true,
	"him": true, "his": true, "how": true, "i": true, "if": true,
	"in": true, "into": true, "is": true, "it": true, "its": true,
	"just": true, "like": true, "me": true, "more": true, "most": true,
	"my": true, "no": true, "not": true, "now": true, "of": true,
	"off": true, "on": true, "one": true, "only": true, "or": true,
	"other": true, "our": true, "out": true, "over": true, "own": true,
	"same": true, "she": true, "should": true, "so": true, "some": true,
	"such": true, "than": true, "that": true, "the": true, "their": true,
	"them": true, "then": true, "there": true, "these": true, "they": true,
	"this": true, "those": true, "through": true, "to": true, "too": true,
	"under": true, "up": true, "us": true, "very": true, "was": true,
	"we": true, "were": true, "what": true, "when": true, "where": true,
	"which": true, "who": true, "why": true, "will": true, "with": true,
	"would": true, "you": true, "your": true,
}
