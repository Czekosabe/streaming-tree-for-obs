# Troubleshooting

Common problems and their fixes, covering the backend/frontend
development workflow, MediaMTX/OBS, FFmpeg, and Twitch/YouTube account
integration.

---

## Common problems

**The panel shows "Backend unavailable".**
The backend is not running, or is running on a different port. Start it in a
second terminal (`cd apps/server && go run ./cmd/server`) and use the refresh
button in the "Backend" card. This is an expected, fully handled state — the
panel does not crash. Your configuration is safe: it lives in the backend
database, which is why the dashboard cannot show it while the backend is down.

**My destinations disappeared.**
The backend is probably using a different database than before. Check the
`path=` value in the startup log and whether `STREAMING_TREE_DB_PATH` or
`STREAMING_TREE_DATA_DIR` is set in that terminal.

**A seeded destination I deleted did not come back.**
That is intended. The seed runs once, on a brand-new database, and is recorded
like any other migration.

**The platform settings dialog says "Secure storage unavailable" for the
stream key.** The operating system credential store could not be reached:
common causes are a Linux session with no Secret Service running, a locked
macOS Keychain, or a permission failure. The rest of the application is
unaffected - SQLite, MediaMTX and everything else keep working - but a stream
key cannot be saved until the store becomes available. This is not polled
automatically; reopen the dialog after fixing the underlying cause to check
again.

**I deleted a destination while the credential store was unavailable - is its
stream key still out there?** Possibly. Platform deletion does not block on a
credential store it cannot reach (see "Stream key security"), so a key set
earlier, when the store was reachable, may still exist under that platform's
old ID. It is inert: the ID is never reused and nothing in this application
can look it up again. If this matters to you, use your OS credential manager
directly to remove any leftover entry under the `streaming-tree-for-obs`
service name.

**`go: command not found` or `'go' is not recognized`.**
Go is not installed or is not on `PATH`. Install it from <https://go.dev/dl/>
and open a new terminal window.

**`npm install` fails with "Cannot find native binding".**
Your Node version is older than the native dependencies require. Upgrade Node to
22.12+ or 24 LTS, delete `apps/web/node_modules` and
`apps/web/package-lock.json`, then install again.

**Port 8080 or 5173 is already in use.**
Backend: start it with a different `STREAMING_TREE_PORT` and add the new address
to `VITE_DEV_API_PROXY_TARGET` in `apps/web/.env.local`.
Frontend: Vite will offer the next free port; remember to add the new origin to
`STREAMING_TREE_ALLOWED_ORIGINS`.

**Interface changes are not visible.**
Check that `npm run dev` is still running and that there are no errors in the
browser console. If needed, reload the page bypassing the cache
(`Ctrl + Shift + R`).

**A label shows in English while the interface is set to Polish.**
That is the fallback working: the Polish entry is missing. Run
`npm run i18n:check` — it prints the exact path of every missing key.

### MediaMTX and OBS

**"MediaMTX is not installed yet."**
Expected on a fresh setup. Use the **Install MediaMTX** button in the sidebar or
on the **Streams** page. Nothing is downloaded until you confirm.

**"The MediaMTX binary found is not the supported version."**
Only v1.19.3 is supported, and an unsupported build is never started because the
generated configuration targets that exact schema. Either remove
`STREAMING_TREE_MEDIAMTX_PATH` and use the managed installation, or point it at
a v1.19.3 binary. If a managed installation is stale, delete
`runtime/mediamtx` and reinstall.

**"The downloaded file did not match the official checksum."**
Nothing was installed — the archive was discarded. Retry; if it keeps happening,
suspect a proxy or security product rewriting downloads. Never work around this
by installing manually from an unverified source.

**"There is no official MediaMTX release for this operating system..."**
Your OS/architecture is outside the supported matrix. Obtain a v1.19.3 binary
yourself and set `STREAMING_TREE_MEDIAMTX_PATH` to it.

**"The configured port is already used by another application."**
Something else holds 1935 or 9997. Streaming Tree **never terminates another
process to free a port**. Stop the other application, or set
`STREAMING_TREE_MEDIAMTX_RTMP_ADDRESS` / `STREAMING_TREE_MEDIAMTX_API_ADDRESS`
to free ports. Remember to update OBS if you change the RTMP port.

Finding the holder:

```bash
# Linux / macOS
lsof -i :1935
```

```powershell
# Windows PowerShell
Get-NetTCPConnection -LocalPort 1935 | Select-Object OwningProcess
```

**"MediaMTX failed repeatedly and will not be restarted automatically."**
The crash-loop guard tripped: five failures within five minutes. Automatic
restarts stop deliberately so the loop does not run forever. Look at the backend
log for the MediaMTX output, fix the cause, then press **Start**.

**"MediaMTX started but did not become ready in time."**
The process launched but its Control API never answered. Usually the Control API
port is blocked or occupied by something that accepts connections without
answering correctly. Check `STREAMING_TREE_MEDIAMTX_API_ADDRESS`.

**OBS is connected but the panel still says "Waiting for OBS".**
Check that OBS uses **Custom...** with exactly the Server and Stream Key shown
in the panel — a mismatched stream key publishes to a path this configuration
does not allow. Also confirm OBS is actually streaming, not just configured, and
that the service reports **Running**.

**Ingest says "Status unavailable".**
MediaMTX is running but the backend cannot read its Control API. Restarting the
service usually clears it.

**MediaMTX keeps running after I close the backend.**
It should not: shutdown stops and reaps it. If it happens, note how the backend
was terminated — a `SIGKILL` to the backend gives it no chance to clean up — and
end the `mediamtx` process manually.

### FFmpeg and destination branches

**A destination shows the blocker "FFmpeg is not available."**
No compatible FFmpeg was found. Install one from a source you trust and make
sure it is on `PATH`, or set `STREAMING_TREE_FFMPEG_PATH` to it, then restart
the backend (FFmpeg is only re-probed periodically or at startup). Streaming
Tree never installs FFmpeg for you — see
[Why there is no managed FFmpeg download](connecting-platforms.md#outgoing-streaming-with-ffmpeg).

**A destination shows the blocker "The available FFmpeg is missing a
required capability."**
The located FFmpeg failed at least one capability probe (RTMP input/output,
RTMPS output, the FLV muxer, or `-progress` support) even though it parses
`-version` fine. Most general-purpose FFmpeg builds pass all of these;
check whether yours was built with RTMP support disabled.

**A destination fails immediately with an "unsupported codec" error.**
FLV/RTMP cannot carry every codec, and this stage never transcodes. Change
the source (in OBS) to a codec FLV can carry — H.264 video, AAC audio are
the safe, universally supported choice — rather than expecting Streaming
Tree to silently re-encode.

**A destination keeps restarting and then shows "FFmpeg failed repeatedly
and will not be restarted automatically."**
The same crash-loop guard as MediaMTX's, applied per destination: five
failures within five minutes. Check the destination's `lastError` on the
Streams page, fix the underlying cause (commonly: the destination server is
unreachable, or the port/URL is wrong), then press **Start** again — the
restart counter resets on a fresh explicit start.

**A destination is stuck on "Waiting for input."**
This is expected whenever OBS is not currently publishing to the local
ingest — the branch is deliberately paused, not failing. It resumes on its
own once OBS reconnects, as long as you have not pressed **Stop** since.

**I configured an output server URL but saving it fails validation.**
Only `rtmp://` and `rtmps://` are accepted, a host is required, the port (if
present) must be valid, and the URL may not contain user-info (`user@host`),
a `#fragment`, or control characters. A path (like `/app`) is fine — many
providers use one. The stream key never belongs in this field at all; it has
its own field.

**Is my stream key visible anywhere I should worry about?**
See [Stream-key exposure on the command line](connecting-platforms.md#outgoing-streaming-with-ffmpeg)
for the one honestly-documented limitation: it is briefly present as an
FFmpeg process argument while that destination is running, which on most
operating systems a process list on the *same machine* could observe. It is
never logged, never in an API response, and never on disk outside the OS
credential store.

### Twitch account integration

**"Configure a Twitch Client ID above before connecting an account."**
No Client ID is configured yet, or it failed validation. Register an
application at the
[Twitch Developer Console](https://dev.twitch.tv/console/apps) and either
set `STREAMING_TREE_TWITCH_CLIENT_ID` or paste it into the Settings page —
see [Registering a Twitch application](connecting-platforms.md#connected-accounts-and-twitch-metadata).

**The Client ID field in Settings is disabled and I can't change it.**
It is set by the `STREAMING_TREE_TWITCH_CLIENT_ID` environment variable,
which always wins over anything saved in the database. Unset it (and
restart the backend) if you want to manage the Client ID from Settings
instead.

**Saving a new Client ID fails with a conflict.**
A database-managed Client ID cannot be changed while any Twitch account is
still connected, since a different application can mean invalidated
tokens for existing accounts. Disconnect every Twitch account first.

**"Authorization was denied on Twitch."**
You (or whoever completed the device-code flow) chose not to authorize the
application on Twitch's own page. Click **Connect Twitch** again to start a
fresh attempt.

**"This code expired before it was used."**
The user code has a limited lifetime. Start a new attempt and complete it
more quickly, or check that the device you used to open the verification
link actually reached Twitch (network issues on that device look the same
as simply not finishing in time).

**"The authorization did not grant every required permission."**
Twitch's own authorization page let you decline part of what was requested.
Reconnect and make sure the full permission is granted; Streaming Tree only
ever asks for one scope (`channel:manage:broadcast`), so there is nothing
to selectively decline without breaking metadata publishing.

**An account shows "Reconnect required."**
Twitch could not confirm the account's access on the last check (the token
could not be validated and the automatic refresh also failed — commonly
because the refresh token expired from 30 days of disuse, or the account's
authorization was revoked directly on Twitch). Click **Reconnect** to
re-authorize the same account; nothing about the account's identity or any
destination links needs to be re-entered.

**"Secure storage is currently unavailable" on a Twitch action.**
The same operating-system credential store used for stream keys also holds
Twitch token bundles, and it could not be reached — see "Secure storage
unavailable" above for common causes. Connected accounts and their links
are unaffected in SQLite; only the token bundle-dependent actions
(validate, category search, publish) are blocked until the store is
reachable again.

**"Twitch could not be reached" / a publish or category search fails
intermittently.** A transient network issue talking to Twitch, or Twitch
itself being unavailable. Nothing local was changed; try again.

**"Twitch's rate limit was reached; try again shortly."**
Twitch's own API rate limit (visible in its `Ratelimit-*` response headers)
was hit. This is Twitch-side, not a Streaming Tree limit; wait a short
while and retry.

**The Publish button is disabled and says to select a category first.**
The saved category text has no matching Twitch category ID — either it was
typed by hand without picking a search result, or an older save predates
the category picker. Open the metadata editor, use the category search box,
and select a real result; that stores both the display name and the ID
publishing actually needs.

**Publishing is disabled with a note about unsaved changes.**
Save your local edits first. Publish always sends exactly what is currently
saved in Streaming Tree's database, never an in-progress, unsaved draft —
this is deliberate, not a bug.

### YouTube account integration

**"Configure a YouTube Client ID above before connecting a channel."**
No Client ID is configured yet. Create a Google Cloud project, enable
YouTube Data API v3, create a Desktop-app OAuth client, and either set
`STREAMING_TREE_YOUTUBE_CLIENT_ID` or paste the Client ID into the
Settings page — see
[Registering a Google Cloud project](connecting-platforms.md#connected-accounts-and-youtube-metadata).

**The Client ID field in Settings is disabled and I can't change it.**
It is set by the `STREAMING_TREE_YOUTUBE_CLIENT_ID` environment variable,
which always wins over anything saved in the database — independent of
Twitch's own Client ID variable.

**Saving a new Client ID fails with a conflict.**
A database-managed YouTube Client ID cannot be changed while any YouTube
account is still connected. Disconnect every YouTube account first.

**"Authorization was denied on Google."**
You chose not to approve access on Google's own consent page. Click
**Connect YouTube** again to start a fresh attempt.

**"This attempt expired before it was completed."**
The authorization attempt has a bounded lifetime. Start a new attempt and
complete the Google sign-in more promptly.

**A channel-selection screen appears after signing in.**
The Google account you authorized owns more than one YouTube channel.
Streaming Tree never guesses which one you meant — pick the correct
channel explicitly from the list shown.

**"The authorized channel does not match the account being reconnected."**
During a reconnect, a different YouTube channel was authorized than the
one this connected account represents. Reconnect must authorize the exact
same channel; if you meant to connect a different channel, disconnect this
one first and connect the other as a new account.

**An account shows "Reconnect required."**
Google could not confirm the account's access on the last check. This is
often expected if your Google Cloud project's OAuth consent screen is
still in **Testing** publishing status — Google expires authorization
after seven days in that state regardless of what Streaming Tree
requests. Click **Reconnect** to re-authorize the same channel.

**"Secure storage is currently unavailable" on a YouTube action.**
The same operating-system credential store used for stream keys and
Twitch tokens also holds YouTube token bundles, and it could not be
reached — see "Secure storage unavailable" above. Connected accounts,
links, and selected broadcasts are unaffected in SQLite; only token-
dependent actions (validate, broadcast/category listing, publish) are
blocked until the store is reachable again.

**"YouTube could not be reached" / a publish or listing fails
intermittently.** A transient network issue talking to Google/YouTube, or
the API itself being unavailable. Nothing local was changed; try again.

**"YouTube's API quota was exceeded; try again later."**
Your Google Cloud project's daily YouTube Data API quota (10,000 units by
default) was exhausted. This is Google-side, not a Streaming Tree limit;
it resets daily.

**"Live streaming is not enabled for this channel."**
The connected YouTube channel has not enabled live streaming in YouTube
Studio. Enable it there, then retry.

**The broadcast selector is empty.**
No active or upcoming broadcast was found for the linked channel. Create
and schedule one in YouTube Studio — Streaming Tree does not create a
broadcast for you.

**The Publish button is disabled and says to select a broadcast or
category first.** Select a live broadcast in the destination's own
**Selected broadcast** section, and/or open the metadata editor's category
field and pick a real region-scoped result — both are required before
publishing, and neither is guessed automatically.

**Publishing is disabled with a note about unsaved changes.**
Save your local edits first, exactly like Twitch — Publish always sends
what is currently saved, never an in-progress draft.

### Twitch engagement

**"Additional Twitch permission is required" on the Engagement page.**
The connected Twitch account has only the metadata scope
(`channel:manage:broadcast`); reading chat and events needs five more,
narrowly-scoped permissions. Click **Authorize engagement access** to
start the upgrade — your existing stream key and metadata publishing are
completely unaffected while you do.

**The upgrade shows a new code/consent step even though the account is
already connected.** That is expected: the upgrade reuses the same
Device Code Flow as the initial connection, requesting the union of the
account's current scopes plus the engagement ones. Complete it the same
way you completed the original connection.

**"The authorized identity does not match" during the upgrade.** A
different Twitch login completed the device-code activation than the one
already connected. The upgrade must authorize the *same* account;
disconnect and reconnect as a new account instead if you actually meant
to switch identities.

**The Enable toggle is disabled or shows "Blocked."** Either the
permission upgrade above has not been completed yet, or the account
itself needs reconnecting for an unrelated reason (see "An account shows
'Reconnect required'" under Twitch account integration above) — the
connector's own state and blocker code explain which.

**A connector shows "Reconnecting" repeatedly, or a "possible data gap"
timestamp appeared.** Twitch does not replay events lost during an
ordinary connection loss; the connector reconnects automatically with
bounded backoff and recreates its subscriptions, and is honest about the
gap rather than pretending nothing was missed. This is expected
behavior, not an error — check the connector's own reconnect count and
last-event timestamp to see whether it has recovered.

**A connector shows "Error" and does not reconnect on its own.** Most
commonly, Twitch revoked the authorization directly (on Twitch's own
site) or removed the subscription version this application uses. Use
**Restart connector**, and if that also fails, disconnect and reconnect
the underlying Twitch account.

**The recent-events feed says "Disconnected" or never shows anything.**
The Server-Sent Events connection to the backend dropped, or the
connector itself is not `connected` yet — check the connector card's own
state first; the feed only ever shows what the backend's Event Bus
actually received.

**Does disabling engagement affect my stream key or metadata publishing?**
No. A connected account's engagement connector, its metadata-publishing
capability, and a destination's stream key are three separate facts —
see [Engagement Event Bus and Twitch chat/events](engagement-architecture.md).
Enabling or disabling the connector never starts, stops, or otherwise
touches a destination's FFmpeg branch.
