package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
	"gopkg.in/yaml.v2"
)

const DefaultPath = "~/.hetty/config.yaml"

type Config struct {
	Addr string `yaml:"addr"`
}

func Default() Config {
	return Config{
		Addr: ":8080",
	}
}

// Load reads config from path. Returns defaults if the file doesn't exist yet.
func Load(path string) (Config, error) {
	cfg := Default()

	expandedPath, err := homedir.Expand(path)
	if err != nil {
		return cfg, fmt.Errorf("config: failed to expand path: %w", err)
	}

	data, err := os.ReadFile(expandedPath)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("config: failed to read file: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("config: failed to parse file: %w", err)
	}

	return cfg, nil
}

// Save writes config to path, creating parent directories as needed.
func Save(path string, cfg Config) error {
	expandedPath, err := homedir.Expand(path)
	if err != nil {
		return fmt.Errorf("config: failed to expand path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(expandedPath), 0o700); err != nil {
		return fmt.Errorf("config: failed to create directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("config: failed to marshal config: %w", err)
	}

	if err := os.WriteFile(expandedPath, data, 0o600); err != nil {
		return fmt.Errorf("config: failed to write file: %w", err)
	}

	return nil
}
