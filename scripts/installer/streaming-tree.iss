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
; Deliberately NO AppMutex directive. It was tried (mirroring
; internal/runtime/singleinstance's own mutex) on the assumption -
; based on a documentation read that turned out to be wrong or
; incomplete - that PrepareToInstall's own cooperative-shutdown logic
; in [Code] below would run first, with AppMutex's native "please
; close it" prompt only as a fallback. A real captured Windows CI
; /LOG proved otherwise: AppMutex is checked at Setup's own very early
; startup - before PrepareToInstall, which is tied to a later wizard
; page - so its native prompt ("Setup has detected that ... is
; currently running ... click OK to continue, or Cancel to exit")
; fired and, under /SUPPRESSMSGBOXES, defaulted to Cancel (Got EAbort
; exception), aborting Setup before the cooperative-shutdown mechanism
; ever got a chance to run at all. That mechanism (CheckForMutexes
; against the exact same mutex, inside RequestCooperativeShutdownIfRunning)
; already detects a running instance itself and actually attempts to
; resolve it, rather than only prompting - AppMutex added no benefit
; and actively defeated it by firing first.
; Per-user install, no elevation - the recommended Inno Setup constant for
; exactly this (resolves to %LOCALAPPDATA%\Programs\<AppName>).
DefaultDirName={localappdata}\Programs\{#MyAppName}
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes
; Deliberately no PrivilegesRequiredOverridesAllowed. A real local test
; against Inno Setup 6.7.3 (docs/windows-packaging.md §28) proved that
; PrivilegesRequiredOverridesAllowed=dialog - present since this file's
; very first commit, with no comment ever explaining why - makes Setup
; append /ALLUSERS to its own internal re-launch and install in
; administrative mode (HKEY_LOCAL_MACHINE) whenever the current account
; is a member of the Administrators group and no dialog can be shown to
; ask (i.e. every silent/automated invocation, and even an interactive
; run unless the operator notices and deliberately picks "install for
; just me") - silently contradicting this project's own absolute,
; repeatedly documented "per-user, no elevation, ever" contract for a
; very common class of real Windows account. PrivilegesRequired=lowest
; alone (no overrides allowed) is Inno's own documented guarantee of
; always non administrative install mode, confirmed by the identical
; real test with this line removed.
PrivilegesRequired=lowest
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

; A prior round kept the Start Menu shortcut as a mandatory, unconditional
; [Icons] line, reasoning that a persistently-running local application
; warranted it. On review, that is a preference, not a concrete product/
; Windows requirement, so both shortcuts are now standard, independent
; task choices, matching normal Windows installer convention: Start Menu
; selected by default, desktop unselected by default. Neither choice
; affects the application's own Apps & Features/uninstall registration
; (docs/windows-packaging.md §14) - the app remains fully discoverable
; and uninstallable either way. Both choices persist across an update via
; Inno's own native UsePreviousTasks (default "yes", confirmed against
; jrsoftware.org/ishelp/topic_setup_useprevioustasks.htm - "it will use
; the task settings of the previous installation as the default settings
; presented to the user in the wizard") - no custom [Code] needed for
; this; a prior round's own RegisterPreviousData/GetPreviousData
; reimplementation of exactly this native behavior has been removed.
[Tasks]
Name: "startmenuicon"; Description: "Create a &Start Menu shortcut"; GroupDescription: "Shortcuts:"
Name: "desktopicon"; Description: "Create a &desktop shortcut"; GroupDescription: "Shortcuts:"; Flags: unchecked

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: startmenuicon
Name: "{group}\Uninstall {#MyAppName}"; Filename: "{uninstallexe}"; Tasks: startmenuicon
; {userdesktop} always resolves to the real current user's Desktop, even
; under a custom {app} install path (docs/windows-packaging.md §1/§8) -
; Inno's own uninstaller removes an [Icons]-declared shortcut
; automatically, so no [UninstallDelete] entry is needed for it.
Name: "{userdesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Run]
; postinstall: only ever shown/offered on the final wizard page, never
; during a silent run's own non-existent UI. skipifsilent (Inno's own
; documented flag for exactly this) makes Setup skip actually executing
; this entry under /SILENT or /VERYSILENT - which is also what the
; built-in updater's own silent invocation (internal/updater/
; helper_windows.go) always uses - so the silent updater path never
; launches a duplicate process here, natively, with no custom [Code]
; needed (docs/windows-packaging.md §9/§15). nowait: Setup's own wizard
; does not block waiting for the launched application to exit. This does
; not race the application's own default-browser self-launch
; (docs/windows-packaging.md §6): that happens once its own HTTP
; listener is ready, same as any other launch of the installed exe.
Filename: "{app}\{#MyAppExeName}"; Description: "Launch {#MyAppName}"; Flags: postinstall skipifsilent nowait

; No [UninstallRun] section: four real Windows CI attempts (docs/
; progress.md) to run the purge helper declaratively through
; [UninstallRun]'s own Check: mechanism each failed the same way -
; ShouldPurgeUserData's own explicit Log() call never fired at all,
; across a command-line-switch attempt, an environment-variable
; attempt, and a RunOnceId removal, with no conclusive documented
; explanation found for why Inno never even evaluated the Check
; function. Rather than continue guessing at that declarative
; mechanism, InitializeUninstall below now calls the purge helper
; directly via Pascal Script's own documented Exec() function - a
; simple, synchronous, imperative call inside the exact function
; already confirmed (via real captured Log() ground truth) to run
; correctly with the correct PurgeUserDataChecked value, removing the
; entire cross-mechanism question.

; No [Registry] entries: no auto-startup registration, no service
; installation - both explicitly out of scope (docs/windows-packaging.md
; §18).

[Code]
// docs/windows-packaging.md §26 - the manual/test-upgrade and explicit
// data-purge remediation cycle. Every function below was written against
// Inno Setup's own current documentation (jrsoftware.org/ishelp) and,
// where documentation proved wrong or incomplete for this project's
// actual observed behavior, against real captured Windows CI /LOG
// evidence instead: CheckForMutexes' exact signature and
// CreateCustomForm's signature came from documentation and held up;
// AppMutex's claimed firing order relative to PrepareToInstall did not
// (see AppMutex's own removal note in [Setup] above), and
// [UninstallRun]'s own Check: mechanism never fired at all across four
// separate real diagnostic rounds for a reason no documentation search
// ever explained, which is why the purge helper is now invoked
// directly via Exec() instead of through [UninstallRun].

const
  // The registry location Inno Setup itself writes real per-user
  // ("HKCU", matching PrivilegesRequired=lowest) uninstall/Apps & Features
  // metadata to - built from the exact same AppId GUID in [Setup] above,
  // never a second identity. This is what lets Setup discover a real
  // previously-installed version without any assumption about process
  // names, folder existence, or any executable other than this one's own
  // Inno-registered identity (docs/windows-packaging.md §2's own explicit
  // requirement).
  UninstallRegSubkey = 'Software\Microsoft\Windows\CurrentVersion\Uninstall\{C067013C-D143-49F8-9510-D078482D6DA4}_is1';

// A real local test against Inno Setup 6.7.3 originally found the
// uninstall entry registered under HKEY_LOCAL_MACHINE and, at the time,
// this was attributed to "admin-capable accounts get HKLM even under
// PrivilegesRequired=lowest" and worked around with an
// IsAdminInstallMode()-based root choice here. That explanation did not
// hold up: PrivilegesRequired=lowest's own official documentation
// (jrsoftware.org/ishelp/topic_setup_privilegesrequired.htm) states
// Setup "will always run in non administrative install mode" regardless
// of account type, and a corrective, evidence-based re-test (real
// Inno /LOG capture) found the true, different cause -
// PrivilegesRequiredOverridesAllowed=dialog (present since this file's
// very first commit, never actually justified in it) made Setup append
// /ALLUSERS to its own internal re-launch and genuinely install in
// administrative mode whenever the current account is an Administrators
// group member and no dialog can be shown to ask otherwise - contradicting
// this project's own absolute "per-user, no elevation, ever" contract, not
// merely a registry-detection inconvenience. That directive is now
// removed ([Setup] above) - Administrative install mode: No / Install
// mode root key: HKEY_CURRENT_USER, confirmed again by the identical real
// /LOG capture with the fix applied, on the identical admin-capable
// account that previously showed HKEY_LOCAL_MACHINE.
//
// Detection below therefore treats HKEY_CURRENT_USER as the one
// canonical root a correctly-behaving build of this installer ever
// writes to - never IsAdminInstallMode()-based selection. It still
// checks HKEY_LOCAL_MACHINE defensively, because real pre-fix local
// test/dev history on this exact project could have left a stale
// administrative-mode entry behind (this project has never published a
// public release, so there is no real external user with a legitimate
// "legacy" install to migrate - any HKLM entry found here can only be
// residue from testing this installer itself before this fix). Finding
// one is never silently resolved by picking a root: it is surfaced as an
// explicit, actionable message, and Setup refuses to proceed and create
// a second, parallel per-user registration alongside it.
var
  HkcuInstalledVersion, HklmInstalledVersion: String;
  HkcuInstalledFound, HklmInstalledFound: Boolean;

var
  // Set once in InitializeSetup, read again by UpdateReadyMemo below so
  // the Ready-to-Install summary and the fresh/update/repair/downgrade
  // decision always agree - never re-queried a second time from the
  // registry.
  DetectedPrevVersionFound: Boolean;
  DetectedPrevVersion: String;

// SplitVersion splits a version string such as "0.1.0-manualtest+a0e2fb8"
// into Core ("0.1.0") and Prerelease ("manualtest") - build metadata
// after "+" is discarded, never meaningful for ordering. Mirrors the
// version shape internal/buildinfo.go actually produces (a strict
// "MAJOR.MINOR.PATCH" release version, or "MAJOR.MINOR.PATCH-<label>+
// <commit>" for a manual/test packaged build) - Pascal Script cannot
// call that Go logic directly, so this is a narrow, purpose-built
// mirror of it, not a general-purpose semver parser.
procedure SplitVersion(const V: String; var Core, Prerelease: String);
var
  PlusPos, DashPos: Integer;
  NoBuild: String;
begin
  PlusPos := Pos('+', V);
  if PlusPos > 0 then
    NoBuild := Copy(V, 1, PlusPos - 1)
  else
    NoBuild := V;

  DashPos := Pos('-', NoBuild);
  if DashPos > 0 then
  begin
    Core := Copy(NoBuild, 1, DashPos - 1);
    Prerelease := Copy(NoBuild, DashPos + 1, Length(NoBuild) - DashPos);
  end else begin
    Core := NoBuild;
    Prerelease := '';
  end;
end;

// CompareVersionCores does exact integer comparison of two dotted-numeric
// cores ("0.1.0" vs "0.2.0"), component by component - never a
// lexicographic string comparison (docs/windows-packaging.md §13's
// explicit requirement: "10.0.0" must compare greater than "9.0.0").
function CompareVersionCores(A, B: String): Integer;
var
  APart, BPart: String;
  ADot, BDot: Integer;
  ANum, BNum: Integer;
begin
  Result := 0;
  while (Result = 0) and ((A <> '') or (B <> '')) do
  begin
    ADot := Pos('.', A);
    if ADot = 0 then begin APart := A; A := ''; end
    else begin APart := Copy(A, 1, ADot - 1); A := Copy(A, ADot + 1, Length(A) - ADot); end;

    BDot := Pos('.', B);
    if BDot = 0 then begin BPart := B; B := ''; end
    else begin BPart := Copy(B, 1, BDot - 1); B := Copy(B, BDot + 1, Length(B) - BDot); end;

    ANum := StrToIntDef(APart, 0);
    BNum := StrToIntDef(BPart, 0);

    if ANum < BNum then Result := -1
    else if ANum > BNum then Result := 1;
  end;
end;

// CompareAppVersions is the one narrow, correct installer-side version
// comparison this project needs (docs/windows-packaging.md §13): -1/0/1
// as A is older/equal/newer than B. A release version outranks a
// prerelease of the identical core (e.g. "0.1.0" > "0.1.0-manualtest+
// abc"), matching the operator's own explicit "manual-test versions must
// not accidentally be treated as Stable production updates" requirement
// stated the other direction - a manual-test build must never look newer
// than the real release it was cut from. Two versions sharing the same
// core AND the same prerelease tag (regardless of build metadata) compare
// EQUAL - two different manual-test builds of the nominal same version
// are a same-version reinstall/repair, never an "update" or "downgrade"
// against each other.
function CompareAppVersions(const A, B: String): Integer;
var
  ACore, APre, BCore, BPre: String;
begin
  SplitVersion(A, ACore, APre);
  SplitVersion(B, BCore, BPre);

  Result := CompareVersionCores(ACore, BCore);
  if Result <> 0 then
    exit;

  if (APre = '') and (BPre <> '') then
    Result := 1
  else if (APre <> '') and (BPre = '') then
    Result := -1
  else
    Result := 0;
end;

// InitializeSetup implements docs/windows-packaging.md §2/§3/§13: reads
// the real previously-installed version (if any) from this application's
// own stable AppId's registry entry, and blocks a silent downgrade
// outright while requiring an explicit interactive confirmation for one.
// Fresh installs and any update/repair (installed version <= this
// installer's version) proceed with no extra gate here - Inno's own
// DisableDirPage=auto/UsePreviousAppDir=yes defaults (both left
// unoverridden in [Setup] above, confirmed against jrsoftware.org/ishelp)
// already show the directory page on fresh install and reuse the
// existing install location silently on update, which is exactly the
// behavior docs/windows-packaging.md §1/§4 require without needing any
// custom code for that part.
function InitializeSetup(): Boolean;
begin
  Result := True;
  HkcuInstalledFound := RegQueryStringValue(HKEY_CURRENT_USER, UninstallRegSubkey, 'DisplayVersion', HkcuInstalledVersion);
  HklmInstalledFound := RegQueryStringValue(HKEY_LOCAL_MACHINE, UninstallRegSubkey, 'DisplayVersion', HklmInstalledVersion);

  if HkcuInstalledFound and HklmInstalledFound then
  begin
    if not WizardSilent() then
      MsgBox('{#MyAppName} appears to be registered both as a per-user install (version ' + HkcuInstalledVersion +
        ') and as an administrative/all-users install (version ' + HklmInstalledVersion + ').' + #13#10#13#10 +
        'This installer only supports a per-user install and cannot safely resolve this automatically. ' +
        'Please uninstall the administrative/all-users copy first (from an elevated "Apps & Features"), then run this installer again.',
        mbError, MB_OK);
    Log('Refusing to proceed: both HKCU (' + HkcuInstalledVersion + ') and HKLM (' + HklmInstalledVersion + ') uninstall entries exist for this AppId.');
    Result := False;
    exit;
  end;

  if HklmInstalledFound then
  begin
    // Never a real external user's legacy install - this project has
    // never published a public release. Only this installer's own
    // pre-fix test/dev history could have produced this. Refuse rather
    // than silently creating a second, parallel per-user registration
    // alongside it.
    if not WizardSilent() then
      MsgBox('An existing administrative/all-users installation of {#MyAppName} (version ' + HklmInstalledVersion + ') was found.' + #13#10#13#10 +
        'This installer only supports a per-user install and cannot upgrade an administrative install. ' +
        'Please uninstall it first (from an elevated "Apps & Features"), then run this installer again.',
        mbError, MB_OK);
    Log('Refusing to proceed: an administrative/all-users install (' + HklmInstalledVersion + ') was found under HKLM.');
    Result := False;
    exit;
  end;

  DetectedPrevVersionFound := HkcuInstalledFound;
  DetectedPrevVersion := HkcuInstalledVersion;
  if not DetectedPrevVersionFound then
    exit; // Fresh install - nothing to compare against.

  if CompareAppVersions(DetectedPrevVersion, '{#MyAppVersion}') <= 0 then
    exit; // Installed version is older than or equal to this installer - update or repair, always allowed.

  // Installed version is newer than this installer - a real downgrade.
  if WizardSilent() then
  begin
    Log('Refusing silent downgrade: installed version ' + DetectedPrevVersion +
      ' is newer than installer version {#MyAppVersion}.');
    Result := False;
    exit;
  end;

  if MsgBox(
    'A newer version of {#MyAppName} (' + DetectedPrevVersion + ') is already installed.' + #13#10 +
    'This installer is for version {#MyAppVersion}, which is older than what is currently installed.' + #13#10#13#10 +
    'Installing it anyway will downgrade your installation. Continue anyway?',
    mbConfirmation, MB_YESNO) = IDNO then
    Result := False;
end;

// UpdateReadyMemo adds the version-context summary docs/windows-
// packaging.md §13 asks for ("Installed version: X / Installer version:
// Y / Operation: Z") to the Ready-to-Install page, ahead of Inno's own
// standard directory/group/tasks summary lines - shown only
// interactively (the Ready page itself is skipped entirely under
// /SILENT and /VERYSILENT, so this never affects a silent run).
function UpdateReadyMemo(Space, NewLine, MemoUserInfoInfo, MemoDirInfo, MemoTypeInfo,
  MemoComponentsInfo, MemoGroupInfo, MemoTasksInfo: String): String;
var
  OperationLine: String;
  Cmp: Integer;
begin
  if DetectedPrevVersionFound then
  begin
    Cmp := CompareAppVersions(DetectedPrevVersion, '{#MyAppVersion}');
    if Cmp = 0 then
      OperationLine := 'Repair / reinstall (same version already installed)'
    else if Cmp < 0 then
      OperationLine := 'Update'
    else
      OperationLine := 'Downgrade';
    Result := 'Installed version: ' + DetectedPrevVersion + NewLine +
      'Installer version: {#MyAppVersion}' + NewLine +
      'Operation: ' + OperationLine + NewLine + NewLine;
  end else begin
    Result := 'Installer version: {#MyAppVersion}' + NewLine +
      'Operation: Fresh install' + NewLine + NewLine;
  end;

  Result := Result + MemoDirInfo + NewLine + NewLine + MemoGroupInfo + NewLine + NewLine + MemoTasksInfo;
end;

// Task-choice persistence across an update is handled entirely by
// Inno's own native UsePreviousTasks (default "yes", never overridden
// in [Setup] above) - confirmed against jrsoftware.org/ishelp/
// topic_setup_useprevioustasks.htm to already do exactly what a prior
// round's own RegisterPreviousData/GetPreviousData/InitializeWizard
// reimplementation did by hand. That custom code has been removed as
// redundant; no [Code] is needed here for this requirement.

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
  // Set only by the custom uninstall-confirmation dialog below (or the
  // silent/test path) - read directly by InitializeUninstall itself
  // once RequestCooperativeShutdownIfRunning has confirmed the
  // application is stopped, via a direct Exec() call rather than
  // Inno's own [UninstallRun]/Check: mechanism (see that section's own
  // removal note in this script for why).
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
// upgrade" requirement: fires before Inno's own CloseApplications
// detection (Inno Setup's own documented contract for this function -
// AppMutex is a separate, earlier check, deliberately not used here;
// see its own removal note in [Setup] above), so a successful
// cooperative shutdown here means the operator never sees any "please
// close it" prompt at all - the normal, automatic path this whole
// mechanism exists for.
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
// checkbox unchecked. A real Windows CI run found a custom command-
// line switch does not survive Inno's own uninstaller-relaunch-from-
// TEMP mechanism (a running process cannot delete its own .exe, so
// Uninstall.exe copies itself to TEMP and relaunches that copy - see
// PurgeUserDataEnvVar's own doc comment for the fuller two-phase
// picture this was only half of); GetEnv reads this test-only
// environment variable instead.
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
// the operator should not need Task Manager here either - and the
// purge Exec() call below requires the application to already be
// stopped.
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
  PurgeExitCode: Integer;
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
    // MsgBox is never auto-suppressed by /SUPPRESSMSGBOXES (unlike
    // Inno's own built-in prompts) - a real, documented gotcha for
    // custom [Code] dialogs. Guarded here so an automated silent test
    // exercising this failure path can never hang waiting for a click
    // nobody will make; the interactive operator still sees it.
    if not UninstallSilent() then
      MsgBox('Streaming Tree for OBS could not be closed automatically. ' +
        'Please open the application and use "Quit Streaming Tree" from its ' +
        'tray icon, then run this uninstaller again.', mbError, MB_OK);
    Result := False;
    exit;
  end;

  // Runs the purge helper directly via Exec() rather than through
  // Inno's own [UninstallRun]/Check: mechanism - see this script's own
  // removal note where that section used to be for why. {app} is still
  // valid here: nothing has removed any files yet at this point in
  // InitializeUninstall. 0 = SW_HIDE (no window for the purge
  // helper's own console-free process to show anyway).
  if PurgeUserDataChecked then
  begin
    if not Exec(ExpandConstant('{app}\{#MyAppExeName}'), '-purge-user-data', '',
      0, ewWaitUntilTerminated, PurgeExitCode) then
    begin
      if not UninstallSilent() then
        MsgBox('Streaming Tree for OBS could not remove its saved data: the purge helper ' +
          'could not be started. The application itself will still be uninstalled.', mbError, MB_OK);
    end else if PurgeExitCode <> 0 then
    begin
      if not UninstallSilent() then
        MsgBox('Streaming Tree for OBS could not fully remove its saved data (exit code ' +
          IntToStr(PurgeExitCode) + '). The application itself will still be uninstalled.', mbError, MB_OK);
    end;
  end;

  Result := True;
end;
