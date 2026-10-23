package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"unicode/utf8"

	"github.com/rwx-cloud/rwx/internal/errors"
)

var errCompactCursor = errors.New("compact results cursor invalidated")

type compactFragment struct {
	Collection string          `json:"collection"`
	Position   json.RawMessage `json:"position"`
	Offset     int             `json:"offset"`
	JSON       string          `json:"json"`
	Complete   bool            `json:"complete"`
}

type compactPartial struct {
	text  string
	index int
}

func (c Client) getCompactDetails(cfg RunDetailsConfig) (map[string]any, error) {
	remaining := 10000
	for attempt := 0; attempt < 3; attempt++ {
		result, err := c.compactTree(cfg, map[string]bool{}, &remaining)
		if !errors.Is(err, errCompactCursor) {
			return result, err
		}
	}
	return nil, errors.New("compact results kept changing or cursors expired; retry the command")
}

func (c Client) compactTree(cfg RunDetailsConfig, ancestors map[string]bool, remaining *int) (map[string]any, error) {
	if len(ancestors) >= 128 {
		return nil, errors.New("compact results exceed the traversal depth limit")
	}
	result, err := c.compactPages(cfg, remaining)
	if err != nil {
		return nil, err
	}
	id, ok := result["id"].(string)
	if !ok || id == "" || ancestors[id] {
		return nil, errors.New("invalid compact results task identity or cycle")
	}
	ancestors[id] = true
	defer delete(ancestors, id)
	for _, collection := range []string{"tasks", "subtasks"} {
		children, _ := result[collection].([]any)
		for i, child := range children {
			summary, ok := child.(map[string]any)
			if !ok {
				return nil, errors.New("invalid compact results child summary")
			}
			childID, ok := summary["id"].(string)
			if !ok || childID == "" || ancestors[childID] {
				return nil, errors.New("invalid compact results child identity or cycle")
			}
			children[i], err = c.compactTree(RunDetailsConfig{RunID: childID, Compact: true}, ancestors, remaining)
			if err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}

func (c Client) compactPages(cfg RunDetailsConfig, remaining *int) (map[string]any, error) {
	var result map[string]any
	collections := map[string][]any{}
	partials := map[string]compactPartial{}
	seen := map[string]bool{}
	cursor := ""
	for {
		if *remaining <= 0 {
			return nil, errors.New("compact results exceed the 10000 request traversal limit")
		}
		*remaining--
		page, fragments, err := c.compactPage(cfg, cursor)
		if err != nil {
			return nil, err
		}
		if result == nil {
			result = page
		} else if page["id"] != result["id"] {
			return nil, errCompactCursor
		}
		for _, name := range []string{"tasks", "subtasks", "messages", "dependencies", "scoped_dependencies", "values", "links"} {
			source := page
			if name == "values" || name == "links" {
				source, _ = page["outputs"].(map[string]any)
			}
			if entries, ok := source[name].([]any); ok {
				collections[name] = append(collections[name], entries...)
			}
		}
		for _, fragment := range fragments {
			switch fragment.Collection {
			case "messages", "values", "links", "dependencies", "scoped_dependencies":
			default:
				return nil, errors.New("invalid compact results fragment collection")
			}
			key := fragment.Collection + ":" + string(fragment.Position)
			partial, exists := partials[key]
			if !exists {
				partial.index = len(collections[fragment.Collection])
				collections[fragment.Collection] = append(collections[fragment.Collection], nil)
			}
			if fragment.Offset != utf8.RuneCountInString(partial.text) {
				return nil, errors.New("invalid compact results fragment offset")
			}
			partial.text += fragment.JSON
			partials[key] = partial
			if fragment.Complete {
				var entry any
				if err := json.Unmarshal([]byte(partial.text), &entry); err != nil {
					return nil, errors.Wrap(err, "invalid compact results fragment JSON")
				}
				collections[fragment.Collection][partial.index] = entry
				delete(partials, key)
			}
		}
		truncated, ok := page["truncated"].(bool)
		if !ok {
			return nil, errors.New("server does not support compact results; use --verbose for legacy results")
		}
		next, _ := page["next_cursor"].(string)
		if !truncated {
			if next != "" || len(partials) != 0 {
				return nil, errors.New("incomplete compact results traversal")
			}
			break
		}
		if next == "" || seen[next] {
			return nil, errors.New("invalid compact results continuation")
		}
		seen[next] = true
		cursor = next
	}
	for _, name := range []string{"tasks", "subtasks", "messages", "dependencies", "scoped_dependencies"} {
		if _, exists := result[name]; exists {
			result[name] = append([]any{}, collections[name]...)
		}
	}
	result["outputs"] = map[string]any{
		"values": append([]any{}, collections["values"]...),
		"links":  append([]any{}, collections["links"]...),
	}
	delete(result, "fragments")
	delete(result, "truncated")
	delete(result, "next_cursor")
	return result, nil
}

func (c Client) compactPage(cfg RunDetailsConfig, cursor string) (map[string]any, []compactFragment, error) {
	params := url.Values{"view": {"compact"}, "limit": {"100"}}
	if cfg.TaskKey != "" {
		params.Set("task_key", cfg.TaskKey)
	}
	if cursor != "" {
		params.Set("cursor", cursor)
	}
	endpoint := "/mint/api/results/" + url.PathEscape(cfg.RunID) + "?" + params.Encode()
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "unable to create new HTTP request")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.RoundTrip(req)
	if err != nil {
		return nil, nil, errors.Wrap(err, "HTTP request failed")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil {
		return nil, nil, err
	}
	if len(body) > 8<<20 {
		return nil, nil, errors.New("compact results response exceeds 8 MiB")
	}
	var metadata struct {
		ErrorCode string            `json:"error_code"`
		Fragments []compactFragment `json:"fragments"`
	}
	_ = json.Unmarshal(body, &metadata)
	if (resp.StatusCode == http.StatusConflict && metadata.ErrorCode == "stale_cursor") ||
		(resp.StatusCode == http.StatusBadRequest && metadata.ErrorCode == "invalid_cursor") {
		return nil, nil, errCompactCursor
	}
	if resp.StatusCode == http.StatusUnprocessableEntity && cfg.TaskKey != "" {
		if err := parseAmbiguousTaskKeyError(bytes.NewReader(body), cfg.TaskKey); err != nil {
			return nil, nil, err
		}
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	var page map[string]any
	if err := decodeResponseJSON(resp, &page); err != nil {
		if metadata.ErrorCode != "" {
			return nil, nil, fmt.Errorf("compact results %s: %w", metadata.ErrorCode, err)
		}
		return nil, nil, err
	}
	if err := json.Unmarshal(body, &metadata); err != nil {
		return nil, nil, errors.Wrap(err, "invalid compact results metadata")
	}
	return page, metadata.Fragments, nil
}
