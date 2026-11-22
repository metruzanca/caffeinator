package server

import (
	"context"
	"testing"
	"time"

	"github.com/metruzanca/caffeinator/agent"
	"github.com/stretchr/testify/require"
)

type fakeLearner struct {
	calls         int
	gotTurns      [][]agent.Turn
	gotSubjects   []agent.User
	includeServer []bool
	facts         []agent.Fact
	err           error
}

func (f *fakeLearner) Learn(ctx context.Context, turns []agent.Turn, subject agent.User, includeServer bool) ([]agent.Fact, error) {
	f.calls++
	f.gotTurns = append(f.gotTurns, turns)
	f.gotSubjects = append(f.gotSubjects, subject)
	f.includeServer = append(f.includeServer, includeServer)
	return f.facts, f.err
}

func testCollector(t *testing.T, learner memoryLearner) *passiveCollector {
	t.Helper()

	return newPassiveCollector(learner, passiveCollectorConfig{
		Interval:         time.Minute,
		MaxBatchMessages: 20,
		ExtractTimeout:   time.Second,
	})
}

func TestPassiveCollectorGroupsByAuthor(t *testing.T) {
	collector := testCollector(t, &fakeLearner{})

	collector.add(toAgentTurn(testMessage("1", "channel", "guild", "111", "I am Alice").Message))
	collector.add(toAgentTurn(testMessage("2", "channel", "guild", "222", "I am Bob").Message))
	collector.add(toAgentTurn(testMessage("3", "channel", "guild", "111", "I work on Go").Message))

	require.Len(t, collector.pending["111"], 2)
	require.Len(t, collector.pending["222"], 1)
}

func TestPassiveCollectorKeepsAuthorsSeparate(t *testing.T) {
	learner := &fakeLearner{}
	collector := testCollector(t, learner)

	collector.add(toAgentTurn(testMessage("1", "channel", "guild", "111", "I am Alice").Message))
	collector.add(toAgentTurn(testMessage("2", "channel", "guild", "222", "I am Bob").Message))
	collector.flush()

	require.Equal(t, 2, learner.calls)
	require.Len(t, learner.gotTurns, 2)

	require.Equal(t, "111", learner.gotSubjects[0].ID)
	require.Len(t, learner.gotTurns[0], 1)
	require.Equal(t, "I am Alice", learner.gotTurns[0][0].Content)

	require.Equal(t, "222", learner.gotSubjects[1].ID)
	require.Len(t, learner.gotTurns[1], 1)
	require.Equal(t, "I am Bob", learner.gotTurns[1][0].Content)

	for _, includeServer := range learner.includeServer {
		require.False(t, includeServer)
	}
}

func TestPassiveCollectorCapsBatch(t *testing.T) {
	learner := &fakeLearner{}
	collector := newPassiveCollector(learner, passiveCollectorConfig{
		Interval:         time.Minute,
		MaxBatchMessages: 2,
		ExtractTimeout:   time.Second,
	})

	collector.add(toAgentTurn(testMessage("1", "channel", "guild", "111", "one").Message))
	collector.add(toAgentTurn(testMessage("2", "channel", "guild", "111", "two").Message))
	collector.add(toAgentTurn(testMessage("3", "channel", "guild", "111", "three").Message))

	require.Len(t, collector.pending["111"], 2)
	require.Equal(t, "two", collector.pending["111"][0].Content)
	require.Equal(t, "three", collector.pending["111"][1].Content)
}
