//go:build windows

package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

const (
	// parentWaitTimeout bounds how long the helper waits for the
	// original application process to actually exit (docs/updater.md
	// §21/§23) - generous for a graceful shutdown, not unbounded.
	parentWaitTimeout = 3 * time.Minute

	// installerTimeout bounds the silent Inno Setup run itself.
	installerTimeout = 5 * time.Minute

	// restartLaunchDelay is a short pause before restarting the
	// installed application, giving the installer's own file handles a
	// moment to fully release - a small, defensive margin, not a
	// correctness requirement (the installer has already exited by this
	// point).
	restartLaunchDelay = 500 * time.Millisecond

	// resultFileName is the one-shot result record's fixed name inside
	// the update-staging subtree (docs/updater.md §26) - the next
	// application startup reads, surfaces, and deletes it.
	resultFileName = "install-result.json"
)

// runHelper is the real Windows implementation of RunHelper - see
// docs/updater.md §21 for the full ten-step design this follows in
// order.
func runHelper(args HelperArgs) HandoffResult {
	result := HandoffResult{ToVersion: args.ExpectedVersion}

	// Step 3: open a handle to the parent process now, while it is
	// (almost certainly) still running - binding the handle to this
	// specific process object closes the PID-reuse race entirely
	// (docs/updater.md §23).
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(args.ParentPID))
	if err != nil {
		// The parent may have already exited before the helper managed
		// to open a handle - proceed as "already exited" rather than
		// failing, since that is the common, harmless case for a fast
		// shutdown.
		return finishHelper(args, proceedWithInstall(args, &result))
	}
	defer windows.CloseHandle(handle)

	// Step 5: wait, bounded.
	event, waitErr := windows.WaitForSingleObject(handle, uint32(parentWaitTimeout.Milliseconds()))
	if waitErr != nil || event != windows.WAIT_OBJECT_0 {
		result.Outcome = OutcomeParentDidNotExit
		writeHelperResult(args, result)
		return result
	}

	return finishHelper(args, proceedWithInstall(args, &result))
}

func finishHelper(args HelperArgs, result HandoffResult) HandoffResult {
	writeHelperResult(args, result)
	return result
}

// proceedWithInstall implements steps 6-10: re-verify, install, verify
// version, restart.
func proceedWithInstall(args HelperArgs, result *HandoffResult) HandoffResult {
	stagingDir := filepath.Dir(args.CandidatePath)

	// Step 6: re-verify the staged installer before executing anything -
	// defense in depth against staging-directory tampering between the
	// first verification (Manager.Download) and now. The candidate's own
	// file name embeds a 12-hex-character prefix of its SHA-256
	// (download.go's "verified-<version>-<sha12>.exe" convention) -
	// re-derived here rather than passed as a separate argument, so the
	// helper's closed argument set stays minimal.
	if err := reverifyCandidate(args.CandidatePath); err != nil {
		result.Outcome = OutcomeReverifyFailed
		return *result
	}

	// Captured before the installer runs, purely for the result
	// record's own "updated from X to Y" wording - never affects
	// whether the install proceeds.
	if before, err := readInstalledVersion(args.TargetExePath); err == nil {
		result.FromVersion = before
	}

	// Step 7: run the real Inno Setup installer, silently, never /DIR=
	// (docs/updater.md §20 - same-AppId upgrade preserves the existing
	// install location automatically).
	logPath := filepath.Join(stagingDir, "install.log")
	installCmd := exec.Command(args.CandidatePath, //nolint:gosec // candidatePath is application-verified, not user input.
		"/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/LOG="+logPath)

	installDone := make(chan error, 1)
	if err := installCmd.Start(); err != nil {
		result.Outcome = fmt.Sprintf(outcomeInstallerFailedFmt, -1)
		return *result
	}
	go func() { installDone <- installCmd.Wait() }()

	select {
	case waitErr := <-installDone:
		exitCode := 0
		if waitErr != nil {
			if exitErr, ok := waitErr.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}
		if exitCode != 0 {
			result.Outcome = fmt.Sprintf(outcomeInstallerFailedFmt, exitCode)
			return *result
		}
	case <-time.After(installerTimeout):
		_ = installCmd.Process.Kill()
		result.Outcome = fmt.Sprintf(outcomeInstallerFailedFmt, -2)
		return *result
	}

	// Step 8: verify the installed executable actually reports the
	// intended version - the installer's own exit code 0 is never
	// trusted alone (docs/updater.md §26).
	installedVersion, err := readInstalledVersion(args.TargetExePath)
	if err != nil || installedVersion != args.ExpectedVersion {
		result.Outcome = OutcomeVersionMismatch
		return *result
	}

	// Step 9/10: restart the installed application, then leave the
	// result for the freshly-restarted process to read once.
	time.Sleep(restartLaunchDelay)
	restartCmd := exec.Command(args.TargetExePath) //nolint:gosec // targetExePath is application-verified, not user input.
	if err := restartCmd.Start(); err != nil {
		result.Outcome = OutcomeRestartFailed
		return *result
	}

	result.Outcome = OutcomeOK
	return *result
}

// reverifyCandidate confirms the on-disk candidate's SHA-256 prefix
// still matches the one encoded in its own file name.
func reverifyCandidate(candidatePath string) error {
	base := filepath.Base(candidatePath)
	const prefix = "verified-"
	if !strings.HasPrefix(base, prefix) {
		return fmt.Errorf("candidate %q is not a recognized verified artifact name", base)
	}
	// "verified-<version>-<sha12><ext>"
	rest := strings.TrimPrefix(base, prefix)
	lastDash := strings.LastIndex(rest, "-")
	if lastDash < 0 {
		return fmt.Errorf("candidate %q has an unrecognized name shape", base)
	}
	shaSegment := rest[lastDash+1:]
	shaSegment = strings.TrimSuffix(shaSegment, filepath.Ext(shaSegment))
	if len(shaSegment) != 12 {
		return fmt.Errorf("candidate %q does not embed a 12-character hash prefix", base)
	}

	f, err := os.Open(candidatePath) // #nosec G304 -- candidatePath is application-owned staging content.
	if err != nil {
		return err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := copyForHash(hasher, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(hasher.Sum(nil))
	if actual[:12] != shaSegment {
		return fmt.Errorf("candidate %q failed re-verification: hash prefix mismatch", base)
	}
	return nil
}

func copyForHash(hasher interface{ Write([]byte) (int, error) }, f *os.File) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, err := f.Read(buf)
		if n > 0 {
			hasher.Write(buf[:n])
			total += int64(n)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return total, nil
			}
			return total, err
		}
	}
}

// readInstalledVersion runs the freshly-installed executable with
// --version and parses its first line ("<ProductName> <version>") -
// reuses the existing, unchanged buildinfo/--version output rather than
// inventing a second version-reporting mechanism.
func readInstalledVersion(exePath string) (string, error) {
	out, err := exec.Command(exePath, "--version").Output() //nolint:gosec // exePath is application-verified, not user input.
	if err != nil {
		return "", err
	}
	return parseVersionOutput(string(out))
}

// parseVersionOutput extracts the version token from cmd/server's own
// --version output ("<ProductName> <version>", first line - see
// handleVersionFlag in cmd/server/main.go) - split out from
// readInstalledVersion so the parsing logic itself is testable without
// actually running a process.
func parseVersionOutput(output string) (string, error) {
	firstLine := strings.SplitN(output, "\n", 2)[0]
	parts := strings.Fields(firstLine)
	if len(parts) == 0 {
		return "", fmt.Errorf("--version produced no output")
	}
	return parts[len(parts)-1], nil
}

// writeHelperResult writes the small, bounded, no-secret result record
// (docs/updater.md §26) beside the candidate, for the restarted
// application to read once on its next startup.
func writeHelperResult(args HelperArgs, result HandoffResult) {
	record := struct {
		Outcome     string `json:"outcome"`
		FromVersion string `json:"fromVersion"`
		ToVersion   string `json:"toVersion"`
		At          string `json:"at"`
	}{
		Outcome: result.Outcome, FromVersion: result.FromVersion, ToVersion: result.ToVersion,
		At: time.Now().UTC().Format(time.RFC3339),
	}

	raw, err := json.Marshal(record)
	if err != nil {
		return
	}
	stagingDir := filepath.Dir(args.CandidatePath)
	resultPath := filepath.Join(stagingDir, resultFileName)
	_ = os.WriteFile(resultPath, raw, 0o600)
}

// Flag names cmd/server/main.go uses to build HelperArgs - kept here so
// the exact spellings are owned by this package, never duplicated.
const (
	FlagUpdateHelper    = "update-helper"
	FlagParentPID       = "parent-pid"
	FlagCandidate       = "candidate"
	FlagTargetExe       = "target-exe"
	FlagExpectedVersion = "expected-version"
)

// ValidateHelperArgs checks that every required field is present and
// that CandidatePath actually resolves inside a real "updates"
// subdirectory - the one structural check RunHelper's caller performs
// before ever executing anything (docs/updater.md §22).
func ValidateHelperArgs(parentPID int, candidate, targetExe, expectedVersion string) (HelperArgs, error) {
	if parentPID <= 0 || candidate == "" || targetExe == "" || expectedVersion == "" {
		return HelperArgs{}, fmt.Errorf("update-helper: missing required arguments")
	}
	absCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return HelperArgs{}, err
	}
	if filepath.Base(filepath.Dir(absCandidate)) != updatesSubdir {
		return HelperArgs{}, fmt.Errorf("update-helper: candidate path is not inside the update staging directory")
	}
	absTarget, err := filepath.Abs(targetExe)
	if err != nil {
		return HelperArgs{}, err
	}
	return HelperArgs{
		ParentPID: parentPID, CandidatePath: absCandidate,
		TargetExePath: absTarget, ExpectedVersion: expectedVersion,
	}, nil
}
