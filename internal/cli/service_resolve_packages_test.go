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

func TestService_ResolvingPackages(t *testing.T) {
	t.Run("skips call values containing expressions", func(t *testing.T) {
		s := setupTest(t)

		s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
			return &api.PackageVersionsResult{
				LatestMajor: map[string]string{"nodejs/install": "1.2.3"},
			}, nil
		}

		originalContents := `
tasks:
  - key: export
    run: echo "rwx/greeting" > $RWX_VALUES/package-name
  - key: embedded
    call: ${{ run.dir }}/integration/run-test.yml
  - key: dynamic
    call: ${{ tasks.export.values.package-name }} 1.0.6
  - key: package
    call: nodejs/install
`
		err := os.WriteFile(filepath.Join(s.tmp, "foo.yaml"), []byte(originalContents), 0o644)
		require.NoError(t, err)

		_, err = s.service.ResolvePackages(cli.ResolvePackagesConfig{
			RwxDirectory:        s.tmp,
			LatestVersionPicker: cli.PickLatestMajorVersion,
		})
		require.NoError(t, err)

		contents, err := os.ReadFile(filepath.Join(s.tmp, "foo.yaml"))
		require.NoError(t, err)
		require.Contains(t, string(contents), "${{ run.dir }}/integration/run-test.yml")
		require.Contains(t, string(contents), "${{ tasks.export.values.package-name }} 1.0.6")
		require.Contains(t, string(contents), "nodejs/install 1.2.3")
		require.NotContains(t, s.mockStderr.String(), "Unable to find the package")
	})

	t.Run("when no files provided", func(t *testing.T) {
		t.Run("when no yaml files found in the default directory", func(t *testing.T) {
			s := setupTest(t)

			mintDir := s.tmp

			err := os.WriteFile(filepath.Join(mintDir, "foo.txt"), []byte("some txt"), 0o644)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(mintDir, "bar.json"), []byte("some json"), 0o644)
			require.NoError(t, err)

			_, err = s.service.ResolvePackages(cli.ResolvePackagesConfig{
				RwxDirectory:        mintDir,
				LatestVersionPicker: cli.PickLatestMajorVersion,
			})

			require.Error(t, err)
			require.Contains(t, err.Error(), fmt.Sprintf("no files provided, and no yaml files found in directory %s", mintDir))
		})

		t.Run("when yaml files are found in the specified directory", func(t *testing.T) {
			s := setupTest(t)

			mintDir := s.tmp

			err := os.WriteFile(filepath.Join(mintDir, "foo.txt"), []byte("some txt"), 0o644)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(mintDir, "bar.yaml"), []byte(`
tasks:
  - key: foo
    call: nodejs/install 1.2.3
`), 0o644)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(mintDir, "baz.yaml"), []byte(`
tasks:
  - key: foo
    call: nodejs/install
`), 0o644)
			require.NoError(t, err)

			nestedDir := filepath.Join(mintDir, "some", "nested", "dir")
			err = os.MkdirAll(nestedDir, 0o755)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(nestedDir, "tasks.yaml"), []byte(`
tasks:
  - key: foo
    call: nodejs/install
`), 0o644)
			require.NoError(t, err)

			s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
				return &api.PackageVersionsResult{
					LatestMajor: map[string]string{"nodejs/install": "1.3.0"},
				}, nil
			}

			_, err = s.service.ResolvePackages(cli.ResolvePackagesConfig{
				RwxDirectory:        mintDir,
				LatestVersionPicker: cli.PickLatestMajorVersion,
			})
			require.NoError(t, err)

			var contents []byte

			contents, err = os.ReadFile(filepath.Join(mintDir, "bar.yaml"))
			require.NoError(t, err)
			require.Contains(t, string(contents), "nodejs/install 1.2.3")

			contents, err = os.ReadFile(filepath.Join(mintDir, "baz.yaml"))
			require.NoError(t, err)
			require.Contains(t, string(contents), "nodejs/install 1.3.0")

			contents, err = os.ReadFile(filepath.Join(mintDir, "some", "nested", "dir", "tasks.yaml"))
			require.NoError(t, err)
			require.Contains(t, string(contents), "nodejs/install 1.3.0")
		})
	})

	t.Run("with files", func(t *testing.T) {
		t.Run("when the package versions cannot be retrieved", func(t *testing.T) {
			s := setupTest(t)

			s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
				return nil, errors.New("cannot get package versions")
			}

			err := os.WriteFile(filepath.Join(s.tmp, "foo.yaml"), []byte(""), 0o644)
			require.NoError(t, err)

			_, err = s.service.ResolvePackages(cli.ResolvePackagesConfig{
				RwxDirectory:        s.tmp,
				LatestVersionPicker: cli.PickLatestMajorVersion,
			})

			require.Error(t, err)
			require.Contains(t, err.Error(), "cannot get package versions")
		})

		t.Run("when all packages have a version", func(t *testing.T) {
			s := setupTest(t)

			s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
				return &api.PackageVersionsResult{
					LatestMajor: map[string]string{"nodejs/install": "1.3.0"},
				}, nil
			}

			err := os.WriteFile(filepath.Join(s.tmp, "foo.yaml"), []byte(`
tasks:
  - key: foo
    call: nodejs/install 1.2.3
`), 0o644)
			require.NoError(t, err)

			_, err = s.service.ResolvePackages(cli.ResolvePackagesConfig{
				RwxDirectory:        s.tmp,
				LatestVersionPicker: cli.PickLatestMajorVersion,
			})
			require.NoError(t, err)

			contents, err := os.ReadFile(filepath.Join(s.tmp, "foo.yaml"))
			require.NoError(t, err)
			require.Equal(t, `
tasks:
  - key: foo
    call: nodejs/install 1.2.3
`, string(contents))

			require.Contains(t, s.mockStdout.String(), "No packages to resolve.")
		})

		t.Run("when there are packages to resolve across multiple files", func(t *testing.T) {
			s := setupTest(t)

			s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
				return &api.PackageVersionsResult{
					LatestMajor: map[string]string{
						"nodejs/install": "1.2.3",
						"ruby/install":   "1.0.1",
						"golang/install": "1.3.5",
					},
				}, nil
			}

			originalFooContents := `
tasks:
  - key: foo
    call: nodejs/install
  - key: bar
    call: ruby/install 0.0.1
  - key: baz
    call: golang/install
`
			err := os.WriteFile(filepath.Join(s.tmp, "foo.yaml"), []byte(originalFooContents), 0o644)
			require.NoError(t, err)

			originalBarContents := `
tasks:
  - key: foo
    call: ruby/install
`
			err = os.WriteFile(filepath.Join(s.tmp, "bar.yaml"), []byte(originalBarContents), 0o644)
			require.NoError(t, err)

			t.Run("updates all files", func(t *testing.T) {
				_, err = s.service.ResolvePackages(cli.ResolvePackagesConfig{
					RwxDirectory:        s.tmp,
					LatestVersionPicker: cli.PickLatestMajorVersion,
				})
				require.NoError(t, err)

				var contents []byte

				contents, err = os.ReadFile(filepath.Join(s.tmp, "foo.yaml"))
				require.NoError(t, err)
				require.Equal(t, `tasks:
  - key: foo
    call: nodejs/install 1.2.3
  - key: bar
    call: ruby/install 0.0.1
  - key: baz
    call: golang/install 1.3.5
`, string(contents))

				contents, err = os.ReadFile(filepath.Join(s.tmp, "bar.yaml"))
				require.NoError(t, err)
				require.Equal(t, `tasks:
  - key: foo
    call: ruby/install 1.0.1
`, string(contents))
			})

			t.Run("indicates packages were resolved", func(t *testing.T) {
				err := os.WriteFile(filepath.Join(s.tmp, "foo.yaml"), []byte(originalFooContents), 0o644)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(s.tmp, "bar.yaml"), []byte(originalBarContents), 0o644)
				require.NoError(t, err)

				_, err = s.service.ResolvePackages(cli.ResolvePackagesConfig{
					RwxDirectory:        s.tmp,
					LatestVersionPicker: cli.PickLatestMajorVersion,
				})

				require.NoError(t, err)
				require.Contains(t, s.mockStdout.String(), "Resolved the following packages:")
				require.Contains(t, s.mockStdout.String(), "golang/install → 1.3.5")
				require.Contains(t, s.mockStdout.String(), "nodejs/install → 1.2.3")
				require.Contains(t, s.mockStdout.String(), "ruby/install → 1.0.1")
			})

			t.Run("when a single file is targeted", func(t *testing.T) {
				err := os.WriteFile(filepath.Join(s.tmp, "foo.yaml"), []byte(originalFooContents), 0o644)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(s.tmp, "bar.yaml"), []byte(originalBarContents), 0o644)
				require.NoError(t, err)

				_, err = s.service.ResolvePackages(cli.ResolvePackagesConfig{
					RwxDirectory:        s.tmp,
					Files:               []string{filepath.Join(s.tmp, "bar.yaml")},
					LatestVersionPicker: cli.PickLatestMajorVersion,
				})
				require.NoError(t, err)

				contents, err := os.ReadFile(filepath.Join(s.tmp, "foo.yaml"))
				require.NoError(t, err)
				require.Equal(t, originalFooContents, string(contents))

				contents, err = os.ReadFile(filepath.Join(s.tmp, "bar.yaml"))
				require.NoError(t, err)
				require.Equal(t, `tasks:
  - key: foo
    call: ruby/install 1.0.1
`, string(contents))
			})
		})
	})

	t.Run("resolves base.config package reference", func(t *testing.T) {
		t.Run("pins a versionless base.config to the latest version", func(t *testing.T) {
			s := setupTest(t)

			s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
				return &api.PackageVersionsResult{
					LatestMajor: map[string]string{"rwx/base": "1.2.3"},
				}, nil
			}

			originalContents := `base:
  image: ubuntu:24.04
  config: rwx/base

tasks:
  - key: foo
    run: echo hello
`
			err := os.WriteFile(filepath.Join(s.tmp, "foo.yaml"), []byte(originalContents), 0o644)
			require.NoError(t, err)

			_, err = s.service.ResolvePackages(cli.ResolvePackagesConfig{
				Files:               []string{filepath.Join(s.tmp, "foo.yaml")},
				LatestVersionPicker: cli.PickLatestMajorVersion,
			})
			require.NoError(t, err)

			contents, err := os.ReadFile(filepath.Join(s.tmp, "foo.yaml"))
			require.NoError(t, err)
			require.Contains(t, string(contents), "config: rwx/base 1.2.3")
		})

		t.Run("leaves an already-pinned base.config untouched", func(t *testing.T) {
			s := setupTest(t)

			s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
				return &api.PackageVersionsResult{
					LatestMajor: map[string]string{"rwx/base": "1.2.3"},
				}, nil
			}

			originalContents := `base:
  image: ubuntu:24.04
  config: rwx/base 1.0.0

tasks:
  - key: foo
    run: echo hello
`
			err := os.WriteFile(filepath.Join(s.tmp, "foo.yaml"), []byte(originalContents), 0o644)
			require.NoError(t, err)

			_, err = s.service.ResolvePackages(cli.ResolvePackagesConfig{
				Files:               []string{filepath.Join(s.tmp, "foo.yaml")},
				LatestVersionPicker: cli.PickLatestMajorVersion,
			})
			require.NoError(t, err)

			contents, err := os.ReadFile(filepath.Join(s.tmp, "foo.yaml"))
			require.NoError(t, err)
			require.Equal(t, originalContents, string(contents))
		})

		t.Run("leaves config: none untouched", func(t *testing.T) {
			s := setupTest(t)

			s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
				return &api.PackageVersionsResult{
					LatestMajor: map[string]string{"nodejs/install": "1.2.3"},
				}, nil
			}

			originalContents := `base:
  image: ubuntu:24.04
  config: none

tasks:
  - key: foo
    call: nodejs/install
`
			err := os.WriteFile(filepath.Join(s.tmp, "foo.yaml"), []byte(originalContents), 0o644)
			require.NoError(t, err)

			_, err = s.service.ResolvePackages(cli.ResolvePackagesConfig{
				Files:               []string{filepath.Join(s.tmp, "foo.yaml")},
				LatestVersionPicker: cli.PickLatestMajorVersion,
			})
			require.NoError(t, err)

			contents, err := os.ReadFile(filepath.Join(s.tmp, "foo.yaml"))
			require.NoError(t, err)
			require.Contains(t, string(contents), "config: none")
			require.Contains(t, string(contents), "call: nodejs/install 1.2.3")
		})
	})
}
