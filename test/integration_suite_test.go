package integration_test

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type input struct {
	args []string
}

type result struct {
	stdout   string
	stderr   string
	exitCode int
}

func mintCmd(t *testing.T, input input) *exec.Cmd {
	const rwxPath = "../rwx"
	_, err := os.Stat(rwxPath)
	require.NoError(t, err, "integration tests depend on a built rwx binary at %s", rwxPath)

	cmd := exec.Command(rwxPath, input.args...)

	t.Logf("Executing command: %s\n with env %s\n", cmd.String(), cmd.Env)

	return cmd
}

func runMint(t *testing.T, input input) result {
	cmd := mintCmd(t, input)
	var stdoutBuffer, stderrBuffer bytes.Buffer
	cmd.Stdout = &stdoutBuffer
	cmd.Stderr = &stderrBuffer

	err := cmd.Run()

	exitCode := 0

	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		require.True(t, ok, "rwx exited with an error that wasn't an ExitError")
		exitCode = exitErr.ExitCode()
	}

	return result{
		stdout:   strings.TrimSuffix(stdoutBuffer.String(), "\n"),
		stderr:   strings.TrimSuffix(stderrBuffer.String(), "\n"),
		exitCode: exitCode,
	}
}

func TestImageBuild(t *testing.T) {
	t.Run("fails when --tag is used with --no-pull", func(t *testing.T) {
		input := input{
			args: []string{"image", "build", "--access-token", "fake-for-test", "--target", "test-task", "--no-pull", "--tag", "my-tag", "test.yml"},
		}

		result := runMint(t, input)

		require.Equal(t, 1, result.exitCode)
		require.Contains(t, result.stderr, "cannot use --tag with --no-pull")
	})
}

func TestMintRun(t *testing.T) {
	t.Run("errors if an init parameter is specified without a flag", func(t *testing.T) {
		input := input{
			args: []string{"run", "--access-token", "fake-for-test", "--file", "./hello-world.mint.yaml", "init=foo"},
		}

		result := runMint(t, input)

		require.Equal(t, 1, result.exitCode)
		require.Contains(t, result.stderr, "You have specified a task target with an equals sign: \"init=foo\".")
		require.Contains(t, result.stderr, "You may have meant to specify --init \"init=foo\".")
	})

	t.Run("accepts --target flag for targeting tasks", func(t *testing.T) {
		input := input{
			args: []string{"run", "--access-token", "fake-for-test", "./hello-world.mint.yaml", "--target", "task1"},
		}

		result := runMint(t, input)

		require.NotContains(t, result.stderr, "unknown flag")
	})

	t.Run("accepts multiple --target flags", func(t *testing.T) {
		input := input{
			args: []string{"run", "--access-token", "fake-for-test", "./hello-world.mint.yaml", "--target", "task1", "--target", "task2"},
		}

		result := runMint(t, input)

		require.NotContains(t, result.stderr, "unknown flag")
	})

	t.Run("errors if --file is used with positional task argument", func(t *testing.T) {
		input := input{
			args: []string{"run", "--access-token", "fake-for-test", "--file", "./hello-world.mint.yaml", "task1"},
		}

		result := runMint(t, input)

		require.Equal(t, 1, result.exitCode)
		require.Contains(t, result.stderr, "positional arguments are not supported")
	})
}
