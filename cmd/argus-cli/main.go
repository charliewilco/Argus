package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/charliewilco/argus/internal/app"
)

const defaultServerURL = "http://localhost:8080"

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

	root.AddCommand(newHealthCommand())
	root.AddCommand(newConnectionsCommand())

	return root
}

func newHealthCommand() *cobra.Command {
	var serverURL string

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check Argus server health",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHealth(cmd.OutOrStdout(), http.DefaultClient, serverURL)
		},
	}

	cmd.Flags().StringVar(&serverURL, "server", defaultServerURL, "Argus server base URL")

	return cmd
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

func runHealth(out io.Writer, client *http.Client, serverURL string) error {
	if out == nil {
		out = io.Discard
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}

	target := strings.TrimRight(serverURL, "/") + "/healthz"
	resp, err := client.Get(target)
	if err != nil {
		return fmt.Errorf("check health %q: %w", target, err)
	}
	defer resp.Body.Close()

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("decode health response: %w", err)
	}
	payload["http_status"] = resp.StatusCode

	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal health response: %w", err)
	}

	_, err = fmt.Fprintf(out, "%s\n", body)
	return err
}
