package telemetry

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCollector_Record_WhenEnabled(t *testing.T) {
	c := NewCollector()
	c.Record("test_event", map[string]any{"key": "value"})

	events := c.Drain()
	require.Len(t, events, 1)
	require.Equal(t, "test_event", events[0].Event)
	require.Equal(t, "value", events[0].Props["key"])
	require.NotEmpty(t, events[0].Timestamp)
	require.NotEmpty(t, events[0].OS)
	require.NotEmpty(t, events[0].Arch)
}

func TestCollector_Record_StampsAgent(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("CLAUDECODE", "1")

	c := NewCollector()
	c.Record("test_event", nil)

	events := c.Drain()
	require.Len(t, events, 1)
	require.Equal(t, "claude_code", events[0].Agent)
}

func TestCollector_Record_OmitsAgentWhenNoneDetected(t *testing.T) {
	clearAgentEnv(t)

	c := NewCollector()
	c.Record("test_event", nil)

	events := c.Drain()
	require.Len(t, events, 1)
	require.Empty(t, events[0].Agent)
}

func TestCollector_Drain_ClearsQueue(t *testing.T) {
	c := NewCollector()
	c.Record("e1", nil)
	c.Record("e2", nil)

	events := c.Drain()
	require.Len(t, events, 2)

	events = c.Drain()
	require.Empty(t, events)
}

func TestCollector_ThreadSafe(t *testing.T) {
	c := NewCollector()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Record("concurrent", nil)
		}()
	}
	wg.Wait()

	events := c.Drain()
	require.Len(t, events, 100)
}
