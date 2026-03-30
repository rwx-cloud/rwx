package versions

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

const latestVersionHeader = "X-Rwx-Cli-Latest-Version"

const SkillVersionCacheTTL = 2 * time.Hour

type versionInterceptor struct {
	http.RoundTripper
	backend      Backend
	skillBackend Backend
	skillOnce    *sync.Once
}

func (vi versionInterceptor) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := vi.RoundTripper.RoundTrip(r)
	if err == nil {
		if lv := resp.Header.Get(latestVersionHeader); lv != "" {
			_ = SetCliLatestVersion(lv)
			SaveLatestVersionToFile(vi.backend)
		}

		vi.skillOnce.Do(func() {
			go vi.fetchSkillLatestVersion()
		})
	}
	return resp, err
}

type skillLatestVersionResponse struct {
	Version string `json:"version"`
}

func (vi versionInterceptor) fetchSkillLatestVersion() {
	if vi.skillBackend != nil {
		if modTime, err := vi.skillBackend.ModTime(); err == nil {
			if time.Since(modTime) < SkillVersionCacheTTL {
				return
			}
		}
	}

	req, err := http.NewRequest(http.MethodGet, "/api/skill/latest", nil)
	if err != nil {
		return
	}

	resp, err := vi.RoundTripper.RoundTrip(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var result skillLatestVersionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return
	}

	if result.Version == "" {
		return
	}

	if err := SetSkillLatestVersion(result.Version); err != nil {
		return
	}

	SaveLatestSkillVersionToFile(vi.skillBackend)
}

func NewRoundTripper(rt http.RoundTripper, backend Backend, skillBackend Backend) http.RoundTripper {
	return versionInterceptor{
		RoundTripper: rt,
		backend:      backend,
		skillBackend: skillBackend,
		skillOnce:    &sync.Once{},
	}
}
