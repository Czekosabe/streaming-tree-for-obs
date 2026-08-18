package updater

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
)

// resultRecord mirrors the JSON shape helper_windows.go's
// writeHelperResult produces.
type resultRecord struct {
	Outcome     string `json:"outcome"`
	FromVersion string `json:"fromVersion"`
	ToVersion   string `json:"toVersion"`
	At          string `json:"at"`
}

// consumePostUpdateResult reads and deletes the one-shot post-update
// result file, if present (docs/updater.md §26). Called once, from
// Start, on every application startup - harmless when no update was
// ever attempted (the file simply does not exist), and safe to call on
// any platform (the file is only ever produced by the Windows helper,
// but reading a missing file is a normal, silent no-op here).
func (m *Manager) consumePostUpdateResult() {
	if m.dataDir == "" {
		return
	}
	path := filepath.Join(m.dataDir, updatesSubdir, "install-result.json")

	raw, err := os.ReadFile(path) // #nosec G304 -- path is application-owned, not user input.
	if err != nil {
		return // No result file - the normal case.
	}
	_ = os.Remove(path)

	var record resultRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		m.logger.Warn("could not parse the post-update result record", slog.Any("error", err))
		return
	}

	m.mu.Lock()
	m.postUpdateOutcome = record.Outcome
	m.postUpdateFromVersion = record.FromVersion
	m.postUpdateToVersion = record.ToVersion
	m.mu.Unlock()

	m.logger.Info("consumed post-update result",
		slog.String("outcome", record.Outcome),
		slog.String("from", record.FromVersion),
		slog.String("to", record.ToVersion),
	)
}
