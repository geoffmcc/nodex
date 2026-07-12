package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/geoffmcc/nodex/internal/version"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	_ = ctx
	fmt.Printf("Nodex %s\n", version.Version)
	fmt.Printf("Go: %s\n", version.GoVersion)
	fmt.Printf("Commit: %s\n", version.Commit)
	fmt.Printf("Built: %s\n", version.BuildDate)
	return nil
}
