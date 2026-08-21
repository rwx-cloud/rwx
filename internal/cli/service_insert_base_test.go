package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestService_InsertBase(t *testing.T) {
	type baseLayerSetup struct {
		s            *testSetup
		apiImage     string
		apiConfig    string
		apiArch      string
		apiCallCount int
		apiError     func(callCount int) error
		workingDir   string
		mintDir      string
	}

	setupBaseLayer := func(t *testing.T) *baseLayerSetup {
		s := setupTest(t)

		bl := &baseLayerSetup{
			s:            s,
			apiImage:     "ubuntu:24.04",
			apiConfig:    "rwx/base 1.0.0",
			apiArch:      "x86_64",
			apiCallCount: 0,
			apiError:     func(callCount int) error { return nil },
		}

		bl.workingDir = filepath.Join(s.tmp, "subdir1/subdir2")
		err := os.MkdirAll(bl.workingDir, 0o755)
		require.NoError(t, err)

		bl.mintDir = filepath.Join(s.tmp, "subdir1/.mint")
		err = os.MkdirAll(bl.mintDir, 0o755)
		require.NoError(t, err)

		err = os.Chdir(bl.workingDir)
		require.NoError(t, err)

		s.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
			bl.apiCallCount += 1
			if err := bl.apiError(bl.apiCallCount); err != nil {
				return api.DefaultBaseResult{}, err
			}

			return api.DefaultBaseResult{
				Image:  bl.apiImage,
				Config: bl.apiConfig,
				Arch:   bl.apiArch,
			}, nil
		}

		return bl
	}

	t.Run("when no yaml files found in the default directory", func(t *testing.T) {
		bl := setupBaseLayer(t)

		err := os.WriteFile(filepath.Join(bl.mintDir, "bar.json"), []byte("some json"), 0o644)
		require.NoError(t, err)

		_, err = bl.s.service.InsertBase(cli.InsertBaseConfig{})

		require.Error(t, err)
		require.Contains(t, err.Error(), fmt.Sprintf("no files provided, and no yaml files found in directory %s", bl.mintDir))
	})

	t.Run("when yaml file is actually json", func(t *testing.T) {
		bl := setupBaseLayer(t)

		err := os.WriteFile(filepath.Join(bl.mintDir, "bar.yaml"), []byte(`{
"tasks": [
  { "key": "a" },
  { "key": "b" }
]
}`), 0o644)
		require.NoError(t, err)

		_, err = bl.s.service.InsertBase(cli.InsertBaseConfig{})

		require.NoError(t, err)
		require.Equal(t, "", bl.s.mockStderr.String())
		require.Contains(t, bl.s.mockStdout.String(), "No run files needed base updates")
	})

	t.Run("when yaml file doesn't include base", func(t *testing.T) {
		bl := setupBaseLayer(t)

		err := os.WriteFile(filepath.Join(bl.mintDir, "foo.txt"), []byte("some txt"), 0o644)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(bl.mintDir, "bar.yaml"), []byte(`
tasks:
  - key: a
  - key: b
`), 0o644)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(bl.mintDir, "baz.yaml"), []byte(`
not-my-key:
  - key: qux
    call: mint/setup-node 1.2.3
`), 0o644)
		require.NoError(t, err)

		t.Run("adds base to file", func(t *testing.T) {
			_, err = bl.s.service.InsertBase(cli.InsertBaseConfig{})
			require.NoError(t, err)

			var contents []byte

			contents, err = os.ReadFile(filepath.Join(bl.mintDir, "bar.yaml"))
			require.NoError(t, err)
			require.Equal(t, `base:
  image: ubuntu:24.04
  config: rwx/base 1.0.0

tasks:
  - key: a
  - key: b
`, string(contents))

			require.Equal(t, fmt.Sprintf(
				"Updated base in the following run definitions:\n%s\n",
				"\t../.mint/bar.yaml → ubuntu:24.04",
			), bl.s.mockStdout.String())

			contents, err = os.ReadFile(filepath.Join(bl.mintDir, "baz.yaml"))
			require.NoError(t, err)
			require.Equal(t, `
not-my-key:
  - key: qux
    call: mint/setup-node 1.2.3
`, string(contents))
		})

		t.Run("adds base to only a targeted file", func(t *testing.T) {
			bl.s.mockStdout.Reset()

			err := os.WriteFile(filepath.Join(bl.mintDir, "bar.yaml"), []byte(`
tasks:
  - key: a
  - key: b
`), 0o644)
			require.NoError(t, err)

			originalQuxContents := `
tasks:
  - key: a
  - key: b
`
			err = os.WriteFile(filepath.Join(bl.mintDir, "qux.yaml"), []byte(originalQuxContents), 0o644)
			require.NoError(t, err)

			_, err = bl.s.service.InsertBase(cli.InsertBaseConfig{
				Files: []string{"../.mint/bar.yaml"},
			})
			require.NoError(t, err)

			var contents []byte

			contents, err = os.ReadFile(filepath.Join(bl.mintDir, "bar.yaml"))
			require.NoError(t, err)
			require.Equal(t, `base:
  image: ubuntu:24.04
  config: rwx/base 1.0.0

tasks:
  - key: a
  - key: b
`, string(contents))

			require.Equal(t, fmt.Sprintf(
				"Updated base in the following run definitions:\n%s\n",
				"\t../.mint/bar.yaml → ubuntu:24.04",
			), bl.s.mockStdout.String())

			contents, err = os.ReadFile(filepath.Join(bl.mintDir, "qux.yaml"))
			require.NoError(t, err)
			require.Equal(t, originalQuxContents, string(contents))
		})

		t.Run("errors when given a file that does not exist", func(t *testing.T) {
			_, err := bl.s.service.InsertBase(cli.InsertBaseConfig{
				Files: []string{"does-not-exist.yaml"},
			})
			require.Error(t, err)
			require.Equal(t, "reading rwx directory entries at does-not-exist.yaml: file does not exist", err.Error())
		})
	})

	t.Run("when yaml file has a base with deprecated os and tag", func(t *testing.T) {
		bl := setupBaseLayer(t)

		err := os.WriteFile(filepath.Join(bl.mintDir, "ci.yaml"), []byte(`on:
  github:
    push: {}

base:
  os: ubuntu 24.04
  tag: 1.2

tasks:
  - key: a
  - key: b
`), 0o644)
		require.NoError(t, err)

		_, err = bl.s.service.InsertBase(cli.InsertBaseConfig{})
		require.NoError(t, err)

		contents, err := os.ReadFile(filepath.Join(bl.mintDir, "ci.yaml"))
		require.NoError(t, err)
		require.Equal(t, `on:
  github:
    push: {}

base:
  image: ubuntu:24.04
  config: rwx/base 1.0.0

tasks:
  - key: a
  - key: b
`, string(contents))

		require.Contains(t, bl.s.mockStdout.String(), "Updated base in the following run definitions")
		require.Contains(t, bl.s.mockStdout.String(), "ci.yaml → ubuntu:24.04")
	})

	t.Run("when yaml file has a base with deprecated os, tag, and arch", func(t *testing.T) {
		bl := setupBaseLayer(t)

		err := os.WriteFile(filepath.Join(bl.mintDir, "ci.yaml"), []byte(`base:
  os: ubuntu 24.04
  tag: 1.2
  arch: arm64

tasks:
  - key: a
`), 0o644)
		require.NoError(t, err)

		_, err = bl.s.service.InsertBase(cli.InsertBaseConfig{})
		require.NoError(t, err)

		contents, err := os.ReadFile(filepath.Join(bl.mintDir, "ci.yaml"))
		require.NoError(t, err)
		require.Equal(t, `base:
  image: ubuntu:24.04
  config: rwx/base 1.0.0
  arch: arm64

tasks:
  - key: a
`, string(contents))
	})

	t.Run("when yaml file has a base with only tag (no os)", func(t *testing.T) {
		bl := setupBaseLayer(t)

		err := os.WriteFile(filepath.Join(bl.mintDir, "ci.yaml"), []byte(`base:
  image: debian:12
  tag: 1.2

tasks:
  - key: a
`), 0o644)
		require.NoError(t, err)

		_, err = bl.s.service.InsertBase(cli.InsertBaseConfig{})
		require.NoError(t, err)

		contents, err := os.ReadFile(filepath.Join(bl.mintDir, "ci.yaml"))
		require.NoError(t, err)
		require.Equal(t, `base:
  image: debian:12
  config: rwx/base 1.0.0

tasks:
  - key: a
`, string(contents))
	})

	t.Run("when yaml file has a modern base (no deprecated fields)", func(t *testing.T) {
		bl := setupBaseLayer(t)

		err := os.WriteFile(filepath.Join(bl.mintDir, "ci.yaml"), []byte(`base:
  image: ubuntu:24.04
  config: rwx/base 1.0.0

tasks:
  - key: a
`), 0o644)
		require.NoError(t, err)

		_, err = bl.s.service.InsertBase(cli.InsertBaseConfig{})
		require.NoError(t, err)

		contents, err := os.ReadFile(filepath.Join(bl.mintDir, "ci.yaml"))
		require.NoError(t, err)
		require.Equal(t, `base:
  image: ubuntu:24.04
  config: rwx/base 1.0.0

tasks:
  - key: a
`, string(contents))

		require.Contains(t, bl.s.mockStdout.String(), "No run files needed base updates")
	})

	t.Run("with multiple yaml files", func(t *testing.T) {
		bl := setupBaseLayer(t)

		err := os.WriteFile(filepath.Join(bl.mintDir, "one.yaml"), []byte(`tasks:
  - key: a
  - key: b
`), 0o644)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(bl.mintDir, "two.yaml"), []byte(`tasks:
  - key: c
  - key: d
`), 0o644)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(bl.mintDir, "three.yaml"), []byte(`tasks:
  - key: e
  - key: f
`), 0o644)
		require.NoError(t, err)

		t.Run("updates all files", func(t *testing.T) {
			_, err = bl.s.service.InsertBase(cli.InsertBaseConfig{})
			require.NoError(t, err)

			var contents []byte

			contents, err = os.ReadFile(filepath.Join(bl.mintDir, "one.yaml"))
			require.NoError(t, err)
			require.Equal(t, `base:
  image: ubuntu:24.04
  config: rwx/base 1.0.0

tasks:
  - key: a
  - key: b
`, string(contents))

			contents, err = os.ReadFile(filepath.Join(bl.mintDir, "two.yaml"))
			require.NoError(t, err)
			require.Equal(t, `base:
  image: ubuntu:24.04
  config: rwx/base 1.0.0

tasks:
  - key: c
  - key: d
`, string(contents))

			contents, err = os.ReadFile(filepath.Join(bl.mintDir, "three.yaml"))
			require.NoError(t, err)
			require.Equal(t, `base:
  image: ubuntu:24.04
  config: rwx/base 1.0.0

tasks:
  - key: e
  - key: f
`, string(contents))

			require.Equal(t, fmt.Sprintf(
				"Updated base in the following run definitions:\n%s\n%s\n%s\n",
				"\t../.mint/one.yaml → ubuntu:24.04",
				"\t../.mint/three.yaml → ubuntu:24.04",
				"\t../.mint/two.yaml → ubuntu:24.04",
			), bl.s.mockStdout.String())
		})

		t.Run("when the api request to get the default base fails", func(t *testing.T) {
			bl.apiCallCount = 0

			err := os.WriteFile(filepath.Join(bl.mintDir, "one.yaml"), []byte(`tasks:
  - key: a
  - key: b
`), 0o644)
			require.NoError(t, err)

			contentsOne, err := os.ReadFile(filepath.Join(bl.mintDir, "one.yaml"))
			require.NoError(t, err)

			bl.apiError = func(callCount int) error {
				return errors.New("API request failed")
			}

			_, err = bl.s.service.InsertBase(cli.InsertBaseConfig{})
			require.Error(t, err)
			require.Contains(t, err.Error(), "API request failed")

			var contents []byte

			contents, err = os.ReadFile(filepath.Join(bl.mintDir, "one.yaml"))
			require.NoError(t, err)
			require.Equal(t, string(contentsOne), string(contents))
		})
	})

	t.Run("when yaml file with only embedded runs doesn't include base", func(t *testing.T) {
		bl := setupBaseLayer(t)

		err := os.WriteFile(filepath.Join(bl.mintDir, "foo.yaml"), []byte(`
tasks:
  - key: a
    call: ${{ run.dir }}/bar.yaml
`), 0o644)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(bl.mintDir, "bar.yaml"), []byte(`
tasks:
  - key: b
    run: /bin/true
`), 0o644)
		require.NoError(t, err)

		t.Run("does not add base to file", func(t *testing.T) {
			_, err = bl.s.service.InsertBase(cli.InsertBaseConfig{})
			require.NoError(t, err)

			var contents []byte

			contents, err = os.ReadFile(filepath.Join(bl.mintDir, "foo.yaml"))
			require.NoError(t, err)
			require.Equal(t, `
tasks:
  - key: a
    call: ${{ run.dir }}/bar.yaml
`, string(contents))
		})
	})

	t.Run("when yaml file has a top-level package key and doesn't include base", func(t *testing.T) {
		bl := setupBaseLayer(t)

		err := os.WriteFile(filepath.Join(bl.mintDir, "foo.yaml"), []byte(`
package:
  name: my-package
  version: 1.0.0

tasks:
  - key: a
    run: /bin/true
`), 0o644)
		require.NoError(t, err)

		t.Run("does not add base to file", func(t *testing.T) {
			_, err = bl.s.service.InsertBase(cli.InsertBaseConfig{})
			require.NoError(t, err)

			var contents []byte

			contents, err = os.ReadFile(filepath.Join(bl.mintDir, "foo.yaml"))
			require.NoError(t, err)
			require.Equal(t, `
package:
  name: my-package
  version: 1.0.0

tasks:
  - key: a
    run: /bin/true
`, string(contents))
		})
	})

	t.Run("when yaml file calls into the packages directory and doesn't include base", func(t *testing.T) {
		bl := setupBaseLayer(t)

		err := os.WriteFile(filepath.Join(bl.mintDir, "foo.yaml"), []byte(`tasks:
  - key: a
    call: ${{ run.dir }}/packages/my-package
`), 0o644)
		require.NoError(t, err)

		t.Run("adds base to file", func(t *testing.T) {
			_, err = bl.s.service.InsertBase(cli.InsertBaseConfig{})
			require.NoError(t, err)

			var contents []byte

			contents, err = os.ReadFile(filepath.Join(bl.mintDir, "foo.yaml"))
			require.NoError(t, err)
			require.Equal(t, `base:
  image: ubuntu:24.04
  config: rwx/base 1.0.0

tasks:
  - key: a
    call: ${{ run.dir }}/packages/my-package
`, string(contents))
		})
	})

	t.Run("when yaml file calls with an expression and 'with' and doesn't include base", func(t *testing.T) {
		bl := setupBaseLayer(t)

		err := os.WriteFile(filepath.Join(bl.mintDir, "foo.yaml"), []byte(`tasks:
  - key: a
    call: ${{ run.dir }}/my-package
    with:
      some-param: some-value
`), 0o644)
		require.NoError(t, err)

		t.Run("adds base to file", func(t *testing.T) {
			_, err = bl.s.service.InsertBase(cli.InsertBaseConfig{})
			require.NoError(t, err)

			var contents []byte

			contents, err = os.ReadFile(filepath.Join(bl.mintDir, "foo.yaml"))
			require.NoError(t, err)
			require.Equal(t, `base:
  image: ubuntu:24.04
  config: rwx/base 1.0.0

tasks:
  - key: a
    call: ${{ run.dir }}/my-package
    with:
      some-param: some-value
`, string(contents))
		})
	})

	t.Run("when yaml file calls with an expression and 'use' and doesn't include base", func(t *testing.T) {
		bl := setupBaseLayer(t)

		err := os.WriteFile(filepath.Join(bl.mintDir, "foo.yaml"), []byte(`tasks:
  - key: a
    use: setup
    call: ${{ run.dir }}/my-package
`), 0o644)
		require.NoError(t, err)

		t.Run("adds base to file", func(t *testing.T) {
			_, err = bl.s.service.InsertBase(cli.InsertBaseConfig{})
			require.NoError(t, err)

			var contents []byte

			contents, err = os.ReadFile(filepath.Join(bl.mintDir, "foo.yaml"))
			require.NoError(t, err)
			require.Equal(t, `base:
  image: ubuntu:24.04
  config: rwx/base 1.0.0

tasks:
  - key: a
    use: setup
    call: ${{ run.dir }}/my-package
`, string(contents))
		})
	})
}
