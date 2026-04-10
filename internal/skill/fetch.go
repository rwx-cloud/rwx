package skill

import (
	"io"
	"net/http"

	"github.com/rwx-cloud/rwx/internal/errors"
)

const skillContentURL = "https://raw.githubusercontent.com/rwx-cloud/skills/main/plugins/rwx/skills/rwx/SKILL.md"

// FetchSkillContent downloads the latest SKILL.md content from GitHub.
func FetchSkillContent() (string, error) {
	resp, err := http.Get(skillContentURL)
	if err != nil {
		return "", errors.Wrap(err, "unable to fetch skill content")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("unable to fetch skill content: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "unable to read skill content")
	}

	return string(body), nil
}
