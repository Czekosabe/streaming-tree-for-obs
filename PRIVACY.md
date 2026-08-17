# Privacy

Streaming Tree for OBS, created by **Czekosabe**
(<https://github.com/Czekosabe>). Canonical repository:
<https://github.com/Czekosabe/streaming-tree-for-obs>.

This document distinguishes two different things: what the application does
**locally, on your own machine**, and what network activity happens **only
when you explicitly enable a provider integration or click an external
link**. It describes the application as it exists today; see
`docs/product-identity-legal.md` for the audit this document is based on.

## Local application state

- Streaming Tree for OBS is **local-first**. It runs as a local backend
  process plus a local web UI you open in your own browser; there is no
  Streaming Tree for OBS cloud account or first-party server.
- Application configuration and state (destination settings, alert rules,
  goal/widget configuration, and so on) are stored in a local SQLite
  database, in your operating system's standard per-user application-data
  directory.
- Destination stream keys and provider OAuth token bundles (Twitch,
  YouTube) are stored using your **operating system's own credential
  store** - Windows Credential Manager, macOS Keychain, or the Linux
  Secret Service, depending on platform - never in a plain application
  file, and never in the SQLite database.
- The only thing this application stores in your browser (`localStorage`)
  is your interface language preference. No stream key, token, or other
  secret is ever written to browser storage.
- Public OBS overlay routes (chat overlay, alert overlay, audio overlay,
  goal/supporter widgets) are **local application routes**, served by your
  own backend for you to add as an OBS Browser Source. They are not hosted
  by a Streaming Tree for OBS cloud service.

## Network activity you explicitly enable

- When you connect a **Twitch** or **YouTube** account, or configure a
  **StreamElements** donation source, this application communicates
  directly with that provider's own service to authenticate and exchange
  data (chat, events, donations, metadata) - it does not go through any
  Streaming Tree for OBS server, because none exists.
- If you use the managed **MediaMTX** installation option, the backend
  downloads MediaMTX from its documented official source at your request.
  This only happens when you explicitly choose managed installation.
- Outgoing streams (FFmpeg) go directly to the destination platforms you
  configure, using the stream key you provide.

We do not claim "Streaming Tree never connects to the internet" - the
provider integrations above necessarily do, once you enable them. We also
do not claim "no data ever leaves your computer" - the same is true for the
same reason. What we do say precisely: the application itself has **no
first-party cloud service or telemetry pipeline today**, and no network
request happens as a side effect of simply running the application with no
providers configured.

## Telemetry and analytics

There is currently no analytics, crash-reporting, or telemetry dependency
or code path anywhere in this application - confirmed by direct source
audit, not merely undocumented. If this ever changes, it will be a
deliberate, separately documented product decision, not a silent addition.

## Updater

Stage 20's automatic-update/version-check mechanism is **not implemented
yet**. The application currently performs no update check and contacts no
update server.

## Creator support

Selecting **"Support the creator"** in the About & Legal page opens
<https://streamelements.com/czekosabe/tip> in your default browser.

Streaming Tree for OBS itself:

- does not receive your card or payment credentials;
- does not process the transaction;
- does not receive whatever you enter on that external page.

Any information you enter on that external StreamElements (or successor
payment-provider) page is handled entirely under that service's own
privacy policy and terms - not this application's. We make no claims about
StreamElements' fees, taxation, or financial handling; that is between you
and StreamElements.

## Questions

This is a source-available, community project, not a company with a
dedicated privacy contact. The most reliable way to ask a question or
report a concern is to open an issue on the canonical repository:
<https://github.com/Czekosabe/streaming-tree-for-obs>.
