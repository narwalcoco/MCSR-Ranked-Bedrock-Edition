// Package config loads runtime settings for the queen service.
//
// Settings are read from environment variables (with optional .env support
// handled by the caller). Defaults are tuned for local development.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config holds all runtime settings for the queen service.
type Config struct {
	// Env is "dev" or "prod". Affects default log format.
	Env string

	// HTTPAddr is the listener address for the public API (e.g. ":8080").
	HTTPAddr string

	// DatabasePath is the SQLite file location. Use ":memory:" for tests.
	DatabasePath string

	// ResourcesDir points at the top-level resources/ directory.
	ResourcesDir string

	// SeedsManifestPath is the seed manifest JSON, RELATIVE to ResourcesDir
	// (e.g. "seeds/manifest.json"). The queen anchors the manifest at ResourcesDir
	// so paths stay meaningful regardless of the working directory.
	SeedsManifestPath string

	// GameTimeout caps how long a single match may run before being cancelled.
	GameTimeout time.Duration

	// LogLevel is one of: debug, info, warn, error.
	LogLevel string

	// LogFormat is one of: text, json.
	LogFormat string
}

// Default returns sensible defaults for local development.
func Default() Config {
	return Config{
		Env:              "dev",
		HTTPAddr:         ":8080",
		DatabasePath:     "data/queen.db",
		ResourcesDir:      "resources",
		SeedsManifestPath: "seeds/manifest.json",
		GameTimeout:      30 * time.Minute,
		LogLevel:         "info",
		LogFormat:        "text",
	}
}

// FromEnv loads config from process environment, layered on top of defaults.
func FromEnv() (Config, error) {
	c := Default()

	if v := os.Getenv("MCSR_ENV"); v != "" {
		c.Env = strings.ToLower(v)
	}
	if v := os.Getenv("MCSR_HTTP_ADDR"); v != "" {
		c.HTTPAddr = v
	}
	if v := os.Getenv("MCSR_DB_PATH"); v != "" {
		c.DatabasePath = v
	}
	if v := os.Getenv("MCSR_RESOURCES_DIR"); v != "" {
		c.ResourcesDir = v
	}
	if v := os.Getenv("MCSR_SEEDS_MANIFEST"); v != "" {
		c.SeedsManifestPath = v
	}
	if v := os.Getenv("MCSR_GAME_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return c, fmt.Errorf("MCSR_GAME_TIMEOUT: %w", err)
		}
		c.GameTimeout = d
	}
	if v := os.Getenv("MCSR_LOG_LEVEL"); v != "" {
		c.LogLevel = strings.ToLower(v)
	}
	if v := os.Getenv("MCSR_LOG_FORMAT"); v != "" {
		c.LogFormat = strings.ToLower(v)
	}

	// In prod, prefer JSON logs unless the operator overrides.
	if c.Env == "prod" && os.Getenv("MCSR_LOG_FORMAT") == "" {
		c.LogFormat = "json"
	}

	return c, c.Validate()
}

// Validate checks that the config is internally consistent.
func (c Config) Validate() error {
	var errs []error

	if c.HTTPAddr == "" {
		errs = append(errs, errors.New("HTTPAddr must not be empty"))
	}
	if c.DatabasePath == "" {
		errs = append(errs, errors.New("DatabasePath must not be empty"))
	}
	if c.ResourcesDir == "" {
		errs = append(errs, errors.New("ResourcesDir must not be empty"))
	}
	if c.SeedsManifestPath == "" {
		errs = append(errs, errors.New("SeedsManifestPath must not be empty"))
	}

	if c.GameTimeout <= 0 {
		errs = append(errs, errors.New("GameTimeout must be positive"))
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("LogLevel must be one of debug|info|warn|error, got %q", c.LogLevel))
	}
	switch c.LogFormat {
	case "text", "json":
	default:
		errs = append(errs, fmt.Errorf("LogFormat must be one of text|json, got %q", c.LogFormat))
	}

	if len(errs) > 0 {
		return fmt.Errorf("invalid config: %v", errors.Join(errs...))
	}
	return nil
}

// ResolveWritePaths turns the DatabasePath into an absolute path anchored at
// baseDir (typically the binary's directory), so the SQLite file lives next
// to the queen binary. ResourcesDir is left untouched — read paths are not
// anchored here, use ResolveReadPaths for that.
func (c Config) ResolveWritePaths(baseDir string) Config {
	c.DatabasePath = resolveAbs(baseDir, c.DatabasePath)
	return c
}

// ResolveReadPaths turns the ResourcesDir into an absolute path anchored at
// baseDir (typically the process CWD). This keeps `make run-queen` simple:
// standing in the repo root, `./bin/queen` finds resources/ automatically.
func (c Config) ResolveReadPaths(baseDir string) Config {
	c.ResourcesDir = resolveAbs(baseDir, c.ResourcesDir)
	return c
}

func resolveAbs(base, p string) string {
	if p == "" || p == ":memory:" {
		return p
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}
