// Worker agent: manages Bedrock Dedicated Server instances and reports
// their status to the queen.
//
// Phase 0 stub: prints version info and exits. Phase 3 adds:
//   - Registration with the queen
//   - Slot advertisement + lifecycle
//   - Real BDS process management
package main

import (
	"flag"
	"fmt"
	"log/slog"

	"github.com/mcsr-ranked-bedrock/pkg/shared/logging"
	"github.com/mcsr-ranked-bedrock/pkg/shared/version"
)

func main() {
	var (
		logLevel = flag.String("log-level", "info", "debug|info|warn|error")
		logFmt   = flag.String("log-format", "text", "text|json")
	)
	flag.Parse()

	logging.Setup(logging.Options{Level: *logLevel, Format: *logFmt})

	slog.Info("worker stub starting",
		"version", version.Version,
		"commit", version.Commit,
		"go", fmt.Sprintf("%s", version.Get("worker").GoVersion),
		"note", "Phase 0 placeholder. Real implementation lands in Phase 3.",
	)

	slog.Info("worker stub exiting")
}
