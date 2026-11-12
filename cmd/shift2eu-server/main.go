package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// SIGTERM, the container platform's stop, and SIGINT cancel ctx, which
	// starts the graceful shutdown: run stops accepting connections, lets the
	// requests in flight finish within shutdownTimeout and returns 0. Once ctx is
	// cancelled, stop restores the default behaviour, so a second signal ends
	// the process at once.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	context.AfterFunc(ctx, stop)

	os.Exit(run(ctx, os.Args[1:], func() (string, bool) { return os.LookupEnv(tokenEnvVar) }, os.Stderr, nil))
}
