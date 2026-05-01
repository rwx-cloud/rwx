package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/rwx-cloud/rwx/internal/messages"
	"github.com/rwx-cloud/rwx/internal/telemetry"
)

type CheckOutputFormat int

const (
	CheckOutputMultiLine CheckOutputFormat = iota
	CheckOutputOneLine
	CheckOutputJSON
	CheckOutputNone
)

type CheckConfig struct {
	RwxDirectory       string
	OutputFormat       CheckOutputFormat
	Timeout            time.Duration
	Files              []string
	Fix                bool
	TelemetryCollector *telemetry.Collector
}

type CheckResult struct {
	Diagnostics []CheckDiagnostic
	FileCount   int
	FixedCount  int
}

type CheckDiagnostic struct {
	Severity   string
	Message    string
	FilePath   string
	Line       int
	Column     int
	StackTrace []messages.StackEntry
}

type textEdit struct {
	StartLine int
	StartChar int
	EndLine   int
	EndChar   int
	NewText   string
}

type fileFixResult struct {
	OriginalPath string
	DiagFilePath string
	NewContent   string
	FixCount     int
}

func NewCheckConfig(rwxDir string, formatString string, timeout time.Duration, files []string, fix bool) (CheckConfig, error) {
	var format CheckOutputFormat

	switch formatString {
	case "none":
		format = CheckOutputNone
	case "oneline":
		format = CheckOutputOneLine
	case "text", "multiline":
		format = CheckOutputMultiLine
	case "json":
		format = CheckOutputJSON
	default:
		return CheckConfig{}, errors.New("unknown output format, expected one of: none, oneline, multiline, json, text")
	}

	return CheckConfig{
		RwxDirectory: rwxDir,
		OutputFormat: format,
		Timeout:      timeout,
		Files:        files,
		Fix:          fix,
	}, nil
}

func Check(ctx context.Context, cfg CheckConfig, stdout io.Writer) (*CheckResult, error) {
	nodePath, warning, err := findNode(cfg.TelemetryCollector)
	if err != nil {
		return nil, err
	}
	if warning != "" {
		fmt.Fprintln(os.Stderr, warning)
	}

	serverJS, err := ensureBundle()
	if err != nil {
		return nil, err
	}

	rwxDirectoryPath, err := cli.FindAndValidateRwxDirectoryPath(cfg.RwxDirectory)
	if err != nil {
		return nil, errors.Wrap(err, "unable to find .rwx directory")
	}
	if rwxDirectoryPath == "" {
		return nil, errors.New("no .rwx or .mint directory found")
	}

	yamlFiles, err := cli.GetFileOrDirectoryYAMLEntries(cfg.Files, rwxDirectoryPath)
	if err != nil {
		return nil, err
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, nodePath, serverJS, "--stdio")
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, errors.Wrap(err, "unable to create stdin pipe")
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.Wrap(err, "unable to create stdout pipe")
	}
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "unable to start language server")
	}

	conn := newJSONRPCConn(stdoutPipe, stdinPipe)
	go conn.readLoop(ctx)

	diagnostics, fixes, err := runCheckProtocol(ctx, conn, rwxDirectoryPath, yamlFiles, cfg.Fix)

	// Always attempt graceful shutdown regardless of diagnostic errors
	shutdownErr := shutdownServer(conn, stdinPipe, cmd)

	if err != nil {
		return nil, err
	}
	if shutdownErr != nil {
		return nil, shutdownErr
	}

	fixedCount := 0
	fixedFiles := make(map[string]bool)
	for _, fixResult := range fixes {
		info, err := os.Stat(fixResult.OriginalPath)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to stat %s", fixResult.OriginalPath)
		}
		if err := os.WriteFile(fixResult.OriginalPath, []byte(fixResult.NewContent), info.Mode()); err != nil {
			return nil, errors.Wrapf(err, "unable to write fix to %s", fixResult.OriginalPath)
		}
		fixedCount += fixResult.FixCount
		fixedFiles[fixResult.DiagFilePath] = true
	}

	// Remove diagnostics from files that had fixes applied, since those
	// diagnostics are no longer accurate after the file has been rewritten.
	if len(fixedFiles) > 0 {
		filtered := diagnostics[:0]
		for _, d := range diagnostics {
			if !fixedFiles[d.FilePath] {
				filtered = append(filtered, d)
			}
		}
		diagnostics = filtered
	}

	result := &CheckResult{
		Diagnostics: diagnostics,
		FileCount:   len(yamlFiles),
		FixedCount:  fixedCount,
	}

	if outputErr := outputCheckResult(stdout, cfg.OutputFormat, result); outputErr != nil {
		return nil, errors.Wrap(outputErr, "unable to output check results")
	}

	return result, nil
}

func runCheckProtocol(ctx context.Context, conn *jsonrpcConn, rwxDirectoryPath string, yamlFiles []cli.RwxDirectoryEntry, fix bool) ([]CheckDiagnostic, []fileFixResult, error) {
	// initialize
	initParams := map[string]any{
		"processId": nil,
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"diagnostic": map[string]any{},
			},
		},
		"rootUri": pathToURI(rwxDirectoryPath),
	}
	_, err := conn.request(ctx, "initialize", initParams)
	if err != nil {
		return nil, nil, errors.WrapSentinel(errors.Wrap(err, "LSP initialize failed"), errors.ErrLSP)
	}

	// initialized
	if err := conn.notify("initialized", map[string]any{}); err != nil {
		return nil, nil, errors.WrapSentinel(errors.Wrap(err, "LSP initialized notification failed"), errors.ErrLSP)
	}

	// Open files and pull diagnostics
	var allDiagnostics []CheckDiagnostic
	var allFixes []fileFixResult

	for _, entry := range yamlFiles {
		absPath, err := filepath.Abs(entry.OriginalPath)
		if err != nil {
			absPath = entry.OriginalPath
		}
		absPath, err = filepath.EvalSymlinks(absPath)
		if err != nil {
			// Fall back to the abs path without symlink resolution
			absPath, _ = filepath.Abs(entry.OriginalPath)
		}

		uri := "file://" + absPath

		// textDocument/didOpen
		didOpenParams := map[string]any{
			"textDocument": map[string]any{
				"uri":        uri,
				"languageId": "yaml",
				"version":    1,
				"text":       entry.FileContents,
			},
		}
		if err := conn.notify("textDocument/didOpen", didOpenParams); err != nil {
			return nil, nil, errors.WrapSentinel(errors.Wrapf(err, "didOpen failed for %s", entry.OriginalPath), errors.ErrLSP)
		}

		// textDocument/diagnostic (pull diagnostics)
		diagParams := map[string]any{
			"textDocument": map[string]any{
				"uri": uri,
			},
		}
		result, err := conn.request(ctx, "textDocument/diagnostic", diagParams)
		if err != nil {
			return nil, nil, errors.WrapSentinel(errors.Wrapf(err, "diagnostic request failed for %s", entry.OriginalPath), errors.ErrLSP)
		}

		fileDiags, err := parseDiagnosticResult(result, entry.OriginalPath)
		if err != nil {
			return nil, nil, errors.WrapSentinel(errors.Wrapf(err, "parsing diagnostics for %s", entry.OriginalPath), errors.ErrLSP)
		}
		allDiagnostics = append(allDiagnostics, fileDiags...)

		if fix && len(fileDiags) > 0 {
			rawItems := parseRawDiagnosticItems(result)
			if len(rawItems) > 0 {
				edits, err := requestCodeActions(ctx, conn, uri, rawItems)
				if err != nil {
					return nil, nil, errors.WrapSentinel(errors.Wrapf(err, "code action request failed for %s", entry.OriginalPath), errors.ErrLSP)
				}
				if len(edits) > 0 {
					newContent := applyTextEdits(entry.FileContents, edits)
					allFixes = append(allFixes, fileFixResult{
						OriginalPath: absPath,
						DiagFilePath: entry.OriginalPath,
						NewContent:   newContent,
						FixCount:     len(edits),
					})
				}
			}
		}
	}

	return allDiagnostics, allFixes, nil
}

func shutdownServer(conn *jsonrpcConn, stdinPipe io.WriteCloser, cmd *exec.Cmd) error {
	// Use a fresh context so the shutdown timeout is independent of the parent
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	_, _ = conn.request(shutdownCtx, "shutdown", nil)
	_ = conn.notify("exit", nil)
	_ = stdinPipe.Close()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-done:
		return nil
	case <-shutdownCtx.Done():
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return nil
	}
}

func parseDiagnosticResult(result json.RawMessage, filePath string) ([]CheckDiagnostic, error) {
	var report struct {
		Items []struct {
			Severity int    `json:"severity"`
			Message  string `json:"message"`
			Range    struct {
				Start struct {
					Line      int `json:"line"`
					Character int `json:"character"`
				} `json:"start"`
			} `json:"range"`
			RelatedInformation []struct {
				Location struct {
					URI   string `json:"uri"`
					Range struct {
						Start struct {
							Line      int `json:"line"`
							Character int `json:"character"`
						} `json:"start"`
					} `json:"range"`
				} `json:"location"`
				Message string `json:"message"`
			} `json:"relatedInformation"`
		} `json:"items"`
	}

	if err := json.Unmarshal(result, &report); err != nil {
		return nil, errors.Wrap(err, "unable to parse diagnostic response")
	}

	var diagnostics []CheckDiagnostic
	for _, item := range report.Items {
		severity := lspSeverityString(item.Severity)

		var stackTrace []messages.StackEntry
		for _, ri := range item.RelatedInformation {
			fileName := strings.TrimPrefix(ri.Location.URI, "file://")
			stackTrace = append(stackTrace, messages.StackEntry{
				FileName: fileName,
				Line:     ri.Location.Range.Start.Line + 1,
				Column:   ri.Location.Range.Start.Character + 1,
				Name:     ri.Message,
			})
		}

		diagnostics = append(diagnostics, CheckDiagnostic{
			Severity:   severity,
			Message:    item.Message,
			FilePath:   filePath,
			Line:       item.Range.Start.Line + 1, // LSP lines are 0-based
			Column:     item.Range.Start.Character + 1,
			StackTrace: stackTrace,
		})
	}

	return diagnostics, nil
}

func lspSeverityString(severity int) string {
	switch severity {
	case 1:
		return "error"
	case 2:
		return "warning"
	case 3:
		return "info"
	case 4:
		return "hint"
	default:
		return "error"
	}
}

func parseRawDiagnosticItems(result json.RawMessage) []json.RawMessage {
	var report struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(result, &report); err != nil {
		return nil
	}
	return report.Items
}

func requestCodeActions(ctx context.Context, conn *jsonrpcConn, uri string, rawDiagnostics []json.RawMessage) ([]textEdit, error) {
	params := map[string]any{
		"textDocument": map[string]any{
			"uri": uri,
		},
		"range": map[string]any{
			"start": map[string]any{"line": 0, "character": 0},
			"end":   map[string]any{"line": math.MaxInt32, "character": 0},
		},
		"context": map[string]any{
			"diagnostics": rawDiagnostics,
		},
	}

	result, err := conn.request(ctx, "textDocument/codeAction", params)
	if err != nil {
		return nil, errors.Wrap(err, "codeAction request failed")
	}

	var actions []struct {
		Edit struct {
			Changes map[string][]struct {
				Range struct {
					Start struct {
						Line      int `json:"line"`
						Character int `json:"character"`
					} `json:"start"`
					End struct {
						Line      int `json:"line"`
						Character int `json:"character"`
					} `json:"end"`
				} `json:"range"`
				NewText string `json:"newText"`
			} `json:"changes"`
		} `json:"edit"`
	}

	if err := json.Unmarshal(result, &actions); err != nil {
		return nil, errors.Wrap(err, "unable to parse code actions response")
	}

	var edits []textEdit
	for _, action := range actions {
		for _, fileEdits := range action.Edit.Changes {
			for _, e := range fileEdits {
				edits = append(edits, textEdit{
					StartLine: e.Range.Start.Line,
					StartChar: e.Range.Start.Character,
					EndLine:   e.Range.End.Line,
					EndChar:   e.Range.End.Character,
					NewText:   e.NewText,
				})
			}
		}
	}

	return edits, nil
}

func applyTextEdits(content string, edits []textEdit) string {
	if len(edits) == 0 {
		return content
	}

	// Sort edits bottom-to-top so earlier offsets remain valid
	sorted := make([]textEdit, len(edits))
	copy(sorted, edits)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].StartLine != sorted[j].StartLine {
			return sorted[i].StartLine > sorted[j].StartLine
		}
		return sorted[i].StartChar > sorted[j].StartChar
	})

	// Deduplicate overlapping edits — keep the first (bottom-most) edit for each range
	deduped := sorted[:0]
	for i, edit := range sorted {
		if i > 0 {
			prev := deduped[len(deduped)-1]
			if edit.EndLine > prev.StartLine || (edit.EndLine == prev.StartLine && edit.EndChar > prev.StartChar) {
				continue
			}
		}
		deduped = append(deduped, edit)
	}
	sorted = deduped

	lines := strings.Split(content, "\n")

	for _, edit := range sorted {
		startLine := edit.StartLine
		startChar := edit.StartChar
		endLine := edit.EndLine
		endChar := edit.EndChar

		if startLine >= len(lines) {
			startLine = len(lines) - 1
			startChar = len(lines[startLine])
		}
		if endLine >= len(lines) {
			endLine = len(lines) - 1
			endChar = len(lines[endLine])
		}

		prefix := lines[startLine][:startChar]
		suffix := lines[endLine][endChar:]

		replacement := prefix + edit.NewText + suffix
		replacementLines := strings.Split(replacement, "\n")

		newLines := make([]string, 0, len(lines)-endLine+startLine+len(replacementLines))
		newLines = append(newLines, lines[:startLine]...)
		newLines = append(newLines, replacementLines...)
		newLines = append(newLines, lines[endLine+1:]...)
		lines = newLines
	}

	return strings.Join(lines, "\n")
}

func pathToURI(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	absPath, err = filepath.EvalSymlinks(absPath)
	if err != nil {
		absPath, _ = filepath.Abs(path)
	}
	return "file://" + absPath
}

func outputCheckResult(w io.Writer, format CheckOutputFormat, result *CheckResult) error {
	switch format {
	case CheckOutputMultiLine:
		return outputCheckMultiLine(w, result)
	case CheckOutputOneLine:
		return outputCheckOneLine(w, result)
	case CheckOutputJSON:
		return outputCheckJSON(w, result)
	case CheckOutputNone:
		return nil
	}
	return nil
}

func outputCheckMultiLine(w io.Writer, result *CheckResult) error {
	for _, d := range result.Diagnostics {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "%s:%d:%d  [%s]\n", d.FilePath, d.Line, d.Column, d.Severity)
		fmt.Fprintln(w, d.Message)
		for _, entry := range d.StackTrace {
			if entry.Name != "" {
				fmt.Fprintf(w, "  at %s (%s:%d:%d)\n", entry.Name, entry.FileName, entry.Line, entry.Column)
			} else {
				fmt.Fprintf(w, "  at %s:%d:%d\n", entry.FileName, entry.Line, entry.Column)
			}
		}
	}

	pluralizedProblems := "problems"
	if len(result.Diagnostics) == 1 {
		pluralizedProblems = "problem"
	}

	pluralizedFiles := "files"
	if result.FileCount == 1 {
		pluralizedFiles = "file"
	}

	fmt.Fprintf(w, "\nChecked %d %s and found %d %s.\n", result.FileCount, pluralizedFiles, len(result.Diagnostics), pluralizedProblems)

	if result.FixedCount > 0 {
		pluralizedFixes := "fixes"
		if result.FixedCount == 1 {
			pluralizedFixes = "fix"
		}
		fmt.Fprintf(w, "Applied %d %s.\n", result.FixedCount, pluralizedFixes)
	}

	return nil
}

func outputCheckOneLine(w io.Writer, result *CheckResult) error {
	for _, d := range result.Diagnostics {
		fmt.Fprintf(w, "%-8s%s:%d:%d - %s\n", d.Severity, d.FilePath, d.Line, d.Column, strings.TrimSuffix(strings.ReplaceAll(d.Message, "\n", " "), " "))
	}
	return nil
}

func outputCheckJSON(w io.Writer, result *CheckResult) error {
	diagnostics := result.Diagnostics
	if diagnostics == nil {
		diagnostics = []CheckDiagnostic{}
	}
	output := struct {
		Diagnostics []CheckDiagnostic
		FileCount   int
		FixedCount  int
	}{
		Diagnostics: diagnostics,
		FileCount:   result.FileCount,
		FixedCount:  result.FixedCount,
	}
	return json.NewEncoder(w).Encode(output)
}
