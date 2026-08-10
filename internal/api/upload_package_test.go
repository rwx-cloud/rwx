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

	t.Run("surfaces the error body on a failure response", func(t *testing.T) {
		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				Status:     "422 Unprocessable Entity",
				StatusCode: 422,
				Body:       io.NopCloser(strings.NewReader(`{"error":"invalid package"}`)),
			}, nil
		})

		_, err := c.UploadPackage(api.UploadPackageConfig{
			FileName: "package.zip",
			Contents: bytes.NewReader([]byte("zip-bytes")),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid package")
	})

	t.Run("falls back to the status when the error body is not JSON", func(t *testing.T) {
		c := api.NewClientWithRoundTrip(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				Status:     "500 Internal Server Error",
				StatusCode: 500,
				Body:       io.NopCloser(strings.NewReader("boom")),
			}, nil
		})

		_, err := c.UploadPackage(api.UploadPackageConfig{
			FileName: "package.zip",
			Contents: bytes.NewReader([]byte("zip-bytes")),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "500 Internal Server Error")
	})
}
