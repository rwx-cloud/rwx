package main

import (
	"os"
	"time"

	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/rwx-cloud/rwx/internal/lsp"

	"github.com/spf13/cobra"
)

var (
	LintFailure = errors.Wrap(HandledError, "lint failure")

	LintRwxDirectory     string
	LintWarningsAsErrors bool
	LintOutputFormat     string
	LintTimeout          time.Duration
	LintFix              bool

	lintCmd = &cobra.Command{
		GroupID: "definitions",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return requireAccessToken()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat := LintOutputFormat
			if Json {
				outputFormat = "json"
			}

			result, err := service.Lint(cli.LintConfig{
				Check: func() (*cli.LintCheckResult, error) {
					cfg, err := lsp.NewCheckConfig(
						LintRwxDirectory,
						outputFormat,
						LintTimeout,
						args,
						LintFix,
					)
					if err != nil {
						return nil, err
					}

					checkResult, err := lsp.Check(cmd.Context(), cfg, os.Stdout)
					if err != nil {
						return nil, err
					}

					diagnostics := make([]cli.LintDiagnostic, len(checkResult.Diagnostics))
					for i, d := range checkResult.Diagnostics {
						diagnostics[i] = cli.LintDiagnostic{Severity: d.Severity}
					}

					return &cli.LintCheckResult{
						Diagnostics: diagnostics,
						FileCount:   checkResult.FileCount,
					}, nil
				},
				Fix: LintFix,
			})
			if err != nil {
				return err
			}

			if LintShouldFail(result, LintWarningsAsErrors) {
				return LintFailure
			}

			return nil
		},
		Short: "Lint RWX configuration files",
		Use:   "lint [flags] [file...]",
	}
)

func LintShouldFail(result *cli.LintResult, warningsAsErrors bool) bool {
	return result.HasError || (warningsAsErrors && result.WarningCount > 0)
}

func init() {
	lintCmd.Flags().BoolVar(&LintWarningsAsErrors, "warnings-as-errors", false, "treat warnings as errors")
	lintCmd.Flags().StringVarP(&LintRwxDirectory, "dir", "d", "", "the directory your RWX configuration files are located in, typically `.rwx`. By default, the CLI traverses up until it finds a `.rwx` directory.")
	lintCmd.Flags().StringVarP(&LintOutputFormat, "output", "o", "multiline", "output format: text, multiline, oneline, json, none")
	lintCmd.Flags().DurationVar(&LintTimeout, "timeout", 30*time.Second, "timeout for the LSP check operation")
	lintCmd.Flags().BoolVar(&LintFix, "fix", false, "automatically apply available fixes")
}
