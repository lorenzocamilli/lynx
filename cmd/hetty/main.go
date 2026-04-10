package main

import (
	"context"
	"flag"
	llog "log"

	"go.uber.org/zap"

	"github.com/dstotijn/hetty/pkg/log"
)

func main() {
	addr := flag.String("addr", ":8080", `TCP address to listen on, in the form "host:port".`)
	cert := flag.String("cert", "~/.hetty/hetty_cert.pem", "Path to root CA certificate. Creates file if it doesn't exist.")
	key := flag.String("key", "~/.hetty/hetty_key.pem", "Path to root CA private key. Creates file if it doesn't exist.")
	db := flag.String("db", "~/.hetty/hetty.db", "Database file path. Creates file if it doesn't exist.")
	flag.Parse()

	logger, err := log.NewZapLogger(false, false)
	if err != nil {
		llog.Fatalf("Failed to create logger: %v", err)
	}
	//nolint:errcheck
	defer logger.Sync()

	if err := run(context.Background(), *addr, *cert, *key, *db, logger); err != nil {
		logger.Fatal("Command failed.", zap.Error(err))
	}
}
