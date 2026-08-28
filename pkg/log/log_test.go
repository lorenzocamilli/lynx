package log_test

import (
	"strings"
	"testing"

	"go.uber.org/zap/zapcore"

	"github.com/lorenzocamilli/lynx/pkg/log"
)

func TestNewZapLogger(t *testing.T) {
	t.Parallel()

	t.Run("default config builds a usable logger", func(t *testing.T) {
		t.Parallel()

		logger, err := log.NewZapLogger(false, false, "")
		if err != nil {
			t.Fatalf("NewZapLogger failed: %v", err)
		}
		defer func() { _ = logger.Sync() }()

		if !logger.Core().Enabled(zapcore.InfoLevel) {
			t.Error("expected the production default level to enable Info")
		}
	})

	t.Run("verbose+json builds without error", func(t *testing.T) {
		t.Parallel()

		logger, err := log.NewZapLogger(true, true, "debug")
		if err != nil {
			t.Fatalf("NewZapLogger failed: %v", err)
		}
		defer func() { _ = logger.Sync() }()

		if !logger.Core().Enabled(zapcore.DebugLevel) {
			t.Error("expected explicit \"debug\" level to enable Debug")
		}
	})

	t.Run("explicit level overrides the default", func(t *testing.T) {
		t.Parallel()

		logger, err := log.NewZapLogger(false, false, "error")
		if err != nil {
			t.Fatalf("NewZapLogger failed: %v", err)
		}
		defer func() { _ = logger.Sync() }()

		if logger.Core().Enabled(zapcore.WarnLevel) {
			t.Error("expected \"error\" level to disable Warn")
		}

		if !logger.Core().Enabled(zapcore.ErrorLevel) {
			t.Error("expected \"error\" level to enable Error")
		}
	})

	t.Run("invalid level returns an error", func(t *testing.T) {
		t.Parallel()

		_, err := log.NewZapLogger(false, false, "not-a-level")
		if err == nil {
			t.Fatal("expected an error for an invalid log level")
		}

		if wantSubstr := "invalid log level"; !strings.Contains(err.Error(), wantSubstr) {
			t.Errorf("error = %q, want it to contain %q", err.Error(), wantSubstr)
		}
	})
}

func TestNopLogger(t *testing.T) {
	t.Parallel()

	nop := log.NewNopLogger()

	// These must not panic; NopLogger is a no-op sink used as a fallback
	// when no logger is configured.
	nop.Debugw("debug", "k", "v")
	nop.Infow("info", "k", "v")
	nop.Errorw("error", "k", "v")
}
