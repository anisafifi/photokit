// Command photokit is a cross-platform CLI for automating photography
// production tasks: metadata, copyright attribution, signature watermarking
// and batch processing.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"photokit/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Execute(ctx, os.Args[1:])
	stop()
	os.Exit(code)
}
