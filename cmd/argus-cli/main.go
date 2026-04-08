package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultServerURL = "http://localhost:8080"

func main() {
	app := newCLIApp(os.Stdout, http.DefaultClient)
	if err := app.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "argus-cli: %v\n", err)
		os.Exit(1)
	}
}

type cliApp struct {
	out    io.Writer
	client *http.Client
}

func newCLIApp(out io.Writer, client *http.Client) *cliApp {
	if out == nil {
		out = io.Discard
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}

	return &cliApp{out: out, client: client}
}

func (a *cliApp) Run(args []string) error {
	if len(args) == 0 {
		a.printUsage()
		return nil
	}

	switch args[0] {
	case "help", "-h", "--help":
		a.printUsage()
		return nil
	case "health":
		return a.runHealth(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func (a *cliApp) runHealth(args []string) error {
	fs := flag.NewFlagSet("health", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	serverURL := fs.String("server", defaultServerURL, "Argus server base URL")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse health flags: %w", err)
	}

	target := strings.TrimRight(*serverURL, "/") + "/healthz"
	resp, err := a.client.Get(target)
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

	_, err = fmt.Fprintf(a.out, "%s\n", body)
	return err
}

func (a *cliApp) printUsage() {
	_, _ = fmt.Fprintln(a.out, "Argus CLI")
	_, _ = fmt.Fprintln(a.out, "")
	_, _ = fmt.Fprintln(a.out, "Usage:")
	_, _ = fmt.Fprintln(a.out, "  argus-cli health [--server http://localhost:8080]")
}
