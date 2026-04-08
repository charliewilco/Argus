package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/charliewilco/argus/internal/app"
)

func main() {
	if err := newRootCommand().ExecuteContext(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "argus-cli: %v\n", err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "argus-cli",
		Short: "Argus CLI",
	}

	root.AddCommand(newConnectionsCommand())

	return root
}

func newConnectionsCommand() *cobra.Command {
	var providerID string

	cmd := &cobra.Command{
		Use:   "connections",
		Short: "List configured connections",
		RunE: func(cmd *cobra.Command, _ []string) error {
			runtime, err := app.NewCLIRuntime(cmd.Context(), app.Options{})
			if err != nil {
				return fmt.Errorf("argus-cli connections: initialize runtime: %w", err)
			}
			defer runtime.Close()

			items, err := runtime.Connections.ListConnections(runtime.Context, runtime.Config.TenantID, providerID)
			if err != nil {
				return fmt.Errorf("argus-cli connections: list connections: %w", err)
			}

			for _, item := range items {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", item.ConnectionID, item.Provider, item.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&providerID, "provider", "", "Filter by provider ID")

	return cmd
}
