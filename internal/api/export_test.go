package api

import "time"

// StubListRunsRetrySleep replaces the ListRuns retry backoff sleep so external
// (api_test) tests can exercise retry paths without real delays. It returns a
// function that restores the original sleep.
func StubListRunsRetrySleep(fn func(time.Duration)) func() {
	original := listRunsRetrySleep
	listRunsRetrySleep = fn
	return func() { listRunsRetrySleep = original }
}
