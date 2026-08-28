package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lorenzocamilli/lynx/pkg/config"
)

func TestDefault(t *testing.T) {
	t.Parallel()

	cfg := config.Default()

	if cfg.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want %q", cfg.Host, "127.0.0.1")
	}

	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}

	if cfg.MaxBodyBytes != config.DefaultMaxBodyBytes {
		t.Errorf("MaxBodyBytes = %d, want %d", cfg.MaxBodyBytes, config.DefaultMaxBodyBytes)
	}

	if cfg.LogLevel != config.DefaultLogLevel {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, config.DefaultLogLevel)
	}

	if len(cfg.RedactHeaders) != 0 {
		t.Errorf("RedactHeaders = %v, want empty", cfg.RedactHeaders)
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "does-not-exist.yaml")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	want := config.Default()
	if cfg.Host != want.Host || cfg.Port != want.Port || cfg.MaxBodyBytes != want.MaxBodyBytes ||
		cfg.LogLevel != want.LogLevel || len(cfg.RedactHeaders) != 0 {
		t.Errorf("cfg = %+v, want the default config %+v", cfg, want)
	}
}

func TestLoadValidFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yaml")
	yamlContent := "host: 0.0.0.0\nport: 9090\nmaxBodyBytes: 1024\nlogLevel: debug\nredactHeaders:\n  - Authorization\n  - Cookie\n"

	if err := os.WriteFile(path, []byte(yamlContent), 0o600); err != nil {
		t.Fatalf("failed to write test fixture: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host = %q, want %q", cfg.Host, "0.0.0.0")
	}

	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want 9090", cfg.Port)
	}

	if cfg.MaxBodyBytes != 1024 {
		t.Errorf("MaxBodyBytes = %d, want 1024", cfg.MaxBodyBytes)
	}

	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}

	if len(cfg.RedactHeaders) != 2 || cfg.RedactHeaders[0] != "Authorization" || cfg.RedactHeaders[1] != "Cookie" {
		t.Errorf("RedactHeaders = %v, want [Authorization Cookie]", cfg.RedactHeaders)
	}
}

func TestLoadMalformedFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("port: [this is not valid yaml"), 0o600); err != nil {
		t.Fatalf("failed to write test fixture: %v", err)
	}

	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected an error loading malformed YAML")
	}
}

func TestLoadUnreadableFile(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("running as root can read files regardless of permissions")
	}

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("port: 8080"), 0o000); err != nil {
		t.Fatalf("failed to write test fixture: %v", err)
	}

	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected an error loading a file without read permissions")
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.yaml")

	want := config.Config{
		Host:          "127.0.0.1",
		Port:          1234,
		MaxBodyBytes:  2048,
		LogLevel:      "warn",
		RedactHeaders: []string{"Authorization"},
	}

	if err := config.Save(path, want); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if got.Host != want.Host || got.Port != want.Port || got.MaxBodyBytes != want.MaxBodyBytes ||
		got.LogLevel != want.LogLevel || len(got.RedactHeaders) != 1 || got.RedactHeaders[0] != "Authorization" {
		t.Errorf("round-tripped config = %+v, want %+v", got, want)
	}
}

func TestSaveCreatesParentDirWithRestrictivePermissions(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Unix file permissions don't apply on Windows")
	}

	dir := t.TempDir()
	nestedDir := filepath.Join(dir, "nested")
	path := filepath.Join(nestedDir, "config.yaml")

	if err := config.Save(path, config.Default()); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	dirInfo, err := os.Stat(nestedDir)
	if err != nil {
		t.Fatalf("failed to stat created directory: %v", err)
	}

	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("directory permissions = %o, want %o", perm, 0o700)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat created file: %v", err)
	}

	if perm := fileInfo.Mode().Perm(); perm != 0o600 {
		t.Errorf("file permissions = %o, want %o", perm, 0o600)
	}
}

func TestSaveMkdirAllFailsWhenParentIsAFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	blockingFile := filepath.Join(dir, "blocker")

	if err := os.WriteFile(blockingFile, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("failed to write blocking file: %v", err)
	}

	// "blocker" already exists as a regular file, so MkdirAll can't create a
	// directory at that path for the config file to live under.
	path := filepath.Join(blockingFile, "config.yaml")

	if err := config.Save(path, config.Default()); err == nil {
		t.Fatal("expected an error when the parent path is not a directory")
	}
}

func TestSaveWriteFileFailsWhenPathIsADirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("failed to create directory at the target path: %v", err)
	}

	if err := config.Save(path, config.Default()); err == nil {
		t.Fatal("expected an error when the target path is a directory")
	}
}

func TestSaveOverwritesExistingFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yaml")

	if err := config.Save(path, config.Config{Port: 1111}); err != nil {
		t.Fatalf("first Save failed: %v", err)
	}

	if err := config.Save(path, config.Config{Port: 2222}); err != nil {
		t.Fatalf("second Save failed: %v", err)
	}

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if got.Port != 2222 {
		t.Errorf("Port = %d, want 2222", got.Port)
	}
}
