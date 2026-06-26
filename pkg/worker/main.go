// Worker agent: manages Bedrock Dedicated Server instances and reports
// their status to the queen.
//
// Phase 3: registers with the queen on startup, sends periodic heartbeats,
// advertises slots, and supports a --fake mode for dev testing.
//
//	--fake        no real BDS processes, just log slot state changes
//	--queen       queen HTTP address (default http://127.0.0.1:8080)
//	--name        human-readable worker name (default "local-worker")
//	--host        host/IP where BDS instances are reachable (default 127.0.0.1)
//	--port        this worker's own API port (default 0, unused in Phase 3)
//	--max-slots   max concurrent BDS instances (default 4)
//	--heartbeat   heartbeat interval (default 15s)
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mcsr-ranked-bedrock/pkg/shared/logging"
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
		fakeFlag      = flag.Bool("fake", false, "fake mode: log slot changes, no real BDS")
		queenFlag     = flag.String("queen", "http://127.0.0.1:8080", "queen HTTP address")
		nameFlag      = flag.String("name", "local-worker", "worker display name")
		hostFlag      = flag.String("host", "127.0.0.1", "BDS host/IP advertised to the queen")
		portFlag      = flag.Int("port", 0, "worker's own API port (Phase 4+)")
		maxSlotsFlag  = flag.Int("max-slots", 4, "max concurrent BDS instances")
		heartbeatFlag = flag.Duration("heartbeat", 15*time.Second, "heartbeat interval")
		logLevel      = flag.String("log-level", "info", "debug|info|warn|error")
		logFmt        = flag.String("log-format", "text", "text|json")
	)
	flag.Parse()

	logger := logging.Setup(logging.Options{Level: *logLevel, Format: *logFmt})

	logger.Info("worker starting",
		"version", version.Version,
		"commit", version.Commit,
		"queen", *queenFlag,
		"name", *nameFlag,
		"host", *hostFlag,
		"fake", *fakeFlag,
		"max_slots", *maxSlotsFlag,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Register with the queen.
	workerID, _, err := register(ctx, *queenFlag, *nameFlag, *hostFlag, *portFlag, *maxSlotsFlag)
	if err != nil {
		return fmt.Errorf("register with queen: %w", err)
	}
	logger.Info("registered with queen", "id", workerID, "max_slots", *maxSlotsFlag)

	// Start heartbeat loop.
	hbDone := make(chan struct{})
	go func() {
		defer close(hbDone)
		runHeartbeats(ctx, *queenFlag, workerID, *heartbeatFlag, logger)
	}()

	if *fakeFlag {
		logger.Info("fake mode enabled — no real BDS processes will be started")
	}

	// Wait for shutdown signal.
	<-ctx.Done()
	logger.Info("shutting down")

	// Drain this worker before exiting.
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer drainCancel()
	if err := drain(drainCtx, *queenFlag, workerID); err != nil {
		logger.Warn("drain failed (queen may already consider us offline)", "err", err)
	}

	<-hbDone
	logger.Info("worker stopped cleanly")
	return nil
}

// --------------------------------------------------------------------------
// Queen API calls
// --------------------------------------------------------------------------

type registerBody struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	MaxSlots int    `json:"max_slots"`
}

type registerResponse struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	AuthToken string `json:"auth_token"`
	MaxSlots  int    `json:"max_slots"`
}

func register(ctx context.Context, queenAddr, name, host string, port, maxSlots int) (id int64, token string, err error) {
	body, err := json.Marshal(registerBody{Name: name, Host: host, Port: port, MaxSlots: maxSlots})
	if err != nil {
		return 0, "", fmt.Errorf("marshal register body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, queenAddr+"/workers", bytes.NewReader(body))
	if err != nil {
		return 0, "", fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("POST /workers: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return 0, "", fmt.Errorf("POST /workers returned %d", resp.StatusCode)
	}

	var reg registerResponse
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return 0, "", fmt.Errorf("decode register response: %w", err)
	}
	return reg.ID, reg.AuthToken, nil
}

func runHeartbeats(ctx context.Context, queenAddr string, workerID int64, interval time.Duration, logger *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Send first heartbeat immediately.
	sendHeartbeat(ctx, queenAddr, workerID, logger)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sendHeartbeat(ctx, queenAddr, workerID, logger)
		}
	}
}

func sendHeartbeat(ctx context.Context, queenAddr string, workerID int64, logger *slog.Logger) {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		fmt.Sprintf("%s/workers/%d/heartbeat", queenAddr, workerID), nil)
	if err != nil {
		logger.Warn("heartbeat: cannot build request", "err", err)
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Warn("heartbeat: queen unreachable", "err", err)
		return
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Warn("heartbeat: unexpected status", "status", resp.StatusCode)
		return
	}
}

func drain(ctx context.Context, queenAddr string, workerID int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/workers/%d/drain", queenAddr, workerID), nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	return nil
}
