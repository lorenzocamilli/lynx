package main

import (
	"context"
	"flag"
	"fmt"
	llog "log"

	"go.uber.org/zap"

	"github.com/lorenzocamilli/lynx/pkg/config"
	"github.com/lorenzocamilli/lynx/pkg/log"
)

func main() {
	showVersion := flag.Bool("version", false, "Print version and exit.")
	flag.BoolVar(showVersion, "v", false, "Print version and exit (shorthand).")
	flag.Parse()

	if *showVersion {
		fmt.Printf("lynx %s\n", version)
		return
	}

	cfg, err := config.Load(config.DefaultPath)
	if err != nil {
		llog.Fatalf("Failed to load config: %v", err)
	}

	logger, err := log.NewZapLogger(false, false, cfg.LogLevel)
	if err != nil {
		llog.Fatalf("Failed to create logger: %v", err)
	}
	//nolint:errcheck
	defer logger.Sync()

	if err := run(context.Background(), cfg, logger); err != nil {
		logger.Fatal("Command failed.", zap.Error(err))
	}
}
