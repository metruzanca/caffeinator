package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/charmbracelet/log"
	"github.com/metruzanca/caffeinator/agent"
	"github.com/metruzanca/caffeinator/internal/config"
	"github.com/metruzanca/caffeinator/internal/memes"
	"github.com/spf13/viper"
)

type memesRuntime struct {
	Store         *agent.MemeStore
	Dir           string
	Runner        *memes.Runner
	Scheduler     *memes.Scheduler
	Positive      map[string]bool
	Configured    bool
	Subreddits    []string
	Expression    string
	Timezone      string
	ScheduleError string
	Keep          int
	Retention     int
	PostsPerRun   int
}

func newMemesFromConfig() *memesRuntime {
	if !viper.GetBool("MEMES_ENABLED") {
		log.Info("meme memory disabled")
		return nil
	}

	apiKey := strings.TrimSpace(viper.GetString("OPENROUTER_API_KEY"))
	if apiKey == "" {
		log.Warn("meme memory disabled: OPENROUTER_API_KEY is not set")
		return nil
	}

	memoryDir := strings.TrimSpace(viper.GetString("MEMORY_DIR"))
	if memoryDir == "" {
		memoryDir = "./memory"
	}
	path := strings.TrimSpace(viper.GetString("MEMES_PATH"))
	if path == "" {
		path = filepath.Join(memoryDir, "memes.json")
	}
	dir := strings.TrimSpace(viper.GetString("MEMES_DIR"))
	if dir == "" {
		dir = filepath.Join(memoryDir, "memes")
	}

	store, err := agent.NewMemeStore(path)
	if err != nil {
		log.Error("meme memory disabled", "err", err)
		return nil
	}

	subreddits := parseList(viper.GetString("MEMES_SUBREDDITS"))
	if len(subreddits) == 0 {
		log.Warn("meme memory disabled: MEMES_SUBREDDITS is empty")
		return nil
	}

	model := strings.TrimSpace(viper.GetString("MEMES_ANALYSIS_MODEL"))
	if model == "" {
		model = strings.TrimSpace(viper.GetString("OPENROUTER_MODEL"))
	}
	if model == "" {
		model = "deepseek/deepseek-v4.1-flash-1017"
	}
	baseURL := strings.TrimSpace(viper.GetString("OPENROUTER_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}

	analysisClient := &agent.ChatClient{
		APIKey:         apiKey,
		AppName:        config.BotName(),
		AppURL:         config.AppURL(),
		Model:          model,
		BaseURL:        baseURL,
		MaxTokens:      512,
		SystemPrompt:   memes.AnalysisSystemPrompt,
		ResponseFormat: &agent.ResponseFormat{Type: "json_object"},
	}

	userAgent := strings.TrimSpace(viper.GetString("REDDIT_USER_AGENT"))
	reddit := memes.NewRedditClient(memes.RedditConfig{
		Session:     viper.GetString("REDDIT_SESSION"),
		UserAgent:   userAgent,
		IncludeNSFW: viper.GetBool("MEMES_INCLUDE_NSFW"),
		MinGap:      memes.DefaultRedditMinGap,
	})
	fetcher := memes.NewImageFetcher(userAgent, viper.GetInt("MEMES_MAX_IMAGE_MB"))
	analyzer := memes.NewAnalyzer(
		analysisClient,
		time.Duration(viper.GetInt("MEMES_ANALYSIS_TIMEOUT_SECONDS"))*time.Second,
	)
	runner := memes.NewRunner(memes.RunnerConfig{
		Reddit:        reddit,
		Images:        fetcher,
		Analyzer:      analyzer,
		Store:         store,
		Dir:           dir,
		Subreddits:    subreddits,
		Period:        viper.GetString("MEMES_TOP_PERIOD"),
		ScanLimit:     viper.GetInt("MEMES_SCAN_LIMIT"),
		PostsPerRun:   viper.GetInt("MEMES_POSTS_PER_RUN"),
		Keep:          viper.GetInt("MEMES_MAX_STORED"),
		RetentionDays: viper.GetInt("MEMES_RETENTION_DAYS"),
	})

	scheduleError := ""
	scheduler, err := memes.NewScheduler(
		runner,
		viper.GetString("MEMES_CRON"),
		viper.GetString("MEMES_TIMEZONE"),
		time.Duration(viper.GetInt("MEMES_REFRESH_TIMEOUT_SECONDS"))*time.Second,
	)
	if err != nil {
		log.Error("meme schedule disabled", "err", err)
		scheduleError = err.Error()
		scheduler = nil
	}

	runtime := &memesRuntime{
		Store:         store,
		Dir:           dir,
		Runner:        runner,
		Scheduler:     scheduler,
		Positive:      parseReactionSet(viper.GetString("MEMES_POSITIVE_REACTIONS")),
		Configured:    reddit.Configured(),
		Subreddits:    runner.Subreddits(),
		Expression:    strings.TrimSpace(viper.GetString("MEMES_CRON")),
		Timezone:      strings.TrimSpace(viper.GetString("MEMES_TIMEZONE")),
		ScheduleError: scheduleError,
		Keep:          runner.Keep(),
		Retention:     runner.RetentionDays(),
		PostsPerRun:   runner.PostsPerRun(),
	}
	if scheduler != nil {
		runtime.Expression = scheduler.Expression()
		runtime.Timezone = scheduler.Timezone()
	}

	log.Info("meme memory enabled",
		"path", path,
		"dir", dir,
		"subreddits", strings.Join(runtime.Subreddits, ","),
		"period", runner.Period(),
		"cron", runtime.Expression,
		"timezone", runtime.Timezone,
		"reddit_session", runtime.Configured,
	)
	if !runtime.Configured {
		log.Warn("meme memory needs REDDIT_SESSION to scrape Reddit")
	}
	return runtime
}

func (b *Bot) onMemeReaction(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	if b == nil || b.memes == nil || b.memes.Store == nil {
		return
	}
	if s == nil || s.State == nil || s.State.User == nil || r == nil {
		return
	}
	if r.GuildID == "" || r.UserID == "" || r.UserID == s.State.User.ID {
		return
	}
	if !b.memes.Positive[normalizeReactionEmoji(r.Emoji.Name)] {
		return
	}
	if member, err := s.State.Member(r.GuildID, r.UserID); err == nil &&
		member != nil && member.User != nil && member.User.Bot {
		return
	}

	meme, ok := b.memes.Store.FindByMessage(r.MessageID)
	if !ok {
		return
	}
	updated, counted, err := b.memes.Store.AddReaction(meme.ID, r.UserID)
	if err != nil {
		log.Error("cannot record meme reaction", "meme", meme.ID, "err", err)
		return
	}
	if !counted {
		return
	}
	log.Info("meme reaction recorded",
		"meme", meme.ID,
		"reactions", updated.Reactions,
		"channel", r.ChannelID,
		"user", r.UserID,
	)
}

func loadMemeImage(dir string, selected []agent.Meme) (*discordgo.File, string, error) {
	if len(selected) == 0 {
		return nil, "", nil
	}
	meme := selected[0]
	if strings.TrimSpace(dir) == "" || strings.TrimSpace(meme.ImageFile) == "" {
		return nil, meme.ID, nil
	}
	data, mediaType, err := memes.LoadImageFile(dir, meme.ImageFile)
	if err != nil {
		log.Warn("cannot load meme image", "meme", meme.ID, "err", err)
		return nil, meme.ID, err
	}
	return &discordgo.File{
		Name:        meme.ImageFile,
		ContentType: mediaType,
		Reader:      bytes.NewReader(data),
	}, meme.ID, nil
}

func parseReactionSet(raw string) map[string]bool {
	set := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		name := normalizeReactionEmoji(part)
		if name != "" {
			set[name] = true
		}
	}
	if len(set) == 0 {
		for _, emoji := range []string{"😂", "🤣", "💀", "🔥", "👍", "❤", "😆"} {
			set[emoji] = true
		}
	}
	return set
}

func normalizeReactionEmoji(name string) string {
	name = strings.TrimSpace(name)
	var builder strings.Builder
	for _, r := range name {
		if r == '\uFE0F' || r == '\uFE0E' {
			continue
		}
		if r >= 0x1F3FB && r <= 0x1F3FF {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func parseList(raw string) []string {
	var values []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

type memeCommandService struct {
	runtime *memesRuntime
}

func (s memeCommandService) StatusText() string {
	if s.runtime == nil {
		return "Meme memory is not enabled."
	}

	status := s.runtime.Store.Status()
	lines := []string{"Meme memory is enabled."}
	lines = append(lines, "Subreddits: "+strings.Join(s.runtime.Subreddits, ", "))
	lines = append(lines, "Top period: "+s.runtime.Runner.Period())
	if s.runtime.ScheduleError != "" {
		lines = append(lines, "Schedule error: "+truncateRunes(s.runtime.ScheduleError, 300))
	} else if s.runtime.Expression == "" {
		lines = append(lines, "Schedule: not configured")
	} else {
		lines = append(lines, fmt.Sprintf("Schedule: `%s` (%s)", s.runtime.Expression, s.runtime.Timezone))
	}
	if s.runtime.Scheduler != nil {
		if next := s.runtime.Scheduler.Next(); !next.IsZero() {
			lines = append(lines, fmt.Sprintf("Next run: <t:%d:f> (<t:%d:R>)", next.Unix(), next.Unix()))
		}
	}
	if status.LastRun.IsZero() {
		lines = append(lines, "Last run: never")
	} else {
		lines = append(lines, fmt.Sprintf("Last run: <t:%d:f>", status.LastRun.Unix()))
	}
	if status.LastError != "" {
		lines = append(lines, "Last error: "+truncateRunes(status.LastError, 300))
	}
	lines = append(lines, fmt.Sprintf("Stored: %d/%d", status.Stored, s.runtime.Keep))
	lines = append(lines, fmt.Sprintf("Per run: up to %d new memes", s.runtime.PostsPerRun))
	lines = append(lines, fmt.Sprintf("Retention: %d days for unused, unreacted memes", s.runtime.Retention))
	lines = append(lines, fmt.Sprintf("Reactions: %d", status.Reactions))
	if s.runtime.Configured {
		lines = append(lines, "Reddit session: configured")
	} else {
		lines = append(lines, "Reddit session: missing, set REDDIT_SESSION")
	}
	return strings.Join(lines, "\n")
}

func (s memeCommandService) Refresh(ctx context.Context) (string, error) {
	if s.runtime == nil || s.runtime.Runner == nil {
		return "", errors.New("meme memory is not enabled")
	}
	result, err := s.runtime.Runner.Refresh(ctx)
	if err != nil {
		return "", err
	}
	return memes.FormatResult(result), nil
}

func (s memeCommandService) ListText(limit int) string {
	if s.runtime == nil {
		return "Meme memory is not enabled."
	}
	stored := s.runtime.Store.ListNewestFirst()
	if len(stored) == 0 {
		return "No memes stored yet."
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > len(stored) {
		limit = len(stored)
	}

	var builder strings.Builder
	for index, meme := range stored[:limit] {
		fmt.Fprintf(&builder, "%d. %s r/%s", index+1, meme.AddedAt.Format("2006-01-02"), meme.Subreddit)
		if meme.Reactions > 0 {
			fmt.Fprintf(&builder, ", %d reactions", meme.Reactions)
		}
		if meme.Uses > 0 {
			fmt.Fprintf(&builder, ", used %d times", meme.Uses)
		}
		builder.WriteString("\n   ")
		builder.WriteString(truncateRunes(meme.Title, 120))
		builder.WriteString("\n   ")
		builder.WriteString(truncateRunes(strings.TrimSpace(meme.Caption+" "+meme.Context), 220))
		if meme.Link != "" {
			builder.WriteString("\n   ")
			builder.WriteString(meme.Link)
		}
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}
