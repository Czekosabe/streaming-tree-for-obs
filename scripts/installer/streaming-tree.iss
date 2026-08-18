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
; credential store are never referenced anywhere in this script, so the
; installer/uninstaller can never touch them - only the program files this
; script explicitly lists are ever installed or removed.
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
OutputBaseFilename=StreamingTreeForOBS-Setup-{#MyAppVersion}
Compression=lzma
SolidCompression=yes
WizardStyle=modern
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

; No [Registry] entries: no auto-startup registration, no service
; installation - both explicitly out of scope (docs/windows-packaging.md
; §18).
