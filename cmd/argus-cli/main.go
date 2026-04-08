package main

import (
	"fmt"
	"os"

	"github.com/charliewilco/argus/cmd/argus-cli/root"
)

func main() {
	cmd, err := root.NewCommand()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
