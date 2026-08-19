package cli

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type RetryConfig struct {
	Target           api.RetryTarget
	Kind             string
	WithoutToolCache []string
	Debug            *bool
	DebugPlacement   string
	OutputJSON       bool
}

func (c RetryConfig) Validate() error {
	if c.Target.ID == "" {
		return errors.WrapSentinel(errors.New("you must specify a run ID or task ID"), errors.ErrBadRequest)
	}
	return nil
}

func (s Service) Retry(cfg RetryConfig) (*api.RequestRetryResult, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	options, err := s.APIClient.GetRetryOptions(cfg.Target)
	if err != nil {
		return nil, err
	}

	interactive := s.StdoutIsTTY && !cfg.OutputJSON
	var input *bufio.Reader
	if interactive {
		input = bufio.NewReader(s.Stdin)
	}

	request, err := s.selectRetryRequest(input, interactive, cfg, options)
	if err != nil {
		return nil, err
	}
	result, err := s.APIClient.RequestRetry(request)
	if err == nil {
		return &result, nil
	}

	var requestErr *api.RetryRequestError
	if !errors.As(err, &requestErr) {
		return nil, err
	}
	if interactive && cfg.bare() && requestErr.Options.Retryable && len(requestErr.Options.Kinds) > 0 {
		fmt.Fprintln(s.Stdout, "The available retry options changed.")
		fmt.Fprintln(s.Stdout)

		request, err = s.selectRetryRequest(input, true, RetryConfig{Target: cfg.Target}, requestErr.Options)
		if err != nil {
			return nil, err
		}
		result, err = s.APIClient.RequestRetry(request)
		if err == nil {
			return &result, nil
		}
		if !errors.As(err, &requestErr) {
			return nil, err
		}
	}
	if cfg.OutputJSON {
		return nil, requestErr
	}
	return nil, retryEndpointError(requestErr, request)
}

func (s Service) selectRetryRequest(
	input *bufio.Reader,
	interactive bool,
	cfg RetryConfig,
	options api.RetryOptions,
) (api.RequestRetryConfig, error) {
	if !options.Retryable {
		reason := options.UnavailableReason
		if reason == "" {
			reason = "This run or task cannot be retried in its current state."
		}
		return api.RequestRetryConfig{}, retryBadRequest(reason)
	}

	kind := cfg.Kind
	if kind == "" {
		if !interactive {
			return api.RequestRetryConfig{}, retryKindSelectionError("retry kind selection is required", cfg.Target, options.Kinds)
		}
		selected, err := s.promptForRetryKind(input, options.Kinds)
		if err != nil {
			return api.RequestRetryConfig{}, err
		}
		kind = selected
	}
	if !retryKindAvailable(kind, options.Kinds) {
		return api.RequestRetryConfig{}, retryKindSelectionError(fmt.Sprintf("retry kind %q is not available", kind), cfg.Target, options.Kinds)
	}

	toolCacheNames := uniqueStrings(cfg.WithoutToolCache)
	if kind == "no-tool-cache" {
		if len(toolCacheNames) == 0 {
			if !interactive {
				return api.RequestRetryConfig{}, retryToolCacheSelectionError(
					"tool cache selection is required for retry kind \"no-tool-cache\"",
					cfg.Target,
					options.ToolCaches,
				)
			}
			selected, err := s.promptForRetryToolCaches(input, options.ToolCaches)
			if err != nil {
				return api.RequestRetryConfig{}, err
			}
			toolCacheNames = selected
		}
		for _, name := range toolCacheNames {
			if !retryToolCacheAvailable(name, options.ToolCaches) {
				return api.RequestRetryConfig{}, retryToolCacheSelectionError(
					fmt.Sprintf("tool cache %q is not available", name),
					cfg.Target,
					options.ToolCaches,
				)
			}
		}
	} else if len(toolCacheNames) > 0 {
		return api.RequestRetryConfig{}, retryBadRequest("--without-tool-cache can only be used with --kind no-tool-cache")
	}

	debug, debugPlacement, err := s.selectRetryDebugOptions(input, interactive, cfg.Debug, cfg.DebugPlacement, options.Debug)
	if err != nil {
		return api.RequestRetryConfig{}, err
	}

	return api.RequestRetryConfig{
		Target:         cfg.Target,
		Kind:           kind,
		Debug:          debug,
		DebugPlacement: debugPlacement,
		ToolCacheNames: toolCacheNames,
	}, nil
}

func (c RetryConfig) bare() bool {
	return c.Kind == "" &&
		len(c.WithoutToolCache) == 0 &&
		c.Debug == nil &&
		c.DebugPlacement == ""
}

func (s Service) promptForRetryKind(input *bufio.Reader, kinds []api.RetryKind) (string, error) {
	if len(kinds) == 0 {
		return "", retryBadRequest("no retry kinds are available")
	}

	fmt.Fprintln(s.Stdout, "Select a retry kind:")
	for index, kind := range kinds {
		fmt.Fprintf(s.Stdout, "  %d. %s (%s)\n", index+1, kind.Label, kind.Value)
		if kind.Description != "" {
			fmt.Fprintf(s.Stdout, "     %s\n", kind.Description)
		}
	}
	fmt.Fprintln(s.Stdout)
	fmt.Fprintf(s.Stdout, "Enter a number (1-%d): ", len(kinds))

	choice, err := readRetryChoice(input, len(kinds), "retry kind")
	if err != nil {
		return "", err
	}
	return kinds[choice-1].Value, nil
}

func (s Service) promptForRetryToolCaches(input *bufio.Reader, caches []api.RetryToolCache) ([]string, error) {
	if len(caches) == 0 {
		return nil, retryBadRequest("no tool caches are available")
	}

	fmt.Fprintln(s.Stdout, "Select one or more tool caches:")
	for index, cache := range caches {
		fmt.Fprintf(s.Stdout, "  %d. %s\n", index+1, cache.Name)
		if cache.UsageDescription != "" {
			fmt.Fprintf(s.Stdout, "     %s\n", cache.UsageDescription)
		}
	}
	fmt.Fprintln(s.Stdout)
	fmt.Fprintf(s.Stdout, "Enter numbers separated by commas (1-%d): ", len(caches))

	line, err := readRetryLine(input, "no tool caches selected")
	if err != nil {
		return nil, err
	}
	parts := strings.FieldsFunc(line, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	if len(parts) == 0 {
		return nil, retryBadRequest("no tool caches selected")
	}

	selected := make([]string, 0, len(parts))
	seen := make(map[int]bool)
	for _, part := range parts {
		choice, err := strconv.Atoi(part)
		if err != nil || choice < 1 || choice > len(caches) {
			return nil, retryBadRequest(fmt.Sprintf("invalid tool cache selection: %s", line))
		}
		if !seen[choice] {
			selected = append(selected, caches[choice-1].Name)
			seen[choice] = true
		}
	}
	return selected, nil
}

func (s Service) selectRetryDebugOptions(
	input *bufio.Reader,
	interactive bool,
	debug *bool,
	placement string,
	options api.RetryDebugOptions,
) (*bool, string, error) {
	if debug == nil && placement != "" {
		enabled := true
		debug = &enabled
	}

	if debug == nil && interactive && options.Supported {
		selected, err := s.promptForRetryDebug(input)
		if err != nil {
			return nil, "", err
		}
		debug = &selected
	}

	if debug == nil {
		return nil, "", nil
	}
	if !*debug {
		if placement != "" {
			return nil, "", retryBadRequest("--debug-placement cannot be used with --debug=false")
		}
		return debug, "", nil
	}
	if !options.Supported {
		reason := options.DisabledReason
		if reason == "" {
			reason = "A breakpoint is not available for this retry target."
		}
		return nil, "", retryBadRequest(reason)
	}

	if placement == "" {
		if interactive {
			selected, err := s.promptForRetryDebugPlacement(input, options.Placements)
			if err != nil {
				return nil, "", err
			}
			placement = selected
		} else {
			placement = options.DefaultPlacement
			if placement == "" && len(options.Placements) == 1 {
				placement = options.Placements[0]
			}
		}
	}
	if !stringAvailable(placement, options.Placements) {
		return nil, "", retryBadRequest(fmt.Sprintf(
			"debug placement %q is not available; choose one of: %s",
			placement,
			strings.Join(options.Placements, ", "),
		))
	}
	return debug, placement, nil
}

func (s Service) promptForRetryDebug(input *bufio.Reader) (bool, error) {
	fmt.Fprint(s.Stdout, "Open a breakpoint? [y/N]: ")
	line, err := readRetryLine(input, "no breakpoint selection provided")
	if err != nil {
		return false, err
	}
	switch strings.ToLower(line) {
	case "", "n", "no":
		return false, nil
	case "y", "yes":
		return true, nil
	default:
		return false, retryBadRequest(fmt.Sprintf("invalid breakpoint selection: %s", line))
	}
}

func (s Service) promptForRetryDebugPlacement(input *bufio.Reader, placements []string) (string, error) {
	if len(placements) == 0 {
		return "", retryBadRequest("no breakpoint placements are available")
	}
	fmt.Fprintln(s.Stdout, "Select breakpoint placement:")
	for index, placement := range placements {
		fmt.Fprintf(s.Stdout, "  %d. %s\n", index+1, placement)
	}
	fmt.Fprintln(s.Stdout)
	fmt.Fprintf(s.Stdout, "Enter a number (1-%d): ", len(placements))

	choice, err := readRetryChoice(input, len(placements), "breakpoint placement")
	if err != nil {
		return "", err
	}
	return placements[choice-1], nil
}

func readRetryChoice(input *bufio.Reader, count int, selectionName string) (int, error) {
	line, err := readRetryLine(input, fmt.Sprintf("no %s selected", selectionName))
	if err != nil {
		return 0, err
	}
	choice, err := strconv.Atoi(line)
	if err != nil || choice < 1 || choice > count {
		return 0, retryBadRequest(fmt.Sprintf("invalid %s selection: %s", selectionName, line))
	}
	return choice, nil
}

func readRetryLine(input *bufio.Reader, emptyMessage string) (string, error) {
	line, err := input.ReadString('\n')
	if err != nil && line == "" {
		return "", retryBadRequest(emptyMessage)
	}
	return strings.TrimSpace(line), nil
}

func retryKindAvailable(value string, kinds []api.RetryKind) bool {
	for _, kind := range kinds {
		if kind.Value == value {
			return true
		}
	}
	return false
}

func retryToolCacheAvailable(name string, caches []api.RetryToolCache) bool {
	for _, cache := range caches {
		if cache.Name == name {
			return true
		}
	}
	return false
}

func stringAvailable(value string, choices []string) bool {
	for _, choice := range choices {
		if choice == value {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	unique := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		if !seen[value] {
			unique = append(unique, value)
			seen[value] = true
		}
	}
	return unique
}

func retryKindSelectionError(reason string, target api.RetryTarget, kinds []api.RetryKind) error {
	var message strings.Builder
	message.WriteString(reason)
	message.WriteString("\n\nAvailable retry kinds:\n")
	for _, kind := range kinds {
		fmt.Fprintf(&message, "  %s - %s\n", kind.Value, kind.Label)
		if kind.Description != "" {
			fmt.Fprintf(&message, "    %s\n", kind.Description)
		}
	}
	if len(kinds) > 0 {
		fmt.Fprintf(
			&message,
			"\nChoose a kind and retry:\n  %s --kind %s %s",
			retryCommand(target.Type),
			kinds[0].Value,
			target.ID,
		)
	}
	return retryBadRequest(message.String())
}

func retryToolCacheSelectionError(reason string, target api.RetryTarget, caches []api.RetryToolCache) error {
	var message strings.Builder
	message.WriteString(reason)
	message.WriteString("\n\nAvailable tool caches:\n")
	for _, cache := range caches {
		fmt.Fprintf(&message, "  %s\n", cache.Name)
		if cache.UsageDescription != "" {
			fmt.Fprintf(&message, "    %s\n", cache.UsageDescription)
		}
	}
	if len(caches) > 0 {
		fmt.Fprintf(
			&message,
			"\nChoose one or more tool caches and retry:\n  %s --kind no-tool-cache --without-tool-cache %s %s",
			retryCommand(target.Type),
			caches[0].Name,
			target.ID,
		)
	}
	return retryBadRequest(message.String())
}

func retryCommand(targetType api.RetryTargetType) string {
	switch targetType {
	case api.RetryTargetRun:
		return "rwx runs retry"
	case api.RetryTargetTask:
		return "rwx tasks retry"
	default:
		return "rwx retry"
	}
}

func retryEndpointError(requestErr *api.RetryRequestError, request api.RequestRetryConfig) error {
	var message strings.Builder
	message.WriteString(requestErr.Message)

	showKinds := false
	showToolCaches := false
	showDebugPlacements := false
	showAll := len(requestErr.Errors) == 0
	for _, fieldError := range requestErr.Errors {
		switch fieldError.Field {
		case "kind":
			fmt.Fprintf(&message, "\n\nRetry kind: %s", fieldError.Message)
			showKinds = true
		case "tool_cache_names":
			fmt.Fprintf(&message, "\n\nTool cache: %s", fieldError.Message)
			showToolCaches = true
		case "debug":
			fmt.Fprintf(&message, "\n\nBreakpoint: %s", fieldError.Message)
		case "debug_placement":
			fmt.Fprintf(&message, "\n\nBreakpoint placement: %s", fieldError.Message)
			showDebugPlacements = true
		default:
			fmt.Fprintf(&message, "\n\nRetry: %s", fieldError.Message)
			showAll = true
		}
	}
	if showAll {
		showKinds = len(requestErr.Options.Kinds) > 0
		showToolCaches = len(requestErr.Options.ToolCaches) > 0
	}

	if showKinds {
		message.WriteString("\n\nAvailable retry kinds:\n")
		for _, kind := range requestErr.Options.Kinds {
			fmt.Fprintf(&message, "  %s - %s\n", kind.Value, kind.Label)
			if kind.Description != "" {
				fmt.Fprintf(&message, "    %s\n", kind.Description)
			}
		}
	}
	if showAll && (requestErr.Options.Debug.Supported || requestErr.Options.Debug.DisabledReason != "") {
		message.WriteString("\n\nBreakpoint:\n")
		if requestErr.Options.Debug.Supported {
			message.WriteString("  Supported\n")
			message.WriteString("  Placements: ")
			for index, placement := range requestErr.Options.Debug.Placements {
				if index > 0 {
					message.WriteString(", ")
				}
				message.WriteString(placement)
				if placement == requestErr.Options.Debug.DefaultPlacement {
					message.WriteString(" (default)")
				}
			}
			message.WriteString("\n")
		} else {
			fmt.Fprintf(&message, "  %s\n", requestErr.Options.Debug.DisabledReason)
		}
	}
	if showToolCaches {
		message.WriteString("\n\nAvailable tool caches:\n")
		for _, cache := range requestErr.Options.ToolCaches {
			fmt.Fprintf(&message, "  %s\n", cache.Name)
			if cache.UsageDescription != "" {
				fmt.Fprintf(&message, "    %s\n", cache.UsageDescription)
			}
		}
	}
	if showDebugPlacements {
		message.WriteString("\n\nAvailable breakpoint placements:\n")
		for _, placement := range requestErr.Options.Debug.Placements {
			fmt.Fprintf(&message, "  %s", placement)
			if placement == requestErr.Options.Debug.DefaultPlacement {
				message.WriteString(" (default)")
			}
			message.WriteString("\n")
		}
	}

	switch {
	case showKinds && len(requestErr.Options.Kinds) > 0:
		fmt.Fprintf(
			&message,
			"\nTry:\n  %s %s --kind %s",
			retryCommand(request.Target.Type),
			request.Target.ID,
			requestErr.Options.Kinds[0].Value,
		)
	case showToolCaches && len(requestErr.Options.ToolCaches) > 0:
		fmt.Fprintf(
			&message,
			"\nTry:\n  %s %s --kind no-tool-cache --without-tool-cache %s",
			retryCommand(request.Target.Type),
			request.Target.ID,
			requestErr.Options.ToolCaches[0].Name,
		)
	case showDebugPlacements && len(requestErr.Options.Debug.Placements) > 0:
		kind := request.Kind
		if !retryKindAvailable(kind, requestErr.Options.Kinds) && len(requestErr.Options.Kinds) > 0 {
			kind = requestErr.Options.Kinds[0].Value
		}
		placement := requestErr.Options.Debug.DefaultPlacement
		if placement == "" {
			placement = requestErr.Options.Debug.Placements[0]
		}
		fmt.Fprintf(
			&message,
			"\nTry:\n  %s %s --kind %s --debug --debug-placement %s",
			retryCommand(request.Target.Type),
			request.Target.ID,
			kind,
			placement,
		)
	}

	return retryBadRequest(strings.TrimRight(message.String(), "\n"))
}

func retryBadRequest(message string) error {
	return errors.WrapSentinel(errors.New(message), errors.ErrBadRequest)
}
