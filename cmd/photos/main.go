// Command photos uses the Photos.sh image tools from a terminal and from
// scripts.
//
// One static binary. Its only dependency is the Photos.sh Go library
// (github.com/photos-sh/photos-go), which itself needs nothing beyond the
// Go standard library.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/photos-sh/photos-cli/internal/cli"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

func main() {
	// Ctrl-C (and a polite kill) cancels the context: requests in flight are
	// dropped and no half-written file is left. A second Ctrl-C is not
	// caught and ends the process at once.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		stop()
	}()

	code := cli.Main(ctx, cli.NewEnv(version), os.Args[1:])
	stop()
	os.Exit(code)
}
