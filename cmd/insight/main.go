// Package main provides the entry point for the insight hooks relay service.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func run() int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	return 0
}

func main() {
	os.Exit(run())
}
