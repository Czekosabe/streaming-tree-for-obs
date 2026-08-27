//go:build windows

package tray

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

// Real Windows implementation: raw Win32 syscalls only
// (Shell32.dll/User32.dll via syscall.NewLazyDLL, exactly the pattern
// internal/runtime/browserlaunch/nativealert/singleinstance already
// use through golang.org/x/sys/windows's own thin wrapper around the
// same mechanism) - no third-party systray library, no CGO. Every
// struct layout, constant, and call sequence below (including the
// documented SetForegroundWindow/TrackPopupMenu/PostMessage dismissal
// fix, and the NOTIFYICON_VERSION_4 callback-message shape) was
// verified against learn.microsoft.com's own API reference before
// being written, since a wrong struct layout here is a memory-safety
// bug, not a compile error.

var (
	modShell32 = syscall.NewLazyDLL("shell32.dll")
	modUser32  = syscall.NewLazyDLL("user32.dll")

	procShellNotifyIconW         = modShell32.NewProc("Shell_NotifyIconW")
	procRegisterClassExW         = modUser32.NewProc("RegisterClassExW")
	procUnregisterClassW         = modUser32.NewProc("UnregisterClassW")
	procCreateWindowExW          = modUser32.NewProc("CreateWindowExW")
	procDestroyWindow            = modUser32.NewProc("DestroyWindow")
	procDefWindowProcW           = modUser32.NewProc("DefWindowProcW")
	procGetMessageW              = modUser32.NewProc("GetMessageW")
	procTranslateMessage         = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW         = modUser32.NewProc("DispatchMessageW")
	procPostMessageW             = modUser32.NewProc("PostMessageW")
	procPostQuitMessage          = modUser32.NewProc("PostQuitMessage")
	procCreatePopupMenu          = modUser32.NewProc("CreatePopupMenu")
	procDestroyMenu              = modUser32.NewProc("DestroyMenu")
	procAppendMenuW              = modUser32.NewProc("AppendMenuW")
	procTrackPopupMenu           = modUser32.NewProc("TrackPopupMenu")
	procSetForegroundWindow      = modUser32.NewProc("SetForegroundWindow")
	procGetCursorPos             = modUser32.NewProc("GetCursorPos")
	procCreateIconFromResourceEx = modUser32.NewProc("CreateIconFromResourceEx")
	procDestroyIcon              = modUser32.NewProc("DestroyIcon")
	procRegisterWindowMessageW   = modUser32.NewProc("RegisterWindowMessageW")
	modKernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procGetModuleHandleW         = modKernel32.NewProc("GetModuleHandleW")
)

const (
	nimAdd        = 0x00000000
	nimModify     = 0x00000001
	nimDelete     = 0x00000002
	nimSetVersion = 0x00000004

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	// nifShowTip forces the standard hover tooltip under
	// NOTIFYICON_VERSION_4 (see notifyIconVersion4 below) - without it,
	// Vista+'s modern tray callback shape suppresses szTip's plain
	// tooltip entirely in favor of a richer pop-up mechanism this
	// package does not implement, which is exactly why the tray showed
	// no hover identification at all before this flag was added: a real
	// bug, found by re-reading this file's own construction of
	// NOTIFYICONDATAW against the documented NIF_* flag table, not by
	// guessing.
	nifShowTip = 0x00000080

	notifyIconVersion4 = 4

	wmApp         = 0x8000
	wmDestroy     = 0x0002
	wmClose       = 0x0010
	wmCommand     = 0x0111
	wmNull        = 0x0000
	wmLButtonUp   = 0x0202
	wmRButtonUp   = 0x0205
	wmContextMenu = 0x007B
	ninSelect     = wmApp + 0
	ninKeySelect  = wmApp + 1

	trayCallbackMsg = wmApp + 100
	// wmTrayQuit is a private message this package posts to its own
	// window to request a clean shutdown from any goroutine - distinct
	// from wmClose, which Windows itself can also send (e.g. via
	// Task Manager's "End task" on the hidden window, unlikely but
	// handled the same way).
	wmTrayQuit = wmApp + 101

	mfString    = 0x00000000
	mfGrayed    = 0x00000001
	mfSeparator = 0x00000800

	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100

	lrDefaultColor = 0x00000000

	wsExToolWindow     = 0x00000080
	wsOverlappedWindow = 0x00CF0000 // WS_OVERLAPPED|WS_CAPTION|WS_SYSMENU|WS_THICKFRAME|WS_MINIMIZEBOX|WS_MAXIMIZEBOX

	cmdStatus        = 1000
	cmdOpenDashboard = 1001
	cmdOpenLogs      = 1002
	cmdCheckUpdates  = 1003
	cmdQuit          = 1004

	className = "StreamingTreeForOBSTrayWindow"

	// shutdownRequestMessageName is the RegisterWindowMessageW name
	// both this package and scripts/installer/streaming-tree.iss's
	// Pascal Script register - see shutdownRequestMsg's own doc
	// comment above. RegisterWindowMessageW guarantees the same string
	// always resolves to the same OS-assigned message id system-wide,
	// so no shared numeric constant needs to cross the Go/Pascal
	// Script boundary - only this exact string does.
	shutdownRequestMessageName = "StreamingTreeForOBS.RequestGracefulShutdown"
)

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     syscall.Handle
	hIcon         syscall.Handle
	hCursor       syscall.Handle
	hbrBackground syscall.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       syscall.Handle
}

type point struct{ X, Y int32 }

type msg struct {
	Hwnd     syscall.Handle
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       point
	LPrivate uint32
}

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// notifyIconDataW mirrors NOTIFYICONDATAW exactly (field order, sizes)
// - see the package doc comment for how this was verified.
type notifyIconDataW struct {
	CbSize           uint32
	HWnd             syscall.Handle
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            syscall.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         guid
	HBalloonIcon     syscall.Handle
}

// handle is the running tray's own state - fields are touched only
// from the locked OS thread the message loop owns, except stopOnce/
// doneCh, which are the safe cross-goroutine surface.
type handle struct {
	hwnd  syscall.Handle
	hMenu syscall.Handle
	hIcon syscall.Handle
	class *uint16
	opts  Options

	// taskbarCreatedMsg is the registered "TaskbarCreated" message id
	// (RegisterWindowMessageW), broadcast to every top-level window
	// when Explorer restarts - the documented signal that every
	// previously-added tray icon has been silently dropped and must be
	// re-added (learn.microsoft.com/windows/win32/shell/taskbar). 0
	// only if registration itself failed, in which case this handling
	// is simply skipped rather than treated as a fatal setup error - a
	// tray icon that cannot survive an Explorer restart is still far
	// better than no tray icon at all.
	taskbarCreatedMsg uint32

	// shutdownRequestMsg is a second RegisterWindowMessageW-registered
	// message (docs/windows-packaging.md §26): the Windows installer's
	// own Pascal Script uses the identical registered name to obtain
	// the same OS-assigned message id, then FindWindow+PostMessage's
	// it to this hidden window when it detects the application already
	// running (via the same AppMutex the single-instance package
	// already owns) - a real, physical Windows manual-test finding
	// that a manual installer upgrade over a running instance could
	// previously only proceed after the operator manually killed the
	// process in Task Manager. Handled identically to the tray's own
	// cmdQuit menu item below: it calls the exact same OnQuit callback
	// (main.go wires this to the same context-cancellation function
	// tray Quit, web Quit, and the built-in updater's install handoff
	// already converge on), never a second, parallel shutdown path. 0
	// only if registration itself failed, in which case an external
	// installer simply cannot reach this mechanism and falls back to
	// Inno Setup's own native AppMutex "please close it" prompt.
	shutdownRequestMsg uint32

	stopOnce sync.Once
	doneCh   chan struct{}
}

// wndProcCallback is set once, in Run, so the WNDPROC trampoline below
// can reach the one live handle - a Win32 window procedure has no way
// to carry a Go closure directly, so the running handle is looked up
// via the window's own GWLP_USERDATA-equivalent: here, simply a
// package-level pointer, since this application only ever runs one
// tray icon at a time (docs/windows-tray.md: exactly one per desktop
// instance).
var (
	activeMu sync.Mutex
	active   *handle
)

// Run creates the tray icon and its hidden window, then starts the
// Win32 message loop on a newly locked OS thread. Setup (class
// registration, window/icon creation) happens synchronously before Run
// returns, so a real failure is reported immediately rather than
// discovered later; the message loop itself then runs for the tray's
// lifetime in the background.
func Run(opts Options) (Handle, error) {
	h := &handle{opts: opts, doneCh: make(chan struct{})}

	setupErr := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(h.doneCh)

		if err := h.setup(); err != nil {
			setupErr <- err
			return
		}
		setupErr <- nil

		activeMu.Lock()
		active = h
		activeMu.Unlock()

		h.messageLoop()

		activeMu.Lock()
		if active == h {
			active = nil
		}
		activeMu.Unlock()
	}()

	if err := <-setupErr; err != nil {
		return nil, err
	}
	return h, nil
}

func (h *handle) Stop() {
	h.stopOnce.Do(func() {
		procPostMessageW.Call(uintptr(h.hwnd), wmTrayQuit, 0, 0)
	})
	<-h.doneCh
}

func (h *handle) setup() error {
	classNamePtr, err := syscall.UTF16PtrFromString(className)
	if err != nil {
		return fmt.Errorf("tray: encode class name: %w", err)
	}
	h.class = classNamePtr

	if taskbarCreatedName, err := syscall.UTF16PtrFromString("TaskbarCreated"); err == nil {
		id, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(taskbarCreatedName)))
		h.taskbarCreatedMsg = uint32(id)
	}

	if shutdownRequestName, err := syscall.UTF16PtrFromString(shutdownRequestMessageName); err == nil {
		id, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(shutdownRequestName)))
		h.shutdownRequestMsg = uint32(id)
	}

	hInstance, _, _ := procGetModuleHandleW.Call(0)

	wndProc := syscall.NewCallback(wndProcTrampoline)

	var wc wndClassExW
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	wc.lpfnWndProc = wndProc
	wc.hInstance = syscall.Handle(hInstance)
	wc.lpszClassName = classNamePtr

	atom, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		return fmt.Errorf("tray: RegisterClassExW failed: %w", callErr)
	}

	// A real, hidden, never-shown top-level window - not HWND_MESSAGE.
	// TrackPopupMenu's own documented SetForegroundWindow-based
	// dismissal fix requires a real top-level window, which a true
	// message-only window's semantics (no z-order, not enumerable)
	// conflict with. WS_EX_TOOLWINDOW keeps it out of the taskbar even
	// if it were ever accidentally shown, which it never is (no
	// WS_VISIBLE, no ShowWindow call).
	titlePtr, _ := syscall.UTF16PtrFromString(h.opts.Tooltip)
	hwnd, _, callErr := procCreateWindowExW.Call(
		uintptr(wsExToolWindow),
		uintptr(unsafe.Pointer(classNamePtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(wsOverlappedWindow),
		// Position/size are irrelevant: this window is never shown (no
		// WS_VISIBLE, ShowWindow is never called), so plain 0s are used
		// rather than CW_USEDEFAULT - CW_USEDEFAULT is the negative
		// value 0x80000000, and passing it correctly through a
		// syscall.Proc.Call's uintptr arguments requires deliberate
		// sign-extension that a window nobody ever sees does not need
		// to bother getting exactly right.
		0, 0,
		0, 0,
		0, 0,
		hInstance,
		0,
	)
	if hwnd == 0 {
		return fmt.Errorf("tray: CreateWindowExW failed: %w", callErr)
	}
	h.hwnd = syscall.Handle(hwnd)

	hIcon, err := loadIconFromICOBytes(h.opts.IconICO)
	if err != nil {
		procDestroyWindow.Call(uintptr(h.hwnd))
		return fmt.Errorf("tray: load icon: %w", err)
	}
	h.hIcon = hIcon

	if ret, callErr := h.addIcon(); ret == 0 {
		procDestroyIcon.Call(uintptr(h.hIcon))
		procDestroyWindow.Call(uintptr(h.hwnd))
		return fmt.Errorf("tray: Shell_NotifyIconW(NIM_ADD) failed: %w", callErr)
	}

	return nil
}

// addIcon issues NIM_ADD followed by NIM_SETVERSION(NOTIFYICON_VERSION_4)
// - the exact two-call sequence setup uses once at startup, and the
// TaskbarCreated handler in wndProc reuses identically to restore the
// icon after Explorer itself restarts (docs/windows-tray.md; see
// taskbarCreatedMsg's own doc comment).
// addIconFlags is the exact UFlags value the icon is added with -
// pulled out as its own pure function so the tooltip regression this
// fixed (NIF_SHOWTIP was missing, which silently suppresses the
// standard hover tooltip once NOTIFYICON_VERSION_4 is requested - see
// nifShowTip's own doc comment) is directly, meaningfully unit-
// testable, not just something a human has to notice by re-reading
// the Shell_NotifyIconW call site.
func addIconFlags() uint32 {
	return nifMessage | nifIcon | nifTip | nifShowTip
}

func (h *handle) addIcon() (ret uintptr, callErr error) {
	nid := h.buildNotifyIconData()
	nid.UFlags = addIconFlags()
	ret, _, callErr = procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	if ret == 0 {
		return ret, callErr
	}

	// NOTIFYICON_VERSION_4: modern per-monitor-DPI-aware callback
	// message shape and keyboard-accessible activation (NIN_SELECT/
	// NIN_KEYSELECT) - must be requested once, immediately after
	// NIM_ADD.
	verNid := h.buildNotifyIconData()
	verNid.UVersion = notifyIconVersion4
	procShellNotifyIconW.Call(nimSetVersion, uintptr(unsafe.Pointer(&verNid)))

	return ret, nil
}

func (h *handle) buildNotifyIconData() notifyIconDataW {
	var nid notifyIconDataW
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = h.hwnd
	nid.UID = 1
	nid.UCallbackMessage = trayCallbackMsg
	nid.HIcon = h.hIcon
	copyUTF16(nid.SzTip[:], h.opts.Tooltip)
	return nid
}

func copyUTF16(dst []uint16, s string) {
	encoded, err := syscall.UTF16FromString(s)
	if err != nil {
		return
	}
	n := len(encoded)
	if n > len(dst) {
		n = len(dst)
	}
	copy(dst[:n], encoded[:n])
	if n == len(dst) {
		dst[len(dst)-1] = 0
	}
}

func (h *handle) messageLoop() {
	for {
		var m msg
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 { // 0 = WM_QUIT, -1 = error
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	h.teardown()
}

func (h *handle) teardown() {
	nid := h.buildNotifyIconData()
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	if h.hMenu != 0 {
		procDestroyMenu.Call(uintptr(h.hMenu))
	}
	if h.hIcon != 0 {
		procDestroyIcon.Call(uintptr(h.hIcon))
	}
	if h.hwnd != 0 {
		procDestroyWindow.Call(uintptr(h.hwnd))
	}
	procUnregisterClassW.Call(uintptr(unsafe.Pointer(h.class)), 0)
}

// wndProcTrampoline is the one Win32 WNDPROC this package registers -
// syscall.NewCallback requires this exact func(uintptr,...)uintptr
// shape, so it cannot be a method; it looks up the single running
// handle (see `active` above) and dispatches to it.
func wndProcTrampoline(hwnd, uMsg, wParam, lParam uintptr) uintptr {
	activeMu.Lock()
	h := active
	activeMu.Unlock()

	if h == nil || syscall.Handle(hwnd) != h.hwnd {
		ret, _, _ := procDefWindowProcW.Call(hwnd, uMsg, wParam, lParam)
		return ret
	}
	return h.wndProc(uint32(uMsg), wParam, lParam)
}

func (h *handle) wndProc(uMsg uint32, wParam, lParam uintptr) uintptr {
	switch uMsg {
	case trayCallbackMsg:
		event := uint32(lParam) & 0xffff // LOWORD(lParam) under NOTIFYICON_VERSION_4
		switch event {
		case wmRButtonUp, wmContextMenu, ninKeySelect:
			h.showMenu()
		case wmLButtonUp, ninSelect:
			if h.opts.OnOpenDashboard != nil {
				h.opts.OnOpenDashboard()
			}
		}
		return 0

	case wmCommand:
		id := uint32(wParam) & 0xffff
		h.handleCommand(id)
		return 0

	case wmTrayQuit, wmClose:
		procPostQuitMessage.Call(0)
		return 0

	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}

	if h.taskbarCreatedMsg != 0 && uMsg == h.taskbarCreatedMsg {
		// Explorer just (re)started and dropped every previously-added
		// tray icon - re-add ours immediately rather than leaving it
		// silently gone until the process happens to restart.
		h.addIcon()
		return 0
	}

	if h.shutdownRequestMsg != 0 && uMsg == h.shutdownRequestMsg {
		// An external process (the Windows installer, upgrading or
		// uninstalling over a running instance) is asking this
		// application to shut down cooperatively - route through the
		// exact same OnQuit callback the tray's own Quit menu item
		// uses, never a separate teardown path.
		if h.opts.OnQuit != nil {
			h.opts.OnQuit()
		}
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(uintptr(h.hwnd), uintptr(uMsg), wParam, lParam)
	return ret
}

func (h *handle) handleCommand(id uint32) {
	switch id {
	case cmdOpenDashboard:
		if h.opts.OnOpenDashboard != nil {
			h.opts.OnOpenDashboard()
		}
	case cmdOpenLogs:
		if h.opts.OnOpenLogs != nil {
			h.opts.OnOpenLogs()
		}
	case cmdCheckUpdates:
		if h.opts.OnCheckForUpdates != nil {
			h.opts.OnCheckForUpdates()
		}
	case cmdQuit:
		if h.opts.OnQuit != nil {
			h.opts.OnQuit()
		}
	}
}

// showMenu rebuilds the popup menu fresh every time (status text and
// update-eligibility can both have changed since it was last shown),
// then runs the documented SetForegroundWindow/TrackPopupMenu/
// PostMessage(WM_NULL) sequence learn.microsoft.com's own
// TrackPopupMenu reference describes as required for a tray icon's
// menu to dismiss correctly on an outside click.
func (h *handle) showMenu() {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)

	appendMenuString(hMenu, mfString, cmdOpenDashboard, "Open Streaming Tree")
	appendMenuString(hMenu, mfString, cmdOpenLogs, "Open Logs && Diagnostics")
	appendSeparator(hMenu)

	statusLabel := "Ingest: unknown"
	if h.opts.StatusLabel != nil {
		statusLabel = h.opts.StatusLabel()
	}
	appendMenuString(hMenu, mfString|mfGrayed, cmdStatus, statusLabel)
	appendSeparator(hMenu)

	updatesLabel, updatesEnabled := "Check for updates", false
	if h.opts.UpdatesLabel != nil {
		updatesLabel, updatesEnabled = h.opts.UpdatesLabel()
	}
	updatesFlags := uint32(mfString)
	if !updatesEnabled {
		updatesFlags |= mfGrayed
	}
	appendMenuString(hMenu, updatesFlags, cmdCheckUpdates, updatesLabel)
	appendSeparator(hMenu)

	appendMenuString(hMenu, mfString, cmdQuit, "Quit Streaming Tree")

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	procSetForegroundWindow.Call(uintptr(h.hwnd))
	ret, _, _ := procTrackPopupMenu.Call(
		hMenu, uintptr(tpmRightButton|tpmReturnCmd),
		uintptr(pt.X), uintptr(pt.Y),
		0, uintptr(h.hwnd), 0,
	)
	procPostMessageW.Call(uintptr(h.hwnd), wmNull, 0, 0)

	if ret != 0 {
		h.handleCommand(uint32(ret))
	}
}

func appendMenuString(hMenu uintptr, flags uint32, id uintptr, text string) {
	textPtr, err := syscall.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	procAppendMenuW.Call(hMenu, uintptr(flags), id, uintptr(unsafe.Pointer(textPtr)))
}

func appendSeparator(hMenu uintptr) {
	procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
}

// icoDirEntry mirrors one ICONDIRENTRY record (16 bytes) inside a
// Windows .ico file's directory (docs/menurc/resource-file-formats).
type icoDirEntry struct {
	Width       byte
	Height      byte
	ColorCount  byte
	Reserved    byte
	Planes      uint16
	BitCount    uint16
	BytesInRes  uint32
	ImageOffset uint32
}

// loadIconFromICOBytes parses an in-memory .ico file's own ICONDIR
// header, picks the frame closest to a small tray-appropriate size
// itself (rather than through LookupIconIdFromDirectoryEx - tried
// first, but its return value did not behave as documented against a
// real single-frame PNG icon in practice, so frame selection is done
// directly here instead, where it is simple and fully verifiable),
// then builds a real HICON from that frame's own bytes via
// CreateIconFromResourceEx - the documented pattern for loading an
// icon that was never baked into the .exe as a Win32 resource (this
// one arrives via go:embed instead).
func loadIconFromICOBytes(data []byte) (syscall.Handle, error) {
	if len(data) < 6 {
		return 0, fmt.Errorf("ico data too short")
	}
	count := int(data[4]) | int(data[5])<<8
	if count < 1 || len(data) < 6+count*16 {
		return 0, fmt.Errorf("ico directory truncated")
	}

	entries := make([]icoDirEntry, count)
	for i := 0; i < count; i++ {
		off := 6 + i*16
		e := &entries[i]
		e.Width = data[off]
		e.Height = data[off+1]
		e.ColorCount = data[off+2]
		e.Reserved = data[off+3]
		e.Planes = uint16(data[off+4]) | uint16(data[off+5])<<8
		e.BitCount = uint16(data[off+6]) | uint16(data[off+7])<<8
		e.BytesInRes = uint32(data[off+8]) | uint32(data[off+9])<<8 | uint32(data[off+10])<<16 | uint32(data[off+11])<<24
		e.ImageOffset = uint32(data[off+12]) | uint32(data[off+13])<<8 | uint32(data[off+14])<<16 | uint32(data[off+15])<<24
	}

	const smallIconSize = 16
	idx := selectClosestFrame(entries, smallIconSize)

	e := entries[idx]
	if uint64(e.ImageOffset)+uint64(e.BytesInRes) > uint64(len(data)) {
		return 0, fmt.Errorf("ico frame %d bytes out of range", idx)
	}
	frame := data[e.ImageOffset : e.ImageOffset+e.BytesInRes]

	hIcon, _, callErr := procCreateIconFromResourceEx.Call(
		uintptr(unsafe.Pointer(&frame[0])), uintptr(len(frame)),
		1,          // fIcon = TRUE
		0x00030000, // dwVer, per the documented required value
		smallIconSize, smallIconSize,
		lrDefaultColor,
	)
	if hIcon == 0 {
		return 0, fmt.Errorf("CreateIconFromResourceEx failed: %w", callErr)
	}
	return syscall.Handle(hIcon), nil
}

// selectClosestFrame returns the index of entries' own frame whose
// (square) size is closest to target - an ICONDIRENTRY's Width/Height
// of 0 conventionally means 256, per the documented .ico format.
func selectClosestFrame(entries []icoDirEntry, target int) int {
	best := 0
	bestDiff := -1
	for i, e := range entries {
		size := int(e.Width)
		if size == 0 {
			size = 256
		}
		diff := size - target
		if diff < 0 {
			diff = -diff
		}
		if bestDiff == -1 || diff < bestDiff {
			best, bestDiff = i, diff
		}
	}
	return best
}
