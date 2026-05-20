package throttle

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMaxConcurrencyFromEnv(t *testing.T) {
	t.Run("returns 0 when unset so caller can default", func(t *testing.T) {
		t.Setenv("RWX_RESULTS_MAX_CONCURRENCY", "")
		require.Equal(t, 0, MaxConcurrencyFromEnv())
	})

	t.Run("parses a valid integer", func(t *testing.T) {
		t.Setenv("RWX_RESULTS_MAX_CONCURRENCY", "7")
		require.Equal(t, 7, MaxConcurrencyFromEnv())
	})

	t.Run("rejects non-integer", func(t *testing.T) {
		t.Setenv("RWX_RESULTS_MAX_CONCURRENCY", "abc")
		require.Equal(t, 0, MaxConcurrencyFromEnv())
	})

	t.Run("rejects zero and negatives", func(t *testing.T) {
		t.Setenv("RWX_RESULTS_MAX_CONCURRENCY", "0")
		require.Equal(t, 0, MaxConcurrencyFromEnv())
		t.Setenv("RWX_RESULTS_MAX_CONCURRENCY", "-1")
		require.Equal(t, 0, MaxConcurrencyFromEnv())
	})
}

func TestAcquire_FirstSlotIsFree(t *testing.T) {
	cfg := Config{LockDir: t.TempDir(), MaxConcurrency: 2, Stderr: &bytes.Buffer{}}

	result, err := Acquire(context.Background(), "test", cfg)
	require.NoError(t, err)
	require.NotNil(t, result.Slot)
	require.False(t, result.Waited)
	require.False(t, result.TimedOut)
	result.Slot.Release()
}

func TestAcquire_WaitsForSlotToFree(t *testing.T) {
	cfg := Config{LockDir: t.TempDir(), MaxConcurrency: 1, Stderr: &bytes.Buffer{}}

	first, err := Acquire(context.Background(), "test", cfg)
	require.NoError(t, err)
	require.NotNil(t, first.Slot)

	releaseAfter := 150 * time.Millisecond
	go func() {
		time.Sleep(releaseAfter)
		first.Slot.Release()
	}()

	second, err := Acquire(context.Background(), "test", cfg)
	require.NoError(t, err)
	require.NotNil(t, second.Slot)
	require.True(t, second.Waited)
	require.False(t, second.TimedOut)
	require.GreaterOrEqual(t, second.WaitDuration, releaseAfter)
	second.Slot.Release()
}

func TestAcquire_TimesOutWhenAllSlotsHeld(t *testing.T) {
	cfg := Config{
		LockDir:        t.TempDir(),
		MaxConcurrency: 1,
		WaitTimeout:    150 * time.Millisecond,
		Stderr:         &bytes.Buffer{},
	}

	first, err := Acquire(context.Background(), "test", cfg)
	require.NoError(t, err)
	defer first.Slot.Release()

	start := time.Now()
	second, err := Acquire(context.Background(), "test", cfg)
	require.NoError(t, err)
	require.Nil(t, second.Slot)
	require.True(t, second.TimedOut)
	require.True(t, second.Waited)
	require.GreaterOrEqual(t, time.Since(start), cfg.WaitTimeout)
}

func TestAcquire_RespectsContextCancellation(t *testing.T) {
	cfg := Config{LockDir: t.TempDir(), MaxConcurrency: 1, Stderr: &bytes.Buffer{}}

	first, err := Acquire(context.Background(), "test", cfg)
	require.NoError(t, err)
	defer first.Slot.Release()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(80 * time.Millisecond)
		cancel()
	}()

	_, err = Acquire(ctx, "test", cfg)
	require.ErrorIs(t, err, context.Canceled)
}

func TestAcquire_EmitsWaitAndTimeoutMessages(t *testing.T) {
	var stderr bytes.Buffer
	cfg := Config{
		LockDir:        t.TempDir(),
		MaxConcurrency: 1,
		WaitTimeout:    150 * time.Millisecond,
		Stderr:         &stderr,
	}

	first, err := Acquire(context.Background(), "test", cfg)
	require.NoError(t, err)
	defer first.Slot.Release()

	_, err = Acquire(context.Background(), "test", cfg)
	require.NoError(t, err)
	require.Contains(t, stderr.String(), "Waiting for an `rwx test` slot")
	require.Contains(t, stderr.String(), "Gave up waiting")
}

func TestAcquire_ConcurrentRequestsRespectCap(t *testing.T) {
	cfg := Config{LockDir: t.TempDir(), MaxConcurrency: 2, Stderr: &bytes.Buffer{}}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var holdersAtPeak int
	current := 0

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := Acquire(context.Background(), "test", cfg)
			require.NoError(t, err)
			require.NotNil(t, result.Slot)

			mu.Lock()
			current++
			if current > holdersAtPeak {
				holdersAtPeak = current
			}
			mu.Unlock()

			time.Sleep(50 * time.Millisecond)

			mu.Lock()
			current--
			mu.Unlock()

			result.Slot.Release()
		}()
	}

	wg.Wait()
	require.LessOrEqual(t, holdersAtPeak, cfg.MaxConcurrency,
		"more slots were held simultaneously than the configured cap")
}
