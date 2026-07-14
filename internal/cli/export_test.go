package cli

import "time"

// SetArtifactReadyMaxWaitForTest overrides the artifact polling deadline so tests don't have to
// wait the full production duration. It returns a function that restores the previous value.
func SetArtifactReadyMaxWaitForTest(d time.Duration) (restore func()) {
	prev := artifactReadyMaxWait
	artifactReadyMaxWait = d
	return func() { artifactReadyMaxWait = prev }
}
