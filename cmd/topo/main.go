package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/arm/topo/internal/output/logger"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			os.Exit(130)
		}
		logger.Error(err.Error())
		os.Exit(1)
	}
}
