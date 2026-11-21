package server

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatNotice(t *testing.T) {
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
			want:    "Bot has restarted. Built from `74d7bc3` feat(chat): require compact inline citations for web search",
		},
		{
			name: "sha only",
			sha:  "74d7bc3",
			want: "Bot has restarted. Built from `74d7bc3`",
		},
		{
			name:    "subject only",
			subject: "feat(chat): require compact inline citations for web search",
			want:    "Bot has restarted. Built from feat(chat): require compact inline citations for web search",
		},
		{
			name: "no build info",
			want: "Bot has restarted.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, formatNotice(tt.sha, tt.subject))
		})
	}
}
