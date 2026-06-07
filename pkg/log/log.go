package log

import (
	"fmt"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger interface {
	Debugw(msg string, v ...interface{})
	Infow(msg string, v ...interface{})
	Errorw(msg string, v ...interface{})
}

// NewZapLogger builds the application logger. levelStr sets the minimum log
// level ("debug", "info", "warn", "error"); an empty string keeps the
// encoding-derived default. verbose enables development output (caller +
// stacktrace) and jsonLogs switches the encoder from console to JSON.
func NewZapLogger(verbose, jsonLogs bool, levelStr string) (*zap.Logger, error) {
	var config zap.Config

	if verbose {
		config = zap.Config{
			Level:       zap.NewAtomicLevelAt(zap.DebugLevel),
			Development: true,
			Encoding:    "json",
			EncoderConfig: zapcore.EncoderConfig{
				TimeKey:        "ts",
				LevelKey:       "level",
				NameKey:        "logger",
				CallerKey:      "caller",
				FunctionKey:    zapcore.OmitKey,
				MessageKey:     "message",
				StacktraceKey:  "stacktrace",
				LineEnding:     zapcore.DefaultLineEnding,
				EncodeLevel:    zapcore.LowercaseLevelEncoder,
				EncodeTime:     zapcore.RFC3339TimeEncoder,
				EncodeDuration: zapcore.SecondsDurationEncoder,
				EncodeCaller:   zapcore.ShortCallerEncoder,
			},
			OutputPaths:      []string{"stderr"},
			ErrorOutputPaths: []string{"stderr"},
		}
	} else {
		config = zap.NewProductionConfig()
	}

	if !jsonLogs {
		config.Encoding = "console"
		config.EncoderConfig = zapcore.EncoderConfig{
			TimeKey:          "T",
			LevelKey:         "L",
			NameKey:          "N",
			CallerKey:        zapcore.OmitKey,
			FunctionKey:      zapcore.OmitKey,
			MessageKey:       "M",
			StacktraceKey:    zapcore.OmitKey,
			ConsoleSeparator: " ",
			LineEnding:       zapcore.DefaultLineEnding,
			EncodeLevel:      zapcore.CapitalColorLevelEncoder,
			EncodeTime: func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
				enc.AppendString(t.Format("2006/01/02 15:04:05"))
			},
			EncodeName: func(loggerName string, enc zapcore.PrimitiveArrayEncoder) {
				// Print logger name in cyan (ANSI code 36).
				enc.AppendString(fmt.Sprintf("\x1b[%dm%s\x1b[0m", uint8(36), "["+loggerName+"]"))
			},
			EncodeDuration: zapcore.StringDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		}

		if verbose {
			config.EncoderConfig.CallerKey = "C"
			config.EncoderConfig.StacktraceKey = "S"
		}
	}

	if levelStr != "" {
		level, err := zapcore.ParseLevel(levelStr)
		if err != nil {
			return nil, fmt.Errorf("log: invalid log level %q: %w", levelStr, err)
		}
		config.Level = zap.NewAtomicLevelAt(level)
	}

	return config.Build()
}

type NopLogger struct{}

func (nop NopLogger) Debugw(_ string, _ ...interface{}) {}
func (nop NopLogger) Infow(_ string, _ ...interface{})  {}
func (nop NopLogger) Errorw(_ string, _ ...interface{}) {}

func NewNopLogger() NopLogger {
	return NopLogger{}
}
