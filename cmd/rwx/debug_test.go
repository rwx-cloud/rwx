package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDebugExposesSessionFlag(t *testing.T) {
	flag := debugCmd.Flags().Lookup("session")

	require.NotNil(t, flag)
	require.Equal(t, "string", flag.Value.Type())
}
