// Package version reports which build of the bot is running.
package version

import (
	"os"
	"runtime/debug"
	"strings"
)

const (
	shortCommitLen = 7
	maxMessageLen  = 100
)

// Commit and Message can be set at build time with -ldflags. Railway supplies
// the same values through the environment, so most builds need neither.
var (
	Commit  string
	Message string
)

// SHA returns the short commit hash of the running build, or an empty string.
func SHA() string {
	return shortCommit(resolveCommit())
}

// Subject returns the first line of the running build's commit message, or an
// empty string.
func Subject() string {
	return cleanMessage(resolveMessage())
}

// Describe returns a single line summary of the build, like
// "74d7bc3 feat(chat): require compact inline citations for web search".
// It returns an empty string when no build information is available.
func Describe() string {
	return describe(SHA(), Subject())
}

func describe(sha, subject string) string {
	switch {
	case sha != "" && subject != "":
		return sha + " " + subject
	case sha != "":
		return sha
	default:
		return subject
	}
}

func resolveCommit() string {
	candidates := []string{
		Commit,
		os.Getenv("RAILWAY_GIT_COMMIT_SHA"),
		os.Getenv("GIT_COMMIT_SHA"),
		os.Getenv("GIT_COMMIT"),
	}
	for _, candidate := range candidates {
		if candidate = strings.TrimSpace(candidate); candidate != "" {
			return candidate
		}
	}
	return buildRevision()
}

func resolveMessage() string {
	candidates := []string{
		Message,
		os.Getenv("RAILWAY_GIT_COMMIT_MESSAGE"),
		os.Getenv("GIT_COMMIT_MESSAGE"),
	}
	for _, candidate := range candidates {
		if candidate = strings.TrimSpace(candidate); candidate != "" {
			return candidate
		}
	}
	return ""
}

// buildRevision reads the VCS stamp Go embeds for builds made from a git
// checkout. It is the local development fallback.
func buildRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return setting.Value
		}
	}
	return ""
}

func shortCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	if len(commit) <= shortCommitLen {
		return commit
	}
	return commit[:shortCommitLen]
}

func cleanMessage(message string) string {
	message = strings.TrimSpace(message)
	if i := strings.IndexAny(message, "\r\n"); i >= 0 {
		message = strings.TrimSpace(message[:i])
	}
	runes := []rune(message)
	if len(runes) <= maxMessageLen {
		return message
	}
	return strings.TrimSpace(string(runes[:maxMessageLen])) + "..."
}
