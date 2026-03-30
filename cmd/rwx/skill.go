package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	semver "github.com/Masterminds/semver/v3"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/rwx-cloud/rwx/internal/skill"
	"github.com/spf13/cobra"
)

var (
	skillCmd = &cobra.Command{
		GroupID: "setup",
		Use:     "skill",
		Short:   "Agent skill related commands",
	}

	skillStatusCmd = &cobra.Command{
		Use:   "status",
		Short: "Show the status of RWX agent skill installations",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := service.SkillStatus()
			if err != nil {
				return err
			}

			if useJsonOutput() {
				return outputSkillStatusJSON(result)
			}

			outputSkillStatusText(result)
			return nil
		},
	}
)

func init() {
	skillCmd.AddCommand(skillStatusCmd)
}

type skillStatusJSON struct {
	Installations []skill.Installation `json:"Installations"`
	LatestVersion string               `json:"LatestVersion,omitempty"`
}

func outputSkillStatusJSON(result *cli.SkillStatusResult) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(skillStatusJSON{
		Installations: result.Installations,
		LatestVersion: result.LatestVersion,
	})
}

func outputSkillStatusText(result *cli.SkillStatusResult) {
	var detected []skill.Installation
	var notDetected []skill.Installation

	for _, inst := range result.Installations {
		if skill.IsDetected(inst) {
			detected = append(detected, inst)
		} else {
			notDetected = append(notDetected, inst)
		}
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)

	if len(detected) > 0 {
		fmt.Fprintln(os.Stdout, "Agent Skill Installations")
		for _, inst := range detected {
			version := inst.Version
			if version == "" {
				version = "installed"
			}
			fmt.Fprintf(w, "  %s\t%s\t%s\n", inst.Scope, shortenPath(inst.Path), version)
		}
		w.Flush()
		fmt.Fprintln(os.Stdout)
	}

	if len(notDetected) > 0 {
		fmt.Fprintln(os.Stdout, "Not detected")
		for _, inst := range notDetected {
			fmt.Fprintf(w, "  %s\t%s\n", inst.Scope, shortenPath(inst.Path))
		}
		w.Flush()
		fmt.Fprintln(os.Stdout)
	}

	if !result.AnyFound {
		fmt.Fprintln(os.Stdout, "To install:")
		fmt.Fprintln(os.Stdout, "  npx skills add rwx-cloud/skills")
		return
	}

	// Show upgrade instructions if any detected installation is outdated.
	if result.LatestVersion == "" {
		return
	}
	latestVersion, err := semver.NewVersion(result.LatestVersion)
	if err != nil {
		return
	}

	var highestOutdated *semver.Version
	outdatedSources := make(map[string]bool)
	for _, inst := range detected {
		if inst.Version == "" {
			outdatedSources[inst.Source] = true
			continue
		}
		v, err := semver.NewVersion(inst.Version)
		if err != nil {
			continue
		}
		if latestVersion.GreaterThan(v) {
			outdatedSources[inst.Source] = true
			if highestOutdated == nil || v.GreaterThan(highestOutdated) {
				highestOutdated = v
			}
		}
	}

	if len(outdatedSources) == 0 {
		return
	}

	if highestOutdated != nil {
		fmt.Fprintf(os.Stdout, "A new version of the RWX agent skill is available: v%s → v%s\n", highestOutdated, latestVersion)
	} else {
		fmt.Fprintf(os.Stdout, "A new version of the RWX agent skill is available: v%s\n", latestVersion)
	}
	if outdatedSources["agents"] {
		fmt.Fprintln(os.Stdout, "To upgrade: npx skills update rwx")
	}
	if outdatedSources["marketplace"] {
		fmt.Fprintln(os.Stdout, "To upgrade the Claude Code marketplace: claude plugin marketplace update rwx && claude plugin update rwx@rwx")
	}
}

func shortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}
