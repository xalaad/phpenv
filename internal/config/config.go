package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const configFileName = "config.json"

// Selection identifies which PHP runtime should be used.
type Selection struct {
	Version     string `json:"version,omitempty"`     // Installs directory name (e.g., php-8.2.12-nts-Win32-vs16-x64)
	CustomPHP   string `json:"custom_php,omitempty"`  // Absolute path to php.exe outside managed installs
	Description string `json:"description,omitempty"` // Optional note for UIs
}

// Profile customizes environment settings for a specific version.
type Profile struct {
	Env           map[string]string `json:"env,omitempty"`
	PathAdditions []string          `json:"path_additions,omitempty"`
}

// Config drives where phpenv stores data and which environment variables it manages.
type Config struct {
	Root                 string             `json:"root"`
	VersionsDir          string             `json:"versions_dir"`
	CacheDir             string             `json:"cache_dir"`
	ShimsDir             string             `json:"shims_dir"`
	Env                  map[string]string  `json:"env"`
	PathAdditions        []string           `json:"path_additions"`
	Global               Selection          `json:"global"`
	VersionProfiles      map[string]Profile `json:"version_profiles,omitempty"`
	DefaultArch          string             `json:"default_arch"`
	DefaultThreadSafety  string             `json:"default_thread_safety"`
	filePath             string             `json:"-"`
	normalizedPathFields bool               `json:"-"`
}

// Load retrieves the current configuration from disk, creating it with defaults if needed.
func Load() (*Config, error) {
	cfgPath, err := Path()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		return nil, err
	}

	cfg := &Config{}
	if _, err := os.Stat(cfgPath); errors.Is(err, os.ErrNotExist) {
		cfg, err = Default()
		if err != nil {
			return nil, err
		}
		cfg.filePath = cfgPath
		if err := cfg.ensureDefaults(); err != nil {
			return nil, err
		}
		if err := cfg.Save(); err != nil {
			return nil, err
		}
		return cfg, nil
	}

	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, cfg); err != nil {
		return nil, err
	}
	cfg.filePath = cfgPath
	if err := cfg.ensureDefaults(); err != nil {
		return nil, err
	}
	// Persist defaults if we had to fill anything in.
	if err := cfg.Save(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Default produces a configuration populated with sensible defaults.
func Default() (*Config, error) {
	root, err := defaultRoot()
	if err != nil {
		return nil, err
	}
	return &Config{
		Root:                root,
		VersionsDir:         filepath.Join(root, "versions"),
		CacheDir:            filepath.Join(root, "cache"),
		ShimsDir:            filepath.Join(root, "shims"),
		Env:                 map[string]string{},
		PathAdditions:       []string{},
		Global:              Selection{},
		VersionProfiles:     map[string]Profile{},
		DefaultArch:         "x64",
		DefaultThreadSafety: "nts",
	}, nil
}

// Save writes the configuration back to disk.
func (c *Config) Save() error {
	if err := c.ensureDefaults(); err != nil {
		return err
	}
	if c.filePath == "" {
		cfgPath, err := Path()
		if err != nil {
			return err
		}
		c.filePath = cfgPath
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.filePath, b, 0o644)
}

// EnsureDirs materializes all directories declared in the configuration.
func (c *Config) EnsureDirs() error {
	for _, dir := range []string{c.Root, c.VersionsDir, c.CacheDir, c.ShimsDir} {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// Path returns the absolute path to the configuration file.
func Path() (string, error) {
	root, err := defaultRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, configFileName), nil
}

// FilePath exposes where the configuration is stored on disk.
func (c *Config) FilePath() string {
	if c.filePath != "" {
		return c.filePath
	}
	if path, err := Path(); err == nil {
		return path
	}
	return ""
}

// ensureDefaults validates and normalizes configuration values.
func (c *Config) ensureDefaults() error {
	if c.Root == "" {
		root, err := defaultRoot()
		if err != nil {
			return err
		}
		c.Root = root
	}
	c.Root = normalizePath(c.Root)

	if c.VersionsDir == "" {
		c.VersionsDir = filepath.Join(c.Root, "versions")
	}
	if c.CacheDir == "" {
		c.CacheDir = filepath.Join(c.Root, "cache")
	}
	if c.ShimsDir == "" {
		c.ShimsDir = filepath.Join(c.Root, "shims")
	}
	c.VersionsDir = normalizePath(c.VersionsDir)
	c.CacheDir = normalizePath(c.CacheDir)
	c.ShimsDir = normalizePath(c.ShimsDir)

	if c.Env == nil {
		c.Env = map[string]string{}
	}
	c.PathAdditions = normalizePathSlice(c.PathAdditions)
	if c.VersionProfiles == nil {
		c.VersionProfiles = map[string]Profile{}
	}
	for key, profile := range c.VersionProfiles {
		if profile.Env == nil {
			profile.Env = map[string]string{}
		}
		profile.PathAdditions = normalizePathSlice(profile.PathAdditions)
		c.VersionProfiles[key] = profile
	}

	if err := c.normalizeSelection(&c.Global); err != nil {
		return err
	}

	if c.DefaultArch == "" {
		c.DefaultArch = "x64"
	}
	c.DefaultArch = strings.ToLower(c.DefaultArch)
	if c.DefaultThreadSafety == "" {
		c.DefaultThreadSafety = "nts"
	}
	c.DefaultThreadSafety = strings.ToLower(c.DefaultThreadSafety)
	if c.DefaultThreadSafety != "nts" && c.DefaultThreadSafety != "ts" {
		return fmt.Errorf("default_thread_safety must be nts or ts")
	}
	return c.EnsureDirs()
}

func defaultRoot() (string, error) {
	if envRoot := os.Getenv("PHPENV_ROOT"); envRoot != "" {
		return normalizePath(envRoot), nil
	}
	if envRoot := os.Getenv("PHP_ENV_ROOT"); envRoot != "" {
		return normalizePath(envRoot), nil
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		if exe, err := os.Executable(); err == nil {
			dir := filepath.Dir(exe)
			if _, err := os.Stat(filepath.Join(dir, "versions")); err == nil {
				return normalizePath(dir), nil
			}
		}
		base = os.Getenv("APPDATA")
	}
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", errors.New("cannot determine base directory for phpenv")
		}
		base = filepath.Join(home, ".phpenv")
	}
	return normalizePath(filepath.Join(base, "phpenv")), nil
}

func normalizePath(p string) string {
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) {
		abs, err := filepath.Abs(p)
		if err == nil {
			p = abs
		}
	}
	return filepath.Clean(p)
}

func normalizePathSlice(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, v := range values {
		if v == "" {
			continue
		}
		n := normalizePath(v)
		if _, ok := seen[strings.ToLower(n)]; ok {
			continue
		}
		seen[strings.ToLower(n)] = struct{}{}
		out = append(out, n)
	}
	return out
}

func (c *Config) normalizeSelection(sel *Selection) error {
	if sel == nil {
		return nil
	}
	sel.Version = strings.TrimSpace(sel.Version)
	sel.Description = strings.TrimSpace(sel.Description)
	if sel.CustomPHP != "" {
		sel.CustomPHP = normalizePath(sel.CustomPHP)
	}
	if sel.Version != "" && sel.CustomPHP != "" {
		return errors.New("selection cannot contain both version and custom_php")
	}
	return nil
}
