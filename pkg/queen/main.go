// Queen is the central matchmaking / orchestration service for
// MCSR Ranked Bedrock. In Phase 2 it exposes /queue, /me, /matches, etc.
// Phase 3+ adds worker registration and BDS lifecycle on top of these.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mcsr-ranked-bedrock/pkg/queen/internal/api"
	"github.com/mcsr-ranked-bedrock/pkg/queen/internal/config"
	"github.com/mcsr-ranked-bedrock/pkg/queen/internal/db"
	"github.com/mcsr-ranked-bedrock/pkg/queen/internal/matchmaker"
	"github.com/mcsr-ranked-bedrock/pkg/queen/internal/store"
	"github.com/mcsr-ranked-bedrock/pkg/shared/logging"
	"github.com/mcsr-ranked-bedrock/pkg/shared/seeds"
	"github.com/mcsr-ranked-bedrock/pkg/shared/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		envFlag       = flag.String("env", "", "environment (dev|prod)")
		addrFlag      = flag.String("addr", "", "HTTP listen address (e.g. :8080)")
		dbFlag        = flag.String("db", "", "SQLite database file path")
		resourcesFlag = flag.String("resources", "", "resources directory (absolute or CWD-relative)")
		matchIntFlg   = flag.Duration("match-interval", time.Second, "matchmaker fallback poll interval (Go duration)")
		logLevelFlg   = flag.String("log-level", "", "debug|info|warn|error")
		logFmtFlg     = flag.String("log-format", "", "text|json")
	)
	flag.Parse()

	cfg, err := config.FromEnv()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if *envFlag != "" {
		cfg.Env = *envFlag
	}
	if *addrFlag != "" {
		cfg.HTTPAddr = *addrFlag
	}
	if *dbFlag != "" {
		cfg.DatabasePath = *dbFlag
	}
	if *resourcesFlag != "" {
		cfg.ResourcesDir = *resourcesFlag
	}
	if *logLevelFlg != "" {
		cfg.LogLevel = *logLevelFlg
	}
	if *logFmtFlg != "" {
		cfg.LogFormat = *logFmtFlg
	}

	// Path anchoring (see config docstrings).
	if exe, err := os.Executable(); err == nil {
		cfg = cfg.ResolveWritePaths(filepath.Dir(exe))
	}
	if cwd, err := os.Getwd(); err == nil {
		cfg = cfg.ResolveReadPaths(cwd)
	}

	logger := logging.Setup(logging.Options{Level: cfg.LogLevel, Format: cfg.LogFormat})
	logger.Info("starting queen",
		"version", version.Version,
		"commit", version.Commit,
		"env", cfg.Env,
		"addr", cfg.HTTPAddr,
		"db", cfg.DatabasePath,
		"resources", cfg.ResourcesDir,
		"match_interval", *matchIntFlg,
	)

	if cfg.DatabasePath != ":memory:" {
		if dir := filepath.Dir(cfg.DatabasePath); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create db dir: %w", err)
			}
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	database, err := db.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			logger.Warn("close db", "err", err)
		}
	}()

	manifest, err := seeds.Load(cfg.ResourcesDir, cfg.SeedsManifestPath)
	if err != nil {
		logger.Warn("seed manifest unavailable; /seeds/categories 503 + matchmaker disabled", "err", err)
		manifest = nil
	} else {
		logger.Info("seed manifest loaded", "categories", len(manifest.List()))
	}

	st := store.New(database)

	mm := matchmaker.New(st, manifest, *matchIntFlg, logger)
	mmDone := make(chan struct{})
	go func() {
		defer close(mmDone)
		if err := mm.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Warn("matchmaker exited with error", "err", err)
		}
	}()

	srv := api.New(database, st, mm, manifest, version.Get("queen"))
	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http listening", "addr", cfg.HTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http shutdown: %w", err)
	}
	<-mmDone // matchmaker listens to ctx — this returns promptly
	logger.Info("queen stopped cleanly")
	return nil
}
