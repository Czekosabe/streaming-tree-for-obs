# Stage 20E — manual/physical verification checklist

This document is the real, ID-based checklist governing task §29
requires, written before any operator is asked to run anything (§37).
It exists because native CI proves the software builds, packages, and
passes its own automated assertions on a real OS/architecture — it
does not prove a human can actually install the app, connect OBS,
watch a browser overlay render, or use the updater UI. Only a physical
human pass on real hardware proves that, and this document is what
that pass follows.

## How to use this checklist

- Every item has a stable ID (`<session>-<number>`), a precondition,
  an exact action to take, and an expected visible result. Do the
  action, compare to the expected result, and record **PASS** or
  **FAIL** plus a short, sanitized note (see the secret rule below).
- Work through one session at a time. A session you cannot physically
  run (no Mac hardware, no second machine for remote-RTMPS, etc.) is
  recorded as **Not verified — environment unavailable**, never as a
  PASS and never skipped silently — see `docs/final-hardening.md` §M/§N
  for why this distinction matters for platform-support claims.
- Do every item in a session in order; a later item in the same
  session often assumes an earlier one already left the app in a
  known state (e.g. "OBS is connected" before "start an outgoing
  branch").
- Report results back using each item's ID (e.g. "A-3: PASS", "D-2:
  FAIL — branch stayed in `waitingForIngest` for 2 minutes after OBS
  connected"). This lets a failure be traced to the exact commit under
  test and re-tested individually once fixed, without repeating every
  passing item.

## The one rule that overrides everything else in this document

**Never paste a secret into chat, ever, for any reason.** This
includes: a stream key, an OAuth access or refresh token, the admin
password, the headless master key, the RTMPS remote-ingest password,
a session cookie value, a CSRF token, a remote-overlay capability URL
(the full URL — not just naming which overlay it's for), or any TLS
private key material. Every action in this document that touches one
of these values says explicitly **"enter this only in the app/OBS/
provider UI — never paste it here"** at the point it applies. If an
item's expected result would require you to reveal a secret to
confirm, report a redacted description instead (e.g. "the field showed
a 40-character value starting with `sk_`", "OBS accepted the
credential and connected"), never the value itself. If in doubt, don't
paste it — describe it instead and ask.

---

## Getting the candidate installer onto a second test machine

Physical sessions below need the real installer/package built and
verified for the exact commit under test. It does not have to be
built locally on the machine you're testing on — every native package
workflow (`windows-package.yml`, `macos-package.yml`,
`linux-package.yml`) uploads its own already-verified package as a
short-retention GitHub Actions artifact once that run's build and
verification steps have actually passed. **This is not a GitHub
Release** — no tag, no publication, and the application's own updater
never reads it; it exists solely so a tester on a different machine
than the developer's can download the exact CI-built, CI-verified
candidate without needing a local toolchain. It may also disappear —
Actions artifacts expire after a bounded retention window (14 days as
of this writing; check the run page for the exact remaining time),
unlike a Release.

To get it:

1. Open the repository on GitHub → **Actions**.
2. Select the relevant workflow (e.g. "Windows package verification"
   for Session A).
3. Find the run for the exact commit under test (the run list shows
   the commit SHA and message) and confirm it finished green.
4. Open that run, scroll to **Artifacts** at the bottom of the run
   summary page.
5. Download the artifact named
   `StreamingTreeForOBS-<version>-manualtest-<shortsha>-<os>-<arch>`
   (e.g. `StreamingTreeForOBS-0.1.0-manualtest-7f5550f95184-windows-amd64`)
   — a `.zip` containing the real installer/package, its `.sha256`
   sidecar, and a small `BUILD-INFO.txt` recording the product,
   version, full commit SHA, OS, architecture, and unsigned status.
6. If desired, recompute the SHA-256 of the extracted installer/package
   and compare it against the `.sha256` sidecar before running it.
7. Run the installer/package as normal for that platform.

## Session A — Windows desktop packaged app

Precondition: a real, physical Windows 10/11 x64 machine (not a VM
substitute for install/uninstall unless that VM is the actual intended
deployment target). The Inno Setup installer built from the commit
under test (via `scripts/build-release.ps1`, or a locally-run
equivalent of the CI package job).

| ID | Action | Expected result |
| --- | --- | --- |
| A-1 | Run the installer, accept defaults. | Install completes without error; a Start Menu entry for "Streaming Tree for OBS" exists. |
| A-2 | Launch the app from the Start Menu entry. | The default browser opens to the app's management UI within a few seconds; no console window is left open. |
| A-3 | Look at the Dashboard. | It loads real (not placeholder) content — platform cards, health status — not a loading spinner stuck forever. |
| A-4 | Close the browser tab, then reopen `http://127.0.0.1:<port>/` (the same address shown at first launch) manually. | The app is still running and the page loads again — closing the tab does not stop the backend. |
| A-5 | Launch the app a second time (Start Menu entry again) while it's already running. | No second instance/second port bind occurs; either the existing browser tab is focused or a clear "already running" message appears — never two independent backends. |
| A-6 | Open About & Legal from the sidebar. | Shows the real version, commit, "Czekosabe" as creator, GPL-3.0-or-later, and working links to the support URL and the four legal documents. |
| A-7 | Use the app's quit/shutdown action. | The backend process actually exits (check Task Manager for `streaming-tree-server.exe`) — it does not linger. |
| A-8 | With the app still running (do not quit it first), launch a newer installer (same version or the next commit's build) over the existing install. | The installer detects the running instance and closes it cooperatively on its own — no Task Manager, no manual "please close the app" step you have to act on. Installation then proceeds and completes automatically; existing configuration/data survives (destinations, connected accounts) — not wiped. This was a real physical-test failure this cycle: previously the operator had to open Task Manager and manually kill the process before the installer could continue (docs/windows-packaging.md §26). |
| A-8b | Relaunch the app after A-8's upgrade. | It starts normally against the same, preserved configuration — same destinations/accounts as before the upgrade. |
| A-9 | Uninstall via Windows "Apps & Features" (or the Start Menu's own Uninstall shortcut). | A confirmation dialog appears with an **unchecked** checkbox labeled "Also remove all Streaming Tree settings, local data, and saved credentials" and a clear "this cannot be undone" warning. Leave it unchecked and confirm. Uninstall completes without error; the Start Menu entry is gone. |
| A-9b | Press Enter on the uninstall confirmation dialog without touching the checkbox (A-9's own dialog, before confirming). | The default button performs the ordinary (non-destructive) uninstall — the checkbox's own unchecked state is what the default button acts on, so a stray Enter press can never trigger data deletion on its own. |
| A-10 | After the A-9 uninstall (checkbox left unchecked), check the per-user AppData data directory. | Application data (SQLite DB, credentials) is left in place, not deleted — uninstalling the program must never delete an operator's stream configuration/history. |
| A-10b | Reinstall after A-10, using a fresh, disposable test profile/machine if you want to also exercise the destructive option below — **do not use your main configured install for this one.** Uninstall again, this time checking "Also remove all Streaming Tree settings, local data, and saved credentials." | Uninstall completes; the per-user AppData data directory is now gone entirely (database, managed assets, managed MediaMTX runtime) and any destination/account credentials this install created are removed from Windows Credential Manager. |
| A-11 | After launch (A-2), look at the Windows notification area (system tray). | Exactly one Streaming Tree icon is present — the real logo's emblem (the neon circuit-tree mark), not the earlier placeholder (docs/windows-tray.md §5). |
| A-11b | Hover the pointer over the tray icon and wait for the tooltip. | A tooltip reading "Streaming Tree for OBS" appears — this was broken (no tooltip at all) before this remediation cycle's fix (docs/windows-tray.md §5c). |
| A-11c | In File Explorer, right-click the installed `streaming-tree-server.exe` (or the Start Menu shortcut) → Properties. | The file icon is the real logo, not a generic Go/Windows default icon; the Details tab shows "Streaming Tree for OBS" as the file description/product name. |
| A-12 | Left-click the tray icon. | The Dashboard opens in the default browser (same as A-2's launch behavior) — never a second backend/second port bind. |
| A-13 | Right-click the tray icon. Compare the menu text's sharpness against another normal Windows tray application's own menu (e.g. right-click a different app's tray icon), at your normal display scaling. | A context menu appears: Open Streaming Tree, Open Logs & Diagnostics, a grayed status line reading "Ingest: ...", Check for updates, Quit Streaming Tree — in that order, with separators as visually distinct groups. The menu text is crisp, not visibly blurry/soft compared to the other app's menu — this was the real defect this cycle's DPI-awareness fix (docs/windows-tray.md §5, `apps/server/cmd/server/winres/winres.json`) addressed; automated tooling confirmed the fix is technically in effect (a real `GetWindowDpiAwarenessContext` query against the live tray window), but the actual crispness judgment is only meaningful from a real human eye on real hardware. |
| A-13b | If your display is scaled below 100%, repeat A-13 at that scale too (in addition to your normal scale). | Same result — crisp, not blurry, at every scale you can practically test. |
| A-14 | Click "Open Logs & Diagnostics" from the tray menu. | The browser opens directly to the Logs & Diagnostics page (`/logs`), not the Dashboard. |
| A-15 | Right-click the tray icon again while OBS is/isn't connected (compare against Session C's state). | The grayed status line matches the real ingest state shown on the Streams page ("Not installed" / "Waiting" / "Receiving") — never stale, never contradicting the web UI. |
| A-16 | Click "Check for updates" from the tray menu (on this build's real eligibility — see G-1). | If grayed: nothing happens (correctly refused). If enabled: behaves the same as triggering a check from the web Updates panel — check `docs/updater.md` §43 for what "eligible" means for the exact build under test. |
| A-17 | Click "Quit Streaming Tree" from the tray menu. | The backend process actually exits (check Task Manager for `streaming-tree-server.exe`, same as A-7) **and** the tray icon disappears immediately — never left behind after the process is gone. |
| A-18 | Relaunch the app, then use the web UI's own Quit action (A-7) instead of the tray's Quit. | The tray icon disappears too — every shutdown path removes it, not only the tray's own Quit item. |
| A-19 | After several install/uninstall/restart cycles in one session, check the Streams page for the local OBS ingest status. | Ingest starts up normally (Waiting/Receiving as OBS connects) — no "port already in use"/"INGEST SERVICE FAILED" error. If this ever does appear, check whether it is real (something else on this machine genuinely owns that port — not a product defect) or a stale Streaming-Tree-owned process (a real defect fixed this cycle via a Windows Job Object safety net — `internal/runtime/procutil`); either way, record which one you found, not just "it failed." |

## Session B — browser/UI navigation

Precondition: the app running (any platform), reachable in a browser.

| ID | Action | Expected result |
| --- | --- | --- |
| B-1 | Click through every sidebar navigation item once. | Every route loads without a blank page, a JS console error, or an infinite spinner. |
| B-2 | Resize the browser window to a narrow (phone-width) viewport. | The sidebar collapses to the mobile drawer pattern; no horizontal scrollbar appears on the page body; no control becomes unreachable. |
| B-3 | Open the language switcher and change to Polish, then back to English. | Every page you already visited re-renders in the selected language with no missing/untranslated keys (a raw key like `chat.status.loading` appearing as literal text would be a failure). |
| B-4 | Trigger a validation error on any form (e.g. leave a required field empty and submit). | A clear, localized error message appears near the field — not a raw error code, not a silent no-op. |
| B-5 | Trigger a real backend-unavailable state (e.g. stop the backend process, then perform an action expecting a response in the still-open browser tab). | The UI shows a clear "unavailable"/error state with a retry option — not an unhandled crash or an indefinitely spinning loader. |
| B-6 | Look at the sidebar/header brand mark (top of the sidebar, next to "Streaming Tree / for OBS"). | Shows the real logo's emblem (the neon circuit-tree mark) — not the old generic purple network-icon placeholder. |
| B-7 | Look at the actual browser tab (not just the app's own UI) for this page. | The tab shows the real logo's emblem as its favicon — not a generic default icon, not the old placeholder. If you have visited this app before this cycle's fix, hard-refresh (Ctrl+Shift+R) once first — see docs/progress.md's favicon cache-busting entry for why a normal refresh might still show an old cached icon from before the fix existed, and confirm a hard refresh resolves it. |
| B-8 | Open About & Legal from the sidebar and look at its own product-identity panel (the one with the product name/version). | Also shows the same real logo emblem — not a generic document icon — consistent with the sidebar. |
| B-9 | Open an existing destination's Platform settings; confirm its own content scrolls normally if it exceeds the modal's height. | The modal's own content area scrolls; the page behind it does not. |
| B-10 | From the same Platform settings modal, click Delete, confirm the deletion. | The confirmation dialog and the settings modal both close; the destination disappears from the Dashboard. |
| B-11 | Immediately after B-10, without refreshing the page, try to scroll the Dashboard (mouse wheel or trackpad). | The page scrolls normally — this was a real physical-test failure this cycle: the page previously stayed permanently unscrollable until a manual browser refresh (docs/progress.md's body-scroll-lock entry). |
| B-12 | On a fresh app/page load (or after a hard refresh), open Add Platform for the first time in that session. Do not touch the Platform field. | The Platform field shows "Select a platform…" — not a real platform like Twitch already selected. This was a real physical-test failure this cycle: the field previously showed a real platform selected while the underlying form state was actually empty (docs/progress.md's explicit-platform-selection entry). |
| B-13 | With B-12's dialog still open and the Platform field untouched, enter a Display name and submit. | Submission is rejected with "Select a platform." — matching the visibly empty Platform field, not contradicting it. |
| B-14 | Explicitly select a real platform (e.g. Twitch), enter a Display name, and submit. | The Platform field's visible selection remains stable while typing the Display name; submission succeeds; the destination is created with the platform you selected. |
| B-15 | Open Add Platform again after B-14's successful creation. | The Platform field again shows "Select a platform…" — it does not inherit the platform you just used. |
| B-16 | While the Display name field has focus and text in it, change the Platform selection. | Display name keeps its typed value and does not lose focus to the Close button. |

## Session C — local OBS ingest

Precondition: a real OBS Studio installation on the same machine as the app (or reachable over the local network), and the app running.

| ID | Action | Expected result |
| --- | --- | --- |
| C-1 | Open Streams in the app; note the server address and stream key shown. | Values are present and match the format OBS expects (`rtmp://127.0.0.1:<port>/<path>` and a key string). Do not paste the actual key into chat — describe it as "present" or "a redacted-looking placeholder if not yet generated." |
| C-2 | In OBS: Settings → Stream → Service "Custom...", paste the server address and stream key from C-1, then click "Start Streaming" in OBS. | Enter these values only in OBS's own settings dialog. |
| C-3 | Watch the Streams page in the app after C-2. | Ingest status changes from "waiting" to "receiving" within a few seconds of OBS actually connecting. |
| C-4 | Stop streaming from OBS. | Ingest status in the app returns to "waiting"/"not receiving" — the app correctly detects OBS disconnecting, not just a client-side timeout guess. |
| C-5 | Start OBS streaming again. | Ingest status returns to "receiving" again — the ingest path is fully reusable, not a one-shot resource. |

## Session D — outgoing destination branch streaming

Precondition: Session C passed (OBS actively streaming to the app) and at least one destination is configured with either a real platform credential or a safe test/private destination — a public broadcast of personal content is not required; a private/unlisted destination or a throwaway test channel is sufficient.

| ID | Action | Expected result |
| --- | --- | --- |
| D-1 | With OBS connected (Session C), start one destination branch. | Branch state moves through starting → live within a reasonable time; no crash, no silent no-op. |
| D-2 | Check the destination platform itself (or its private/test equivalent) for the incoming stream. | The stream is actually visible/receiving there — not just "the app says live" without real confirmation. |
| D-3 | Stop OBS streaming while the branch is live. | The branch detects the lost input and transitions to a waiting/error state — it does not keep reporting "live" with no real input. |
| D-4 | Resume OBS streaming. | The branch recovers (either automatically or via a manual restart action, whichever the app's real behavior is) without requiring a full app restart. |
| D-5 | Click "Stop" on a single live branch, then "Stop all" with more than one branch live. | Both require the confirmation prompts added in Stage 20E-era hardening; both actually stop the real outgoing transmission (re-check on the destination platform, not just in the app's own UI). |

## Session E — Browser Source overlays

Precondition: app running, at least one chat overlay / alert profile / audio profile / goal-or-supporter widget configured with a **local** Browser Source URL. If remote-overlay infrastructure (Stage 20D2C) is genuinely available in this environment, repeat with a remote capability URL too — otherwise record that half as environment-unavailable, not skipped silently.

| ID | Action | Expected result |
| --- | --- | --- |
| E-1 | Add the chat overlay's local Browser Source URL to a real OBS Browser Source. | It renders (transparent background as configured, correct sizing) inside OBS, not just in a regular browser tab. |
| E-2 | Send a real chat message (from a connected account) while the overlay is open in OBS. | The message appears in the overlay within a couple of seconds — proves the live SSE/event pipeline reaches an actual Browser Source, not only a dev-tools-inspected browser tab. |
| E-3 | Trigger a real alert-eligible event (or the nearest safe equivalent this environment can produce) with an alert overlay open. | The alert plays its configured animation/audio inside OBS. |
| E-4 | Close and reopen the OBS Browser Source (simulating OBS restart / scene reload). | The overlay reconnects and resumes showing live content — it does not require restarting the whole app. |
| E-5 | If a goal or supporter widget is configured: open its Browser Source URL. | It renders current progress/values correctly, matching what the management UI shows for the same goal/widget. |
| E-6 (remote only, if available) | Repeat E-1/E-2 using a remote-overlay capability URL instead of the local one. | Same rendering/live-update behavior over the remote path — never paste the capability URL itself into chat; describe it as "the remote overlay URL for <profile name>." |

## Session F — connected-account authentication

Precondition: app running, real Twitch and/or YouTube developer credentials configured server-side (already-implemented integrations only — Kick/TikTok/Streamlabs/Ko-fi remain feasibility-gated and are out of scope here per `docs/final-hardening.md`).

| ID | Action | Expected result |
| --- | --- | --- |
| F-1 | Start a Twitch device-flow connection from Settings. | A user code and verification URL appear; completing the flow on Twitch's own site (in a separate tab) connects the account in the app within the polling interval. |
| F-2 | Start a YouTube OAuth connection from Settings. | The browser is sent to Google's real consent screen; after granting, control returns to the app and the account shows connected. |
| F-3 | Disconnect a connected account. | A confirmation prompt appears explaining the consequence before it actually disconnects. |
| F-4 | Attempt to reconnect the same account after F-3. | The reconnect flow works cleanly a second time — no stuck "already connected" state blocking it. |

## Session G — updater UX (without a fake public release)

Precondition: a packaged build (Session A's install, or the packaged equivalent on another platform). This session tests **UI behavior**, not a real production update (no public release exists yet — §41).

| ID | Action | Expected result |
| --- | --- | --- |
| G-1 | Open the update-check UI (Settings/About, wherever it lives) on a packaged build. | On a manual/test build (any version not shaped `major.minor.patch`, e.g. `0.1.0-manualtest+<commit>`): an honest "Manual/test build — automatic updates are unavailable for this build." message, no check button, no misleading failure (docs/updater.md §43). On a real strict-production-version build: a real check result against the real GitHub API — "up to date," "no Stable release has been published yet" (a calm, non-error state — §43), or a real error if genuinely offline — never a placeholder/fake success. |
| G-2 | If a newer real release genuinely exists at test time, attempt the real check/download/install flow. | Progress is shown; install-and-restart works and the restarted app reports the new version. If no newer release exists, record this item as **Not verified — no newer release available to test against**, not a failure. |
| G-3 | On macOS/Linux (if that platform's session below is being run): open the same update-check UI. | It correctly reports `platform_unsupported` for install (no fabricated install-handoff) — this is documented, expected behavior, not a bug to report. |

## Session H — Linux desktop (if Linux hardware/VM available)

Precondition: a real Linux x64 or ARM64 machine (a VM is acceptable here, unlike Session A, since Linux desktop's own contract doesn't distinguish), the `.deb` package built from the commit under test.

| ID | Action | Expected result |
| --- | --- | --- |
| H-1 | Install the `.deb` via the real package manager (`dpkg -i` / a GUI installer). | Installs cleanly; a desktop launcher entry appears. |
| H-2 | Launch via the desktop launcher entry. | Default browser opens to the management UI, same as Windows Session A-2. |
| H-3 | Repeat A-4 through A-7 (persistence across tab close, single-instance detection, About/Legal content, clean shutdown) on Linux. | Same expected results as their Windows counterparts. |
| H-4 | Uninstall via the real package manager. | Clean removal; application data under the Linux XDG data directory survives, matching A-10's requirement. |

## Session I — Linux headless + real OBS RTMPS (if a headless server + DNS + trusted cert are available)

Precondition: a real Linux headless deployment with `--headless --remote-management --remote-ingest`, a real domain name pointed at it, and a trusted TLS certificate (not self-signed) — the exact environment PRE-20E.1's own automated CI verification already exercises, but this session is the human-operated equivalent using a real OBS installation.

| ID | Action | Expected result |
| --- | --- | --- |
| I-1 | Log into the remote management web UI over HTTPS from a browser. | Session cookie is set; login succeeds with the real admin password (never pasted into chat). |
| I-2 | Generate a remote-ingest publisher credential from the management UI. | A one-time credential is shown once; copy it directly into OBS, never into chat. |
| I-3 | In OBS: Settings → Stream → Custom, enter the RTMPS endpoint (documented format) and the credential from I-2, then start streaming. | OBS connects; certificate is trusted by OBS without a manual override (proves the cert is genuinely trusted, not self-signed). |
| I-4 | Check the management UI's remote-ingest status. | Shows "receiving" while OBS is actively publishing. |
| I-5 | Start a real destination branch while receiving remotely. | Branch goes live, same as Session D, over the remote ingest path. |
| I-6 | Stop OBS. | Remote-ingest status returns to waiting; branch reacts the same way Session D-3 describes. |
| I-7 | Rotate the publisher credential from the management UI. | The old OBS configuration stops working (next connection attempt is rejected). |
| I-8 | Update OBS with the newly rotated credential (I-7) and reconnect. | Connects successfully with the new credential — proves rotation is a real, working operation, not just a UI action with no backend effect. |

If this environment is unavailable, record the whole session as **Not
verified — environment unavailable**, not a failure and not silently
omitted.

## Session J — macOS unsigned package (if real Mac hardware available)

Precondition: real Apple Silicon and/or Intel Mac hardware (Stage 20C1's own unsigned `.dmg`/`.app`). Gatekeeper will warn since this build is genuinely unsigned/unnotarized — that warning is expected, not a bug, and this session never attempts to bypass it with a fake signing identity.

| ID | Action | Expected result |
| --- | --- | --- |
| J-1 | Open the `.dmg`, drag the app to Applications, launch it, and go through the real Gatekeeper "unidentified developer" bypass (right-click → Open, or System Settings → Privacy & Security → Open Anyway). | The app launches after the real, expected Gatekeeper friction — this is Stage 20C1's own documented, current state, not a defect to fix here. |
| J-2 | Repeat A-3 through A-7 (Dashboard loads, persistence, single-instance, About/Legal, clean shutdown) on macOS. | Same expected results as Windows. |
| J-3 | Check About & Legal specifically for any claim of being signed/notarized. | It must **not** claim signed/notarized status — Stage 20C2 remains Planned, externally gated. |
| J-4 | Quit and remove the app (drag to Trash). | Clean removal; application data under `~/Library/Application Support/` (or wherever it actually lives) survives, matching A-10. |

If unavailable, record as **Not verified — no Mac hardware available**.

## Session K — diagnostics: Logs page and support bundle

Precondition: the app running with the Stage 20E diagnostics feature (any platform). Ideally performed after at least one of Sessions C/D/E/F has generated some real log activity.

| ID | Action | Expected result |
| --- | --- | --- |
| K-1 | Open the Logs page. | Recent entries are visible with a timestamp, severity, and subsystem each — not a placeholder screen. |
| K-2 | Filter by severity (e.g. only "Error"). | The list narrows correctly; clearing the filter restores the full list. |
| K-3 | Use the text search filter with a term you know appears in a recent message. | Only matching entries remain. |
| K-4 | Click Refresh. | The list re-fetches (new entries generated since the page loaded, if any, appear). |
| K-5 | Deliberately cause a real, recoverable error (e.g. try an action while the backend is briefly unreachable, or trigger a validation failure server-side) and then check the Logs page. | A relevant log entry appears, with a sensible severity and message — not vague/empty. |
| K-6 | Read several log messages carefully. | No stream key, token, password, or other secret is visible in any message — the redaction notice on the page should also be visibly present. |
| K-7 | Click "Export support bundle." | A `.zip` file downloads with an app-generated filename (not something you typed). |
| K-8 | Open the downloaded `.zip`. | Contains `manifest.json` and `logs.json`, both plain-text-readable; version/OS/arch/subsystem-state fields look correct; log entries are the same redacted shape as K-6. |
| K-9 | Search the extracted bundle's files for anything that looks like a real credential (a stream key format, a token-shaped string, a password). | Nothing found — the bundle only ever contains what K-8 lists, per `docs/final-hardening.md` §C's own exclusion list. Report PASS/FAIL by description only, never by pasting a found value. |

## Session L — install/uninstall/restart/recovery UX

Precondition: whichever of Sessions A/H/J were actually run on this hardware.

| ID | Action | Expected result |
| --- | --- | --- |
| L-1 | With the app configured (destinations, connected accounts) and running, kill the process abruptly (Task Manager "End task" / `kill -9` / Force Quit) rather than using its own shutdown action. | On next launch, the app recovers cleanly — no corrupted-database error, no stuck "already running" false-positive from the killed process's stale lock. |
| L-2 | Restart the host machine entirely with the app set to auto-launch (if that's a supported mode) or by manually relaunching after reboot. | App starts cleanly with all prior configuration intact. |
| L-3 | With a destination branch live, force-quit the app (as in L-1). | On next launch, the branch's `desiredRunning` does NOT silently resume broadcasting on its own — the app must never restart a live outgoing transmission without the operator explicitly starting it again. |
| L-4 | Perform Session A/H/J's full install → use → uninstall cycle twice in a row (fresh install, use, uninstall, fresh install again). | The second install behaves identically to the first — no leftover state from the first cycle causes different behavior. |

## Session M — Stage 20E Dashboard visual realignment (new this round)

Precondition: Session A's install (this round's candidate), the app
running with at least two configured destinations across at least two
different providers (ideally including one Twitch/YouTube/Kick/TikTok
mix) so provider branding and the grid layout are both visible at once.

| ID | Action | Expected result |
| --- | --- | --- |
| M-1 | Open the Dashboard and look at the overall page. | Reads as a streaming control center - destination cards are the clear primary content, spacing feels deliberate rather than cramped, no leftover developer-facing "Backend"/"Go REST API" card and no "System resources"/"Demo" card anywhere on this page. |
| M-2 | Look at each configured destination's provider identity (card, and Platform settings' provider row, and the Metadata tab strip). | A real, recognizable Twitch/YouTube/Kick/TikTok mark is shown in a coloured accent tile - never a plain "TW"/"YT"/"KI"/"TT" letter tile for these four providers. |
| M-3 | Specifically check a TikTok destination's mark (add one via Add Platform if none is configured). | The TikTok glyph itself is clearly visible against its tile (light mark on a dark cyan/pink-tinted tile) - not a near-invisible near-black mark. This was a real defect caught and fixed by this round's own contrast review before any physical test - confirm it actually looks right on a real display. |
| M-4 | Widen the browser window to a large desktop size (1800px+ if your display allows). | The destination-card grid grows past two columns (up to four at very wide widths) rather than staying capped at two forever. |
| M-5 | Look at the right-hand status column. | Shows real configured/enabled/disabled counts plus a real live/starting/error breakdown (or "No destination is currently sending" if none are active) - never a disclaimer claiming no live state exists, and never a "Demo"-badged resource panel. |
| M-6 | Open a destination's Platform settings (the modal with Display name/Enabled/stream key/output settings/account link). | Reads as one coherent settings surface with a clear rhythm between sections - not a stack of equally-heavy bordered boxes. Provider identity (real brand mark) appears at the top of the modal. |
| M-7 | Scroll to the Metadata & Settings area on the Dashboard (below the destination cards). | Heading reads "Stream details" (not "Metadata editor"); the form is compact and capability-driven per provider - no field appears for a capability a provider doesn't actually support. |
| M-8 | Open Logs & Diagnostics from the sidebar. | Now also shows a "Backend" card (Service/Version/Uptime/connectivity) - this moved here from the Dashboard this round; confirm it shows real, correct data. |
| M-9 | Look at the very bottom of the sidebar (below the OBS connection panel). | Shows the real build version/commit (matching About & Legal's own version line) - never a hardcoded "0.1.0". |
| M-10 | Resize the browser window through large desktop → normal laptop → narrow/tablet → phone width, watching for any point where a horizontal scrollbar appears on the page itself. | No horizontal overflow at any width; the right-hand status column moves below the main content below the `xl` breakpoint; the destination grid collapses to fewer columns as width shrinks, down to one column on phone width. |

---

## Recording results

For each item actually run, record: ID, PASS/FAIL, and a one-line
sanitized note (what you saw, redacted screenshot description, or a
non-sensitive log excerpt from the Logs page — never a secret). For a
session not run at all, record the session as **Not verified —
<reason>** once, rather than marking every item in it individually.
