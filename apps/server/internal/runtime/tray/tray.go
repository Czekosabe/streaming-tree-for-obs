// Package tray implements the Stage 20E Windows notification-area
// (system tray) icon for desktop mode - see docs/windows-packaging.md §30 for
// the full contract.
//
// This exists because closing the browser tab does not stop the
// backend: a desktop user with no visible window and no console has no
// way to see the application is still running, open it again, or quit
// it, short of Task Manager. The tray icon is the one persistent,
// always-available piece of desktop UI for that.
//
// Real production behavior lives only in tray_windows.go, built only
// for GOOS=windows via raw Win32 syscalls (Shell_NotifyIconW, a hidden
// window, a popup menu) - no third-party systray library, no CGO, no
// Electron/WebView2/Tauri, no separate helper process. Every other
// platform gets tray_other.go's honest no-op, matching this project's
// existing per-platform pattern (see internal/runtime/singleinstance,
// internal/runtime/browserlaunch, internal/runtime/nativealert).
//
// Menu text is plain English only, not run through the frontend's i18n
// system - this mirrors internal/runtime/nativealert's own existing
// precedent (its MessageBoxW title/message are plain Go strings too):
// native OS-level UI in this application has never been localized,
// only the web frontend is.
package tray

import _ "embed"

// IconICO is the tray icon's raw Windows .ico bytes - see
// assets/tray.ico's own directory and scripts/generate-tray-icon.go
// for how it was produced and why (no final branding art exists yet
// for this project - see apps/web/src/components/layout/BrandMark.tsx).
// Callers pass this straight through as Options.IconICO; it is a
// plain package-level embed (not build-tag-restricted) since it is
// tiny (under 1 KB) and harmless to embed into any platform's binary,
// even though only the Windows build ever actually loads it.
//
//go:embed assets/tray.ico
var IconICO []byte

// Options configures one tray icon instance. All callbacks are
// invoked on the tray's own locked OS thread (see tray_windows.go) -
// none of them may block for long or perform Win32 UI calls of their
// own; each should hand off to the rest of the application quickly
// (e.g. via an existing channel/mutex-guarded call), exactly the way
// OnOpenDashboard/OnQuit below reuse the application's own existing
// mechanisms rather than doing UI work themselves.
type Options struct {
	// Tooltip is the short text shown when the pointer hovers the
	// icon - kept well under NOTIFYICONDATAW's 128-WCHAR szTip limit.
	Tooltip string

	// IconICO is the raw bytes of a Windows .ico file - callers should
	// pass this package's own IconICO var (above) unless a real, final
	// branding icon replaces it later.
	IconICO []byte

	// OnOpenDashboard opens the Dashboard in the default browser via
	// the application's existing browserlaunch mechanism - never
	// starts a second backend.
	OnOpenDashboard func()

	// OnOpenLogs opens the canonical Logs & Diagnostics route, the
	// same way.
	OnOpenLogs func()

	// StatusLabel returns the current concise ingest status text
	// (e.g. "Ingest: Waiting") - called fresh immediately before the
	// menu is shown, never cached, so it is never stale.
	StatusLabel func() string

	// UpdatesLabel returns the current "Check for updates" menu
	// item's label and whether it should be enabled - false when the
	// updater is disabled, a manual/test build, or platform-
	// unsupported (docs/updater.md §35/§43), so the item is honestly
	// grayed out rather than clickable-but-refused.
	UpdatesLabel func() (label string, enabled bool)

	// OnCheckForUpdates delegates to the application's one existing
	// updater.Manager.CheckNow - never a second updater
	// implementation. Only invoked when UpdatesLabel reports enabled.
	OnCheckForUpdates func()

	// OnQuit triggers the exact same graceful-shutdown path the web
	// UI's "Quit Streaming Tree" action and the updater's install
	// handoff already use (cmd/server/main.go's signal.NotifyContext
	// CancelFunc) - never a second shutdown implementation, and never
	// an immediate os.Exit.
	OnQuit func()
}

// Handle controls a running tray icon.
type Handle interface {
	// Stop removes the icon and releases every OS resource it holds.
	// Safe to call from any goroutine, and safe to call more than
	// once. Blocks until the tray's own message loop has actually
	// exited, so no zombie icon can outlive Stop returning.
	Stop()
}
