package cli

import (
	"encoding/json"
	"fmt"

	"github.com/rwx-cloud/rwx/internal/errors"
)

type PackagePublishConfig struct {
	Digest string
	Json   bool
}

type PackagePublishResult struct {
	Digest string
}

func (s Service) PublishPackage(cfg PackagePublishConfig) (*PackagePublishResult, error) {
	if err := s.APIClient.PublishPackage(cfg.Digest); err != nil {
		return nil, errors.Wrapf(err, "unable to publish package %s", cfg.Digest)
	}
	result := &PackagePublishResult{Digest: cfg.Digest}
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		fmt.Fprintf(s.Stdout, "Published package with digest: %s\n", result.Digest)
	}
	return result, nil
}
