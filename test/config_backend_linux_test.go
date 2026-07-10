//go:build linux && cgo

package integration_test

import (
	"bytes"
	"errors"
	"os"
	"os/user"
	"strconv"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigBackendInitializesForUnknownUser(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root to execute the CLI as an unknown user")
	}

	homeDir, err := os.MkdirTemp("", "rwx-unknown-user-home-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(homeDir)) })
	require.NoError(t, os.Chmod(homeDir, 0o755))

	uid := findUnknownUID(t)
	cmd := mintCmd(t, input{args: []string{"completion", "bash"}})
	cmd.Env = append(os.Environ(), "HOME="+homeDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{Uid: uid, Gid: uid},
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err = cmd.Run()
	require.NoError(t, err, stderr.String())
}

func findUnknownUID(t *testing.T) uint32 {
	t.Helper()

	for uid := uint32(1_000_000); uid < 1_001_000; uid++ {
		_, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
		var unknownUserID user.UnknownUserIdError
		if errors.As(err, &unknownUserID) {
			return uid
		}
	}

	t.Fatal("unable to find a UID without a passwd entry")
	return 0
}
