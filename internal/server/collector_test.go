package server

import (
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/metruzanca/caffeinator/internal/memory"
	"github.com/stretchr/testify/require"
)

func testCollector(t *testing.T, extractor memoryExtractor) (*passiveCollector, *memory.Store) {
	t.Helper()

	store, err := memory.NewStore(t.TempDir(), 1000)
	require.NoError(t, err)

	collector := newPassiveCollector(store, extractor, passiveCollectorConfig{
		Interval:         time.Minute,
		MaxBatchMessages: 20,
		MaxTranscript:    8000,
		ExtractTimeout:   time.Second,
	})

	return collector, store
}

func TestPassiveCollectorGroupsByAuthor(t *testing.T) {
	collector, _ := testCollector(t, &fakeExtractor{})

	collector.add(testMessage("1", "channel", "guild", "111", "I am Alice"))
	collector.add(testMessage("2", "channel", "guild", "222", "I am Bob"))
	collector.add(testMessage("3", "channel", "guild", "111", "I work on Go"))

	require.Len(t, collector.pending["111"], 2)
	require.Len(t, collector.pending["222"], 1)
}

func TestPassiveCollectorKeepsAuthorsSeparate(t *testing.T) {
	extractor := &fakeExtractor{}
	collector, _ := testCollector(t, extractor)

	collector.add(testMessage("1", "channel", "guild", "111", "I am Alice"))
	collector.add(testMessage("2", "channel", "guild", "222", "I am Bob"))
	collector.flush()

	require.Equal(t, 2, extractor.calls)
	require.Len(t, extractor.gotTexts, 2)

	require.Contains(t, extractor.gotTexts[0], "111")
	require.Contains(t, extractor.gotTexts[0], "I am Alice")
	require.NotContains(t, extractor.gotTexts[0], "I am Bob")

	require.Contains(t, extractor.gotTexts[1], "222")
	require.Contains(t, extractor.gotTexts[1], "I am Bob")
	require.NotContains(t, extractor.gotTexts[1], "I am Alice")

	for _, subjects := range extractor.gotSubjects {
		require.Len(t, subjects, 1)
	}
}

func TestPassiveCollectorFiltersOtherSubjects(t *testing.T) {
	extractor := &fakeExtractor{
		facts: []memory.Fact{
			{SubjectID: "111", SubjectName: "Alice", Text: "Alice fact.", Category: "identity", Confidence: 0.9},
			{SubjectID: "222", SubjectName: "Bob", Text: "Bob fact.", Category: "identity", Confidence: 0.9},
		},
	}
	collector, store := testCollector(t, extractor)

	collector.add(testMessage("1", "channel", "guild", "111", "I am Alice"))
	collector.add(testMessage("2", "channel", "guild", "222", "I am Bob"))
	collector.flush()

	alice, err := store.Get("111")
	require.NoError(t, err)
	require.Contains(t, alice, "Alice fact.")
	require.NotContains(t, alice, "Bob fact.")

	bob, err := store.Get("222")
	require.NoError(t, err)
	require.Contains(t, bob, "Bob fact.")
	require.NotContains(t, bob, "Alice fact.")
}

func TestBuildPassiveTranscript(t *testing.T) {
	transcript := buildPassiveTranscript([]*discordgo.MessageCreate{
		testMessage("1", "channel", "guild", "111", "I am Alice"),
		testMessage("2", "channel", "guild", "111", "I work on Go"),
	}, 1000)

	require.Contains(t, transcript, "I am Alice")
	require.Contains(t, transcript, "I work on Go")
	require.True(t, strings.HasSuffix(transcript, "\n"))
}
