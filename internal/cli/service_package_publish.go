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
	var name, version string
	if !cfg.Json {
		metadata, err := s.APIClient.GetPackageDocumentation(cfg.Digest)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to look up package %s before publishing", cfg.Digest)
		}
		name, version = metadata.Name, metadata.Version
	}
	if err := s.APIClient.PublishPackage(cfg.Digest); err != nil {
		return nil, errors.Wrapf(err, "unable to publish package %s", cfg.Digest)
	}
	result := &PackagePublishResult{Digest: cfg.Digest}
	if cfg.Json {
		if err := json.NewEncoder(s.Stdout).Encode(result); err != nil {
			return nil, errors.Wrap(err, "unable to encode JSON output")
		}
	} else {
		fmt.Fprintf(s.Stdout, "Published package %s %s with digest: %s\n", name, version, result.Digest)
	}
	return result, nil
}
