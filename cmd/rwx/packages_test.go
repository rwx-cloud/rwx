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

func TestPackagesBuildExposesBuildFlags(t *testing.T) {
	flag := packagesBuildCmd.Flags().Lookup("timestamp")
	require.NotNil(t, flag, "build should expose the --timestamp flag")
	require.Equal(t, "", flag.DefValue, "timestamp normalization should be opt-in")

	require.Nil(t, packagesBuildCmd.Flags().Lookup("private"), "visibility is controlled by the manifest")
	publish := packagesBuildCmd.Flags().Lookup("publish")
	require.NotNil(t, publish)
	require.Equal(t, "false", publish.DefValue, "publishing should be opt-in")

	// Authentication can be supplied by a proxy rather than a local token.
	require.Nil(t, packagesBuildCmd.PreRunE)
}

func TestPackagesPublishRequiresDigest(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"package", "publish"})
	require.NoError(t, err)
	require.Same(t, packagesPublishCmd, cmd)
	require.NoError(t, cmd.Args(cmd, []string{"digest"}))
	require.Error(t, cmd.Args(cmd, nil))
	require.Error(t, cmd.Args(cmd, []string{"one", "two"}))
	require.Nil(t, cmd.PreRunE)
}
