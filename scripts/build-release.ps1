<#
.SYNOPSIS
    Builds a local Stage 20A Windows release: the production frontend
    embedded into a GUI-subsystem Go executable, staged with the four
    mandatory legal documents, and packaged by Inno Setup into a single
    installer with a SHA-256 digest. When Version is a strict
    major.minor.patch release version, also generates the Stage 20B
    release manifest (docs/updater.md §5/§39) from the real installer.

.DESCRIPTION
    See docs/windows-packaging.md for the full architecture this script
    implements. This produces LOCAL build artifacts only:

      - it does not publish anything to GitHub;
      - it does not create a Git tag;
      - it does not create a GitHub Release;
      - it does not sign anything (no production code-signing certificate
        exists for this project yet - see docs/windows-packaging.md §20).

    A development/test package version (e.g. "0.1.0-dev+abc1234") is
    expected, never a real "1.0.0" public release - this is packaging
    infrastructure, not the public 1.0 release. A dev-suffixed version
    never gets a release manifest (docs/updater.md §4's strict version
    format does not accept one) - this is expected, not an error.

.PARAMETER Version
    The application version to inject (internal/buildinfo.releaseVersion).
    Required. Must be non-empty and contain only characters safe for a
    Windows file/product-version field (letters, digits, '.', '-', '+').

.PARAMETER SkipInstaller
    Builds the release executable and staging directory but does not
    invoke the Inno Setup compiler. Useful when only the packaged-runtime
    integration script (scripts/verify-packaged-app.mjs) is needed, since
    that script exercises the staged executable directly, not the
    installer.

.PARAMETER TestAppId
    TEST-ONLY. Overrides the compiled installer's Inno Setup AppId via
    scripts/installer/streaming-tree.iss's own `#ifndef TestAppId` /
    TestAppIdBare mechanism - the exact same mechanism
    scripts/verify-installer.mjs already uses directly against ISCC.exe
    (see that script's own compileTestInstaller). Must be a well-formed
    GUID in braces, e.g. "{DEADBEEF-DEAD-BEEF-DEAD-BEEFDEADBEEF}" -
    anything else fails validation before Inno Setup ever runs.

    Never set by a real release build. Omitting this parameter (the
    default, empty string) leaves the installer's own compiled-in
    default completely untouched - the real, stable production AppId
    (docs/windows-packaging.md §14) - so ordinary invocation of this
    script is byte-for-byte unaffected by this parameter's existence.
    Exists so scripts/verify-updater.mjs (docs/updater.md §41) can build
    its own real, full release pair under a dedicated throwaway identity
    instead of the real production one - a real local-execution hazard
    fixed by this parameter (docs/windows-packaging.md §33).

.EXAMPLE
    powershell -File scripts/build-release.ps1 -Version "0.1.0-dev+local"
#>

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$Version,

    [switch]$SkipInstaller,

    # Builds with `-tags integration`, so the resulting binary is
    # otherwise-identical to a real production release EXCEPT that the
    # updater's GitHub API base URL can be redirected via the
    # STREAMING_TREE_TEST_UPDATE_API_BASE_URL environment variable (see
    # cmd/server/updater_testhook_integration.go). Used only by
    # scripts/verify-updater.mjs (docs/updater.md §41) - never set for a
    # real release build, and never the default.
    [switch]$IntegrationTest,

    # See .PARAMETER TestAppId above. Left as a plain string (not
    # validated by a [ValidatePattern] attribute) so an invalid value
    # produces this script's own clear Fail() message rather than a
    # generic PowerShell parameter-binding error.
    [string]$TestAppId
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Write-Step {
    param([string]$Message)
    Write-Host "==> $Message" -ForegroundColor Cyan
}

function Fail {
    param([string]$Message)
    Write-Host "FAILED: $Message" -ForegroundColor Red
    exit 1
}

# --- 0. Resolve fixed, repository-relative paths ---------------------------
# Never a caller-supplied arbitrary path: every path below is derived from
# this script's own known location, so nothing outside the repository is
# ever touched or deleted.
$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot '..')
$WebDir = Join-Path $RepoRoot 'apps/web'
$ServerDir = Join-Path $RepoRoot 'apps/server'
$EmbeddedFrontendDir = Join-Path $ServerDir 'internal/webassets/embedded'
$EmbeddedLegalDir = Join-Path $ServerDir 'internal/webassets/legal'
$StagingDir = Join-Path $RepoRoot 'build/release/staging'
$OutputDir = Join-Path $RepoRoot 'build/release/output'
$InnoScript = Join-Path $RepoRoot 'scripts/installer/streaming-tree.iss'

# --- 1. Validate the requested version -------------------------------------
Write-Step "Validating requested version '$Version'"
if ($Version -notmatch '^[A-Za-z0-9][A-Za-z0-9.+-]*$') {
    Fail "Version '$Version' contains characters unsafe for a Windows file/product-version field. Allowed: letters, digits, '.', '-', '+'."
}
if ($Version -eq '1.0.0') {
    Fail "Refusing to build version 1.0.0: Stage 20A is packaging infrastructure, not the public 1.0 release. Use an explicit development/test version such as '0.1.0-dev'."
}

# --- 2. Validate required tools ---------------------------------------------
Write-Step 'Validating required build tools'

function Find-RequiredCommand {
    param([string]$Name)
    $cmd = Get-Command $Name -ErrorAction SilentlyContinue
    if (-not $cmd) {
        Fail "Required tool '$Name' was not found on PATH."
    }
    return $cmd.Source
}

$GoPath = Find-RequiredCommand 'go'
$NpmPath = Find-RequiredCommand 'npm'
Write-Host "  go:  $GoPath"
Write-Host "  npm: $NpmPath"

$IsccPath = $null
if (-not $SkipInstaller) {
    $IsccCandidates = @(
        (Join-Path $env:LOCALAPPDATA 'Programs\Inno Setup 6\ISCC.exe'),
        'C:\Program Files (x86)\Inno Setup 6\ISCC.exe',
        'C:\Program Files\Inno Setup 6\ISCC.exe'
    )
    $IsccPath = $IsccCandidates | Where-Object { Test-Path $_ } | Select-Object -First 1
    if (-not $IsccPath) {
        $isccCmd = Get-Command ISCC -ErrorAction SilentlyContinue
        if ($isccCmd) { $IsccPath = $isccCmd.Source }
    }
    if (-not $IsccPath) {
        Fail "Inno Setup's ISCC.exe was not found. Install it (e.g. 'winget install --id JRSoftware.InnoSetup --scope user') or pass -SkipInstaller to build only the executable."
    }
    Write-Host "  ISCC: $IsccPath"
}

# --- 3. Ensure frontend dependencies match the lockfile ---------------------
Write-Step 'Installing frontend dependencies (npm ci)'
Push-Location $WebDir
try {
    npm ci
    if ($LASTEXITCODE -ne 0) { Fail 'npm ci failed.' }
}
finally {
    Pop-Location
}

# --- 4. Run the frontend production build -----------------------------------
Write-Step 'Building the production frontend (npm run build)'
Push-Location $WebDir
try {
    npm run build
    if ($LASTEXITCODE -ne 0) { Fail 'Frontend production build failed.' }
}
finally {
    Pop-Location
}

$DistDir = Join-Path $WebDir 'dist'
if (-not (Test-Path (Join-Path $DistDir 'index.html'))) {
    Fail "Frontend build did not produce $DistDir/index.html."
}

# --- 5. Stage the embedded frontend/legal directories -----------------------
# Bounded, safe cleanup: only ever removes files *inside* these two fixed,
# known subdirectories - never a caller-supplied path, never anything above
# the repository. The tracked .gitkeep placeholder is intentionally
# overwritten along with everything else; it returns on the next `git
# checkout`/clean clone, which is exactly the point of it being a
# placeholder rather than real content.
Write-Step 'Staging the embedded production frontend'
Get-ChildItem -Path $EmbeddedFrontendDir -Force | Remove-Item -Recurse -Force
Copy-Item -Path (Join-Path $DistDir '*') -Destination $EmbeddedFrontendDir -Recurse -Force

Write-Step 'Staging the embedded legal documents'
Get-ChildItem -Path $EmbeddedLegalDir -Force | Remove-Item -Recurse -Force
foreach ($doc in @('LICENSE', 'THIRD_PARTY_NOTICES.md', 'LEGAL.md', 'PRIVACY.md')) {
    $source = Join-Path $RepoRoot $doc
    if (-not (Test-Path $source)) {
        Fail "Required legal document '$doc' is missing from the repository root."
    }
    Copy-Item -Path $source -Destination $EmbeddedLegalDir -Force
}

# --- 6. Build the Windows release executable --------------------------------
Write-Step "Building the Windows release executable (version $Version)"

$CommitHash = (& git -C $RepoRoot rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($CommitHash)) {
    Fail 'Could not resolve the current Git commit.'
}

New-Item -ItemType Directory -Force -Path $StagingDir | Out-Null
Get-ChildItem -Path $StagingDir -Force | Remove-Item -Recurse -Force

$ExePath = Join-Path $StagingDir 'streaming-tree-server.exe'
$LdflagsPkg = 'github.com/streaming-tree/server/internal/buildinfo'
$Ldflags = "-H=windowsgui -X $LdflagsPkg.releaseVersion=$Version -X $LdflagsPkg.releaseCommit=$CommitHash -X $LdflagsPkg.packagedFlag=true"

Push-Location $ServerDir
try {
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    if ($IntegrationTest) {
        go build -tags integration -ldflags $Ldflags -o $ExePath ./cmd/server
    }
    else {
        go build -ldflags $Ldflags -o $ExePath ./cmd/server
    }
    if ($LASTEXITCODE -ne 0) { Fail 'go build of the release executable failed.' }
}
finally {
    Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
    Pop-Location
}

if (-not (Test-Path $ExePath)) {
    Fail "Expected release executable was not produced at $ExePath."
}

# --- 7. Also stage loose copies of the four mandatory legal documents -------
# Redundant with the embedded copies served at /legal/* (both exist so the
# requirement is satisfied even if a future distribution format ever drops
# the HTTP routes) - installed beside the executable, not merged into it.
foreach ($doc in @('LICENSE', 'THIRD_PARTY_NOTICES.md', 'LEGAL.md', 'PRIVACY.md')) {
    Copy-Item -Path (Join-Path $RepoRoot $doc) -Destination $StagingDir -Force
}

Write-Host "Staged release build at: $StagingDir"

# --- 8. Invoke the Inno Setup compiler --------------------------------------
if ($SkipInstaller) {
    Write-Step 'SkipInstaller set - not invoking Inno Setup'
    exit 0
}

Write-Step 'Building the Windows installer (Inno Setup)'
New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
Get-ChildItem -Path $OutputDir -Force -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force

$IsccArgs = @(
    "/DMyAppVersion=$Version",
    "/DStagingDir=$StagingDir",
    "/DOutputDir=$OutputDir"
)

# TEST-ONLY override, never present for a real release build - see
# .PARAMETER TestAppId above. $TestAppId defaults to an empty string, so
# this block - and therefore the ISCC define it adds below - is
# structurally absent from an ordinary invocation, not merely unused.
if ($TestAppId) {
    if ($TestAppId -notmatch '^\{[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}\}$') {
        Fail "TestAppId '$TestAppId' is not a well-formed GUID in braces, e.g. '{DEADBEEF-DEAD-BEEF-DEAD-BEEFDEADBEEF}'."
    }
    # Inno Setup Preprocessor escaping: a literal '{' must be written as
    # '{{' - mirrors scripts/verify-installer.mjs's own compileTestInstaller.
    $EscapedTestAppId = $TestAppId -replace '^\{', '{{'
    $IsccArgs += "/DTestAppId=$EscapedTestAppId"
    Write-Host "  TestAppId:  $TestAppId (TEST-ONLY override - never used for a real release build)" -ForegroundColor Yellow
}

$IsccArgs += $InnoScript

& $IsccPath @IsccArgs
if ($LASTEXITCODE -ne 0) { Fail 'Inno Setup compilation failed.' }

$Installer = Get-ChildItem -Path $OutputDir -Filter '*.exe' | Select-Object -First 1
if (-not $Installer) {
    Fail "Inno Setup reported success but no installer was found in $OutputDir."
}

# --- 9. Generate a SHA-256 digest --------------------------------------------
# .NET's own SHA256 type directly, not the Get-FileHash cmdlet: a real
# Stage 20E CI investigation found Get-FileHash unavailable
# ("CommandNotFoundException") when this script runs nested three
# processes deep (a GitHub Actions pwsh step launching node, which
# launches this script's own powershell.exe) - the built-in
# Microsoft.PowerShell.Utility module's auto-load did not resolve in
# that specific nesting, most plausibly a PSModulePath inheritance gap
# across the pwsh/Windows PowerShell boundary. The .NET types below
# need no module resolution of any kind, on any PowerShell version,
# nested or not.
Write-Step 'Generating the SHA-256 digest'
$Sha256Algorithm = [System.Security.Cryptography.SHA256]::Create()
try {
    $FileStream = [System.IO.File]::OpenRead($Installer.FullName)
    try {
        $HashBytes = $Sha256Algorithm.ComputeHash($FileStream)
    }
    finally {
        $FileStream.Dispose()
    }
}
finally {
    $Sha256Algorithm.Dispose()
}
$HashHex = ([System.BitConverter]::ToString($HashBytes) -replace '-', '').ToLower()
$HashFile = "$($Installer.FullName).sha256"
"$HashHex  $($Installer.Name)" | Out-File -FilePath $HashFile -Encoding ascii -NoNewline

# --- 10. Generate the Stage 20B release manifest ----------------------------
# Only attempted for a strict "major.minor.patch" version - see
# docs/updater.md §4/§5's own strict version format, which a "-dev+..."
# local/test build (this script's own everyday use, per its own doc comment
# above) never matches. This is not a weaker guarantee: a version shaped
# like a real release always gets a manifest, generated from the real
# installer file and self-validated by the exact same validator the
# runtime updater uses (docs/updater.md §39) - the pipeline fails loudly
# only when it could have produced a manifest but produced an invalid one.
$ManifestPath = Join-Path $OutputDir 'streaming-tree-release.json'
if ($Version -match '^\d+\.\d+\.\d+$') {
    Write-Step 'Generating the release manifest'
    Push-Location $ServerDir
    try {
        go run ./cmd/releasemanifest `
            -version $Version `
            -artifact $Installer.FullName `
            -artifact-name $Installer.Name `
            -os windows -arch amd64 -kind installer `
            -out $ManifestPath
        if ($LASTEXITCODE -ne 0) { Fail 'Release manifest generation failed.' }
    }
    finally {
        Pop-Location
    }
}
else {
    Write-Step "Skipping release manifest: '$Version' is not a strict major.minor.patch release version"
    $ManifestPath = $null
}

Write-Host ''
Write-Host 'Release build complete (UNSIGNED - see docs/windows-packaging.md §20).' -ForegroundColor Green
Write-Host "  Executable: $ExePath"
Write-Host "  Installer:  $($Installer.FullName)"
Write-Host "  SHA-256:    $HashFile"
if ($null -ne $ManifestPath) {
    Write-Host "  Manifest:   $ManifestPath"
}
