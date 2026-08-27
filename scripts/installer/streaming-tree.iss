; Streaming Tree for OBS - Inno Setup installer script.
;
; See docs/windows-packaging.md §12-§20 for the full architecture and the
; comparison (WiX / NSIS / Inno Setup) behind choosing Inno Setup. This
; script is invoked exclusively by scripts/build-release.ps1, which passes
; MyAppVersion/StagingDir/OutputDir as preprocessor defines - it is never
; run directly against arbitrary paths.
;
; Per-user install, no administrator elevation, a fixed AppId for stable
; upgrade identity, Start Menu integration, and normal uninstall support.
; Persistent application data (%AppData%\StreamingTree) and the OS
; credential store are never referenced by [Files]/[UninstallDelete] - an
; ordinary uninstall (the checkbox in [Code] below left unchecked, the
; default) never touches either. The one exception is the explicit,
; unchecked-by-default "also remove all my data" uninstall option
; (docs/windows-packaging.md §26): when the operator deliberately opts
; into it, [UninstallRun] below runs the application's own narrowly-
; scoped `-purge-user-data` mode - never a raw file/registry delete of
; the data directory from this script itself, since only the
; application itself knows the real OS-credential-store keys a
; destination/account/donation-source credential was stored under.
;
; UNSIGNED: no production code-signing certificate exists for this project
; yet (docs/windows-packaging.md §20). A future SignTool= directive under
; [Setup] is the prepared hook for when one does; nothing below claims or
; implies this installer or its output is signed.

#ifndef MyAppVersion
  #define MyAppVersion "0.0.0-dev"
#endif
#ifndef StagingDir
  #define StagingDir "..\..\build\release\staging"
#endif
#ifndef OutputDir
  #define OutputDir "..\..\build\release\output"
#endif

#define MyAppName "Streaming Tree for OBS"
#define MyAppPublisher "Czekosabe"
#define MyAppURL "https://github.com/Czekosabe/streaming-tree-for-obs"
#define MyAppExeName "streaming-tree-server.exe"

[Setup]
; Fixed for the lifetime of this project - generated once, never changed.
; This is what gives Inno Setup stable "this is an upgrade of the same
; application" identity across releases (docs/windows-packaging.md §14).
AppId={{C067013C-D143-49F8-9510-D078482D6DA4}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
AppUpdatesURL={#MyAppURL}
; The exact same named mutex internal/runtime/singleinstance already
; creates for the whole process lifetime (Local\ scopes it to this login
; session, matching that package's own per-user model) - applies during
; both Setup and Uninstall (jrsoftware.org/ishelp), so Inno's own native
; "please close it now" fallback prompt is now accurate for this specific
; application, rather than relying on generic CloseApplications file-lock
; detection against a hidden background process with no visible window
; for Restart Manager to message. The real fix for a smooth automatic
; upgrade is the cooperative PrepareToInstall/InitializeUninstall logic
; in [Code] below, which runs first and normally resolves this before
; AppMutex's own prompt would ever fire - this is the documented fallback
; for when that mechanism cannot reach the application at all (e.g. a
; --headless run with no tray window).
AppMutex=Local\StreamingTreeForOBS.SingleInstance
; Per-user install, no elevation - the recommended Inno Setup constant for
; exactly this (resolves to %LOCALAPPDATA%\Programs\<AppName>).
DefaultDirName={localappdata}\Programs\{#MyAppName}
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
; Real MSI-equivalent uninstall registration, real Start Menu entry, real
; Apps & Features entry - all Inno Setup defaults, not disabled.
Uninstallable=yes
OutputDir={#OutputDir}
; Stage 20C1 cross-platform artifact naming (docs/macos-packaging.md
; §21, docs/platform-support.md §16): now that a second platform (macOS)
; produces a release artifact too, every artifact name encodes product,
; version, OS, and architecture. scripts/build-release.ps1 discovers
; this filename dynamically (Get-ChildItem -Filter '*.exe') and passes
; it straight through to the release manifest - nothing else in the
; pipeline hard-codes the old "-Setup-" spelling.
OutputBaseFilename=StreamingTreeForOBS-{#MyAppVersion}-windows-amd64-setup
Compression=lzma
SolidCompression=yes
WizardStyle=modern
; The compiled Setup.exe's own icon - the one canonical icon
; (internal/runtime/tray/assets/tray.ico, itself generated from
; assets/branding/streaming-tree-logo.png by
; scripts/generate-branding-assets.go), not a second copy. The
; installed application's own Start Menu shortcut and "Apps &
; Features"/uninstall entry (UninstallDisplayIcon below) need no
; equivalent directive: both already default to {#MyAppExeName}'s own
; icon, which carries the same artwork via its embedded resource (see
; apps/server/cmd/server/README-icon.txt).
SetupIconFile=..\..\apps\server\internal\runtime\tray\assets\tray.ico
UninstallDisplayIcon={app}\{#MyAppExeName}
; No custom license-acceptance page is added here: the installer does not
; need to gate on it, since Streaming Tree's own GPL licence governs use of
; the software, not installation itself - LICENSE is still installed and
; reachable (see [Files] below and docs/windows-packaging.md §16).
;
; Future signing hook (no certificate exists yet - see docs/windows-
; packaging.md §20):
; SignTool=signtool /f "$qpath-to.pfx$q" /p "$qpassword$q" /tr http://timestamp.digicert.com /td sha256 /fd sha256 $f

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Files]
Source: "{#StagingDir}\{#MyAppExeName}"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#StagingDir}\LICENSE"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#StagingDir}\THIRD_PARTY_NOTICES.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#StagingDir}\LEGAL.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#StagingDir}\PRIVACY.md"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{group}\Uninstall {#MyAppName}"; Filename: "{uninstallexe}"

[Run]
; None: the application launches its own default-browser tab once its HTTP
; server is ready (docs/windows-packaging.md §6) - the installer does not
; separately try to launch it, avoiding a double-launch race with the
; single-instance check.

[UninstallRun]
; docs/windows-packaging.md §26: runs only when the operator explicitly
; checked the "also remove all my data" option in InitializeUninstall
; below (ShouldPurgeUserData) - never for an ordinary uninstall. Per
; Inno Setup's own documented behavior, [UninstallRun] entries execute
; "as the first step of uninstallation," i.e. before this same entry's
; own {#MyAppExeName} is removed by the normal file-removal step that
; follows - the executable this runs is still guaranteed to exist.
; Flags: waituntilterminated (the default for a Run-eligible target
; when no "postinstall"-style flag says otherwise, made explicit here
; since this step's own success is load-bearing for what happens
; next) ensures the purge has actually finished, one way or another,
; before Inno proceeds to remove program files. runascurrentuser: this
; is a per-user, unelevated install (PrivilegesRequired=lowest above) -
; the purge must run as the same user who owns the data, never
; elevated.
Filename: "{app}\{#MyAppExeName}"; Parameters: "-purge-user-data"; \
  Flags: waituntilterminated runascurrentuser; Check: ShouldPurgeUserData; \
  RunOnceId: "PurgeUserData"

; No [Registry] entries: no auto-startup registration, no service
; installation - both explicitly out of scope (docs/windows-packaging.md
; §18).

[Code]
// docs/windows-packaging.md §26 - the manual/test-upgrade and explicit
// data-purge remediation cycle. Every function below was written against
// Inno Setup's own current documentation (jrsoftware.org/ishelp), not
// guessed: PrepareToInstall's "fires before CloseApplications/AppMutex
// detection, handles application shutdown" contract, CheckForMutexes'
// exact signature, [UninstallRun]'s "first step of uninstallation"
// execution order, and CreateCustomForm's signature were each looked up
// directly before being used here.

const
  // Mirrors internal/runtime/singleinstance's own mutexName constant
  // exactly (apps/server/internal/runtime/singleinstance/
  // singleinstance_windows.go) - the one authoritative "is the real
  // application currently running" signal already used everywhere else
  // in this codebase, never re-derived a second way here.
  AppSingleInstanceMutex = 'Local\StreamingTreeForOBS.SingleInstance';

  // Mirrors internal/runtime/tray's own className/shutdownRequestMessageName
  // constants exactly (apps/server/internal/runtime/tray/tray_windows.go).
  // FindWindow locates the tray's real hidden window by its exact window
  // class (never message-only, so FindWindow can see it - see that
  // file's own doc comment on why); RegisterWindowMessageW guarantees
  // this script and the running application resolve the identical
  // OS-assigned message id from the identical string, with no shared
  // numeric constant needing to cross the Pascal Script/Go boundary.
  TrayWindowClassName = 'StreamingTreeForOBSTrayWindow';
  TrayWindowTitle = 'Streaming Tree for OBS';
  ShutdownRequestMessageName = 'StreamingTreeForOBS.RequestGracefulShutdown';

  // Bounds how long this script waits for a cooperative shutdown before
  // giving up and showing an error rather than hanging indefinitely
  // (the operator's own explicit "do not hang indefinitely" / "bounded
  // message" requirement) - generous for the real graceful-shutdown
  // sequence (cfg.ShutdownTimeout plus the MediaMTX/FFmpeg stop
  // escalations it can include, docs/windows-packaging.md), not
  // unbounded.
  GracefulShutdownTimeoutMs = 60000;
  GracefulShutdownPollMs = 500;

var
  // Set only by the custom uninstall-confirmation dialog below - read by
  // ShouldPurgeUserData, the [UninstallRun] Check: function above.
  PurgeUserDataChecked: Boolean;

function FindWindowW(lpClassName, lpWindowName: string): LongWord;
  external 'FindWindowW@user32.dll stdcall';
function PostMessageW(hWnd: LongWord; Msg: LongWord; wParam, lParam: LongWord): BOOL;
  external 'PostMessageW@user32.dll stdcall';
function RegisterWindowMessageW(lpString: string): LongWord;
  external 'RegisterWindowMessageW@user32.dll stdcall';

// RequestCooperativeShutdownIfRunning is shared by both Setup's
// PrepareToInstall and Uninstall's InitializeUninstall below - the one
// canonical "ask the real running application to shut down, then wait"
// implementation, never duplicated. Returns True when the application
// was confirmed not running, either because it already wasn't, or
// because it cooperatively stopped within the bounded wait; False only
// when it was running and did not stop in time.
function RequestCooperativeShutdownIfRunning(): Boolean;
var
  MsgId, Hwnd: LongWord;
  Waited: Integer;
begin
  if not CheckForMutexes(AppSingleInstanceMutex) then
  begin
    Result := True;
    exit;
  end;

  MsgId := RegisterWindowMessageW(ShutdownRequestMessageName);
  Hwnd := FindWindowW(TrayWindowClassName, TrayWindowTitle);
  if (MsgId <> 0) and (Hwnd <> 0) then
    PostMessageW(Hwnd, MsgId, 0, 0);
  // If the window could not be found (e.g. a --headless run has no tray
  // window at all) this deliberately still falls through to the poll
  // loop below rather than failing immediately: the mutex is re-checked
  // regardless, so a shutdown already in progress for any other reason
  // is still correctly detected.

  Waited := 0;
  while CheckForMutexes(AppSingleInstanceMutex) and (Waited < GracefulShutdownTimeoutMs) do
  begin
    Sleep(GracefulShutdownPollMs);
    Waited := Waited + GracefulShutdownPollMs;
  end;

  Result := not CheckForMutexes(AppSingleInstanceMutex);
end;

// docs/windows-packaging.md §26 / the operator's own "manual installer
// upgrade" requirement: fires before CloseApplications/AppMutex
// detection (Inno Setup's own documented contract for this function),
// so a successful cooperative shutdown here means the operator never
// sees any "please close it" prompt at all - the normal, automatic path
// this whole mechanism exists for.
function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  NeedsRestart := False;
  if RequestCooperativeShutdownIfRunning() then
    Result := ''
  else
    Result := 'Streaming Tree for OBS could not be closed automatically. ' +
      'Please open the application and use "Quit Streaming Tree" from its ' +
      'tray icon, then run this installer again.';
end;

// Lets an automated test drive the silent-uninstall purge path (there
// is no GUI checkbox to click under /VERYSILENT) without weakening the
// real operator-facing default - only while already running silently
// (see InitializeUninstall below); an ordinary silent uninstall with
// this variable unset still defaults to PurgeUserDataChecked := False,
// identical to an interactive uninstall where the operator left the
// checkbox unchecked.
//
// A real Windows CI run found the obvious approach - a custom
// /PURGEUSERDATA command-line switch, checked via ParamStr/ParamCount
// - does not survive Inno's own uninstaller: because a running process
// cannot delete its own .exe, Uninstall.exe copies itself to a TEMP
// file and relaunches that copy to do the actual removal work (a real,
// documented Inno Setup mechanism) - and that relaunch reconstructs
// the child's command line using only the switches Inno itself
// recognizes (/VERYSILENT etc. demonstrably survive it; the app's own
// unstandardized custom switch was silently dropped, confirmed by the
// purge step never running despite the uninstaller itself reporting
// success). GetEnv (Inno's own documented Pascal Script function)
// reads an environment variable instead - inherited through ordinary
// Windows child-process creation regardless of which specific
// switches Inno's own relaunch logic understands, since environment
// inheritance is a plain OS-level default Inno has no reason to
// override.
function ShouldPurgeUserDataForTest(): Boolean;
begin
  Result := GetEnv('STREAMING_TREE_TEST_PURGE_USER_DATA') = '1';
end;

// Replaces Inno's default Yes/No uninstall confirmation with a custom
// dialog carrying the explicit, unchecked-by-default data-removal
// option (docs/windows-packaging.md §26) - built with CreateCustomForm
// per Inno's own documented recommendation ("call this instead of
// creating TForm/TSetupForm instances directly"). Also performs the
// same cooperative-shutdown request PrepareToInstall does above, since
// AppMutex applies during Uninstall too and the operator should not
// need Task Manager here either - and the purge step later in
// [UninstallRun] requires the application to already be stopped.
//
// UninstallSilent() (Inno's own documented function for exactly this
// context - distinct from Setup-only WizardSilent()) guards the modal
// dialog itself: scripts/verify-installer.mjs's own automated silent
// uninstall (/VERYSILENT, docs/windows-packaging.md §23) must never
// block on a form nobody can interact with, and PurgeUserDataChecked
// defaults to False (the unchecked-by-default box's own value) so a
// silent uninstall behaves exactly like an interactive one where the
// operator left the checkbox alone - the operator's own "default:
// remove the application ONLY" contract holds either way. The
// cooperative-shutdown request below is NOT silent-gated: it must
// still run under an automated /VERYSILENT test that starts the
// application running first (docs/windows-packaging.md §26's own
// integration test), and is a safe no-op whenever the application is
// already stopped (the common case, including today's existing
// verify-installer.mjs flow).
function InitializeUninstall(): Boolean;
var
  Form: TSetupForm;
  Message: TNewStaticText;
  PurgeCheck: TNewCheckBox;
  Warning: TNewStaticText;
  BtnUninstall, BtnCancel: TNewButton;
begin
  if UninstallSilent() then
  begin
    PurgeUserDataChecked := ShouldPurgeUserDataForTest();
  end else begin
  Form := CreateCustomForm(ScaleX(420), ScaleY(220), False, False);
  try
    Form.Caption := 'Uninstall ' + '{#MyAppName}';

    Message := TNewStaticText.Create(Form);
    Message.Parent := Form;
    Message.Left := ScaleX(8);
    Message.Top := ScaleY(8);
    Message.Width := Form.ClientWidth - ScaleX(16);
    Message.AutoSize := False;
    Message.WordWrap := True;
    Message.Caption :=
      'Do you want to uninstall {#MyAppName}?' + #13#10 +
      'Your destinations, connected accounts, and saved credentials are kept by default.';

    PurgeCheck := TNewCheckBox.Create(Form);
    PurgeCheck.Parent := Form;
    PurgeCheck.Left := ScaleX(8);
    PurgeCheck.Top := ScaleY(64);
    PurgeCheck.Width := Form.ClientWidth - ScaleX(16);
    PurgeCheck.Caption := 'Also remove all Streaming Tree settings, local data, and saved credentials';
    PurgeCheck.Checked := False; // Unchecked by default - the operator's own explicit requirement.

    Warning := TNewStaticText.Create(Form);
    Warning.Parent := Form;
    Warning.Left := ScaleX(8);
    Warning.Top := ScaleY(92);
    Warning.Width := Form.ClientWidth - ScaleX(16);
    Warning.AutoSize := False;
    Warning.WordWrap := True;
    Warning.Caption := 'This cannot be undone.';

    BtnCancel := TNewButton.Create(Form);
    BtnCancel.Parent := Form;
    BtnCancel.Width := ScaleX(75);
    BtnCancel.Height := ScaleY(23);
    BtnCancel.Left := Form.ClientWidth - ScaleX(8) - BtnCancel.Width;
    BtnCancel.Top := Form.ClientHeight - ScaleY(8) - BtnCancel.Height;
    BtnCancel.Caption := 'Cancel';
    BtnCancel.ModalResult := mrCancel;
    BtnCancel.Cancel := True;

    BtnUninstall := TNewButton.Create(Form);
    BtnUninstall.Parent := Form;
    BtnUninstall.Width := ScaleX(75);
    BtnUninstall.Height := ScaleY(23);
    BtnUninstall.Left := BtnCancel.Left - ScaleX(8) - BtnUninstall.Width;
    BtnUninstall.Top := BtnCancel.Top;
    BtnUninstall.Caption := 'Uninstall';
    BtnUninstall.ModalResult := mrOk;
    // The DEFAULT button (responds to a bare Enter press) always maps
    // to the checkbox's own current state, which starts unchecked -
    // an accidental Enter press can therefore never select destructive
    // deletion on its own; the operator must have deliberately checked
    // the box first (the operator's own explicit requirement).
    BtnUninstall.Default := True;

    if Form.ShowModal() <> mrOk then
    begin
      Result := False;
      exit;
    end;
    PurgeUserDataChecked := PurgeCheck.Checked;
    finally
      Form.Free;
    end;
  end;

  if not RequestCooperativeShutdownIfRunning() then
  begin
    MsgBox('Streaming Tree for OBS could not be closed automatically. ' +
      'Please open the application and use "Quit Streaming Tree" from its ' +
      'tray icon, then run this uninstaller again.', mbError, MB_OK);
    Result := False;
    exit;
  end;

  Result := True;
end;

// [UninstallRun]'s own Check: function above.
function ShouldPurgeUserData(): Boolean;
begin
  Result := PurgeUserDataChecked;
end;
