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
	Action           string
	WithoutToolCache []string
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
	if interactive && cfg.bare() && requestErr.Options.Retryable && len(requestErr.Options.Actions) > 0 {
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

	action := cfg.Action
	if action == "" && len(options.Actions) == 1 {
		action = options.Actions[0].Value
	}
	if action == "" {
		if !interactive {
			return api.RequestRetryConfig{}, retryActionSelectionError("retry action selection is required", cfg.Target, options.Actions)
		}
		selected, err := s.promptForRetryAction(input, options.Actions)
		if err != nil {
			return api.RequestRetryConfig{}, err
		}
		action = selected
	}
	if !retryActionAvailable(action, options.Actions) {
		return api.RequestRetryConfig{}, retryActionSelectionError(fmt.Sprintf("retry action %q is not available", action), cfg.Target, options.Actions)
	}

	toolCacheNames := uniqueStrings(cfg.WithoutToolCache)
	if action == "no-tool-cache" {
		if len(toolCacheNames) == 0 && len(options.ToolCaches) == 1 {
			toolCacheNames = []string{options.ToolCaches[0].Name}
		}
		if len(toolCacheNames) == 0 {
			if !interactive {
				return api.RequestRetryConfig{}, retryToolCacheSelectionError(
					"tool cache selection is required for retry action \"no-tool-cache\"",
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
		return api.RequestRetryConfig{}, retryBadRequest("--without-tool-cache can only be used with --action no-tool-cache")
	}

	debug, debugPlacement, err := selectRetryDebugOptions(cfg.DebugPlacement, options.Debug)
	if err != nil {
		return api.RequestRetryConfig{}, err
	}

	return api.RequestRetryConfig{
		Target:         cfg.Target,
		Action:         action,
		Debug:          debug,
		DebugPlacement: debugPlacement,
		ToolCacheNames: toolCacheNames,
	}, nil
}

func (c RetryConfig) bare() bool {
	return c.Action == "" &&
		len(c.WithoutToolCache) == 0 &&
		c.DebugPlacement == ""
}

func (s Service) promptForRetryAction(input *bufio.Reader, actions []api.RetryAction) (string, error) {
	if len(actions) == 0 {
		return "", retryBadRequest("no retry actions are available")
	}

	fmt.Fprintln(s.Stdout, "Select a retry action:")
	for index, action := range actions {
		fmt.Fprintf(s.Stdout, "  %d. %s (%s)\n", index+1, action.Label, action.Value)
		if action.Description != "" {
			fmt.Fprintf(s.Stdout, "     %s\n", action.Description)
		}
	}
	fmt.Fprintln(s.Stdout)
	fmt.Fprintf(s.Stdout, "Enter a number (1-%d): ", len(actions))

	choice, err := readRetryChoice(input, len(actions), "retry action")
	if err != nil {
		return "", err
	}
	return actions[choice-1].Value, nil
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

func selectRetryDebugOptions(placement string, options api.RetryDebugOptions) (*bool, string, error) {
	if placement == "" {
		return nil, "", nil
	}
	if !options.Supported {
		reason := options.DisabledReason
		if reason == "" {
			reason = "A breakpoint is not available for this retry target."
		}
		return nil, "", retryBadRequest(reason)
	}

	if !stringAvailable(placement, options.Placements) {
		return nil, "", retryBadRequest(fmt.Sprintf(
			"breakpoint placement %q is not available; choose one of: %s",
			placement,
			strings.Join(options.Placements, ", "),
		))
	}
	enabled := true
	return &enabled, placement, nil
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

func retryActionAvailable(value string, actions []api.RetryAction) bool {
	for _, action := range actions {
		if action.Value == value {
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

func retryActionSelectionError(reason string, target api.RetryTarget, actions []api.RetryAction) error {
	var message strings.Builder
	message.WriteString(reason)
	message.WriteString("\n\nAvailable retry actions:\n")
	for _, action := range actions {
		fmt.Fprintf(&message, "  %s - %s\n", action.Value, action.Label)
		if action.Description != "" {
			fmt.Fprintf(&message, "    %s\n", action.Description)
		}
	}
	if len(actions) > 0 {
		fmt.Fprintf(
			&message,
			"\nChoose an action and retry:\n  %s --action %s %s",
			retryCommand(target.Type),
			actions[0].Value,
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
			"\nChoose one or more tool caches and retry:\n  %s --action no-tool-cache --without-tool-cache %s %s",
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

	showActions := false
	showToolCaches := false
	showDebugPlacements := false
	showAll := len(requestErr.Errors) == 0
	for _, fieldError := range requestErr.Errors {
		switch fieldError.Field {
		case "action":
			fmt.Fprintf(&message, "\n\nRetry action: %s", fieldError.Message)
			showActions = true
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
		showActions = len(requestErr.Options.Actions) > 0
		showToolCaches = len(requestErr.Options.ToolCaches) > 0
	}

	if showActions {
		message.WriteString("\n\nAvailable retry actions:\n")
		for _, action := range requestErr.Options.Actions {
			fmt.Fprintf(&message, "  %s - %s\n", action.Value, action.Label)
			if action.Description != "" {
				fmt.Fprintf(&message, "    %s\n", action.Description)
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
	case showActions && len(requestErr.Options.Actions) > 0:
		fmt.Fprintf(
			&message,
			"\nTry:\n  %s %s --action %s",
			retryCommand(request.Target.Type),
			request.Target.ID,
			requestErr.Options.Actions[0].Value,
		)
	case showToolCaches && len(requestErr.Options.ToolCaches) > 0:
		fmt.Fprintf(
			&message,
			"\nTry:\n  %s %s --action no-tool-cache --without-tool-cache %s",
			retryCommand(request.Target.Type),
			request.Target.ID,
			requestErr.Options.ToolCaches[0].Name,
		)
	case showDebugPlacements && len(requestErr.Options.Debug.Placements) > 0:
		action := request.Action
		if !retryActionAvailable(action, requestErr.Options.Actions) && len(requestErr.Options.Actions) > 0 {
			action = requestErr.Options.Actions[0].Value
		}
		placement := requestErr.Options.Debug.DefaultPlacement
		if placement == "" {
			placement = requestErr.Options.Debug.Placements[0]
		}
		fmt.Fprintf(
			&message,
			"\nTry:\n  %s %s --action %s --debug %s",
			retryCommand(request.Target.Type),
			request.Target.ID,
			action,
			placement,
		)
	}

	return retryBadRequest(strings.TrimRight(message.String(), "\n"))
}

func retryBadRequest(message string) error {
	return errors.WrapSentinel(errors.New(message), errors.ErrBadRequest)
}
