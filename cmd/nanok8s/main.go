package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// SIGTERM handler is required for `nanok8s boot`, which parks on
	// ctx.Done() after a healthy boot to keep nanok8s.service active.
	// Other subcommands ignore the cancellation but inherit it for free.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
