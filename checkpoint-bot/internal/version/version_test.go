package version

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDescribe(t *testing.T) {
	tests := []struct {
		name    string
		sha     string
		subject string
		want    string
	}{
		{
			name:    "sha and subject",
			sha:     "74d7bc3",
			subject: "feat(chat): require compact inline citations for web search",
			want:    "74d7bc3 feat(chat): require compact inline citations for web search",
		},
		{
			name: "sha only",
			sha:  "74d7bc3",
			want: "74d7bc3",
		},
		{
			name:    "subject only",
			subject: "feat(chat): require compact inline citations for web search",
			want:    "feat(chat): require compact inline citations for web search",
		},
		{
			name: "empty",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, describe(tt.sha, tt.subject))
		})
	}
}

func TestShortCommit(t *testing.T) {
	require.Equal(t, "74d7bc3", shortCommit(" 74d7bc3 "))
	require.Equal(t, "74d7bc3", shortCommit("74d7bc3c55b36df7d674d55965a23b8d54ad69b"))
	require.Equal(t, "", shortCommit("  "))
}

func TestCleanMessage(t *testing.T) {
	require.Equal(t, "fix(memory): prevent truncated extraction responses", cleanMessage("fix(memory): prevent truncated extraction responses"))
	require.Equal(t, "fix(memory): prevent truncated extraction responses", cleanMessage("fix(memory): prevent truncated extraction responses\n\nLonger body here."))
	require.Equal(t, "fix(memory): prevent truncated extraction responses", cleanMessage("  fix(memory): prevent truncated extraction responses\r\nbody"))

	long := strings.Repeat("a", maxMessageLen+50)
	got := cleanMessage(long)
	require.Equal(t, maxMessageLen+3, len(got))
	require.True(t, strings.HasSuffix(got, "..."))
}
