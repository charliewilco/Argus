package root

import (
	"context"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/charliewilco/argus/internal/cliapp"
	"github.com/charliewilco/argus/internal/dlq"
	"github.com/charliewilco/argus/internal/queue"
	"github.com/charliewilco/argus/internal/store"
	"github.com/charliewilco/argus/internal/store/sqlite"
)

const defaultDatabaseURL = "sqlite:./argus.db"

type Config struct {
	DatabaseURL string
	TenantID    string
}

type userError struct {
	message string
	err     error
}

func (e *userError) Error() string { return e.message }
func (e *userError) Unwrap() error { return e.err }

func NewCommand() (*cobra.Command, error) {
	cfg := Config{
		DatabaseURL: envOrDefault("ARGUS_DATABASE_URL", defaultDatabaseURL),
		TenantID:    envOrDefault("ARGUS_TENANT_ID", "default"),
	}

	ctx := context.Background()
	sqliteStore, err := sqlite.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("argus-cli.NewCommand: open store: %w", err)
	}

	jobQueue := queue.NewMemoryQueue()
	connectionsService, err := cliapp.NewConnectionsDomainService(sqliteStore, time.Now)
	if err != nil {
		_ = sqliteStore.Close()
		return nil, fmt.Errorf("argus-cli.NewCommand: build connections service: %w", err)
	}
	dlqService, err := dlq.NewStore(sqliteStore, jobQueue, time.Now)
	if err != nil {
		_ = sqliteStore.Close()
		return nil, fmt.Errorf("argus-cli.NewCommand: build DLQ service: %w", err)
	}
	pipelineRunner, err := cliapp.NewPipelineQueueRunner(jobQueue, time.Now, nil)
	if err != nil {
		_ = sqliteStore.Close()
		return nil, fmt.Errorf("argus-cli.NewCommand: build pipeline runner: %w", err)
	}

	services, err := cliapp.NewServices(connectionsService, dlqService, pipelineRunner)
	if err != nil {
		_ = sqliteStore.Close()
		return nil, fmt.Errorf("argus-cli.NewCommand: build CLI services: %w", err)
	}

	root := &cobra.Command{
		Use:   "argus-cli",
		Short: "Argus command line interface",
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			if cfg.TenantID == "" {
				return &userError{message: "tenant ID cannot be empty", err: fmt.Errorf("argus-cli: tenant ID is required")}
			}
			return nil
		},
		PersistentPostRun: func(_ *cobra.Command, _ []string) {
			_ = sqliteStore.Close()
		},
		SilenceUsage: true,
	}

	root.AddCommand(connectionsCommand(services, cfg))
	root.AddCommand(dlqCommand(services))
	root.AddCommand(pipelineCommand(services))

	return root, nil
}

func connectionsCommand(services *cliapp.Services, cfg Config) *cobra.Command {
	cmd := &cobra.Command{Use: "connections", Short: "Manage provider connections"}

	var provider string
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List connections",
		RunE: func(cmd *cobra.Command, _ []string) error {
			items, err := services.Connections.ListConnections(cmd.Context(), cfg.TenantID, provider)
			if err != nil {
				return &userError{message: "could not list connections", err: fmt.Errorf("argus-cli.connections.list: %w", err)}
			}

			writer := tabwriter.NewWriter(os.Stdout, 2, 8, 2, ' ', 0)
			_, _ = fmt.Fprintln(writer, "CONNECTION ID\tPROVIDER\tTENANT\tCREATED AT (UTC)")
			for _, conn := range items {
				_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", conn.ConnectionID, conn.Provider, conn.TenantID, utcFormat(conn.CreatedAt))
			}
			return writer.Flush()
		},
	}
	listCmd.Flags().StringVar(&provider, "provider", "", "Optional provider ID filter")
	cmd.AddCommand(listCmd)

	return cmd
}

func dlqCommand(services *cliapp.Services) *cobra.Command {
	cmd := &cobra.Command{Use: "dlq", Short: "Dead-letter queue operations"}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List dead-letter jobs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			jobs, err := services.DLQ.List(cmd.Context())
			if err != nil {
				return &userError{message: "could not list DLQ jobs", err: fmt.Errorf("argus-cli.dlq.list: %w", err)}
			}

			writer := tabwriter.NewWriter(os.Stdout, 2, 8, 2, ' ', 0)
			_, _ = fmt.Fprintln(writer, "ID\tTYPE\tATTEMPTS\tFAILED AT (UTC)\tREPLAYED AT (UTC)\tREASON")
			for _, job := range jobs {
				replayedAt := "-"
				if job.ReplayedAt != nil {
					replayedAt = utcFormat(*job.ReplayedAt)
				}
				_, _ = fmt.Fprintf(writer, "%s\t%s\t%d\t%s\t%s\t%s\n", job.ID, job.JobType, job.AttemptCount, utcFormat(job.FailedAt), replayedAt, job.Reason)
			}
			return writer.Flush()
		},
	}

	var id string
	replayCmd := &cobra.Command{
		Use:   "replay",
		Short: "Replay a dead-letter job by ID",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if id == "" {
				return &userError{message: "--id is required", err: fmt.Errorf("argus-cli.dlq.replay: id is required")}
			}

			if err := services.DLQ.Replay(cmd.Context(), id); err != nil {
				if errors.Is(err, store.ErrNotFound) {
					return &userError{message: fmt.Sprintf("DLQ job %q was not found", id), err: fmt.Errorf("argus-cli.dlq.replay: %w", err)}
				}
				return &userError{message: fmt.Sprintf("could not replay DLQ job %q", id), err: fmt.Errorf("argus-cli.dlq.replay: %w", err)}
			}

			_, _ = fmt.Fprintf(os.Stdout, "replayed job %s\n", id)
			return nil
		},
	}
	replayCmd.Flags().StringVar(&id, "id", "", "Dead-letter job ID")
	cmd.AddCommand(listCmd, replayCmd)

	return cmd
}

func pipelineCommand(services *cliapp.Services) *cobra.Command {
	cmd := &cobra.Command{Use: "pipeline", Short: "Pipeline operations"}

	var pipelineID string
	var eventID string
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Queue a manual pipeline run",
		RunE: func(cmd *cobra.Command, _ []string) error {
			jobID, err := services.Pipeline.Run(cmd.Context(), pipelineID, eventID)
			if err != nil {
				return &userError{message: "could not queue pipeline run", err: fmt.Errorf("argus-cli.pipeline.run: %w", err)}
			}

			_, _ = fmt.Fprintf(os.Stdout, "queued manual run job %s for pipeline %s and event %s\n", jobID, pipelineID, eventID)
			return nil
		},
	}
	runCmd.Flags().StringVar(&pipelineID, "pipeline-id", "", "Pipeline ID")
	runCmd.Flags().StringVar(&eventID, "event-id", "", "Event ID")
	_ = runCmd.MarkFlagRequired("pipeline-id")
	_ = runCmd.MarkFlagRequired("event-id")
	cmd.AddCommand(runCmd)

	return cmd
}

func envOrDefault(value, fallback string) string {
	current := os.Getenv(value)
	if current == "" {
		return fallback
	}
	return current
}

func utcFormat(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.UTC().Format(time.RFC3339)
}
