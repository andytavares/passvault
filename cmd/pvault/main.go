// Command pvault syncs 1Password secrets into pass and manages the vault.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/atavares-klaviyo/pass-vault/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err := cli.NewRoot().ExecuteContext(ctx)
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "pvault:", err)
	var exitErr cli.ExitCodeError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.Code)
	}
	os.Exit(1)
}
