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
	Port int `yaml:"port" json:"port"`
}

func Load(path string) (Config, error) {
	expanded, err := homedir.Expand(path)
	if err != nil {
		return Config{}, fmt.Errorf("config: failed to expand path: %w", err)
	}

	data, err := os.ReadFile(expanded)
	if os.IsNotExist(err) {
		return Config{Port: 8080}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("config: failed to read file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("config: failed to parse YAML: %w", err)
	}

	if cfg.Port == 0 {
		cfg.Port = 8080
	}

	return cfg, nil
}

func Save(path string, cfg Config) error {
	expanded, err := homedir.Expand(path)
	if err != nil {
		return fmt.Errorf("config: failed to expand path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(expanded), 0700); err != nil {
		return fmt.Errorf("config: failed to create directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("config: failed to marshal YAML: %w", err)
	}

	if err := os.WriteFile(expanded, data, 0600); err != nil {
		return fmt.Errorf("config: failed to write file: %w", err)
	}

	return nil
}
