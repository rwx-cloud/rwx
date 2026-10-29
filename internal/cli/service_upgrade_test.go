package cli_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/releases"
	"github.com/rwx-cloud/rwx/internal/versions"
	"github.com/stretchr/testify/require"
)

type upgradeServer struct {
	tag    string
	binary string
	digest string
}

func fakeBinary(version string) string {
	return fmt.Sprintf("#!/bin/sh\necho 'rwx version %s'\n", version)
}

func sha256Digest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func setupUpgradeTest(t *testing.T, srv upgradeServer, exe string, current string) *testSetup {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("upgrade tests execute a shell script as the downloaded binary")
	}

	if srv.digest == "" {
		srv.digest = sha256Digest(srv.binary)
	}

	s := setupTest(t)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/rwx-cloud/rwx/releases/latest", "/repos/rwx-cloud/rwx/releases/tags/" + srv.tag:
			writeJSON(t, w, releases.Release{
				TagName: srv.tag,
				Assets: []releases.Asset{
					{
						Name:               "rwx-linux-x86_64",
						BrowserDownloadURL: server.URL + "/download/rwx-linux-x86_64",
						Digest:             srv.digest,
					},
				},
			})
		case "/download/rwx-linux-x86_64":
			_, _ = w.Write([]byte(srv.binary))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	t.Cleanup(cli.SetUpgradeEnvironmentForTest(exe, current, "linux", "amd64"))

	s.config.ReleasesClient = releases.Client{Host: server.Listener.Addr().String(), Scheme: "http"}
	var err error
	s.service, err = cli.NewService(s.config)
	require.NoError(t, err)

	return s
}

func writeExecutable(t *testing.T, path string, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o755))
}

func TestService_Upgrade(t *testing.T) {
	t.Run("replaces the executable with the latest release", func(t *testing.T) {
		exe := filepath.Join(t.TempDir(), "bin", "rwx")
		writeExecutable(t, exe, fakeBinary("v3.0.0"))
		s := setupUpgradeTest(t, upgradeServer{tag: "v3.1.0", binary: fakeBinary("v3.1.0")}, exe, "3.0.0")

		result, err := s.service.Upgrade(cli.UpgradeConfig{})

		require.NoError(t, err)
		require.True(t, result.Upgraded)
		require.Equal(t, "3.0.0", result.CurrentVersion)
		require.Equal(t, "3.1.0", result.TargetVersion)
		require.Contains(t, s.mockStdout.String(), "Upgraded rwx 3.0.0 → 3.1.0")

		content, err := os.ReadFile(exe)
		require.NoError(t, err)
		require.Equal(t, fakeBinary("v3.1.0"), string(content))

		entries, err := os.ReadDir(filepath.Dir(exe))
		require.NoError(t, err)
		require.Len(t, entries, 1, "temporary upgrade files should be cleaned up")
	})

	t.Run("replaces the target of a symlinked executable", func(t *testing.T) {
		dir := t.TempDir()
		exe := filepath.Join(dir, "opt", "rwx")
		link := filepath.Join(dir, "bin", "rwx")
		writeExecutable(t, exe, fakeBinary("v3.0.0"))
		require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
		require.NoError(t, os.Symlink(exe, link))
		s := setupUpgradeTest(t, upgradeServer{tag: "v3.1.0", binary: fakeBinary("v3.1.0")}, link, "3.0.0")

		_, err := s.service.Upgrade(cli.UpgradeConfig{})

		require.NoError(t, err)
		target, err := os.Readlink(link)
		require.NoError(t, err)
		require.Equal(t, exe, target)
		content, err := os.ReadFile(exe)
		require.NoError(t, err)
		require.Equal(t, fakeBinary("v3.1.0"), string(content))
	})

	t.Run("when already up to date", func(t *testing.T) {
		exe := filepath.Join(t.TempDir(), "rwx")
		writeExecutable(t, exe, fakeBinary("v3.1.0"))
		s := setupUpgradeTest(t, upgradeServer{tag: "v3.1.0", binary: fakeBinary("v3.1.0")}, exe, "3.1.0")

		result, err := s.service.Upgrade(cli.UpgradeConfig{})

		require.NoError(t, err)
		require.False(t, result.Upgraded)
		require.Contains(t, s.mockStdout.String(), "rwx 3.1.0 is already up to date")
	})

	t.Run("does not replace a development build", func(t *testing.T) {
		exe := filepath.Join(t.TempDir(), "rwx")
		writeExecutable(t, exe, fakeBinary("dev"))
		s := setupUpgradeTest(t, upgradeServer{tag: "v3.1.0", binary: fakeBinary("v3.1.0")}, exe, "9999+dev")

		result, err := s.service.Upgrade(cli.UpgradeConfig{})

		require.NoError(t, err)
		require.False(t, result.Upgraded)
		content, err := os.ReadFile(exe)
		require.NoError(t, err)
		require.Equal(t, fakeBinary("dev"), string(content))
	})

	t.Run("installs a specific version when requested", func(t *testing.T) {
		exe := filepath.Join(t.TempDir(), "rwx")
		writeExecutable(t, exe, fakeBinary("v3.1.0"))
		s := setupUpgradeTest(t, upgradeServer{tag: "v3.0.0", binary: fakeBinary("v3.0.0")}, exe, "3.1.0")

		result, err := s.service.Upgrade(cli.UpgradeConfig{Version: "3.0.0"})

		require.NoError(t, err)
		require.True(t, result.Upgraded)
		require.Equal(t, "3.0.0", result.TargetVersion)
	})

	t.Run("with --check reports an available upgrade and exits 1", func(t *testing.T) {
		exe := filepath.Join(t.TempDir(), "rwx")
		writeExecutable(t, exe, fakeBinary("v3.0.0"))
		s := setupUpgradeTest(t, upgradeServer{tag: "v3.1.0", binary: fakeBinary("v3.1.0")}, exe, "3.0.0")

		result, err := s.service.Upgrade(cli.UpgradeConfig{Check: true})

		var exitErr *cli.ExitCodeError
		require.ErrorAs(t, err, &exitErr)
		require.Equal(t, 1, exitErr.Code)
		require.True(t, result.UpgradeAvailable)
		require.False(t, result.Upgraded)
		require.Contains(t, s.mockStdout.String(), "A new release of rwx is available: 3.0.0 → 3.1.0")

		content, err := os.ReadFile(exe)
		require.NoError(t, err)
		require.Equal(t, fakeBinary("v3.0.0"), string(content))
	})

	t.Run("with --json outputs the result", func(t *testing.T) {
		exe := filepath.Join(t.TempDir(), "rwx")
		writeExecutable(t, exe, fakeBinary("v3.1.0"))
		s := setupUpgradeTest(t, upgradeServer{tag: "v3.1.0", binary: fakeBinary("v3.1.0")}, exe, "3.1.0")

		_, err := s.service.Upgrade(cli.UpgradeConfig{Json: true})

		require.NoError(t, err)
		var output cli.UpgradeResult
		require.NoError(t, json.Unmarshal([]byte(s.mockStdout.String()), &output))
		require.Equal(t, "3.1.0", output.CurrentVersion)
		require.False(t, output.UpgradeAvailable)
	})

	t.Run("refuses to replace a Homebrew installation", func(t *testing.T) {
		exe := filepath.Join(t.TempDir(), "homebrew", "Cellar", "rwx", "3.0.0", "bin", "rwx")
		writeExecutable(t, exe, fakeBinary("v3.0.0"))
		s := setupUpgradeTest(t, upgradeServer{tag: "v3.1.0", binary: fakeBinary("v3.1.0")}, exe, "3.0.0")

		_, err := s.service.Upgrade(cli.UpgradeConfig{})

		require.ErrorContains(t, err, "brew upgrade rwx-cloud/tap/rwx")
	})

	t.Run("leaves the executable untouched when the checksum does not match", func(t *testing.T) {
		exe := filepath.Join(t.TempDir(), "bin", "rwx")
		writeExecutable(t, exe, fakeBinary("v3.0.0"))
		s := setupUpgradeTest(t, upgradeServer{
			tag:    "v3.1.0",
			binary: fakeBinary("v3.1.0"),
			digest: sha256Digest("tampered"),
		}, exe, "3.0.0")

		_, err := s.service.Upgrade(cli.UpgradeConfig{})

		require.ErrorContains(t, err, "checksum mismatch")
		content, err := os.ReadFile(exe)
		require.NoError(t, err)
		require.Equal(t, fakeBinary("v3.0.0"), string(content))

		entries, err := os.ReadDir(filepath.Dir(exe))
		require.NoError(t, err)
		require.Len(t, entries, 1, "temporary upgrade files should be cleaned up")
	})

	t.Run("leaves the executable untouched when the new binary reports the wrong version", func(t *testing.T) {
		exe := filepath.Join(t.TempDir(), "rwx")
		writeExecutable(t, exe, fakeBinary("v3.0.0"))
		s := setupUpgradeTest(t, upgradeServer{tag: "v3.1.0", binary: fakeBinary("v2.0.0")}, exe, "3.0.0")

		_, err := s.service.Upgrade(cli.UpgradeConfig{})

		require.ErrorContains(t, err, "reported an unexpected version")
		content, err := os.ReadFile(exe)
		require.NoError(t, err)
		require.Equal(t, fakeBinary("v3.0.0"), string(content))
	})

	t.Run("when the install directory is not writable", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root can write to read-only directories")
		}

		dir := filepath.Join(t.TempDir(), "bin")
		exe := filepath.Join(dir, "rwx")
		writeExecutable(t, exe, fakeBinary("v3.0.0"))
		require.NoError(t, os.Chmod(dir, 0o555))
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
		s := setupUpgradeTest(t, upgradeServer{tag: "v3.1.0", binary: fakeBinary("v3.1.0")}, exe, "3.0.0")

		_, err := s.service.Upgrade(cli.UpgradeConfig{})

		resolved, err2 := filepath.EvalSymlinks(exe)
		require.NoError(t, err2)
		require.ErrorContains(t, err, fmt.Sprintf("To upgrade, run: sudo %s upgrade", resolved))
	})

	t.Run("keeps the permissions of the replaced executable", func(t *testing.T) {
		exe := filepath.Join(t.TempDir(), "bin", "rwx")
		writeExecutable(t, exe, fakeBinary("v3.0.0"))
		require.NoError(t, os.Chmod(exe, 0o555))
		s := setupUpgradeTest(t, upgradeServer{tag: "v3.1.0", binary: fakeBinary("v3.1.0")}, exe, "3.0.0")

		_, err := s.service.Upgrade(cli.UpgradeConfig{})

		require.NoError(t, err)
		info, err := os.Stat(exe)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o555), info.Mode().Perm())
	})

	t.Run("with --check on a Homebrew installation suggests brew", func(t *testing.T) {
		exe := filepath.Join(t.TempDir(), "usr", "local", "Cellar", "rwx", "3.0.0", "bin", "rwx")
		writeExecutable(t, exe, fakeBinary("v3.0.0"))
		s := setupUpgradeTest(t, upgradeServer{tag: "v3.1.0", binary: fakeBinary("v3.1.0")}, exe, "3.0.0")

		_, err := s.service.Upgrade(cli.UpgradeConfig{Check: true})

		var exitErr *cli.ExitCodeError
		require.ErrorAs(t, err, &exitErr)
		require.Contains(t, s.mockStdout.String(), "To upgrade, run: brew upgrade rwx-cloud/tap/rwx")
		require.NotContains(t, s.mockStdout.String(), "rwx upgrade\n")
	})

	t.Run("rejects a binary whose version only contains the target version", func(t *testing.T) {
		exe := filepath.Join(t.TempDir(), "rwx")
		writeExecutable(t, exe, fakeBinary("v3.0.0"))
		s := setupUpgradeTest(t, upgradeServer{tag: "v3.1.0", binary: fakeBinary("v13.1.0")}, exe, "3.0.0")

		_, err := s.service.Upgrade(cli.UpgradeConfig{})

		require.ErrorContains(t, err, "reported an unexpected version")
		content, err := os.ReadFile(exe)
		require.NoError(t, err)
		require.Equal(t, fakeBinary("v3.0.0"), string(content))
	})

	t.Run("rejects a version alias", func(t *testing.T) {
		exe := filepath.Join(t.TempDir(), "rwx")
		writeExecutable(t, exe, fakeBinary("v3.0.0"))
		s := setupUpgradeTest(t, upgradeServer{tag: "v3", binary: fakeBinary("v3.1.0")}, exe, "3.0.0")

		_, err := s.service.Upgrade(cli.UpgradeConfig{Version: "3"})

		require.ErrorContains(t, err, `"3" is not a full rwx version`)
	})
}

// readOnlyInstall writes the current binary into a directory the test user cannot
// write to, like a root-owned /usr/local/bin.
func readOnlyInstall(t *testing.T) (dir string, exe string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root can write to read-only directories")
	}

	dir = filepath.Join(t.TempDir(), "bin")
	exe = filepath.Join(dir, "rwx")
	writeExecutable(t, exe, fakeBinary("v3.0.0"))
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	return dir, exe
}

// interactive makes the service behave as if a person is at the terminal and
// has typed answer at any prompt.
func interactive(t *testing.T, s *testSetup, answer string) {
	t.Helper()
	s.mockStdin.WriteString(answer)
	s.config.StdinIsTTY = true
	s.config.StderrIsTTY = true
	var err error
	s.service, err = cli.NewService(s.config)
	require.NoError(t, err)
}

// fakeSudo performs the install, mv and rm commands Upgrade runs under sudo by
// briefly making dir writable, and fails the command named failOn.
func fakeSudo(t *testing.T, dir string, failOn string) (calls *[][]string, run func(args ...string) error) {
	calls = &[][]string{}
	run = func(args ...string) error {
		*calls = append(*calls, args)
		if args[0] == failOn {
			return fmt.Errorf("sudo: %s failed", failOn)
		}

		require.NoError(t, os.Chmod(dir, 0o755))
		defer func() { require.NoError(t, os.Chmod(dir, 0o555)) }()

		switch args[0] {
		case "install":
			mode, err := strconv.ParseUint(args[2], 8, 32)
			require.NoError(t, err)
			content, err := os.ReadFile(args[3])
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(args[4], content, 0o600))
			return os.Chmod(args[4], os.FileMode(mode))
		case "mv":
			return os.Rename(args[2], args[3])
		case "-n":
			return os.Remove(args[3])
		}
		return fmt.Errorf("unexpected sudo command: %v", args)
	}
	return calls, run
}

func TestService_UpgradeWithSudo(t *testing.T) {
	t.Run("replaces a root-owned install after confirmation", func(t *testing.T) {
		dir, exe := readOnlyInstall(t)
		require.NoError(t, os.Chmod(exe, 0o555))
		s := setupUpgradeTest(t, upgradeServer{tag: "v3.1.0", binary: fakeBinary("v3.1.0")}, exe, "3.0.0")
		interactive(t, s, "y\n")
		calls, run := fakeSudo(t, dir, "")
		t.Cleanup(cli.SetSudoForTest(true, run))

		result, err := s.service.Upgrade(cli.UpgradeConfig{})

		require.NoError(t, err)
		require.True(t, result.Upgraded)
		require.Contains(t, s.mockStderr.String(), "which requires root. Replace it using sudo? [y/N]")
		require.Len(t, *calls, 2)
		require.Equal(t, "install", (*calls)[0][0])
		require.Equal(t, []string{"mv", "-f", (*calls)[0][4], result.Path}, (*calls)[1])

		content, err := os.ReadFile(exe)
		require.NoError(t, err)
		require.Equal(t, fakeBinary("v3.1.0"), string(content))
		info, err := os.Stat(exe)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o555), info.Mode().Perm())

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, entries, 1, "the staged binary should be renamed into place")
	})

	t.Run("leaves the install untouched when declined", func(t *testing.T) {
		dir, exe := readOnlyInstall(t)
		s := setupUpgradeTest(t, upgradeServer{tag: "v3.1.0", binary: fakeBinary("v3.1.0")}, exe, "3.0.0")
		interactive(t, s, "n\n")
		calls, run := fakeSudo(t, dir, "")
		t.Cleanup(cli.SetSudoForTest(true, run))

		_, err := s.service.Upgrade(cli.UpgradeConfig{})

		require.ErrorContains(t, err, "To upgrade, run: sudo ")
		require.Empty(t, *calls)
		content, err := os.ReadFile(exe)
		require.NoError(t, err)
		require.Equal(t, fakeBinary("v3.0.0"), string(content))
	})

	t.Run("does not prompt without a terminal", func(t *testing.T) {
		dir, exe := readOnlyInstall(t)
		s := setupUpgradeTest(t, upgradeServer{tag: "v3.1.0", binary: fakeBinary("v3.1.0")}, exe, "3.0.0")
		calls, run := fakeSudo(t, dir, "")
		t.Cleanup(cli.SetSudoForTest(true, run))

		_, err := s.service.Upgrade(cli.UpgradeConfig{})

		require.ErrorContains(t, err, "To upgrade, run: sudo ")
		require.NotContains(t, s.mockStderr.String(), "Replace it using sudo?")
		require.Empty(t, *calls)
	})

	t.Run("does not prompt when sudo is not installed", func(t *testing.T) {
		dir, exe := readOnlyInstall(t)
		s := setupUpgradeTest(t, upgradeServer{tag: "v3.1.0", binary: fakeBinary("v3.1.0")}, exe, "3.0.0")
		interactive(t, s, "y\n")
		calls, run := fakeSudo(t, dir, "")
		t.Cleanup(cli.SetSudoForTest(false, run))

		_, err := s.service.Upgrade(cli.UpgradeConfig{})

		require.ErrorContains(t, err, "To upgrade, run: sudo ")
		require.NotContains(t, s.mockStderr.String(), "Replace it using sudo?")
		require.Empty(t, *calls)
	})

	t.Run("removes the staged binary when the swap fails", func(t *testing.T) {
		dir, exe := readOnlyInstall(t)
		s := setupUpgradeTest(t, upgradeServer{tag: "v3.1.0", binary: fakeBinary("v3.1.0")}, exe, "3.0.0")
		interactive(t, s, "y\n")
		calls, run := fakeSudo(t, dir, "mv")
		t.Cleanup(cli.SetSudoForTest(true, run))

		_, err := s.service.Upgrade(cli.UpgradeConfig{})

		require.ErrorContains(t, err, "with sudo")
		require.Equal(t, "-n", (*calls)[len(*calls)-1][0])
		content, err := os.ReadFile(exe)
		require.NoError(t, err)
		require.Equal(t, fakeBinary("v3.0.0"), string(content))
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, entries, 1, "the staged binary should be removed")
	})
}

func TestNewService_LatestVersionMessage(t *testing.T) {
	previousLatestVersion := versions.GetCliLatestVersion().String()
	t.Cleanup(func() {
		_ = versions.SetCliLatestVersion(previousLatestVersion)
		cli.ResetLatestVersionMessageForTest()
	})
	require.NoError(t, versions.SetCliLatestVersion("99999.0.0"))

	t.Run("outputs the message by default", func(t *testing.T) {
		cli.ResetLatestVersionMessageForTest()
		s := setupSkillTest(t)

		_, err := cli.NewService(s.config)
		require.NoError(t, err)

		require.Contains(t, s.mockStderr.String(), "A new release of rwx is available")
	})

	t.Run("omits the message when hidden", func(t *testing.T) {
		cli.ResetLatestVersionMessageForTest()
		s := setupSkillTest(t)
		s.config.HideLatestVersionMessage = true

		_, err := cli.NewService(s.config)
		require.NoError(t, err)

		require.NotContains(t, s.mockStderr.String(), "A new release of rwx is available")
	})
}

func TestUpgradeAssetName(t *testing.T) {
	tests := []struct {
		goos     string
		goarch   string
		static   bool
		expected string
	}{
		{"linux", "amd64", false, "rwx-linux-x86_64"},
		{"linux", "amd64", true, "rwx-linux-x86_64-static"},
		{"linux", "arm64", false, "rwx-linux-aarch64"},
		{"linux", "arm64", true, "rwx-linux-aarch64"},
		{"darwin", "amd64", false, "rwx-darwin-x86_64"},
		{"darwin", "arm64", false, "rwx-darwin-aarch64"},
		{"windows", "amd64", false, "rwx-windows-x86_64.exe"},
		{"windows", "arm64", false, "rwx-windows-aarch64.exe"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			name, err := cli.UpgradeAssetName(tt.goos, tt.goarch, tt.static)
			require.NoError(t, err)
			require.Equal(t, tt.expected, name)
		})
	}

	t.Run("unsupported platform", func(t *testing.T) {
		_, err := cli.UpgradeAssetName("freebsd", "amd64", false)
		require.EqualError(t, err, "rwx does not publish binaries for freebsd/amd64")
	})
}
