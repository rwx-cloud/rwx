package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/cli"
	"github.com/spf13/cobra"
)

var concurrencyPoolsCmd = &cobra.Command{
	GroupID: "api",
	Use:     "concurrency-pools",
	Short:   "Inspect concurrency pools",
}

func newConcurrencyPoolsListCommand() *cobra.Command {
	var cfg cli.ListConcurrencyPoolsConfig
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List concurrency pools",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := service.ListConcurrencyPools(cfg)
			if err != nil {
				return err
			}
			if useJsonOutput() {
				return printConcurrencyPoolsJSON(os.Stdout, result)
			}
			if len(result.ConcurrencyPools) == 0 {
				fmt.Fprintln(os.Stdout, "No concurrency pools found.")
				return nil
			}
			renderConcurrencyPoolsTable(os.Stdout, result.ConcurrencyPools)
			printConcurrencyPoolsPaginationHint(os.Stderr, result.Pagination)
			return nil
		},
	}
	cmd.Flags().StringVar(&cfg.Search, "search", "", "search pool IDs")
	cmd.Flags().IntVar(&cfg.Limit, "limit", 0, "page size (max 100; server default applies when unset)")
	cmd.Flags().StringVar(&cfg.Cursor, "cursor", "", "cursor for fetching the next page")
	return cmd
}

func newConcurrencyPoolsShowCommand() *cobra.Command {
	var limit int
	var cursor string
	cmd := &cobra.Command{
		Use:     "show <pool-id>",
		Aliases: []string{"get"},
		Short:   "Show a concurrency pool",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := service.ShowConcurrencyPool(cli.ShowConcurrencyPoolConfig{
				PoolID: args[0],
				Limit:  limit,
				Cursor: cursor,
			})
			if err != nil {
				return err
			}
			if useJsonOutput() {
				return printConcurrencyPoolsJSON(os.Stdout, result)
			}
			renderConcurrencyPool(os.Stdout, result)
			printConcurrencyPoolsPaginationHint(os.Stderr, result.Pagination)
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "queue page size (max 100; server default applies when unset)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "cursor for fetching the next queue page")
	return cmd
}

func renderConcurrencyPoolsTable(w io.Writer, pools []api.ConcurrencyPool) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "POOL\tSTATUS\tQUEUE\tCAPACITY\tON OVERFLOW\tUSES (7D)\tLAST USED")
	for _, pool := range pools {
		capacity, overflow := concurrencyPoolConfiguration(pool.LatestConfiguration)
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%d\t%s\n", pool.ID, pool.Status, pool.QueueSize, capacity, overflow, pool.UsageCountLast7Days, optionalString(pool.LastRequestedAt))
	}
	tw.Flush()
}

func renderConcurrencyPool(w io.Writer, result *cli.ShowConcurrencyPoolResult) {
	pool := result.ConcurrencyPool
	capacity, overflow := concurrencyPoolConfiguration(pool.LatestConfiguration)
	fmt.Fprintf(w, "Pool: %s\nStatus: %s\nPaused at: %s\nQueue: %d\nCapacity: %s\nOn overflow: %s\nUses (7D): %d\nLast used: %s\n\n", pool.ID, pool.Status, optionalString(pool.PausedAt), pool.QueueSize, capacity, overflow, pool.UsageCountLast7Days, optionalString(pool.LastRequestedAt))
	fmt.Fprintln(w, "CURRENT QUEUE")
	if len(result.Queue) == 0 {
		fmt.Fprintln(w, "No queued runs.")
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "POSITION\tRUN\tSTATE\tCAPACITY\tON OVERFLOW\tREQUESTED\tACQUIRED")
		for _, lease := range result.Queue {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%d\t%s\t%s\t%s\n", lease.Position, lease.RunID, lease.State, lease.Capacity, lease.OnOverflow, lease.RequestedAt, optionalString(lease.AcquiredAt))
		}
		tw.Flush()
	}
	fmt.Fprintln(w, "\nRECENT RUNS")
	if len(pool.RecentRuns) == 0 {
		fmt.Fprintln(w, "No recent runs.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "RUN\tSTATUS\tREPOSITORY\tBRANCH\tTITLE\tSTARTED\tCOMPLETED")
	for _, run := range pool.RecentRuns {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", run.ID, concurrencyPoolRunStatus(run.Status), optionalString(run.RepositoryName), optionalString(run.Branch), optionalString(run.Title), optionalString(run.StartedAt), optionalString(run.CompletedAt))
	}
	tw.Flush()
}

func concurrencyPoolConfiguration(configuration *api.ConcurrencyPoolConfiguration) (string, string) {
	if configuration == nil {
		return "-", "-"
	}
	return fmt.Sprintf("%d", configuration.Capacity), configuration.OnOverflow
}

func concurrencyPoolRunStatus(status api.ConcurrencyPoolRunStatus) string {
	if status.Execution != "" && status.Execution != "finished" {
		return status.Execution
	}
	return optionalString(status.Result)
}

func optionalString(value *string) string {
	if value == nil || *value == "" {
		return "-"
	}
	return *value
}

func printConcurrencyPoolsPaginationHint(w io.Writer, pagination api.ConcurrencyPoolPagination) {
	if pagination.NextCursor != nil {
		fmt.Fprintf(w, "\nMore results available. Fetch the next page with --cursor %s\n", *pagination.NextCursor)
	}
}

func printConcurrencyPoolsJSON(w io.Writer, result any) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	var value any
	if err := json.Unmarshal(encoded, &value); err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(pascalCaseKeys(value))
}

func init() {
	concurrencyPoolsCmd.AddCommand(newConcurrencyPoolsListCommand(), newConcurrencyPoolsShowCommand())
}
