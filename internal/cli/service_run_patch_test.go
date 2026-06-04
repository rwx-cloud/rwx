package cli_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/git"
	"github.com/rwx-cloud/rwx/internal/mocks"
	"github.com/stretchr/testify/require"
)

var _ cli.APIClient = (*mocks.API)(nil)

type initiateRunResult struct {
	rwxDir []api.RwxDirectoryEntry
	stderr string
}

func initiateRun(t *testing.T, patchFile git.PatchFile, expectedPatchMetadata api.PatchMetadata, opts ...func(*cli.InitiateRunConfig)) initiateRunResult {
	s := setupTest(t)
	s.mockGit.MockGetCommit = "3e76c8295cd0ce4decbf7b56253c902ce296cb25"
	s.mockGit.MockGeneratePatchFile = patchFile

	var receivedRwxDir []api.RwxDirectoryEntry

	runConfig := cli.InitiateRunConfig{Patchable: true}

	for _, opt := range opts {
		opt(&runConfig)
	}

	rwxDir := filepath.Join(s.tmp, ".rwx")
	err := os.MkdirAll(rwxDir, 0o755)
	require.NoError(t, err)

	runConfig.RwxDirectory = rwxDir

	definitionsFile := filepath.Join(rwxDir, "rwx.yml")
	runConfig.MintFilePath = definitionsFile

	definition := "on:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n\nbase:\n  os: ubuntu 24.04\n  tag: 1.0\n\ntasks:\n  - key: foo\n    run: echo 'bar'\n"

	err = os.WriteFile(definitionsFile, []byte(definition), 0o644)
	require.NoError(t, err)

	s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
		return &api.PackageVersionsResult{
			LatestMajor: make(map[string]string),
			LatestMinor: make(map[string]map[string]string),
		}, nil
	}
	s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
		require.Equal(t, expectedPatchMetadata.Sent, cfg.Patch.Sent)
		expectedUntrackedFiles := expectedPatchMetadata.UntrackedFiles
		if expectedUntrackedFiles == nil {
			expectedUntrackedFiles = []string{}
		}
		require.Equal(t, expectedUntrackedFiles, cfg.Patch.UntrackedFiles)
		require.Equal(t, expectedPatchMetadata.UntrackedCount, cfg.Patch.UntrackedCount)
		require.Equal(t, expectedPatchMetadata.LFSFiles, cfg.Patch.LFSFiles)
		receivedRwxDir = cfg.RwxDirectory
		return &api.InitiateRunResult{
			RunID:            "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
			RunURL:           "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
			TargetedTaskKeys: []string{},
			DefinitionPath:   ".mint/mint.yml",
		}, nil
	}
	_, err = s.service.InitiateRun(runConfig)
	require.NoError(t, err)
	return initiateRunResult{rwxDir: receivedRwxDir, stderr: s.mockStderr.String()}
}

func TestService_InitiatingRunPatch(t *testing.T) {
	t.Run("when git is not installed", func(t *testing.T) {
		s := setupTest(t)
		s.mockGit.MockIsInstalled = false
		s.mockGit.MockIsInsideWorkTree = false

		rwxDir := filepath.Join(s.tmp, ".rwx")
		err := os.MkdirAll(rwxDir, 0o755)
		require.NoError(t, err)

		definitionsFile := filepath.Join(rwxDir, "rwx.yml")
		definition := "on:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n\nbase:\n  os: ubuntu 24.04\n  tag: 1.0\n\ntasks:\n  - key: foo\n    run: echo 'bar'\n"
		err = os.WriteFile(definitionsFile, []byte(definition), 0o644)
		require.NoError(t, err)

		s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
			return &api.PackageVersionsResult{
				LatestMajor: make(map[string]string),
				LatestMinor: make(map[string]map[string]string),
			}, nil
		}
		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			require.False(t, cfg.Patch.Sent)
			require.False(t, cfg.Patch.GitInstalled)
			require.False(t, cfg.Patch.GitDirectory)
			require.Equal(t, "Git is not installed", cfg.Patch.ErrorMessage)
			require.Empty(t, cfg.Git.Sha)
			require.Empty(t, cfg.Git.Branch)
			require.Empty(t, cfg.Git.OriginUrl)
			return &api.InitiateRunResult{
				RunID:  "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
				RunURL: "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
			}, nil
		}

		_, err = s.service.InitiateRun(cli.InitiateRunConfig{
			RwxDirectory: rwxDir,
			MintFilePath: definitionsFile,
		})
		require.NoError(t, err)
	})

	t.Run("when not in a git directory", func(t *testing.T) {
		s := setupTest(t)
		s.mockGit.MockIsInstalled = true
		s.mockGit.MockIsInsideWorkTree = false

		rwxDir := filepath.Join(s.tmp, ".rwx")
		err := os.MkdirAll(rwxDir, 0o755)
		require.NoError(t, err)

		definitionsFile := filepath.Join(rwxDir, "rwx.yml")
		definition := "on:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n\nbase:\n  os: ubuntu 24.04\n  tag: 1.0\n\ntasks:\n  - key: foo\n    run: echo 'bar'\n"
		err = os.WriteFile(definitionsFile, []byte(definition), 0o644)
		require.NoError(t, err)

		s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
			return &api.PackageVersionsResult{
				LatestMajor: make(map[string]string),
				LatestMinor: make(map[string]map[string]string),
			}, nil
		}
		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			require.False(t, cfg.Patch.Sent)
			require.True(t, cfg.Patch.GitInstalled)
			require.False(t, cfg.Patch.GitDirectory)
			require.Equal(t, "You are not in a git repository", cfg.Patch.ErrorMessage)
			require.Empty(t, cfg.Git.Sha)
			require.Empty(t, cfg.Git.Branch)
			require.Empty(t, cfg.Git.OriginUrl)
			return &api.InitiateRunResult{
				RunID:  "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
				RunURL: "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
			}, nil
		}

		_, err = s.service.InitiateRun(cli.InitiateRunConfig{
			RwxDirectory: rwxDir,
			MintFilePath: definitionsFile,
		})
		require.NoError(t, err)
	})

	t.Run("when git commit fails", func(t *testing.T) {
		s := setupTest(t)
		s.mockGit.MockIsInstalled = true
		s.mockGit.MockIsInsideWorkTree = true
		s.mockGit.MockGetCommitError = errors.New("no git remote named 'origin' is configured")

		rwxDir := filepath.Join(s.tmp, ".rwx")
		err := os.MkdirAll(rwxDir, 0o755)
		require.NoError(t, err)

		definitionsFile := filepath.Join(rwxDir, "rwx.yml")
		definition := "on:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n\nbase:\n  os: ubuntu 24.04\n  tag: 1.0\n\ntasks:\n  - key: foo\n    run: echo 'bar'\n"
		err = os.WriteFile(definitionsFile, []byte(definition), 0o644)
		require.NoError(t, err)

		s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
			return &api.PackageVersionsResult{
				LatestMajor: make(map[string]string),
				LatestMinor: make(map[string]map[string]string),
			}, nil
		}
		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			require.False(t, cfg.Patch.Sent)
			require.True(t, cfg.Patch.GitInstalled)
			require.True(t, cfg.Patch.GitDirectory)
			require.Equal(t, "no git remote named 'origin' is configured", cfg.Patch.ErrorMessage)
			require.Empty(t, cfg.Git.Sha)
			require.Empty(t, cfg.Git.Branch)
			require.Empty(t, cfg.Git.OriginUrl)
			return &api.InitiateRunResult{
				RunID:  "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
				RunURL: "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
			}, nil
		}

		_, err = s.service.InitiateRun(cli.InitiateRunConfig{
			RwxDirectory: rwxDir,
			MintFilePath: definitionsFile,
		})
		require.NoError(t, err)
	})

	t.Run("when patch generation fails", func(t *testing.T) {
		s := setupTest(t)
		s.mockGit.MockGetCommit = "3e76c8295cd0ce4decbf7b56253c902ce296cb25"
		s.mockGit.MockGetBranch = "main"
		s.mockGit.MockGetOriginUrl = "git@github.com:example/repo.git"
		s.mockGit.MockGeneratePatchFileError = errors.New("unable to generate patch data")

		rwxDir := filepath.Join(s.tmp, ".rwx")
		err := os.MkdirAll(rwxDir, 0o755)
		require.NoError(t, err)

		definitionsFile := filepath.Join(rwxDir, "rwx.yml")
		definition := "on:\n  cli:\n    init:\n      sha: ${{ event.git.sha }}\n\nbase:\n  os: ubuntu 24.04\n  tag: 1.0\n\ntasks:\n  - key: foo\n    run: echo 'bar'\n"
		err = os.WriteFile(definitionsFile, []byte(definition), 0o644)
		require.NoError(t, err)

		s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
			return &api.PackageVersionsResult{
				LatestMajor: make(map[string]string),
				LatestMinor: make(map[string]map[string]string),
			}, nil
		}
		s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
			require.False(t, cfg.Patch.Sent)
			require.True(t, cfg.Patch.GitInstalled)
			require.True(t, cfg.Patch.GitDirectory)
			require.Equal(t, "unable to generate patch data", cfg.Patch.ErrorMessage)
			require.Equal(t, "3e76c8295cd0ce4decbf7b56253c902ce296cb25", cfg.Git.Sha)
			require.Equal(t, "main", cfg.Git.Branch)
			require.Equal(t, "git@github.com:example/repo.git", cfg.Git.OriginUrl)
			return &api.InitiateRunResult{
				RunID:  "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
				RunURL: "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
			}, nil
		}

		_, err = s.service.InitiateRun(cli.InitiateRunConfig{
			RwxDirectory: rwxDir,
			MintFilePath: definitionsFile,
			Patchable:    true,
		})
		require.NoError(t, err)
		require.Contains(t, s.mockStderr.String(), "Warning: failed to generate patch: unable to generate patch data")
	})

	t.Run("when the run is not patchable", func(t *testing.T) {
		// it launches a run but does not patch
		result := initiateRun(t, git.PatchFile{}, api.PatchMetadata{})

		for _, entry := range result.rwxDir {
			require.False(t, strings.HasPrefix(entry.Path, ".patches/"))
		}
	})

	t.Run("when patchable is false", func(t *testing.T) {
		patchFile := git.PatchFile{
			Written:        true,
			UntrackedFiles: git.UntrackedFilesMetadata{Files: []string{"foo.txt"}, Count: 1},
		}
		notPatchable := func(cfg *cli.InitiateRunConfig) { cfg.Patchable = false }

		// it launches a run but does not include the patch
		result := initiateRun(t, patchFile, api.PatchMetadata{}, notPatchable)

		for _, entry := range result.rwxDir {
			require.False(t, strings.HasPrefix(entry.Path, ".patches/"))
		}
	})

	t.Run("patch logging", func(t *testing.T) {
		t.Run("when no patch is written", func(t *testing.T) {
			result := initiateRun(t, git.PatchFile{}, api.PatchMetadata{})
			require.NotContains(t, result.stderr, "Included a git patch")
		})

		t.Run("when a patch is written with no untracked files", func(t *testing.T) {
			patchFile := git.PatchFile{Written: true}
			expectedPatch := api.PatchMetadata{Sent: true}
			result := initiateRun(t, patchFile, expectedPatch)
			require.Contains(t, result.stderr, "Included a git patch for uncommitted changes")
			require.NotContains(t, result.stderr, "untracked file")
		})

		t.Run("when no patch is written but there are untracked files", func(t *testing.T) {
			patchFile := git.PatchFile{
				UntrackedFiles: git.UntrackedFilesMetadata{Files: []string{"foo.txt"}, Count: 1},
			}
			result := initiateRun(t, patchFile, api.PatchMetadata{})
			require.NotContains(t, result.stderr, "Included a git patch")
			require.NotContains(t, result.stderr, "untracked file")
		})

		t.Run("when a patch is written with 1 untracked file", func(t *testing.T) {
			patchFile := git.PatchFile{
				Written:        true,
				UntrackedFiles: git.UntrackedFilesMetadata{Files: []string{"foo.txt"}, Count: 1},
			}
			expectedPatch := api.PatchMetadata{Sent: true}
			result := initiateRun(t, patchFile, expectedPatch)
			require.Contains(t, result.stderr, "Included a git patch for uncommitted changes")
			require.NotContains(t, result.stderr, "untracked file")
		})

		t.Run("when a patch is written with 5 untracked files", func(t *testing.T) {
			files := []string{"a.txt", "b.txt", "c.txt", "d.txt", "e.txt"}
			patchFile := git.PatchFile{
				Written:        true,
				UntrackedFiles: git.UntrackedFilesMetadata{Files: files, Count: 5},
			}
			expectedPatch := api.PatchMetadata{Sent: true}
			result := initiateRun(t, patchFile, expectedPatch)
			require.Contains(t, result.stderr, "Included a git patch for uncommitted changes")
			require.NotContains(t, result.stderr, "untracked file")
		})

		t.Run("when a patch is written with more than 5 untracked files", func(t *testing.T) {
			files := []string{"a.txt", "b.txt", "c.txt", "d.txt", "e.txt", "f.txt", "g.txt"}
			patchFile := git.PatchFile{
				Written:        true,
				UntrackedFiles: git.UntrackedFilesMetadata{Files: files, Count: 7},
			}
			expectedPatch := api.PatchMetadata{Sent: true}
			result := initiateRun(t, patchFile, expectedPatch)
			require.Contains(t, result.stderr, "Included a git patch for uncommitted changes")
			require.NotContains(t, result.stderr, "untracked file")
			require.NotContains(t, result.stderr, "and 2 more")
		})
	})

	t.Run("when the run is patchable", func(t *testing.T) {
		untrackedFiles := git.UntrackedFilesMetadata{
			Files: []string{"foo.txt"},
			Count: 1,
		}
		lfsChangedFiles := git.LFSChangedFilesMetadata{
			Files: []string{"bar.txt"},
			Count: 1,
		}

		patchFile := git.PatchFile{
			Written:         true,
			UntrackedFiles:  untrackedFiles,
			LFSChangedFiles: lfsChangedFiles,
		}

		t.Run("when env RWX_DISABLE_GIT_PATCH is set", func(t *testing.T) {
			t.Setenv("RWX_DISABLE_GIT_PATCH", "1")

			// it launches a run but does not patch
			result := initiateRun(t, patchFile, api.PatchMetadata{})

			for _, entry := range result.rwxDir {
				require.False(t, strings.Contains(entry.Path, ".patches/"))
			}
		})

		t.Run("by default", func(t *testing.T) {
			expectedPatchMetadata := api.PatchMetadata{
				Sent:     true,
				LFSFiles: lfsChangedFiles.Files,
				LFSCount: lfsChangedFiles.Count,
			}

			result := initiateRun(t, patchFile, expectedPatchMetadata)

			patched := false
			for _, entry := range result.rwxDir {
				if strings.Contains(entry.Path, ".patches/") {
					patched = true
				}
			}

			require.True(t, patched)
		})

		t.Run("when init params match git params", func(t *testing.T) {
			s := setupTest(t)
			s.mockGit.MockGetCommit = "3e76c8295cd0ce4decbf7b56253c902ce296cb25"
			s.mockGit.MockGeneratePatchFile = patchFile

			rwxDir := filepath.Join(s.tmp, ".rwx")
			err := os.MkdirAll(rwxDir, 0o755)
			require.NoError(t, err)

			definitionsFile := filepath.Join(rwxDir, "rwx.yml")

			definition := "on:\n  github:\n    push:\n      init:\n        sha: ${{ event.git.sha }}\n\nbase:\n  os: ubuntu 24.04\n  tag: 1.0\n\ntasks:\n  - key: foo\n    run: echo 'bar'\n"

			err = os.WriteFile(definitionsFile, []byte(definition), 0o644)
			require.NoError(t, err)

			runConfig := cli.InitiateRunConfig{
				RwxDirectory: rwxDir,
				MintFilePath: definitionsFile,
				Patchable:    true,
				InitParameters: map[string]string{
					"sha": "3e76c8295cd0ce4decbf7b56253c902ce296cb25", // a git param is passed by --init
				},
			}

			var receivedRwxDir []api.RwxDirectoryEntry
			s.mockAPI.MockGetPackageVersions = func() (*api.PackageVersionsResult, error) {
				return &api.PackageVersionsResult{
					LatestMajor: make(map[string]string),
					LatestMinor: make(map[string]map[string]string),
				}, nil
			}
			s.mockAPI.MockInitiateRun = func(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
				require.False(t, cfg.Patch.Sent) // so we skip the patch
				receivedRwxDir = cfg.RwxDirectory
				return &api.InitiateRunResult{
					RunID:            "785ce4e8-17b9-4c8b-8869-a55e95adffe7",
					RunURL:           "https://cloud.rwx.com/mint/rwx/runs/785ce4e8-17b9-4c8b-8869-a55e95adffe7",
					TargetedTaskKeys: []string{},
					DefinitionPath:   ".mint/mint.yml",
				}, nil
			}

			_, err = s.service.InitiateRun(runConfig)
			require.NoError(t, err)

			// Verify patch generation was skipped entirely — no .patches entries in the rwx directory
			for _, entry := range receivedRwxDir {
				require.False(t, strings.Contains(entry.Path, ".patches"), "expected no .patches entries when init params match git params, found: %s", entry.Path)
			}
		})
	})
}
