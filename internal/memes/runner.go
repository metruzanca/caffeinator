package memes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/metruzanca/caffeinator/agent"
)

// ErrBusy means a refresh is already running.
var ErrBusy = errors.New("meme refresh is already running")

// PostSource provides subreddit top posts.
type PostSource interface {
	Top(ctx context.Context, subreddit, period string, limit int) ([]Post, error)
}

// ImageSource downloads meme images.
type ImageSource interface {
	Fetch(ctx context.Context, rawURL string) (Image, error)
}

// MemeAnalyzer describes a meme image.
type MemeAnalyzer interface {
	Analyze(ctx context.Context, post Post, image Image) (Analysis, error)
}

// Result summarizes one refresh.
type Result struct {
	Added    int
	Skipped  int
	Failed   int
	Pruned   int
	Duration time.Duration
}

// RunnerConfig configures a Runner.
type RunnerConfig struct {
	Reddit        PostSource
	Images        ImageSource
	Analyzer      MemeAnalyzer
	Store         *agent.MemeStore
	Dir           string
	Subreddits    []string
	Period        string
	ScanLimit     int
	PostsPerRun   int
	Keep          int
	RetentionDays int
}

// Runner scrapes, analyzes, and stores memes.
type Runner struct {
	reddit        PostSource
	images        ImageSource
	analyzer      MemeAnalyzer
	store         *agent.MemeStore
	dir           string
	subreddits    []string
	period        string
	scanLimit     int
	postsPerRun   int
	keep          int
	retentionDays int

	mu  sync.Mutex
	now func() time.Time
}

// NewRunner creates a meme runner.
func NewRunner(cfg RunnerConfig) *Runner {
	subreddits := make([]string, 0, len(cfg.Subreddits))
	seen := make(map[string]bool)
	for _, name := range cfg.Subreddits {
		name = normalizeSubreddit(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		subreddits = append(subreddits, name)
	}

	dir := strings.TrimSpace(cfg.Dir)
	if dir == "" {
		dir = filepath.Join("memory", "memes")
	}

	period := normalizePeriod(cfg.Period)
	scanLimit := cfg.ScanLimit
	if scanLimit <= 0 {
		scanLimit = 25
	}
	postsPerRun := cfg.PostsPerRun
	if postsPerRun <= 0 {
		postsPerRun = 5
	}
	if postsPerRun > scanLimit {
		scanLimit = postsPerRun
	}
	keep := cfg.Keep
	if keep <= 0 {
		keep = 50
	}
	retentionDays := cfg.RetentionDays
	if retentionDays <= 0 {
		retentionDays = 30
	}

	return &Runner{
		reddit:        cfg.Reddit,
		images:        cfg.Images,
		analyzer:      cfg.Analyzer,
		store:         cfg.Store,
		dir:           dir,
		subreddits:    subreddits,
		period:        period,
		scanLimit:     scanLimit,
		postsPerRun:   postsPerRun,
		keep:          keep,
		retentionDays: retentionDays,
		now:           time.Now,
	}
}

// Subreddits returns the configured subreddit names.
func (r *Runner) Subreddits() []string {
	if r == nil {
		return nil
	}
	return append([]string(nil), r.subreddits...)
}

// Period returns the configured top period.
func (r *Runner) Period() string {
	if r == nil {
		return ""
	}
	return r.period
}

// ScanLimit returns the maximum posts read per subreddit.
func (r *Runner) ScanLimit() int {
	if r == nil {
		return 0
	}
	return r.scanLimit
}

// PostsPerRun returns the maximum memes analyzed per refresh.
func (r *Runner) PostsPerRun() int {
	if r == nil {
		return 0
	}
	return r.postsPerRun
}

// Keep returns the maximum stored memes.
func (r *Runner) Keep() int {
	if r == nil {
		return 0
	}
	return r.keep
}

// RetentionDays returns the age limit for unreacted memes.
func (r *Runner) RetentionDays() int {
	if r == nil {
		return 0
	}
	return r.retentionDays
}

// Refresh scrapes the configured subreddits and stores new memes.
func (r *Runner) Refresh(ctx context.Context) (Result, error) {
	startedAt := r.now()
	if !r.mu.TryLock() {
		return Result{}, ErrBusy
	}
	defer r.mu.Unlock()

	if r.store == nil {
		return Result{}, errors.New("meme memory is disabled")
	}
	if r.reddit == nil {
		return Result{}, errors.New("reddit is not configured")
	}
	if r.images == nil || r.analyzer == nil {
		return Result{}, errors.New("meme analysis is not configured")
	}
	if len(r.subreddits) == 0 {
		return Result{}, errors.New("no meme subreddits are configured")
	}

	var result Result
	var firstErr error
	existing := make(map[string]bool)
	for _, meme := range r.store.List() {
		existing[meme.ID] = true
	}

	for _, subreddit := range r.subreddits {
		if result.Added >= r.postsPerRun || ctx.Err() != nil {
			break
		}

		posts, err := r.reddit.Top(ctx, subreddit, r.period, r.scanLimit)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			log.Warn("cannot read meme listing", "subreddit", subreddit, "err", err)
			continue
		}

		for _, post := range posts {
			if result.Added >= r.postsPerRun || ctx.Err() != nil {
				break
			}
			if existing[post.ID] {
				result.Skipped++
				continue
			}

			image, err := r.images.Fetch(ctx, post.ImageURL)
			if err != nil {
				result.Failed++
				if firstErr == nil {
					firstErr = err
				}
				log.Warn("cannot download meme image", "meme", post.ID, "err", err)
				continue
			}

			analysis, err := r.analyzer.Analyze(ctx, post, image)
			if err != nil {
				result.Failed++
				if firstErr == nil {
					firstErr = err
				}
				log.Warn("cannot analyze meme", "meme", post.ID, "err", err)
				continue
			}

			name, err := SaveImage(r.dir, post.ID, image)
			if err != nil {
				result.Failed++
				if firstErr == nil {
					firstErr = err
				}
				log.Error("cannot save meme image", "meme", post.ID, "err", err)
				continue
			}

			meme := agent.Meme{
				ID:        post.ID,
				Subreddit: post.Subreddit,
				Title:     post.Title,
				Caption:   analysis.Caption,
				Context:   analysis.Context,
				Tags:      analysis.Tags,
				UseWhen:   analysis.UseWhen,
				Score:     post.Score,
				Link:      post.Permalink,
				ImageFile: name,
				AddedAt:   r.now().UTC(),
			}
			added, err := r.store.Add(meme)
			if err != nil {
				result.Failed++
				if firstErr == nil {
					firstErr = err
				}
				_ = DeleteImage(r.dir, name)
				log.Error("cannot store meme", "meme", post.ID, "err", err)
				continue
			}
			if !added {
				result.Skipped++
				_ = DeleteImage(r.dir, name)
				continue
			}
			existing[post.ID] = true
			result.Added++
		}
	}

	removed, err := r.store.Prune(r.retentionDays, r.keep)
	if err != nil {
		log.Error("cannot prune meme memory", "err", err)
		if firstErr == nil {
			firstErr = err
		}
	} else {
		for _, meme := range removed {
			if err := DeleteImage(r.dir, meme.ImageFile); err != nil {
				log.Warn("cannot delete retired meme image", "meme", meme.ID, "err", err)
			}
			result.Pruned++
		}
	}
	r.cleanOrphanImages()

	result.Duration = r.now().Sub(startedAt)
	errText := ""
	if firstErr != nil {
		errText = firstErr.Error()
	}
	if err := r.store.RecordRun(errText); err != nil {
		log.Error("cannot record meme refresh", "err", err)
	}

	log.Info("meme refresh finished",
		"added", result.Added,
		"skipped", result.Skipped,
		"failed", result.Failed,
		"pruned", result.Pruned,
		"duration", result.Duration,
	)

	if firstErr != nil && result.Added == 0 && result.Skipped == 0 && result.Failed == 0 {
		return result, firstErr
	}
	return result, nil
}

func (r *Runner) cleanOrphanImages() {
	if strings.TrimSpace(r.dir) == "" {
		return
	}
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Warn("cannot read meme image directory", "err", err)
		}
		return
	}

	keep := make(map[string]bool)
	for _, meme := range r.store.List() {
		keep[filepath.Base(meme.ImageFile)] = true
	}

	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if !isImageFileName(entry.Name()) || keep[entry.Name()] {
			continue
		}
		if err := os.Remove(filepath.Join(r.dir, entry.Name())); err != nil {
			log.Warn("cannot remove orphan meme image", "file", entry.Name(), "err", err)
			continue
		}
		removed++
	}
	if removed > 0 {
		log.Debug("removed orphan meme images", "count", removed)
	}
}

func isImageFileName(name string) bool {
	switch strings.ToLower(pathExt(name)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return true
	default:
		return false
	}
}

// FormatResult describes one refresh for a Discord reply.
func FormatResult(result Result) string {
	parts := []string{fmt.Sprintf("Added %d new meme%s.", result.Added, plural(result.Added))}
	if result.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("Skipped %d already stored.", result.Skipped))
	}
	if result.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed.", result.Failed))
	}
	if result.Pruned > 0 {
		parts = append(parts, fmt.Sprintf("Retired %d old meme%s.", result.Pruned, plural(result.Pruned)))
	}
	return strings.Join(parts, " ")
}

func plural(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}
