package cli

import (
	"net/url"
	"strings"

	"github.com/rwx-cloud/rwx/internal/errors"
)

func (s Service) discoveryRepository(repository string) (string, error) {
	if repository == "" {
		origin := s.VCSClient.GetOriginUrl()
		if strings.Contains(origin, "://") {
			parsed, err := url.Parse(origin)
			if err == nil && parsed.Host != "" {
				repository = strings.TrimPrefix(parsed.Path, "/")
			}
		} else if _, path, ok := strings.Cut(origin, ":"); ok {
			repository = path
		}
		repository = strings.TrimSuffix(repository, ".git")
	}

	parts := strings.Split(repository, "/")
	valid := len(parts) >= 2 && !strings.ContainsAny(repository, " \t\r\n\\:?#")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			valid = false
		}
	}
	if !valid {
		return "", errors.New("--organization requires a repository in owner/repo form; provide --repo (results) or --repository (runs list), or configure a Git origin with an owner/repo path")
	}
	return repository, nil
}
