package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPackagesBuildIsReachableAsPackageBuild(t *testing.T) {
	// The feature is documented as `rwx package build`, which resolves through
	// the singular alias on the packages parent.
	require.Contains(t, packagesCmd.Aliases, "package")

	cmd, _, err := rootCmd.Find([]string{"package", "build"})
	require.NoError(t, err)
	require.Equal(t, "build", cmd.Name())
	require.Same(t, packagesBuildCmd, cmd)

	cmd, _, err = rootCmd.Find([]string{"packages", "build"})
	require.NoError(t, err)
	require.Same(t, packagesBuildCmd, cmd)
}

func TestPackagesBuildAcceptsAnOptionalDirectory(t *testing.T) {
	require.NoError(t, packagesBuildCmd.Args(packagesBuildCmd, []string{}))
	require.NoError(t, packagesBuildCmd.Args(packagesBuildCmd, []string{"rwx/package"}))
	require.Error(t, packagesBuildCmd.Args(packagesBuildCmd, []string{"a", "b"}))
}

func TestPackagesBuildExposesTimestampFlagAndRequiresAuth(t *testing.T) {
	flag := packagesBuildCmd.Flags().Lookup("timestamp")
	require.NotNil(t, flag, "build should expose the --timestamp flag")
	require.Equal(t, "", flag.DefValue, "timestamp normalization should be opt-in")

	// Uploading requires an authenticated API call.
	require.NotNil(t, packagesBuildCmd.PreRunE)
}
