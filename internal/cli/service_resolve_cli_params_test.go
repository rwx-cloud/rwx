package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveCliParamsForFile(t *testing.T) {
	t.Run("no changes when no on section exists", func(t *testing.T) {
		tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.yml")
		require.NoError(t, err)
		defer tmpFile.Close()

		content := `
tasks:
  - key: "test"
    run: echo 'hello world'
`
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)

		result, err := ResolveCliParamsForFile(tmpFile.Name())
		require.NoError(t, err)
		require.False(t, result.Rewritten)
	})

	t.Run("no changes when on section is empty", func(t *testing.T) {
		tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.yml")
		require.NoError(t, err)
		defer tmpFile.Close()

		content := `
on:

tasks:
  - key: "test"
    run: echo 'hello world'
`
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)

		result, err := ResolveCliParamsForFile(tmpFile.Name())
		require.NoError(t, err)
		require.False(t, result.Rewritten)
	})

	t.Run("no changes when triggers only have non-git init params", func(t *testing.T) {
		tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.yml")
		require.NoError(t, err)
		defer tmpFile.Close()

		content := `
on:
  github:
    push:
      init:
        foo: bar
        baz: qux

tasks:
  - key: "test"
    run: echo 'hello world'
`
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)

		result, err := ResolveCliParamsForFile(tmpFile.Name())
		require.NoError(t, err)
		require.False(t, result.Rewritten)
	})

	t.Run("no changes when CLI trigger already has git init params", func(t *testing.T) {
		tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.yml")
		require.NoError(t, err)
		defer tmpFile.Close()

		content := `
on:
  cli:
    init:
      sha: ${{ event.git.sha }}
  github:
    push:
      init:
        sha: ${{ event.git.sha }}

tasks:
  - key: "test"
    run: echo 'hello world'
`
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)

		result, err := ResolveCliParamsForFile(tmpFile.Name())
		require.NoError(t, err)
		require.False(t, result.Rewritten)
		require.Equal(t, []string{"sha"}, result.GitParams)

		fileContent, err := os.ReadFile(tmpFile.Name())
		require.NoError(t, err)
		require.Equal(t, content, string(fileContent))
	})

	t.Run("no changes when CLI already has git params", func(t *testing.T) {
		tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.yml")
		require.NoError(t, err)
		defer tmpFile.Close()

		content := `
on:
  cli:
    init:
      sha: ${{ event.git.sha }}
  github:
    push:
      init:
        commit-sha: ${{ event.git.sha }}

tasks:
  - key: "test"
    run: echo 'hello world'
`
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)

		result, err := ResolveCliParamsForFile(tmpFile.Name())
		require.NoError(t, err)
		require.False(t, result.Rewritten)
		require.Equal(t, []string{"commit-sha", "sha"}, result.GitParams)

		fileContent, err := os.ReadFile(tmpFile.Name())
		require.NoError(t, err)
		require.Equal(t, content, string(fileContent))
	})

	t.Run("returns git clone ref param when CLI trigger already has git init params", func(t *testing.T) {
		tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.yml")
		require.NoError(t, err)
		defer tmpFile.Close()

		content := `
on:
  cli:
    init:
      ref: ${{ event.git.sha }}

tasks:
  - key: clone
    call: git/clone 1.8.1
    with:
      ref: ${{ init.ref }}
`
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)

		result, err := ResolveCliParamsForFile(tmpFile.Name())
		require.NoError(t, err)
		require.False(t, result.Rewritten)
		require.Equal(t, []string{"ref"}, result.GitParams)

		fileContent, err := os.ReadFile(tmpFile.Name())
		require.NoError(t, err)
		require.Equal(t, content, string(fileContent))
	})

	t.Run("adds CLI trigger when another trigger has git params", func(t *testing.T) {
		tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.yml")
		require.NoError(t, err)
		defer tmpFile.Close()

		content := `
on:
  github:
    push:
      init:
        sha: ${{ event.git.sha }}

tasks:
  - key: "test"
    run: echo 'hello world'
`
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)

		result, err := ResolveCliParamsForFile(tmpFile.Name())
		require.NoError(t, err)
		require.True(t, result.Rewritten)

		fileContent, err := os.ReadFile(tmpFile.Name())
		require.NoError(t, err)
		require.Contains(t, string(fileContent), "cli:")
		require.Contains(t, string(fileContent), "sha: ${{ event.git.sha }}")
	})

	t.Run("merges git params into existing CLI trigger", func(t *testing.T) {
		tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.yml")
		require.NoError(t, err)
		defer tmpFile.Close()

		content := `
on:
  github:
    push:
      init:
        sha: ${{ event.git.sha }}
  cli:
    init:
      foo: bar

tasks:
  - key: "test"
    run: echo 'hello world'
`
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)

		result, err := ResolveCliParamsForFile(tmpFile.Name())
		require.NoError(t, err)
		require.True(t, result.Rewritten)

		fileContent, err := os.ReadFile(tmpFile.Name())
		require.NoError(t, err)
		require.Contains(t, string(fileContent), "foo: bar")
		require.Contains(t, string(fileContent), "sha: ${{ event.git.sha }}")
	})

	t.Run("succeeds when multiple triggers have same git param mappings", func(t *testing.T) {
		tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.yml")
		require.NoError(t, err)
		defer tmpFile.Close()

		content := `
on:
  github:
    push:
      init:
        sha: ${{ event.git.sha }}
  gitlab:
    push:
      init:
        sha: ${{ event.git.sha }}

tasks:
  - key: "test"
    run: echo 'hello world'
`
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)

		result, err := ResolveCliParamsForFile(tmpFile.Name())
		require.NoError(t, err)
		require.True(t, result.Rewritten)

		fileContent, err := os.ReadFile(tmpFile.Name())
		require.NoError(t, err)
		require.Contains(t, string(fileContent), "cli:")
		require.Contains(t, string(fileContent), "sha: ${{ event.git.sha }}")
	})

	t.Run("detects git/clone package and maps ref to event.git.sha", func(t *testing.T) {
		tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.yml")
		require.NoError(t, err)
		defer tmpFile.Close()

		content := `
on:
  github:
    push:
      init:
        # git/clone ref takes precedence over this mapping
        commit-sha: ${{ event.git.sha }}

tasks:
  - key: clone
    call: git/clone 1.8.1
    with:
      ref: ${{ init.ref }}
`
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)

		result, err := ResolveCliParamsForFile(tmpFile.Name())
		require.NoError(t, err)
		require.True(t, result.Rewritten)

		fileContent, err := os.ReadFile(tmpFile.Name())
		require.NoError(t, err)
		require.Contains(t, string(fileContent), "cli:")
		require.Contains(t, string(fileContent), "ref: ${{ event.git.sha }}")
	})

	t.Run("errors when multiple git/clone packages use different ref params", func(t *testing.T) {
		tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.yml")
		require.NoError(t, err)
		defer tmpFile.Close()

		content := `
tasks:
  - key: clone1
    call: git/clone 1.8.1
    with:
      ref: ${{ init.ref }}
  - key: clone2
    call: git/clone 1.8.1
    with:
      ref: ${{ init.sha }}
`
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)

		result, err := ResolveCliParamsForFile(tmpFile.Name())
		require.Error(t, err)
		require.False(t, result.Rewritten)
		require.Contains(t, err.Error(), "multiple git/clone")
	})

	t.Run("uses init expression when one git/clone has hardcoded ref", func(t *testing.T) {
		tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.yml")
		require.NoError(t, err)
		defer tmpFile.Close()

		content := `
tasks:
  - key: clone1
    call: git/clone 1.8.1
    with:
      ref: main
  - key: clone2
    call: git/clone 1.8.1
    with:
      ref: ${{ init.ref }}
`
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)

		result, err := ResolveCliParamsForFile(tmpFile.Name())
		require.NoError(t, err)
		require.True(t, result.Rewritten)

		fileContent, err := os.ReadFile(tmpFile.Name())
		require.NoError(t, err)
		require.Contains(t, string(fileContent), "cli:")
		require.Contains(t, string(fileContent), "ref: ${{ event.git.sha }}")
	})

	t.Run("adds CLI trigger when git/clone exists but no on section", func(t *testing.T) {
		tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.yml")
		require.NoError(t, err)
		defer tmpFile.Close()

		content := `
tasks:
  - key: clone
    call: git/clone 1.8.1
    with:
      ref: ${{ init.sha }}
`
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)

		result, err := ResolveCliParamsForFile(tmpFile.Name())
		require.NoError(t, err)
		require.True(t, result.Rewritten)

		fileContent, err := os.ReadFile(tmpFile.Name())
		require.NoError(t, err)
		require.Contains(t, string(fileContent), "on:")
		require.Contains(t, string(fileContent), "cli:")
		require.Contains(t, string(fileContent), "sha: ${{ event.git.sha }}")
	})

	t.Run("adds CLI trigger after document separator", func(t *testing.T) {
		tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.yml")
		require.NoError(t, err)
		defer tmpFile.Close()

		content := `---
tasks:
  - key: clone
    call: git/clone 1.8.1
    with:
      ref: ${{ init.sha }}
`
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)

		result, err := ResolveCliParamsForFile(tmpFile.Name())
		require.NoError(t, err)
		require.True(t, result.Rewritten)

		fileContent, err := os.ReadFile(tmpFile.Name())
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(string(fileContent), "---\non:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n"))
		require.Contains(t, string(fileContent), "tasks:")
	})

	t.Run("adds CLI trigger when dispatch trigger has git params", func(t *testing.T) {
		tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.yml")
		require.NoError(t, err)
		defer tmpFile.Close()

		content := `
on:
  dispatch:
    - key: release-cli
      title: "Release"
      init:
        commit: ${{ event.git.sha }}
        version: ${{ event.dispatch.params.version }}

tasks:
  - key: "test"
    run: echo 'hello world'
`
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)

		result, err := ResolveCliParamsForFile(tmpFile.Name())
		require.NoError(t, err)
		require.True(t, result.Rewritten)

		fileContent, err := os.ReadFile(tmpFile.Name())
		require.NoError(t, err)
		require.Contains(t, string(fileContent), "cli:")
		require.Contains(t, string(fileContent), "commit: ${{ event.git.sha }}")
	})

	t.Run("extracts git params from conditional init params", func(t *testing.T) {
		tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.yml")
		require.NoError(t, err)
		defer tmpFile.Close()

		content := `
on:
  github:
    push:
      - if: ${{ init.deploy }}
        init:
          sha: ${{ event.git.sha }}
      - if: ${{ init.test }}
        init:
          sha: ${{ event.git.sha }}

tasks:
  - key: "test"
    run: echo 'hello world'
`
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)

		result, err := ResolveCliParamsForFile(tmpFile.Name())
		require.NoError(t, err)
		require.True(t, result.Rewritten)

		fileContent, err := os.ReadFile(tmpFile.Name())
		require.NoError(t, err)
		require.Contains(t, string(fileContent), "cli:")
		require.Contains(t, string(fileContent), "sha: ${{ event.git.sha }}")
	})

	t.Run("does not extract git params from event.git.ref", func(t *testing.T) {
		tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.yml")
		require.NoError(t, err)
		defer tmpFile.Close()

		content := `
on:
  github:
    push:
      init:
        ref: ${{ event.git.ref }}

tasks:
  - key: "test"
    run: echo 'hello world'
`
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)

		result, err := ResolveCliParamsForFile(tmpFile.Name())
		require.NoError(t, err)
		require.False(t, result.Rewritten)
	})

	t.Run("does not overwrite existing CLI init param with hardcoded value", func(t *testing.T) {
		tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.yml")
		require.NoError(t, err)
		defer tmpFile.Close()

		content := `
on:
  github:
    push:
      init:
        commit-sha: ${{ event.git.sha }}
  cli:
    init:
      commit-sha: HEAD

tasks:
  - key: "test"
    run: echo 'hello world'
`
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)

		result, err := ResolveCliParamsForFile(tmpFile.Name())
		require.NoError(t, err)
		require.False(t, result.Rewritten)

		fileContent, err := os.ReadFile(tmpFile.Name())
		require.NoError(t, err)
		require.Contains(t, string(fileContent), "commit-sha: HEAD")
	})

	t.Run("errors when multiple events use different param names for event.git.sha", func(t *testing.T) {
		tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.yml")
		require.NoError(t, err)
		defer tmpFile.Close()

		content := `
on:
  github:
    push:
      init:
        sha: ${{ event.git.sha }}
    pull_request:
      init:
        other-sha: ${{ event.git.sha }}

tasks:
  - key: "test"
    run: echo 'hello world'
`
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)

		result, err := ResolveCliParamsForFile(tmpFile.Name())
		require.Error(t, err)
		require.False(t, result.Rewritten)
		require.Contains(t, err.Error(), "multiple event triggers")
	})
}
