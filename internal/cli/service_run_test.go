package cli_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/mocks"
	"github.com/stretchr/testify/require"
)

var _ cli.APIClient = (*mocks.API)(nil)

func TestService_InitiatingRun(t *testing.T) {
	t.Run("with a specific mint file and no specific directory", func(t *testing.T) {
		t.Run("with a .mint directory", func(t *testing.T) {
			t.Run("when a directory with files is found", func(t *testing.T) {
				s := setupTest(t)

				runConfig := cli.InitiateRunConfig{}
				baseSpec := "base:\n  image: ubuntu:24.04\n  config: rwx/base 1.0.0\n"
				getDefaultBaseCalled := false

				s.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
					getDefaultBaseCalled = true
					return api.DefaultBaseResult{
						Image:  "ubuntu:24.04",
						Config: "rwx/base 1.0.0",
						Arch:   "x86_64",
					}, nil
				}

				getPackageVersionsCalled := false
				majorPackageVersions := make(map[string]string)
				minorPackageVersions := make(map[string]map[string]string)

				s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
					getPackageVersionsCalled = true
					return &api.PackageVersionsResult{
						LatestMajor: majorPackageVersions,
						LatestMinor: minorPackageVersions,
					}, nil
				}

				branch := "main"
				s.mockGit.MockGetBranch = branch
				sha := "e86ec9c4802fb5f6c7d7220c5f7356278e7ace5a"
				s.mockGit.MockGetCommit = sha
				originUrl := "git@github.com:rwx-cloud/rwx.git"
				s.mockGit.MockGetOriginUrl = originUrl

				originalSpecifiedFileContent := "on:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n\ntasks:\n  - key: foo\n    run: echo 'bar'\n" + baseSpec
				originalRwxDirFileContent := "tasks:\n  - key: mintdir\n    run: echo 'mintdir'\n" + baseSpec
				var receivedSpecifiedFileContent string
				var receivedRwxDir []api.RwxDirectoryEntry

				workingDir := filepath.Join(s.tmp, "some", "path", "to", "working", "directory")
				err := os.MkdirAll(workingDir, 0o755)
				require.NoError(t, err)

				err = os.Chdir(workingDir)
				require.NoError(t, err)

				mintDir := filepath.Join(s.tmp, "some", "path", "to", ".mint")
				err = os.MkdirAll(mintDir, 0o755)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(workingDir, "ci.yml"), []byte(originalSpecifiedFileContent), 0o644)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(mintDir, "mintdir-tasks.yml"), []byte(originalRwxDirFileContent), 0o644)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(mintDir, "mintdir-tasks.json"), []byte("some json"), 0o644)
				require.NoError(t, err)

				nestedDir := filepath.Join(mintDir, "some", "nested", "path")
				err = os.MkdirAll(nestedDir, 0o755)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(nestedDir, "tasks.yaml"), []byte("some nested yaml"), 0o644)
				require.NoError(t, err)

				runConfig.MintFilePath = "ci.yml"
				runConfig.RwxDirectory = ""

				s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
					require.Len(t, cfg.TaskDefinitions, 1)
					require.Equal(t, runConfig.MintFilePath, cfg.TaskDefinitions[0].Path)
					require.Len(t, cfg.RwxDirectory, 9)
					require.Equal(t, ".", cfg.RwxDirectory[0].Path)
					require.Equal(t, "mintdir-tasks.json", cfg.RwxDirectory[1].Path)
					require.Equal(t, "mintdir-tasks.yml", cfg.RwxDirectory[2].Path)
					require.Equal(t, "some", cfg.RwxDirectory[3].Path)
					require.Equal(t, "some/nested", cfg.RwxDirectory[4].Path)
					require.Equal(t, "some/nested/path", cfg.RwxDirectory[5].Path)
					require.Equal(t, "some/nested/path/tasks.yaml", cfg.RwxDirectory[6].Path)
					require.Equal(t, ".workflow", cfg.RwxDirectory[7].Path)
					require.Equal(t, "dir", cfg.RwxDirectory[7].Type)
					require.Equal(t, ".workflow/ci.yml", cfg.RwxDirectory[8].Path)
					require.Equal(t, "file", cfg.RwxDirectory[8].Type)
					require.True(t, cfg.UseCache)
					require.NotNil(t, cfg.Git)
					require.Equal(t, branch, cfg.Git.Branch)
					require.Equal(t, sha, cfg.Git.Sha)
					require.Equal(t, originUrl, cfg.Git.OriginUrl)
					receivedSpecifiedFileContent = cfg.TaskDefinitions[0].FileContents
					receivedRwxDir = cfg.RwxDirectory
					return &api.InitiateRunResult{
						RunID:            "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
						RunURL:           "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
						TargetedTaskKeys: []string{},
						DefinitionPath:   ".mint/ci.yml",
					}, nil
				}

				_, err = s.service.InitiateRun(runConfig)
				require.NoError(t, err)

				require.Equal(t, originalSpecifiedFileContent, receivedSpecifiedFileContent)
				require.NotNil(t, receivedRwxDir)
				require.Equal(t, "", receivedRwxDir[0].FileContents)
				require.Equal(t, "some json", receivedRwxDir[1].FileContents)
				require.Equal(t, originalRwxDirFileContent, receivedRwxDir[2].FileContents)
				require.Equal(t, "", receivedRwxDir[3].FileContents)
				require.Equal(t, "", receivedRwxDir[4].FileContents)
				require.Equal(t, "", receivedRwxDir[5].FileContents)
				require.Equal(t, "some nested yaml", receivedRwxDir[6].FileContents)
				require.Equal(t, "", receivedRwxDir[7].FileContents)
				require.Equal(t, originalSpecifiedFileContent, receivedRwxDir[8].FileContents)

				_ = getDefaultBaseCalled
				_ = getPackageVersionsCalled
			})

			t.Run("when an empty directory is found", func(t *testing.T) {
				s := setupTest(t)

				runConfig := cli.InitiateRunConfig{}
				baseSpec := "base:\n  image: ubuntu:24.04\n  config: rwx/base 1.0.0\n"

				s.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
					return api.DefaultBaseResult{
						Image:  "ubuntu:24.04",
						Config: "rwx/base 1.0.0",
						Arch:   "x86_64",
					}, nil
				}

				s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
					return &api.PackageVersionsResult{
						LatestMajor: make(map[string]string),
						LatestMinor: make(map[string]map[string]string),
					}, nil
				}

				originalSpecifiedFileContent := "on:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n\ntasks:\n  - key: foo\n    run: echo 'bar'\n" + baseSpec
				var receivedSpecifiedFileContent string

				workingDir := filepath.Join(s.tmp, "some", "path", "to", "working", "directory")
				err := os.MkdirAll(workingDir, 0o755)
				require.NoError(t, err)

				err = os.Chdir(workingDir)
				require.NoError(t, err)

				mintDir := filepath.Join(s.tmp, "some", "path", "to", ".mint")
				err = os.MkdirAll(mintDir, 0o755)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(workingDir, "ci.yml"), []byte(originalSpecifiedFileContent), 0o644)
				require.NoError(t, err)

				runConfig.MintFilePath = "ci.yml"
				runConfig.RwxDirectory = ""

				s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
					require.Len(t, cfg.TaskDefinitions, 1)
					require.Equal(t, runConfig.MintFilePath, cfg.TaskDefinitions[0].Path)
					require.Len(t, cfg.RwxDirectory, 3)
					require.Equal(t, ".", cfg.RwxDirectory[0].Path)
					require.Equal(t, ".workflow", cfg.RwxDirectory[1].Path)
					require.Equal(t, "dir", cfg.RwxDirectory[1].Type)
					require.Equal(t, ".workflow/ci.yml", cfg.RwxDirectory[2].Path)
					require.Equal(t, "file", cfg.RwxDirectory[2].Type)
					require.True(t, cfg.UseCache)
					receivedSpecifiedFileContent = cfg.TaskDefinitions[0].FileContents
					return &api.InitiateRunResult{
						RunID:            "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
						RunURL:           "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
						TargetedTaskKeys: []string{},
						DefinitionPath:   ".mint/ci.yml",
					}, nil
				}

				_, err = s.service.InitiateRun(runConfig)
				require.NoError(t, err)

				require.Equal(t, originalSpecifiedFileContent, receivedSpecifiedFileContent)
			})

			t.Run("when a directory is not found", func(t *testing.T) {
				s := setupTest(t)

				runConfig := cli.InitiateRunConfig{}
				baseSpec := "base:\n  image: ubuntu:24.04\n  config: rwx/base 1.0.0\n"
				getDefaultBaseCalled := false

				s.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
					getDefaultBaseCalled = true
					return api.DefaultBaseResult{
						Image:  "ubuntu:24.04",
						Config: "rwx/base 1.0.0",
						Arch:   "x86_64",
					}, nil
				}

				s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
					return &api.PackageVersionsResult{
						LatestMajor: make(map[string]string),
						LatestMinor: make(map[string]map[string]string),
					}, nil
				}

				originalSpecifiedFileContent := "on:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n\ntasks:\n  - key: foo\n    run: echo 'bar'\n" + baseSpec
				var receivedSpecifiedFileContent string

				workingDir := filepath.Join(s.tmp, "some", "path", "to", "working", "directory")
				err := os.MkdirAll(workingDir, 0o755)
				require.NoError(t, err)

				err = os.Chdir(workingDir)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(workingDir, "ci.yml"), []byte(originalSpecifiedFileContent), 0o644)
				require.NoError(t, err)

				runConfig.MintFilePath = "ci.yml"
				runConfig.RwxDirectory = ""

				s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
					require.Len(t, cfg.TaskDefinitions, 1)
					require.Equal(t, runConfig.MintFilePath, cfg.TaskDefinitions[0].Path)
					// When no .rwx directory is specified, a temporary directory is created
					require.Len(t, cfg.RwxDirectory, 3)
					require.Equal(t, ".", cfg.RwxDirectory[0].Path)
					require.Equal(t, "dir", cfg.RwxDirectory[0].Type)
					require.Equal(t, ".workflow", cfg.RwxDirectory[1].Path)
					require.Equal(t, "dir", cfg.RwxDirectory[1].Type)
					require.Equal(t, ".workflow/ci.yml", cfg.RwxDirectory[2].Path)
					require.Equal(t, "file", cfg.RwxDirectory[2].Type)
					require.True(t, cfg.UseCache)
					receivedSpecifiedFileContent = cfg.TaskDefinitions[0].FileContents
					return &api.InitiateRunResult{
						RunID:            "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
						RunURL:           "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
						TargetedTaskKeys: []string{},
						DefinitionPath:   ".mint/ci.yml",
					}, nil
				}

				_, err = s.service.InitiateRun(runConfig)
				require.NoError(t, err)

				// Verify temp .rwx directory was cleaned up
				tmpDir := os.TempDir()
				entries, err := os.ReadDir(tmpDir)
				require.NoError(t, err)
				for _, entry := range entries {
					if entry.IsDir() && strings.HasPrefix(entry.Name(), ".rwx-") {
						t.Errorf("temp .rwx directory should be cleaned up after InitiateRun, but found: %s", entry.Name())
					}
				}

				require.Equal(t, originalSpecifiedFileContent, receivedSpecifiedFileContent)
				require.False(t, getDefaultBaseCalled)
			})
		})

		t.Run("with a .rwx directory", func(t *testing.T) {
			t.Run("when a directory with files is found", func(t *testing.T) {
				s := setupTest(t)

				runConfig := cli.InitiateRunConfig{}
				baseSpec := "base:\n  image: ubuntu:24.04\n  config: rwx/base 1.0.0\n"

				s.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
					return api.DefaultBaseResult{
						Image:  "ubuntu:24.04",
						Config: "rwx/base 1.0.0",
						Arch:   "x86_64",
					}, nil
				}

				s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
					return &api.PackageVersionsResult{
						LatestMajor: make(map[string]string),
						LatestMinor: make(map[string]map[string]string),
					}, nil
				}

				originalSpecifiedFileContent := "on:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n\ntasks:\n  - key: foo\n    run: echo 'bar'\n" + baseSpec
				originalRwxDirFileContent := "tasks:\n  - key: mintdir\n    run: echo 'mintdir'\n" + baseSpec
				var receivedSpecifiedFileContent string
				var receivedRwxDir []api.RwxDirectoryEntry

				workingDir := filepath.Join(s.tmp, "some", "path", "to", "working", "directory")
				err := os.MkdirAll(workingDir, 0o755)
				require.NoError(t, err)

				err = os.Chdir(workingDir)
				require.NoError(t, err)

				rwxDir := filepath.Join(s.tmp, "some", "path", "to", ".rwx")
				err = os.MkdirAll(rwxDir, 0o755)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(workingDir, "ci.yml"), []byte(originalSpecifiedFileContent), 0o644)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(rwxDir, "mintdir-tasks.yml"), []byte(originalRwxDirFileContent), 0o644)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(rwxDir, "mintdir-tasks.json"), []byte("some json"), 0o644)
				require.NoError(t, err)

				nestedDir := filepath.Join(rwxDir, "some", "nested", "path")
				err = os.MkdirAll(nestedDir, 0o755)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(nestedDir, "tasks.yaml"), []byte("some nested yaml"), 0o644)
				require.NoError(t, err)

				runConfig.MintFilePath = "ci.yml"
				runConfig.RwxDirectory = ""

				s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
					require.Len(t, cfg.TaskDefinitions, 1)
					require.Equal(t, runConfig.MintFilePath, cfg.TaskDefinitions[0].Path)
					require.Len(t, cfg.RwxDirectory, 9)
					require.Equal(t, ".", cfg.RwxDirectory[0].Path)
					require.Equal(t, "mintdir-tasks.json", cfg.RwxDirectory[1].Path)
					require.Equal(t, "mintdir-tasks.yml", cfg.RwxDirectory[2].Path)
					require.Equal(t, "some", cfg.RwxDirectory[3].Path)
					require.Equal(t, "some/nested", cfg.RwxDirectory[4].Path)
					require.Equal(t, "some/nested/path", cfg.RwxDirectory[5].Path)
					require.Equal(t, "some/nested/path/tasks.yaml", cfg.RwxDirectory[6].Path)
					require.Equal(t, ".workflow", cfg.RwxDirectory[7].Path)
					require.Equal(t, "dir", cfg.RwxDirectory[7].Type)
					require.Equal(t, ".workflow/ci.yml", cfg.RwxDirectory[8].Path)
					require.Equal(t, "file", cfg.RwxDirectory[8].Type)
					require.True(t, cfg.UseCache)
					receivedSpecifiedFileContent = cfg.TaskDefinitions[0].FileContents
					receivedRwxDir = cfg.RwxDirectory
					return &api.InitiateRunResult{
						RunID:            "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
						RunURL:           "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
						TargetedTaskKeys: []string{},
						DefinitionPath:   ".mint/ci.yml",
					}, nil
				}

				_, err = s.service.InitiateRun(runConfig)
				require.NoError(t, err)

				require.Equal(t, originalSpecifiedFileContent, receivedSpecifiedFileContent)
				require.NotNil(t, receivedRwxDir)
				require.Equal(t, "", receivedRwxDir[0].FileContents)
				require.Equal(t, "some json", receivedRwxDir[1].FileContents)
				require.Equal(t, originalRwxDirFileContent, receivedRwxDir[2].FileContents)
				require.Equal(t, "", receivedRwxDir[3].FileContents)
				require.Equal(t, "", receivedRwxDir[4].FileContents)
				require.Equal(t, "", receivedRwxDir[5].FileContents)
				require.Equal(t, "some nested yaml", receivedRwxDir[6].FileContents)
				require.Equal(t, "", receivedRwxDir[7].FileContents)
				require.Equal(t, originalSpecifiedFileContent, receivedRwxDir[8].FileContents)
			})

			t.Run("when an empty directory is found", func(t *testing.T) {
				s := setupTest(t)

				runConfig := cli.InitiateRunConfig{}
				baseSpec := "base:\n  image: ubuntu:24.04\n  config: rwx/base 1.0.0\n"

				s.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
					return api.DefaultBaseResult{
						Image:  "ubuntu:24.04",
						Config: "rwx/base 1.0.0",
						Arch:   "x86_64",
					}, nil
				}

				s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
					return &api.PackageVersionsResult{
						LatestMajor: make(map[string]string),
						LatestMinor: make(map[string]map[string]string),
					}, nil
				}

				originalSpecifiedFileContent := "on:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n\ntasks:\n  - key: foo\n    run: echo 'bar'\n" + baseSpec
				var receivedSpecifiedFileContent string

				workingDir := filepath.Join(s.tmp, "some", "path", "to", "working", "directory")
				err := os.MkdirAll(workingDir, 0o755)
				require.NoError(t, err)

				err = os.Chdir(workingDir)
				require.NoError(t, err)

				rwxDir := filepath.Join(s.tmp, "some", "path", "to", ".rwx")
				err = os.MkdirAll(rwxDir, 0o755)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(workingDir, "ci.yml"), []byte(originalSpecifiedFileContent), 0o644)
				require.NoError(t, err)

				runConfig.MintFilePath = "ci.yml"
				runConfig.RwxDirectory = ""

				s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
					require.Len(t, cfg.TaskDefinitions, 1)
					require.Equal(t, runConfig.MintFilePath, cfg.TaskDefinitions[0].Path)
					require.Len(t, cfg.RwxDirectory, 3)
					require.Equal(t, ".", cfg.RwxDirectory[0].Path)
					require.Equal(t, ".workflow", cfg.RwxDirectory[1].Path)
					require.Equal(t, "dir", cfg.RwxDirectory[1].Type)
					require.Equal(t, ".workflow/ci.yml", cfg.RwxDirectory[2].Path)
					require.Equal(t, "file", cfg.RwxDirectory[2].Type)
					require.True(t, cfg.UseCache)
					receivedSpecifiedFileContent = cfg.TaskDefinitions[0].FileContents
					return &api.InitiateRunResult{
						RunID:            "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
						RunURL:           "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
						TargetedTaskKeys: []string{},
						DefinitionPath:   ".mint/ci.yml",
					}, nil
				}

				_, err = s.service.InitiateRun(runConfig)
				require.NoError(t, err)

				require.Equal(t, originalSpecifiedFileContent, receivedSpecifiedFileContent)
			})

			t.Run("when a directory is not found", func(t *testing.T) {
				s := setupTest(t)

				runConfig := cli.InitiateRunConfig{}
				baseSpec := "base:\n  image: ubuntu:24.04\n  config: rwx/base 1.0.0\n"
				getDefaultBaseCalled := false

				s.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
					getDefaultBaseCalled = true
					return api.DefaultBaseResult{
						Image:  "ubuntu:24.04",
						Config: "rwx/base 1.0.0",
						Arch:   "x86_64",
					}, nil
				}

				s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
					return &api.PackageVersionsResult{
						LatestMajor: make(map[string]string),
						LatestMinor: make(map[string]map[string]string),
					}, nil
				}

				originalSpecifiedFileContent := "on:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n\ntasks:\n  - key: foo\n    run: echo 'bar'\n" + baseSpec
				var receivedSpecifiedFileContent string

				workingDir := filepath.Join(s.tmp, "some", "path", "to", "working", "directory")
				err := os.MkdirAll(workingDir, 0o755)
				require.NoError(t, err)

				err = os.Chdir(workingDir)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(workingDir, "ci.yml"), []byte(originalSpecifiedFileContent), 0o644)
				require.NoError(t, err)

				runConfig.MintFilePath = "ci.yml"
				runConfig.RwxDirectory = ""

				s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
					require.Len(t, cfg.TaskDefinitions, 1)
					require.Equal(t, runConfig.MintFilePath, cfg.TaskDefinitions[0].Path)
					// When no .rwx directory is specified, a temporary directory is created
					require.Len(t, cfg.RwxDirectory, 3)
					require.Equal(t, ".", cfg.RwxDirectory[0].Path)
					require.Equal(t, "dir", cfg.RwxDirectory[0].Type)
					require.Equal(t, ".workflow", cfg.RwxDirectory[1].Path)
					require.Equal(t, "dir", cfg.RwxDirectory[1].Type)
					require.Equal(t, ".workflow/ci.yml", cfg.RwxDirectory[2].Path)
					require.Equal(t, "file", cfg.RwxDirectory[2].Type)
					require.True(t, cfg.UseCache)
					receivedSpecifiedFileContent = cfg.TaskDefinitions[0].FileContents
					return &api.InitiateRunResult{
						RunID:            "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
						RunURL:           "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
						TargetedTaskKeys: []string{},
						DefinitionPath:   ".mint/ci.yml",
					}, nil
				}

				_, err = s.service.InitiateRun(runConfig)
				require.NoError(t, err)

				// Verify temp .rwx directory was cleaned up
				tmpDir := os.TempDir()
				entries, err := os.ReadDir(tmpDir)
				require.NoError(t, err)
				for _, entry := range entries {
					if entry.IsDir() && strings.HasPrefix(entry.Name(), ".rwx-") {
						t.Errorf("temp .rwx directory should be cleaned up after InitiateRun, but found: %s", entry.Name())
					}
				}

				require.Equal(t, originalSpecifiedFileContent, receivedSpecifiedFileContent)
				require.False(t, getDefaultBaseCalled)
			})

			t.Run("when the directory includes a test-suites directory inside it", func(t *testing.T) {
				s := setupTest(t)

				runConfig := cli.InitiateRunConfig{}
				baseSpec := "base:\n  image: ubuntu:24.04\n  config: rwx/base 1.0.0\n"

				s.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
					return api.DefaultBaseResult{
						Image:  "ubuntu:24.04",
						Config: "rwx/base 1.0.0",
						Arch:   "x86_64",
					}, nil
				}

				s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
					return &api.PackageVersionsResult{
						LatestMajor: make(map[string]string),
						LatestMinor: make(map[string]map[string]string),
					}, nil
				}

				originalSpecifiedFileContent := "on:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n\ntasks:\n  - key: foo\n    run: echo 'bar'\n" + baseSpec
				originalRwxDirFileContent := "tasks:\n  - key: mintdir\n    run: echo 'mintdir'\n" + baseSpec
				var receivedSpecifiedFileContent string
				var receivedRwxDir []api.RwxDirectoryEntry

				workingDir := filepath.Join(s.tmp, "some", "path", "to", "working", "directory")
				err := os.MkdirAll(workingDir, 0o755)
				require.NoError(t, err)

				err = os.Chdir(workingDir)
				require.NoError(t, err)

				rwxDir := filepath.Join(s.tmp, "some", "path", "to", ".rwx")
				err = os.MkdirAll(rwxDir, 0o755)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(workingDir, "ci.yml"), []byte(originalSpecifiedFileContent), 0o644)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(rwxDir, "mintdir-tasks.yml"), []byte(originalRwxDirFileContent), 0o644)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(rwxDir, "mintdir-tasks.json"), []byte("some json"), 0o644)
				require.NoError(t, err)

				testSuitesDir := filepath.Join(rwxDir, "test-suites")
				err = os.MkdirAll(testSuitesDir, 0o755)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(testSuitesDir, "config.yaml"), []byte("some yaml"), 0o644)
				require.NoError(t, err)

				nestedDir := filepath.Join(rwxDir, "some", "nested", "path")
				err = os.MkdirAll(nestedDir, 0o755)
				require.NoError(t, err)

				err = os.WriteFile(filepath.Join(nestedDir, "tasks.yaml"), []byte("some nested yaml"), 0o644)
				require.NoError(t, err)

				runConfig.MintFilePath = "ci.yml"
				runConfig.RwxDirectory = ""

				s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
					require.Len(t, cfg.TaskDefinitions, 1)
					require.Equal(t, runConfig.MintFilePath, cfg.TaskDefinitions[0].Path)
					require.Len(t, cfg.RwxDirectory, 9)
					require.Equal(t, ".", cfg.RwxDirectory[0].Path)
					require.Equal(t, "mintdir-tasks.json", cfg.RwxDirectory[1].Path)
					require.Equal(t, "mintdir-tasks.yml", cfg.RwxDirectory[2].Path)
					require.Equal(t, "some", cfg.RwxDirectory[3].Path)
					require.Equal(t, "some/nested", cfg.RwxDirectory[4].Path)
					require.Equal(t, "some/nested/path", cfg.RwxDirectory[5].Path)
					require.Equal(t, "some/nested/path/tasks.yaml", cfg.RwxDirectory[6].Path)
					require.Equal(t, ".workflow", cfg.RwxDirectory[7].Path)
					require.Equal(t, "dir", cfg.RwxDirectory[7].Type)
					require.Equal(t, ".workflow/ci.yml", cfg.RwxDirectory[8].Path)
					require.Equal(t, "file", cfg.RwxDirectory[8].Type)
					require.True(t, cfg.UseCache)
					receivedSpecifiedFileContent = cfg.TaskDefinitions[0].FileContents
					receivedRwxDir = cfg.RwxDirectory
					return &api.InitiateRunResult{
						RunID:            "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
						RunURL:           "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
						TargetedTaskKeys: []string{},
						DefinitionPath:   ".mint/ci.yml",
					}, nil
				}

				_, err = s.service.InitiateRun(runConfig)
				require.NoError(t, err)

				require.Equal(t, originalSpecifiedFileContent, receivedSpecifiedFileContent)
				require.NotNil(t, receivedRwxDir)
				require.Equal(t, 9, len(receivedRwxDir))
				require.Equal(t, ".", receivedRwxDir[0].Path)
				require.Equal(t, "mintdir-tasks.json", receivedRwxDir[1].Path)
				require.Equal(t, "mintdir-tasks.yml", receivedRwxDir[2].Path)
				require.Equal(t, "some", receivedRwxDir[3].Path)
				require.Equal(t, "some/nested", receivedRwxDir[4].Path)
				require.Equal(t, "some/nested/path", receivedRwxDir[5].Path)
				require.Equal(t, "some/nested/path/tasks.yaml", receivedRwxDir[6].Path)
				require.Equal(t, ".workflow", receivedRwxDir[7].Path)
				require.Equal(t, ".workflow/ci.yml", receivedRwxDir[8].Path)
			})
		})

		t.Run("when base is missing", func(t *testing.T) {
			s := setupTest(t)

			runConfig := cli.InitiateRunConfig{}
			getDefaultBaseCalled := false

			s.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
				getDefaultBaseCalled = true
				return api.DefaultBaseResult{
					Image:  "ubuntu:24.04",
					Config: "rwx/base 1.0.0",
					Arch:   "x86_64",
				}, nil
			}

			s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
				return &api.PackageVersionsResult{
					LatestMajor: make(map[string]string),
					LatestMinor: make(map[string]map[string]string),
				}, nil
			}

			originalSpecifiedFileContent := "on:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n\ntasks:\n  - key: foo\n    run: echo 'bar'\n"
			var receivedSpecifiedFileContent string
			var receivedRwxDirectoryFileContent string

			mintDir := filepath.Join(s.tmp, ".mint")
			err := os.MkdirAll(mintDir, 0o755)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(mintDir, "foo.yml"), []byte(originalSpecifiedFileContent), 0o644)
			require.NoError(t, err)

			runConfig.MintFilePath = ".mint/foo.yml"
			runConfig.RwxDirectory = ".mint"

			s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
				require.Len(t, cfg.TaskDefinitions, 1)
				require.Equal(t, runConfig.MintFilePath, cfg.TaskDefinitions[0].Path)
				require.Len(t, cfg.RwxDirectory, 2)
				require.True(t, cfg.UseCache)
				receivedSpecifiedFileContent = cfg.TaskDefinitions[0].FileContents
				receivedRwxDirectoryFileContent = cfg.RwxDirectory[1].FileContents

				return &api.InitiateRunResult{
					RunID:            "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
					RunURL:           "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
					TargetedTaskKeys: []string{},
					DefinitionPath:   ".mint/foo.yml",
				}, nil
			}

			_, err = s.service.InitiateRun(runConfig)
			require.NoError(t, err)

			require.True(t, getDefaultBaseCalled)
			expectedContent := "on:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n\nbase:\n  image: ubuntu:24.04\n  config: rwx/base 1.0.0\n\ntasks:\n  - key: foo\n    run: echo 'bar'\n"
			require.Equal(t, expectedContent, receivedSpecifiedFileContent)
			require.Equal(t, expectedContent, receivedRwxDirectoryFileContent)
			require.Contains(t, s.mockStderr.String(), "Configured \".mint/foo.yml\" to run on ubuntu:24.04\n")
		})

		t.Run("when package is missing version", func(t *testing.T) {
			s := setupTest(t)

			runConfig := cli.InitiateRunConfig{}
			baseSpec := "base:\n  image: ubuntu:24.04\n  config: rwx/base 1.0.0\n"

			s.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
				return api.DefaultBaseResult{
					Image:  "ubuntu:24.04",
					Config: "rwx/base 1.0.0",
					Arch:   "x86_64",
				}, nil
			}

			getPackageVersionsCalled := false
			majorPackageVersions := make(map[string]string)
			majorPackageVersions["mint/setup-node"] = "1.2.3"

			s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
				getPackageVersionsCalled = true
				return &api.PackageVersionsResult{
					LatestMajor: majorPackageVersions,
					LatestMinor: make(map[string]map[string]string),
				}, nil
			}

			originalSpecifiedFileContent := "on:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n\n" + baseSpec + "tasks:\n  - key: foo\n    call: mint/setup-node\n"
			var receivedSpecifiedFileContent string
			var receivedRwxDirectoryFileContent string

			mintDir := filepath.Join(s.tmp, ".mint")
			err := os.MkdirAll(mintDir, 0o755)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(mintDir, "foo.yml"), []byte(originalSpecifiedFileContent), 0o644)
			require.NoError(t, err)

			runConfig.MintFilePath = ".mint/foo.yml"
			runConfig.RwxDirectory = ".mint"

			s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
				require.Len(t, cfg.TaskDefinitions, 1)
				require.Equal(t, runConfig.MintFilePath, cfg.TaskDefinitions[0].Path)
				require.Len(t, cfg.RwxDirectory, 2)
				require.True(t, cfg.UseCache)
				receivedSpecifiedFileContent = cfg.TaskDefinitions[0].FileContents
				receivedRwxDirectoryFileContent = cfg.RwxDirectory[1].FileContents

				return &api.InitiateRunResult{
					RunID:            "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
					RunURL:           "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
					TargetedTaskKeys: []string{},
					DefinitionPath:   ".mint/foo.yml",
				}, nil
			}

			_, err = s.service.InitiateRun(runConfig)
			require.NoError(t, err)

			require.True(t, getPackageVersionsCalled)
			require.Equal(t, "on:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n\n"+baseSpec+"tasks:\n  - key: foo\n    call: mint/setup-node 1.2.3\n", receivedSpecifiedFileContent)
			require.Equal(t, "on:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n\n"+baseSpec+"tasks:\n  - key: foo\n    call: mint/setup-node 1.2.3\n", receivedRwxDirectoryFileContent)
			require.Contains(t, s.mockStderr.String(), "Configured package mint/setup-node to use version 1.2.3\n")
		})
	})

	t.Run("with no specific mint file and no specific directory", func(t *testing.T) {
		s := setupTest(t)

		runConfig := cli.InitiateRunConfig{
			MintFilePath: "",
			RwxDirectory: "",
		}

		_, err := s.service.InitiateRun(runConfig)

		require.Error(t, err)
		require.Contains(t, err.Error(), "the path to a run definition must be provided")
	})

	t.Run("with a specific mint file and a specific directory", func(t *testing.T) {
		t.Run("when a directory with files is found", func(t *testing.T) {
			s := setupTest(t)

			runConfig := cli.InitiateRunConfig{}
			baseSpec := "base:\n  image: ubuntu:24.04\n  config: rwx/base 1.0.0\n"

			s.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
				return api.DefaultBaseResult{
					Image:  "ubuntu:24.04",
					Config: "rwx/base 1.0.0",
					Arch:   "x86_64",
				}, nil
			}

			s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
				return &api.PackageVersionsResult{
					LatestMajor: make(map[string]string),
					LatestMinor: make(map[string]map[string]string),
				}, nil
			}

			originalSpecifiedFileContent := "on:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n\ntasks:\n  - key: foo\n    run: echo 'bar'\n" + baseSpec
			originalRwxDirFileContent := "tasks:\n  - key: mintdir\n    run: echo 'mintdir'\n" + baseSpec
			var receivedSpecifiedFileContent string
			var receivedRwxDir []api.RwxDirectoryEntry

			workingDir := filepath.Join(s.tmp, "some", "path", "to", "working", "directory")
			err := os.MkdirAll(workingDir, 0o755)
			require.NoError(t, err)

			err = os.Chdir(workingDir)
			require.NoError(t, err)

			mintDir := filepath.Join(s.tmp, "other", "path", "to", ".mint")
			err = os.MkdirAll(mintDir, 0o755)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(workingDir, "ci.yml"), []byte(originalSpecifiedFileContent), 0o644)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(mintDir, "mintdir-tasks.yml"), []byte(originalRwxDirFileContent), 0o644)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(mintDir, "mintdir-tasks.json"), []byte("some json"), 0o644)
			require.NoError(t, err)

			runConfig.MintFilePath = "ci.yml"
			runConfig.RwxDirectory = mintDir

			s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
				require.Len(t, cfg.TaskDefinitions, 1)
				require.Equal(t, runConfig.MintFilePath, cfg.TaskDefinitions[0].Path)
				require.Len(t, cfg.RwxDirectory, 5)
				require.Equal(t, ".", cfg.RwxDirectory[0].Path)
				require.Equal(t, "mintdir-tasks.json", cfg.RwxDirectory[1].Path)
				require.Equal(t, "mintdir-tasks.yml", cfg.RwxDirectory[2].Path)
				require.Equal(t, ".workflow", cfg.RwxDirectory[3].Path)
				require.Equal(t, "dir", cfg.RwxDirectory[3].Type)
				require.Equal(t, ".workflow/ci.yml", cfg.RwxDirectory[4].Path)
				require.Equal(t, "file", cfg.RwxDirectory[4].Type)
				require.True(t, cfg.UseCache)
				receivedSpecifiedFileContent = cfg.TaskDefinitions[0].FileContents
				receivedRwxDir = cfg.RwxDirectory
				return &api.InitiateRunResult{
					RunID:            "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
					RunURL:           "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
					TargetedTaskKeys: []string{},
					DefinitionPath:   ".mint/ci.yml",
				}, nil
			}

			_, err = s.service.InitiateRun(runConfig)
			require.NoError(t, err)

			require.Equal(t, originalSpecifiedFileContent, receivedSpecifiedFileContent)
			require.NotNil(t, receivedRwxDir)
			require.Equal(t, "", receivedRwxDir[0].FileContents)
			require.Equal(t, "some json", receivedRwxDir[1].FileContents)
			require.Equal(t, originalRwxDirFileContent, receivedRwxDir[2].FileContents)
			require.Equal(t, "", receivedRwxDir[3].FileContents)
			require.Equal(t, originalSpecifiedFileContent, receivedRwxDir[4].FileContents)
		})

		t.Run("when an empty directory is found", func(t *testing.T) {
			s := setupTest(t)

			runConfig := cli.InitiateRunConfig{}
			baseSpec := "base:\n  image: ubuntu:24.04\n  config: rwx/base 1.0.0\n"

			s.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
				return api.DefaultBaseResult{
					Image:  "ubuntu:24.04",
					Config: "rwx/base 1.0.0",
					Arch:   "x86_64",
				}, nil
			}

			s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
				return &api.PackageVersionsResult{
					LatestMajor: make(map[string]string),
					LatestMinor: make(map[string]map[string]string),
				}, nil
			}

			originalSpecifiedFileContent := "on:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n\ntasks:\n  - key: foo\n    run: echo 'bar'\n" + baseSpec
			var receivedSpecifiedFileContent string

			workingDir := filepath.Join(s.tmp, "some", "path", "to", "working", "directory")
			err := os.MkdirAll(workingDir, 0o755)
			require.NoError(t, err)

			err = os.Chdir(workingDir)
			require.NoError(t, err)

			mintDir := filepath.Join(s.tmp, "other", "path", "to", ".mint")
			err = os.MkdirAll(mintDir, 0o755)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(workingDir, "ci.yml"), []byte(originalSpecifiedFileContent), 0o644)
			require.NoError(t, err)

			runConfig.MintFilePath = "ci.yml"
			runConfig.RwxDirectory = mintDir

			s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
				require.Len(t, cfg.TaskDefinitions, 1)
				require.Equal(t, runConfig.MintFilePath, cfg.TaskDefinitions[0].Path)
				require.Len(t, cfg.RwxDirectory, 3)
				require.Equal(t, ".", cfg.RwxDirectory[0].Path)
				require.Equal(t, ".workflow", cfg.RwxDirectory[1].Path)
				require.Equal(t, "dir", cfg.RwxDirectory[1].Type)
				require.Equal(t, ".workflow/ci.yml", cfg.RwxDirectory[2].Path)
				require.Equal(t, "file", cfg.RwxDirectory[2].Type)
				require.True(t, cfg.UseCache)
				receivedSpecifiedFileContent = cfg.TaskDefinitions[0].FileContents
				return &api.InitiateRunResult{
					RunID:            "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
					RunURL:           "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
					TargetedTaskKeys: []string{},
					DefinitionPath:   ".mint/ci.yml",
				}, nil
			}

			_, err = s.service.InitiateRun(runConfig)
			require.NoError(t, err)

			require.Equal(t, originalSpecifiedFileContent, receivedSpecifiedFileContent)
		})

		t.Run("when the 'directory' is actually a file", func(t *testing.T) {
			s := setupTest(t)

			runConfig := cli.InitiateRunConfig{}

			workingDir := filepath.Join(s.tmp, "some", "path", "to", "working", "directory")
			err := os.MkdirAll(workingDir, 0o755)
			require.NoError(t, err)

			err = os.Chdir(workingDir)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(workingDir, "ci.yml"), []byte("yaml contents"), 0o644)
			require.NoError(t, err)

			mintDir := filepath.Join(workingDir, ".mint")
			err = os.WriteFile(mintDir, []byte("actually a file"), 0o644)
			require.NoError(t, err)

			runConfig.MintFilePath = "ci.yml"
			runConfig.RwxDirectory = mintDir

			_, err = s.service.InitiateRun(runConfig)

			require.Error(t, err)
			require.Contains(t, err.Error(), "is not a directory")
		})

		t.Run("when the directory is not found", func(t *testing.T) {
			s := setupTest(t)

			runConfig := cli.InitiateRunConfig{}
			baseSpec := "base:\n  image: ubuntu:24.04\n  config: rwx/base 1.0.0\n"

			originalSpecifiedFileContent := "on:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n\ntasks:\n  - key: foo\n    run: echo 'bar'\n" + baseSpec

			workingDir := filepath.Join(s.tmp, "some", "path", "to", "working", "directory")
			err := os.MkdirAll(workingDir, 0o755)
			require.NoError(t, err)

			err = os.Chdir(workingDir)
			require.NoError(t, err)

			mintDir := filepath.Join(s.tmp, "other", "path", "to", ".mint")

			err = os.WriteFile(filepath.Join(workingDir, "ci.yml"), []byte(originalSpecifiedFileContent), 0o644)
			require.NoError(t, err)

			runConfig.MintFilePath = "ci.yml"
			runConfig.RwxDirectory = mintDir

			_, err = s.service.InitiateRun(runConfig)

			require.Error(t, err)
			require.Contains(t, err.Error(), "unable to find .rwx directory")
		})
	})

	t.Run("with no specific mint file and a specific directory", func(t *testing.T) {
		s := setupTest(t)

		runConfig := cli.InitiateRunConfig{
			MintFilePath: "",
			RwxDirectory: "some-dir",
		}

		_, err := s.service.InitiateRun(runConfig)

		require.Error(t, err)
		require.Contains(t, err.Error(), "the path to a run definition must be provided")
	})

	t.Run("when base insertion has errors but no updates", func(t *testing.T) {
		s := setupTest(t)

		runConfig := cli.InitiateRunConfig{}

		// Mock that base insertion fails
		s.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
			return api.DefaultBaseResult{}, fmt.Errorf("invalid YAML syntax")
		}

		s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
			return &api.PackageVersionsResult{
				LatestMajor: make(map[string]string),
				LatestMinor: make(map[string]map[string]string),
			}, nil
		}

		// Create a file with invalid YAML syntax that will cause base resolution to fail
		originalSpecifiedFileContent := "on:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n\n# This is a comment\ntasks:\n  - key: foo\n    run: echo 'bar'\n"

		mintDir := filepath.Join(s.tmp, ".mint")
		err := os.MkdirAll(mintDir, 0o755)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(mintDir, "foo.yml"), []byte(originalSpecifiedFileContent), 0o644)
		require.NoError(t, err)

		runConfig.MintFilePath = ".mint/foo.yml"
		runConfig.RwxDirectory = ".mint"

		// This should not crash even when base resolution fails
		_, err = s.service.InitiateRun(runConfig)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unable to resolve base")

		// Verify that no success message was printed (since there were no successful updates)
		require.NotContains(t, s.mockStderr.String(), "Configured")
	})

	t.Run("resolves CLI git init params", func(t *testing.T) {
		s := setupTest(t)

		runConfig := cli.InitiateRunConfig{}
		baseSpec := "base:\n  image: ubuntu:24.04\n  config: rwx/base 1.0.0\n"

		s.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
			return api.DefaultBaseResult{
				Image:  "ubuntu:24.04",
				Config: "rwx/base 1.0.0",
				Arch:   "x86_64",
			}, nil
		}

		s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
			return &api.PackageVersionsResult{
				LatestMajor: make(map[string]string),
				LatestMinor: make(map[string]map[string]string),
			}, nil
		}

		s.mockGit.MockGetBranch = "main"
		s.mockGit.MockGetCommit = "abc123"
		s.mockGit.MockGetOriginUrl = "git@github.com:rwx-cloud/rwx.git"

		originalFileContent := `
on:
  github:
    push:
      init:
        sha: ${{ event.git.sha }}

tasks:
  - key: foo
    run: echo 'bar'
` + baseSpec

		workingDir := filepath.Join(s.tmp, "working")
		err := os.MkdirAll(workingDir, 0o755)
		require.NoError(t, err)

		err = os.Chdir(workingDir)
		require.NoError(t, err)

		mintDir := filepath.Join(s.tmp, ".mint")
		err = os.MkdirAll(mintDir, 0o755)
		require.NoError(t, err)

		testFile := filepath.Join(mintDir, "test.yml")
		err = os.WriteFile(testFile, []byte(originalFileContent), 0o644)
		require.NoError(t, err)

		runConfig.MintFilePath = testFile
		runConfig.RwxDirectory = ""

		var receivedFileContent string
		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			receivedFileContent = cfg.TaskDefinitions[0].FileContents
			return &api.InitiateRunResult{
				RunID:            "test-run-id",
				RunURL:           "https://cloud.rwx.com/mint/rwx/runs/test-run-id",
				TargetedTaskKeys: []string{},
				DefinitionPath:   testFile,
			}, nil
		}

		_, err = s.service.InitiateRun(runConfig)
		require.NoError(t, err)

		require.Contains(t, receivedFileContent, "cli:")
		require.Contains(t, receivedFileContent, "sha: ${{ event.git.sha }}")
		require.Contains(t, s.mockStderr.String(), "Configured CLI trigger with git init params")

		modifiedContent, err := os.ReadFile(testFile)
		require.NoError(t, err)
		require.Contains(t, string(modifiedContent), "cli:")
	})

	t.Run("when .workflow/<filename> already exists in the rwx directory, uploads to .workflow/<contentHash>/<filename>", func(t *testing.T) {
		s := setupTest(t)

		runConfig := cli.InitiateRunConfig{}
		baseSpec := "base:\n  image: ubuntu:24.04\n  config: rwx/base 1.0.0\n"

		s.mockAPI.MockGetDefaultBase = func() (api.DefaultBaseResult, error) {
			return api.DefaultBaseResult{
				Image:  "ubuntu:24.04",
				Config: "rwx/base 1.0.0",
				Arch:   "x86_64",
			}, nil
		}

		s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
			return &api.PackageVersionsResult{
				LatestMajor: make(map[string]string),
				LatestMinor: make(map[string]map[string]string),
			}, nil
		}

		specifiedContent := "tasks:\n  - key: outside\n    run: echo 'outside'\n" + baseSpec
		existingWorkflowContent := "tasks:\n  - key: existing\n    run: echo 'existing'\n" + baseSpec

		workingDir := filepath.Join(s.tmp, "some", "path", "to", "working", "directory")
		err := os.MkdirAll(workingDir, 0o755)
		require.NoError(t, err)

		err = os.Chdir(workingDir)
		require.NoError(t, err)

		rwxDir := filepath.Join(s.tmp, "some", "path", "to", ".rwx")
		err = os.MkdirAll(filepath.Join(rwxDir, ".workflow"), 0o755)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(workingDir, "ci.yml"), []byte(specifiedContent), 0o644)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(rwxDir, ".workflow", "ci.yml"), []byte(existingWorkflowContent), 0o644)
		require.NoError(t, err)

		runConfig.MintFilePath = "ci.yml"
		runConfig.RwxDirectory = ""

		sum := sha256.Sum256([]byte(specifiedContent))
		expectedHash := hex.EncodeToString(sum[:8])
		expectedNestedDir := ".workflow/" + expectedHash
		expectedNestedFile := expectedNestedDir + "/ci.yml"

		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			require.Len(t, cfg.TaskDefinitions, 1)
			require.Equal(t, runConfig.MintFilePath, cfg.TaskDefinitions[0].Path)

			// Expected: walk of .rwx yields [., .workflow, .workflow/ci.yml],
			// then appendWorkflowUploadEntry adds [.workflow/<hash>, .workflow/<hash>/ci.yml]
			require.Len(t, cfg.RwxDirectory, 5)
			require.Equal(t, ".", cfg.RwxDirectory[0].Path)
			require.Equal(t, ".workflow", cfg.RwxDirectory[1].Path)
			require.Equal(t, "dir", cfg.RwxDirectory[1].Type)
			require.Equal(t, ".workflow/ci.yml", cfg.RwxDirectory[2].Path)
			require.Equal(t, "file", cfg.RwxDirectory[2].Type)
			require.Equal(t, existingWorkflowContent, cfg.RwxDirectory[2].FileContents)

			require.Equal(t, "dir", cfg.RwxDirectory[3].Type)
			require.Equal(t, expectedNestedDir, cfg.RwxDirectory[3].Path)

			require.Equal(t, "file", cfg.RwxDirectory[4].Type)
			require.Equal(t, expectedNestedFile, cfg.RwxDirectory[4].Path)
			require.Equal(t, specifiedContent, cfg.RwxDirectory[4].FileContents)

			return &api.InitiateRunResult{
				RunID:            "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
				RunURL:           "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
				TargetedTaskKeys: []string{},
				DefinitionPath:   ".workflow/ci.yml",
			}, nil
		}

		_, err = s.service.InitiateRun(runConfig)
		require.NoError(t, err)
	})
}
