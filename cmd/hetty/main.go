package main

import (
	"context"
	llog "log"

	"go.uber.org/zap"

	"github.com/dstotijn/hetty/pkg/config"
	"github.com/dstotijn/hetty/pkg/log"
)

func main() {
	cfg, err := config.Load(config.DefaultPath)
	if err != nil {
		llog.Fatalf("Failed to load config: %v", err)
	}

	logger, err := log.NewZapLogger(false, false)
	if err != nil {
		llog.Fatalf("Failed to create logger: %v", err)
	}
	//nolint:errcheck
	defer logger.Sync()

	if err := run(context.Background(), cfg, logger); err != nil {
		logger.Fatal("Command failed.", zap.Error(err))
	}
}
