package versions

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rwx-cloud/rwx/internal/config"
	"github.com/stretchr/testify/require"
)

type mockRoundTripper struct {
	response *http.Response
	err      error
}

func (m *mockRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return m.response, m.err
}

func TestVersionInterceptor(t *testing.T) {
	t.Run("parses version from response header", func(t *testing.T) {
		versionHolder.latestVersion = EmptyVersion

		backend := NewMemoryBackend()
		mock := &mockRoundTripper{
			response: &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader([]byte{})),
				Header:     http.Header{latestVersionHeader: []string{"1000000.0.0"}},
			},
		}

		rt := NewRoundTripper(mock, backend, nil)
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		_, err := rt.RoundTrip(req)

		require.NoError(t, err)
		require.Equal(t, "1000000.0.0", GetCliLatestVersion().String())
		require.True(t, NewVersionAvailable())

		savedVersion, err := backend.Get()
		require.NoError(t, err)
		require.Equal(t, "1000000.0.0", savedVersion)
	})

	t.Run("does not update version when header is missing", func(t *testing.T) {
		versionHolder.latestVersion = EmptyVersion

		backend := NewMemoryBackend()
		mock := &mockRoundTripper{
			response: &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader([]byte{})),
				Header:     http.Header{},
			},
		}

		rt := NewRoundTripper(mock, backend, nil)
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		_, err := rt.RoundTrip(req)

		require.NoError(t, err)
		require.True(t, GetCliLatestVersion().Equal(EmptyVersion))

		savedVersion, err := backend.Get()
		require.NoError(t, err)
		require.Equal(t, "", savedVersion)
	})

	t.Run("does not update version when request errors", func(t *testing.T) {
		versionHolder.latestVersion = EmptyVersion

		backend := NewMemoryBackend()
		mock := &mockRoundTripper{
			response: nil,
			err:      io.EOF,
		}

		rt := NewRoundTripper(mock, backend, nil)
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		_, err := rt.RoundTrip(req)

		require.Error(t, err)
		require.True(t, GetCliLatestVersion().Equal(EmptyVersion))

		savedVersion, err := backend.Get()
		require.NoError(t, err)
		require.Equal(t, "", savedVersion)
	})

	t.Run("works with nil backend", func(t *testing.T) {
		versionHolder.latestVersion = EmptyVersion

		mock := &mockRoundTripper{
			response: &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader([]byte{})),
				Header:     http.Header{latestVersionHeader: []string{"2.0.0"}},
			},
		}

		rt := NewRoundTripper(mock, nil, nil)
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		_, err := rt.RoundTrip(req)

		require.NoError(t, err)
		require.Equal(t, "2.0.0", GetCliLatestVersion().String())
	})

	t.Run("fetches skill latest version when cache is empty", func(t *testing.T) {
		versionHolder.latestSkillVersion = EmptyVersion

		skillBackend := NewMemoryBackend()

		rt := NewRoundTripper(trackingSkillRoundTripper("2.5.0", nil), nil, skillBackend)

		req, _ := http.NewRequest("GET", "http://example.com/some-api", nil)
		_, err := rt.RoundTrip(req)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			return !GetSkillLatestVersion().Equal(EmptyVersion)
		}, time.Second, 10*time.Millisecond)

		require.Equal(t, "2.5.0", GetSkillLatestVersion().String())

		savedVersion, err := skillBackend.Get()
		require.NoError(t, err)
		require.Equal(t, "2.5.0", savedVersion)
	})

	t.Run("skips skill fetch when cache is fresh", func(t *testing.T) {
		versionHolder.latestSkillVersion = EmptyVersion

		skillBackend := NewMemoryBackend()
		_ = skillBackend.Set("1.0.0")

		fetched := make(chan struct{}, 1)
		rt := NewRoundTripper(trackingSkillRoundTripper("2.5.0", fetched), nil, skillBackend)

		req, _ := http.NewRequest("GET", "http://example.com/some-api", nil)
		_, err := rt.RoundTrip(req)
		require.NoError(t, err)

		select {
		case <-fetched:
			t.Fatal("expected skill version fetch to be skipped but API was called")
		case <-time.After(50 * time.Millisecond):
		}

		require.True(t, GetSkillLatestVersion().Equal(EmptyVersion))
	})

	t.Run("fetches skill latest version when cache is stale", func(t *testing.T) {
		versionHolder.latestSkillVersion = EmptyVersion

		skillBackend := NewMemoryBackend()
		_ = skillBackend.Set("1.0.0")
		skillBackend.SetModTime(time.Now().Add(-3 * time.Hour))

		rt := NewRoundTripper(trackingSkillRoundTripper("3.0.0", nil), nil, skillBackend)

		req, _ := http.NewRequest("GET", "http://example.com/some-api", nil)
		_, err := rt.RoundTrip(req)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			return !GetSkillLatestVersion().Equal(EmptyVersion)
		}, time.Second, 10*time.Millisecond)

		require.Equal(t, "3.0.0", GetSkillLatestVersion().String())

		savedVersion, err := skillBackend.Get()
		require.NoError(t, err)
		require.Equal(t, "3.0.0", savedVersion)
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// trackingSkillRoundTripper returns a round tripper that serves skill version
// responses. If fetched is non-nil, it signals when /api/skill/latest is called,
// allowing the "skips" test to prove the endpoint was never hit.
func trackingSkillRoundTripper(version string, fetched chan<- struct{}) roundTripFunc {
	return func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/skill/latest" {
			if fetched != nil {
				defer func() { fetched <- struct{}{} }()
			}
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader([]byte(fmt.Sprintf(`{"version":"%s"}`, version)))),
				Header:     http.Header{},
			}, nil
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader([]byte{})),
			Header:     http.Header{},
		}, nil
	}
}

func TestGetSetSkillLatestVersion(t *testing.T) {
	t.Run("returns EmptyVersion by default", func(t *testing.T) {
		versionHolder.latestSkillVersion = EmptyVersion
		require.True(t, GetSkillLatestVersion().Equal(EmptyVersion))
	})

	t.Run("sets and gets a valid version", func(t *testing.T) {
		versionHolder.latestSkillVersion = EmptyVersion
		err := SetSkillLatestVersion("1.2.3")
		require.NoError(t, err)
		require.Equal(t, "1.2.3", GetSkillLatestVersion().String())
	})

	t.Run("returns error for invalid version", func(t *testing.T) {
		versionHolder.latestSkillVersion = EmptyVersion
		err := SetSkillLatestVersion("not-a-version")
		require.Error(t, err)
		require.True(t, GetSkillLatestVersion().Equal(EmptyVersion))
	})
}

func TestLoadLatestSkillVersionFromFile(t *testing.T) {
	t.Run("when config backend is nil", func(t *testing.T) {
		LoadLatestSkillVersionFromFile(nil)
	})

	t.Run("when the file contains a valid version", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp(os.TempDir(), "versions-test")
		require.NoError(t, err)
		t.Cleanup(func() { os.RemoveAll(tmpDir) })

		err = os.WriteFile(filepath.Join(tmpDir, latestSkillVersionFilename), []byte("3.0.0"), 0o644)
		require.NoError(t, err)

		configBackend, err := config.NewFileBackend([]string{tmpDir})
		require.NoError(t, err)
		backend := NewSkillFileBackend(configBackend)
		versionHolder.latestSkillVersion = EmptyVersion

		LoadLatestSkillVersionFromFile(backend)
		require.Equal(t, "3.0.0", GetSkillLatestVersion().String())
	})

	t.Run("when the file does not exist", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp(os.TempDir(), "versions-test")
		require.NoError(t, err)
		t.Cleanup(func() { os.RemoveAll(tmpDir) })

		configBackend, err := config.NewFileBackend([]string{tmpDir})
		require.NoError(t, err)
		backend := NewSkillFileBackend(configBackend)
		versionHolder.latestSkillVersion = EmptyVersion

		LoadLatestSkillVersionFromFile(backend)
		require.True(t, GetSkillLatestVersion().Equal(EmptyVersion))
	})
}

func TestSaveLatestSkillVersionToFile(t *testing.T) {
	t.Run("when config backend is nil", func(t *testing.T) {
		SaveLatestSkillVersionToFile(nil)
	})

	t.Run("when latest skill version is empty", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp(os.TempDir(), "versions-test")
		require.NoError(t, err)
		t.Cleanup(func() { os.RemoveAll(tmpDir) })

		configBackend, err := config.NewFileBackend([]string{tmpDir})
		require.NoError(t, err)
		backend := NewSkillFileBackend(configBackend)
		versionHolder.latestSkillVersion = EmptyVersion

		SaveLatestSkillVersionToFile(backend)

		_, err = os.Stat(filepath.Join(tmpDir, latestSkillVersionFilename))
		require.True(t, os.IsNotExist(err))
	})

	t.Run("when latest skill version is set", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp(os.TempDir(), "versions-test")
		require.NoError(t, err)
		t.Cleanup(func() { os.RemoveAll(tmpDir) })

		configBackend, err := config.NewFileBackend([]string{tmpDir})
		require.NoError(t, err)
		backend := NewSkillFileBackend(configBackend)
		err = SetSkillLatestVersion("4.5.6")
		require.NoError(t, err)

		SaveLatestSkillVersionToFile(backend)

		contents, err := os.ReadFile(filepath.Join(tmpDir, latestSkillVersionFilename))
		require.NoError(t, err)
		require.Equal(t, "4.5.6", string(contents))
	})
}

func TestLoadLatestVersionFromFile(t *testing.T) {
	t.Run("when config backend is nil", func(t *testing.T) {
		LoadLatestVersionFromFile(nil)
	})

	t.Run("when the file does not exist", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp(os.TempDir(), "versions-test")
		require.NoError(t, err)
		t.Cleanup(func() { os.RemoveAll(tmpDir) })

		configBackend, err := config.NewFileBackend([]string{tmpDir})
		require.NoError(t, err)
		backend := NewFileBackend(configBackend)

		LoadLatestVersionFromFile(backend)
	})

	t.Run("when the file contains a valid version", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp(os.TempDir(), "versions-test")
		require.NoError(t, err)
		t.Cleanup(func() { os.RemoveAll(tmpDir) })

		err = os.WriteFile(filepath.Join(tmpDir, latestVersionFilename), []byte("1.2.3"), 0o644)
		require.NoError(t, err)

		configBackend, err := config.NewFileBackend([]string{tmpDir})
		require.NoError(t, err)
		backend := NewFileBackend(configBackend)
		versionHolder.latestVersion = EmptyVersion

		LoadLatestVersionFromFile(backend)
		require.Equal(t, "1.2.3", GetCliLatestVersion().String())
	})

	t.Run("when the file contains whitespace around the version", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp(os.TempDir(), "versions-test")
		require.NoError(t, err)
		t.Cleanup(func() { os.RemoveAll(tmpDir) })

		err = os.WriteFile(filepath.Join(tmpDir, latestVersionFilename), []byte("  2.0.0\n"), 0o644)
		require.NoError(t, err)

		configBackend, err := config.NewFileBackend([]string{tmpDir})
		require.NoError(t, err)
		backend := NewFileBackend(configBackend)
		versionHolder.latestVersion = EmptyVersion

		LoadLatestVersionFromFile(backend)
		require.Equal(t, "2.0.0", GetCliLatestVersion().String())
	})

	t.Run("when the file is empty", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp(os.TempDir(), "versions-test")
		require.NoError(t, err)
		t.Cleanup(func() { os.RemoveAll(tmpDir) })

		err = os.WriteFile(filepath.Join(tmpDir, latestVersionFilename), []byte(""), 0o644)
		require.NoError(t, err)

		configBackend, err := config.NewFileBackend([]string{tmpDir})
		require.NoError(t, err)
		backend := NewFileBackend(configBackend)
		versionHolder.latestVersion = EmptyVersion

		LoadLatestVersionFromFile(backend)
		require.True(t, GetCliLatestVersion().Equal(EmptyVersion))
	})

	t.Run("when the file contains an invalid version", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp(os.TempDir(), "versions-test")
		require.NoError(t, err)
		t.Cleanup(func() { os.RemoveAll(tmpDir) })

		err = os.WriteFile(filepath.Join(tmpDir, latestVersionFilename), []byte("not-a-version"), 0o644)
		require.NoError(t, err)

		configBackend, err := config.NewFileBackend([]string{tmpDir})
		require.NoError(t, err)
		backend := NewFileBackend(configBackend)
		versionHolder.latestVersion = EmptyVersion

		LoadLatestVersionFromFile(backend)
		require.True(t, GetCliLatestVersion().Equal(EmptyVersion))
	})
}

func TestSaveLatestVersionToFile(t *testing.T) {
	t.Run("when config backend is nil", func(t *testing.T) {
		SaveLatestVersionToFile(nil)
	})

	t.Run("when latest version is empty", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp(os.TempDir(), "versions-test")
		require.NoError(t, err)
		t.Cleanup(func() { os.RemoveAll(tmpDir) })

		configBackend, err := config.NewFileBackend([]string{tmpDir})
		require.NoError(t, err)
		backend := NewFileBackend(configBackend)
		versionHolder.latestVersion = EmptyVersion

		SaveLatestVersionToFile(backend)

		_, err = os.Stat(filepath.Join(tmpDir, latestVersionFilename))
		require.True(t, os.IsNotExist(err))
	})

	t.Run("when latest version is set", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp(os.TempDir(), "versions-test")
		require.NoError(t, err)
		t.Cleanup(func() { os.RemoveAll(tmpDir) })

		configBackend, err := config.NewFileBackend([]string{tmpDir})
		require.NoError(t, err)
		backend := NewFileBackend(configBackend)
		err = SetCliLatestVersion("3.4.5")
		require.NoError(t, err)

		SaveLatestVersionToFile(backend)

		contents, err := os.ReadFile(filepath.Join(tmpDir, latestVersionFilename))
		require.NoError(t, err)
		require.Equal(t, "3.4.5", string(contents))
	})

	t.Run("overwrites garbage file with valid version", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp(os.TempDir(), "versions-test")
		require.NoError(t, err)
		t.Cleanup(func() { os.RemoveAll(tmpDir) })

		err = os.WriteFile(filepath.Join(tmpDir, latestVersionFilename), []byte("garbage-data"), 0o644)
		require.NoError(t, err)

		configBackend, err := config.NewFileBackend([]string{tmpDir})
		require.NoError(t, err)
		backend := NewFileBackend(configBackend)
		err = SetCliLatestVersion("5.0.0")
		require.NoError(t, err)

		SaveLatestVersionToFile(backend)

		contents, err := os.ReadFile(filepath.Join(tmpDir, latestVersionFilename))
		require.NoError(t, err)
		require.Equal(t, "5.0.0", string(contents))
	})
}

func TestIsHomebrewPath(t *testing.T) {
	require.True(t, IsHomebrewPath("/opt/homebrew/Cellar/rwx/3.1.0/bin/rwx"))
	require.True(t, IsHomebrewPath("/usr/local/Cellar/rwx/3.1.0/bin/rwx"))
	require.True(t, IsHomebrewPath("/usr/local/Caskroom/rwx/3.1.0/rwx"))
	require.True(t, IsHomebrewPath("/home/linuxbrew/.linuxbrew/Cellar/rwx/3.1.0/bin/rwx"))
	require.False(t, IsHomebrewPath("/usr/local/bin/rwx"))
	require.False(t, IsHomebrewPath("/home/user/.local/bin/rwx"))
}
