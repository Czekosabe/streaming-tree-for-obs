//go:build windows

// Real Windows update-installer handoff (docs/updater.md §19/§21/§22).
package updater

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// WindowsHandoff is the real Handoff implementation for packaged
// Windows installs.
type WindowsHandoff struct {
	// dataDir is the per-user application data directory
	// (config.Config.DataDir) - the helper copy and its arguments live
	// under dataDir/updates, exactly like Manager's own download
	// staging (docs/updater.md §16/§33).
	dataDir string
}

// NewPlatformHandoff returns the real Windows Handoff implementation.
func NewPlatformHandoff(dataDir string) Handoff {
	return &WindowsHandoff{dataDir: dataDir}
}

// Available implements the installed-context verification from
// docs/updater.md §19: Inno Setup automatically creates unins000.exe/
// unins000.dat beside the installed executable for every install it
// performs - no installer-script change was needed to produce this
// marker. Their presence beside the running executable is treated as
// proof this is a genuine Inno-Setup-installed instance.
func (h *WindowsHandoff) Available() (bool, string) {
	exe, err := os.Executable()
	if err != nil {
		return false, BlockerNotInstalledCtx
	}
	return checkInstalledMarkers(filepath.Dir(exe))
}

// checkInstalledMarkers is Available's own directory check, split out
// so tests can exercise it against a real temporary directory instead
// of this test binary's own real location.
func checkInstalledMarkers(dir string) (bool, string) {
	if !isRegularFile(filepath.Join(dir, "unins000.exe")) || !isRegularFile(filepath.Join(dir, "unins000.dat")) {
		return false, BlockerNotInstalledCtx
	}
	return true, ""
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// Begin implements docs/updater.md §21 steps 1-2: copies the running
// executable into the update-staging subtree (never the install
// directory - the installer needs to freely replace files there a
// moment later) and launches it in strict internal update-helper mode
// with the closed, application-generated argument set from
// docs/updater.md §22. Returns once the helper process has been
// launched successfully, not once installation finishes.
func (h *WindowsHandoff) Begin(ctx context.Context, candidatePath, expectedVersion string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve running executable: %w", err)
	}

	stagingDir := filepath.Join(h.dataDir, updatesSubdir)
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return fmt.Errorf("create update staging directory: %w", err)
	}

	helperPath := filepath.Join(stagingDir, "update-helper.exe")
	if err := copyExecutable(exe, helperPath); err != nil {
		return fmt.Errorf("stage update helper: %w", err)
	}

	helperArgs, err := ValidateHelperArgs(os.Getpid(), candidatePath, exe, expectedVersion)
	if err != nil {
		return err
	}

	cmd := exec.Command(helperPath, //nolint:gosec // helperPath is a fresh application-owned copy, not user input.
		"-"+FlagUpdateHelper,
		"-"+FlagParentPID, strconv.Itoa(helperArgs.ParentPID),
		"-"+FlagCandidate, helperArgs.CandidatePath,
		"-"+FlagTargetExe, helperArgs.TargetExePath,
		"-"+FlagExpectedVersion, helperArgs.ExpectedVersion,
	)
	cmd.Dir = stagingDir
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch update helper: %w", err)
	}
	// Deliberately not Wait()-ed: the helper is meant to outlive this
	// process's own shutdown (docs/updater.md §21).
	return nil
}

func copyExecutable(src, dst string) error {
	in, err := os.Open(src) // #nosec G304 -- src is this process's own executable.
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst) // #nosec G304 -- dst is inside the application-owned staging directory.
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
