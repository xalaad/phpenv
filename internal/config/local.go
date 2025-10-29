package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const (
	localConfigName = ".phpenv.json"
)

// LocalConfig customizes phpenv behaviour for a specific directory tree.
type LocalConfig struct {
	Use           Selection         `json:"use"`
	Env           map[string]string `json:"env,omitempty"`
	PathAdditions []string          `json:"path_additions,omitempty"`
	filePath      string            `json:"-"`
}

// FindLocal scans from start upwards for a .phpenv.json and returns the first match.
func FindLocal(start string) (*LocalConfig, error) {
	if start == "" {
		return nil, errors.New("start path is empty")
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		return nil, err
	}
	for {
		candidate := filepath.Join(dir, localConfigName)
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			cfg, err := readLocal(candidate)
			if err != nil {
				return nil, err
			}
			return cfg, nil
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	return nil, os.ErrNotExist
}

// SaveLocal writes a LocalConfig to the provided directory.
func SaveLocal(dir string, cfg *LocalConfig) error {
	if dir == "" {
		return errors.New("directory required")
	}
	if cfg == nil {
		return errors.New("local config is nil")
	}
	if err := cfg.ensureDefaults(); err != nil {
		return err
	}
	path := filepath.Join(dir, localConfigName)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	cfg.filePath = path
	return nil
}

func readLocal(path string) (*LocalConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg LocalConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	cfg.filePath = path
	if err := cfg.ensureDefaults(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *LocalConfig) ensureDefaults() error {
	if c.Env == nil {
		c.Env = map[string]string{}
	}
	c.PathAdditions = normalizePathSlice(c.PathAdditions)
	return (&Config{}).normalizeSelection(&c.Use)
}
