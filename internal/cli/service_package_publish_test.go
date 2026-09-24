package cli_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/stretchr/testify/require"
)

func TestService_PublishPackage(t *testing.T) {
	for _, build := range []bool{false, true} {
		for _, jsonOutput := range []bool{false, true} {
			s := setupTest(t)
			dir := writePackageFixture(t, filepath.Join(s.tmp, "pkg"))
			var calls []string
			s.mockAPI.MockUploadPackage = func(api.UploadPackageConfig) (*api.UploadPackageResult, error) {
				calls = append(calls, "upload")
				return &api.UploadPackageResult{Digest: "abc123"}, nil
			}
			s.mockAPI.MockPublishPackage = func(digest string) error {
				require.Equal(t, "abc123", digest)
				calls = append(calls, "publish")
				return nil
			}
			if build {
				result, err := s.service.BuildPackage(cli.PackageBuildConfig{Directory: dir, Publish: true, Json: jsonOutput})
				require.NoError(t, err)
				require.Equal(t, "abc123", result.Digest)
				require.Equal(t, []string{"upload", "publish"}, calls)
			} else {
				result, err := s.service.PublishPackage(cli.PackagePublishConfig{Digest: "abc123", Json: jsonOutput})
				require.NoError(t, err)
				require.Equal(t, "abc123", result.Digest)
				require.Equal(t, []string{"publish"}, calls)
			}
			if jsonOutput {
				require.JSONEq(t, `{"Digest":"abc123"}`, s.mockStdout.String())
			} else {
				require.Equal(t, "Published package with digest: abc123\n", s.mockStdout.String())
			}
		}
	}
}

func TestService_BuildPackagePublishFailures(t *testing.T) {
	for _, stage := range []string{"upload", "empty digest", "publish"} {
		t.Run(stage, func(t *testing.T) {
			s := setupTest(t)
			dir := writePackageFixture(t, filepath.Join(s.tmp, "pkg"))
			failure := errors.New("registry rejected request")
			s.mockAPI.MockUploadPackage = func(api.UploadPackageConfig) (*api.UploadPackageResult, error) {
				if stage == "upload" {
					return nil, failure
				}
				if stage == "empty digest" {
					return &api.UploadPackageResult{}, nil
				}
				return &api.UploadPackageResult{Digest: "retry-digest"}, nil
			}
			published := false
			s.mockAPI.MockPublishPackage = func(digest string) error {
				published = true
				require.Equal(t, "retry-digest", digest)
				return failure
			}
			result, err := s.service.BuildPackage(cli.PackageBuildConfig{Directory: dir, Publish: true})
			require.Error(t, err)
			require.Nil(t, result)
			require.Empty(t, s.mockStdout.String())
			require.Equal(t, stage == "publish", published)
			if stage != "empty digest" {
				require.ErrorIs(t, err, failure)
			}
			if stage == "publish" {
				require.ErrorContains(t, err, "unable to publish package retry-digest")
			}
		})
	}
}
