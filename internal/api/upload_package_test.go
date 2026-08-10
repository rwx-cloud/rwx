package api_test

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/rwx-cloud/rwx/internal/api"
	internalErrors "github.com/rwx-cloud/rwx/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestAPIClient_UploadPackage(t *testing.T) {
	t.Run("posts the archive as multipart form data and parses the digest", func(t *testing.T) {
		var gotPath, gotMethod, gotAccept, gotFieldName, gotFileName string
		var gotFileContents []byte

		roundTrip := func(req *http.Request) (*http.Response, error) {
			gotPath = req.URL.Path
			gotMethod = req.Method
			gotAccept = req.Header.Get("Accept")

			mediaType, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
			require.NoError(t, err)
			require.Equal(t, "multipart/form-data", mediaType)

			reader := multipart.NewReader(req.Body, params["boundary"])
			part, err := reader.NextPart()
			require.NoError(t, err)

			gotFieldName = part.FormName()
			gotFileName = part.FileName()
			gotFileContents, err = io.ReadAll(part)
			require.NoError(t, err)

			_, err = reader.NextPart()
			require.ErrorIs(t, err, io.EOF, "only one part should be sent")

			return &http.Response{
				Status:     "201 Created",
				StatusCode: 201,
				Body:       io.NopCloser(strings.NewReader(`{"digest":"sha256:deadbeef"}`)),
			}, nil
		}

		c := api.NewClientWithRoundTrip(roundTrip)

		result, err := c.UploadPackage(api.UploadPackageConfig{
			FileName: "package.zip",
			Contents: bytes.NewReader([]byte("zip-bytes")),
		})
		require.NoError(t, err)
		require.Equal(t, "sha256:deadbeef", result.Digest)

		require.Equal(t, "/mint/api/leaves", gotPath)
		require.Equal(t, http.MethodPost, gotMethod)
		require.Equal(t, "application/json", gotAccept)
		require.Equal(t, "file", gotFieldName)
		require.Equal(t, "package.zip", gotFileName)
		require.Equal(t, []byte("zip-bytes"), gotFileContents)
	})

	// The bodies below are verbatim responses captured from cloud.rwx.com.
	t.Run("surfaces real package validation errors verbatim", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			body string
			want string
		}{
			{
				name: "missing rwx-package.yml lists the files that were uploaded",
				body: `{"error":"Upload did not contain rwx-package.yml\n\nFiles in upload:\n* README.md"}`,
				want: "Upload did not contain rwx-package.yml\n\nFiles in upload:\n* README.md",
			},
			{
				name: "missing README.md",
				body: `{"error":"Upload did not contain README.md\n\nFiles in upload:\n* rwx-package.yml"}`,
				want: "Upload did not contain README.md",
			},
			{
				name: "invalid yaml",
				body: `{"error":"(\u003cunknown\u003e): did not find expected ',' or ']' while parsing a flow sequence at line 1 column 7"}`,
				want: "did not find expected ',' or ']' while parsing a flow sequence at line 1 column 7",
			},
			{
				name: "missing name",
				body: `{"error":"mint-leaf.yml did not contain a name"}`,
				want: "mint-leaf.yml did not contain a name",
			},
			{
				name: "invalid version",
				body: `{"error":"mint-leaf.yml did not contain a valid version"}`,
				want: "mint-leaf.yml did not contain a valid version",
			},
			{
				name: "name owned by another organization",
				body: "{\"error\":\"mint-leaf.yml has a leaf name owned by `someoneelse`, but request was made under organization `rwx`\"}",
				want: "has a leaf name owned by `someoneelse`, but request was made under organization `rwx`",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						Status:     "400 Bad Request",
						StatusCode: 400,
						Body:       io.NopCloser(strings.NewReader(tc.body)),
					}, nil
				})

				_, err := c.UploadPackage(api.UploadPackageConfig{
					FileName: "package.zip",
					Contents: bytes.NewReader([]byte("zip-bytes")),
				})
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.want)
			})
		}
	})

	t.Run("classifies an unauthorized response so telemetry does not bucket it as unknown", func(t *testing.T) {
		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				Status:     "401 Unauthorized",
				StatusCode: 401,
				Body:       io.NopCloser(strings.NewReader(`{"error":"Unauthorized"}`)),
			}, nil
		})

		_, err := c.UploadPackage(api.UploadPackageConfig{
			FileName: "package.zip",
			Contents: bytes.NewReader([]byte("zip-bytes")),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "Unauthorized")
		require.ErrorIs(t, err, internalErrors.ErrUnauthenticated)
	})

	t.Run("classifies a server error", func(t *testing.T) {
		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				Status:     "500 Internal Server Error",
				StatusCode: 500,
				Body:       io.NopCloser(strings.NewReader(`{"status":500,"error":"Internal Server Error"}`)),
			}, nil
		})

		_, err := c.UploadPackage(api.UploadPackageConfig{
			FileName: "package.zip",
			Contents: bytes.NewReader([]byte("zip-bytes")),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "Internal Server Error")
		require.ErrorIs(t, err, internalErrors.ErrInternalServerError)
	})

	t.Run("falls back to the status when the error body is not JSON", func(t *testing.T) {
		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				Status:     "502 Bad Gateway",
				StatusCode: 502,
				Body:       io.NopCloser(strings.NewReader("<html>boom</html>")),
			}, nil
		})

		_, err := c.UploadPackage(api.UploadPackageConfig{
			FileName: "package.zip",
			Contents: bytes.NewReader([]byte("zip-bytes")),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "502 Bad Gateway")
	})
}
