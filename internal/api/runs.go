package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/rwx-cloud/rwx/internal/errors"
)

// RunFilterValidationEntry is one leaf of the structured 400 returned when a
// closed-enum status filter has an unknown value. SuggestedValue is the single
// best correction (nil for gibberish), distinct from the soft-200
// RunFilterSuggestion leaf whose `suggestions` is a list.
type RunFilterValidationEntry struct {
	Key            string   `json:"key"`
	ProvidedValue  string   `json:"provided_value"`
	SuggestedValue *string  `json:"suggested_value"`
	ValidValues    []string `json:"valid_values"`
}

// InvalidRunFilterError wraps the structured 400 body so the command can render
// a "did you mean" hint per entry, falling back to the valid set when the server
// has no single suggestion.
type InvalidRunFilterError struct {
	Entries []RunFilterValidationEntry
}

func (e *InvalidRunFilterError) Error() string {
	var b strings.Builder
	b.WriteString("invalid run filter:")
	for _, entry := range e.Entries {
		b.WriteString(fmt.Sprintf("\n  %s %q is not valid", entry.Key, entry.ProvidedValue))
		if entry.SuggestedValue != nil && *entry.SuggestedValue != "" {
			b.WriteString(fmt.Sprintf(" (did you mean %q?)", *entry.SuggestedValue))
		} else if len(entry.ValidValues) > 0 {
			b.WriteString(fmt.Sprintf(" (valid: %s)", strings.Join(entry.ValidValues, ", ")))
		}
	}
	return b.String()
}

func (e *InvalidRunFilterError) Unwrap() error {
	return errors.ErrBadRequest
}

// RateLimitedError surfaces a 429 from the runs index. The server replies with an
// empty body, so the CLI synthesizes its own message from the Retry-After window.
type RateLimitedError struct {
	RetryAfterSeconds int
}

func (e *RateLimitedError) Error() string {
	if e.RetryAfterSeconds > 0 {
		return fmt.Sprintf("rate limited by RWX (100 requests/min). Retry after %d seconds.", e.RetryAfterSeconds)
	}
	return "rate limited by RWX (100 requests/min). Please retry shortly."
}

// queryParams maps the config onto the index's accepted parameters. Each filter
// is sent in the scalar form for a lone value and the plural `[]` form once there
// is more than one.
func (c ListRunsConfig) queryParams() url.Values {
	params := url.Values{}

	setFilterParam(params, "repository_name", "repository_names[]", c.RepositoryNames)
	setFilterParam(params, "branch", "branch_names[]", c.Branches)
	setFilterParam(params, "commit_sha", "commit_shas[]", c.CommitShas)
	setFilterParam(params, "definition_path", "definition_paths[]", c.DefinitionPaths)
	setFilterParam(params, "result_status", "result_statuses[]", c.ResultStatuses)
	setFilterParam(params, "execution_status", "execution_statuses[]", c.ExecutionStatuses)

	if c.MyRuns {
		params.Set("my_runs", "true")
	}
	if c.Limit > 0 {
		params.Set("limit", strconv.Itoa(c.Limit))
	}
	if c.Cursor != "" {
		params.Set("cursor", c.Cursor)
	}

	return params
}

func setFilterParam(params url.Values, scalarKey, pluralKey string, values []string) {
	switch len(values) {
	case 0:
		return
	case 1:
		params.Set(scalarKey, values[0])
	default:
		for _, value := range values {
			params.Add(pluralKey, value)
		}
	}
}

func (c Client) ListRuns(cfg ListRunsConfig) (*ListRunsResult, error) {
	endpoint := "/mint/api/runs?" + cfg.queryParams().Encode()

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create new HTTP request")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.RoundTrip(req)
	if err != nil {
		return nil, errors.Wrap(err, "HTTP request failed")
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		result := ListRunsResult{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, errors.Wrap(err, "unable to parse API response")
		}
		return &result, nil
	case http.StatusBadRequest:
		return nil, parseInvalidRunFilterError(resp.Body)
	case http.StatusTooManyRequests:
		// The 429 body is empty; Retry-After carries the window in seconds.
		retryAfter, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		return nil, &RateLimitedError{RetryAfterSeconds: retryAfter}
	default:
		msg := extractErrorMessage(resp.Body)
		if msg == "" {
			msg = fmt.Sprintf("Unable to call RWX API - %s", resp.Status)
		}
		return nil, classifyHTTPStatusError(resp.StatusCode, msg)
	}
}

// parseInvalidRunFilterError handles both 400 shapes from the index: the
// structured status-filter body ({"errors": [...]}) and the malformed-cursor body
// ({"error": "Invalid cursor"}).
func parseInvalidRunFilterError(body io.Reader) error {
	bodyBytes, readErr := io.ReadAll(body)
	if readErr != nil {
		return errors.Wrap(errors.ErrBadRequest, "Unable to call RWX API - 400 Bad Request")
	}

	structured := struct {
		Errors []RunFilterValidationEntry `json:"errors"`
	}{}
	if err := json.Unmarshal(bodyBytes, &structured); err == nil && len(structured.Errors) > 0 {
		return &InvalidRunFilterError{Entries: structured.Errors}
	}

	msg := extractErrorMessage(strings.NewReader(string(bodyBytes)))
	if msg == "" {
		msg = "Unable to call RWX API - 400 Bad Request"
	}
	return errors.Wrap(errors.ErrBadRequest, msg)
}
