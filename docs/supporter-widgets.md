# Stage 18B — supporter/activity widgets, richer counters, dashboards

This is the canonical Stage 18B contract, written after auditing the real
Stage 18A implementation (`internal/domain/goals`, `internal/goals`,
`internal/httpapi/goals.go`, `internal/httpapi/public_widgets.go`,
`internal/storage/sqlite/migrations/0025_goals.sql`, `internal/domain/
engagement`, the Twitch/YouTube/StreamElements normalizers, and the
frontend `models/goals.ts`/`api/goals-schemas.ts`) — never from remembered
architecture. Stage 18B **extends** the existing `WidgetProfile`/public
widget system; it does not create a parallel one.

## §0. Audit findings that shape every decision below

- `internal/domain/goals.WidgetProfile` currently has **no `Kind` field at
  all** — it is implicitly always a goal widget, and `GoalID` is a
  mandatory, non-empty string. Stage 18B widens this struct; Stage 18A rows
  must load unchanged (`kind` defaults to `"goal"`).
- The current highest SQLite migration is `0025_goals.sql`. Stage 18B's
  migration is `0026_supporter_widgets.sql`.
- `internal/goals.Manager` is the exact precedent for "one manager,
  subscribed at the Event Bus's current position, never `Subscribe(0)`":
  `snap := bus.Snapshot(); bus.Subscribe(snap.NewestSequence)`. Stage 18B's
  own manager copies this pattern exactly.
- `internal/domain/engagement.Type` is the full normalized vocabulary:
  `chat.message`, `chat.message_deleted`, `chat.cleared`, `moderation`,
  `follow`, `subscription`, `resubscription`, `gifted_subscription`,
  `subscription_gift_batch`, `bits`, `raid`, `channel_point_redemption`,
  `stream.online`, `stream.offline`, `youtube.membership`,
  `youtube.membership_milestone`, `youtube.super_chat`,
  `youtube.super_sticker`, `donation`.
- `engagement.Event.User` is optional and may be `Anonymous: true` with no
  `DisplayName` (confirmed for Twitch anonymous cheers and StreamElements
  tips with no `Donation.User.Username`) — never a stable `ProviderUserID`
  for a StreamElements donor. `engagement.Event.Message` is always plain,
  pre-concatenated text (`Message.Text`), never HTML/Markdown. `Quantity`
  is set for `bits` (cheer amount) and `raid` (viewer count). `Money` is
  set for `donation`/`youtube.super_chat`/`youtube.super_sticker`, integer
  `AmountMicros`, uppercase `Currency`, no FX anywhere in this codebase.
- `evt.ConnectedAccountID` already doubles as either a real
  `connected_accounts` id or a donation-source id (confirmed in
  `internal/provider/streamelements/normalize.go`'s own doc comment) — the
  exact mechanism Stage 18A's own account/source filters already rely on.
  Stage 18B reuses it unchanged.
- The public widget stream is deliberately a 1.5s poll-and-diff
  (`internal/httpapi/public_widgets.go`'s `widgetPollInterval`), not a
  push/replay system, because the stream only ever carries the current
  snapshot. Stage 18B keeps this mechanism for every kind (§10 below).

## §1. Scope

Stage 18B adds eight new widget-profile kinds on top of the existing
`goal` kind, plus one composition kind (`dashboard`), all served by the
existing generic `/overlay/widgets/{publicSlug}` route:

`latest_follower`, `latest_subscriber`, `latest_donation`,
`largest_donation`, `recent_supporters`, `event_ticker`,
`session_counter`, `dashboard`.

Completing this stage closes Stage 18 as a whole. Stage 19 (TikTok LIVE,
conditional) and Stage 20 (updater) are untouched by this task.

## §2. No widget designer, no template format (decision, not a gap)

Audited: Stage 18A's own closing scope explicitly left a free-form
designer/template format conditional on "if justified once the foundation
exists." Every Stage 18B widget is a semantic, bounded layout (a follower
name, a donation amount, a bounded list, a grid of existing widgets) —
none of them need arbitrary geometry, layered assets, or a document
format. Decision: **no `visualdesign.Document` involvement, no new
template/package schema, no free-form designer.** Bounded style fields
(reusing/extending Stage 18A's own color/orientation/font model) plus
bounded dashboard grid composition (§9) are sufficient. Stage 14's visual
template/package format is not widened by this stage.

## §3. Privacy boundary: event-derived state is runtime-only

Stage 18A persists only numbers (`current`, `target`, `baseline`). Stage
18B widgets present identifiable, user-authored content — display names,
donation messages, ticker rows. This project has deliberately never
persisted chat/engagement content (`engagement.Event` itself is never
stored; connectors normalize and publish, nothing writes it to SQLite).
Stage 18B preserves that boundary exactly:

**Persisted** (widget **configuration** only, in `widget_profiles` and its
new child tables): kind, name, enabled, public slug, provider/account
filters, style, max item bounds, required currency, counter metric, event
ticker's own type allowlist, dashboard grid/children.

**Never persisted**: any follower/subscriber/donor display name, any
donation message, any recent-supporter row, any ticker entry, any
`providerEventId`/`DedupeKey` used only for runtime presentation, any raw
`engagement.Event`, any raw provider payload.

All "latest"/"largest"/"recent"/"ticker"/counter presentation state lives
only in the new runtime manager's in-memory maps (§7). On backend restart:
Stage 18A goal numbers survive (unchanged); every Stage 18B event-derived
projection starts empty/zero, because nothing is ever read back from
retained Event Bus history or a provider's own history — restart behavior
is identical in kind to Stage 18A's own "current position only, never
replay" rule, just applied to presentation state instead of accumulated
numbers. The frontend states this honestly (§14).

## §4. One runtime manager, one Bus subscription

`internal/supporterwidgets.Manager` (new package, mirrors
`internal/goals`'s own role: runtime only, imports
`internal/domain/goals` for `WidgetProfile`/`ProviderID` types, never
imports a provider package). Exactly one Event Bus subscription, at
current position (`Snapshot().NewestSequence`, never `Subscribe(0)`).

Flow: provider connector → normalized `engagement.Event` → the existing
Event Bus → this one manager → every matching widget profile's runtime
projection updated → the existing generic public widget surface reads the
projection back out through the manager.

On every event, the manager calls `domainSvc.ListWidgetProfiles(ctx, "")`
(the same "re-read current config on every event" pattern
`internal/goals.Manager.handleEvent` already uses for goals) and updates
every enabled, non-`dashboard`, non-`goal` profile whose kind/filters
match. This means widget config changes take effect on the very next
event with no separate cache-invalidation channel required. One thousand
widget profiles still means exactly one Bus subscription — never one per
profile (§4, §24 of the governing task).

Dashboard profiles hold no runtime projection of their own; a dashboard's
public snapshot is composed at read time from its children's own current
projections (§9).

## §5. Profile kind model (extends `WidgetProfile`, not a parallel system)

```go
type WidgetProfileKind string

const (
    WidgetProfileKindGoal             WidgetProfileKind = "goal"
    WidgetProfileKindLatestFollower   WidgetProfileKind = "latest_follower"
    WidgetProfileKindLatestSubscriber WidgetProfileKind = "latest_subscriber"
    WidgetProfileKindLatestDonation   WidgetProfileKind = "latest_donation"
    WidgetProfileKindLargestDonation  WidgetProfileKind = "largest_donation"
    WidgetProfileKindRecentSupporters WidgetProfileKind = "recent_supporters"
    WidgetProfileKindEventTicker      WidgetProfileKind = "event_ticker"
    WidgetProfileKindSessionCounter   WidgetProfileKind = "session_counter"
    WidgetProfileKindDashboard        WidgetProfileKind = "dashboard"
)
```

`WidgetProfile.GoalID` becomes conditional: required and validated (must
reference a real goal) only when `Kind == WidgetProfileKindGoal`; empty
for every other kind. `WidgetProfile` gains: `Providers []ProviderID`,
`Accounts []string` (filters — meaningful for every kind except `goal`,
which keeps using the referenced *goal's own* filters, and `dashboard`,
which has none of its own), `ShowProvider`, `ShowTime`, `ShowMessage`
(latest_donation only, defaults to **false**), `MaxItems` (recent_
supporters/event_ticker), `Currency` (largest_donation/session_counter's
money metric), `Metric` (session_counter), `EventTypes` (event_ticker's
own closed allowlist subset), `Columns` and `Children` (dashboard). Every
existing Stage 18A `goal`-kind field (`TitleOverride`, `ShowCurrent`,
`ShowTarget`, `ShowPercent`, style fields) is unchanged and ignored by
every non-`goal` kind.

Existing rows: `kind` defaults to `'goal'`, `goal_id` keeps its existing
value, every new field defaults to its safe zero value. No existing goal
widget's behavior changes because the enum widened.

## §6. Subscription-family semantics (one decision, reused everywhere)

Reused by `latest_subscriber`, `recent_supporters`, `event_ticker`, and
`session_counter`'s `new_subscriptions`/`resubscriptions`/`gifted_
subscriptions` metrics — never redefined per-widget:

- **New** (a genuinely new subscriber/member): `subscription`,
  `gifted_subscription` (one per actual recipient), `youtube.membership`.
- **Continuing** (not new): `resubscription`, `youtube.membership_
  milestone`.
- **Batch summary** (never counted itself — its own individual
  `gifted_subscription` recipients are counted instead, exactly like
  Stage 18A's own goal contribution rule): `subscription_gift_batch`.

`latest_subscriber` tracks **New** only. `recent_supporters` and
`session_counter`'s `new_subscriptions` also use **New** only.
`event_ticker` may show `resubscription`/`youtube.membership_milestone`
as activity and may show a `subscription_gift_batch` summary row, but
never lets the batch imply N additional new subscriptions in any counter
or recent-supporters context.

## §7. Supporter family (closed table, used by `recent_supporters`)

`subscription`, `gifted_subscription` (per recipient), `youtube.
membership`, `resubscription` (ongoing support is still support), `bits`,
`donation`, `youtube.super_chat`, `youtube.super_sticker`. Never:
`follow` (not a support act), ordinary chat, moderation, raids (a raid is
attention, not support - it may appear in the ticker, never in recent
supporters), `subscription_gift_batch` (would double-count against its
own recipients), `youtube.membership_milestone` (ticker-only activity,
not a new support event).

## §8. Event ticker family (closed table)

`follow`, `subscription`, `resubscription`, `gifted_subscription`,
`subscription_gift_batch`, `bits`, `raid`, `donation`, `youtube.
super_chat`, `youtube.super_sticker`, `youtube.membership`, `youtube.
membership_milestone`. Never: `chat.message`, `chat.message_deleted`,
`chat.cleared`, `moderation`, `stream.online`, `stream.offline`,
`channel_point_redemption`, or any future/unknown type — a type absent
from this table is silently ignored, never shown merely because its Go
constant exists. A profile's own `EventTypes` field is validated as a
subset of this table.

## §9. Widget semantics

**`latest_follower`**: eligible on `follow` only. Projection: presentation
item id, display name (when available), provider label, observed time.
No provider total, no persistence, no history fetch.

**`latest_subscriber`**: eligible on the "New" subscription family (§6)
only. Same projection shape as latest_follower.

**`latest_donation`**: eligible on `donation`/`youtube.super_chat`/
`youtube.super_sticker`. Projection: display name or "Anonymous" (from
`User.Anonymous`), exact `AmountMicros`+`Currency` (whatever currency
that one event carries — no comparison, so no configured currency
required), optional plain-text message **only when `ShowMessage` is
true** (default false), provider label, observed time. Never payment-rail
fields (never carried by `engagement.Event` in the first place).

**`largest_donation`**: same eligible family as latest_donation, but
requires exactly one configured `Currency`. Only events whose
`Money.Currency` exactly matches are candidates (no FX, ever). Compare
exact integer `AmountMicros`. **Tie rule: a strictly larger amount
replaces the current winner; an equal amount does not.** Runtime-only;
restart clears it. UI wording must say "current session" / "since reset",
never "lifetime largest."

**`recent_supporters`**: bounded list (§7 family), newest first, max
`MaxItems` (1-20, default 5). Each row: presentation item id, display
name/Anonymous, provider, kind-appropriate value (amount+currency for
money, quantity for Bits, "new subscriber"/"gift"/"resub" label for
subscription-family), observed time. No `subscription_gift_batch` rows
(§6/§7). No unbounded growth — oldest evicted past `MaxItems`.

**`event_ticker`**: bounded list (§8 family, filtered further by the
profile's own `EventTypes` allowlist), max `MaxItems` (1-50, default 10).
Built by one provider-independent presentation builder (§11) — a
`subscription_gift_batch` row is a distinct "N viewers gifted subs" batch
summary item, never implying its own recipients are separately counted
anywhere else.

**`session_counter`**: one closed `Metric` (§13), optionally provider/
account filtered. Value is a plain observed count (or, for `support_
amount`, exact micros in one required `Currency`) since backend start or
explicit reset — never a persisted, cross-restart total, and never
confused with a Stage 18A goal.

**`dashboard`**: composes 1-8 existing widget profile ids in a bounded
grid (1-4 columns, bounded row/column spans). References by internal id,
never copies state. No nested dashboards — a `dashboard` referencing a
`dashboard` is rejected outright (prevents cycles entirely, not merely
discourages them). Deleting a widget profile referenced by a dashboard is
rejected (`409 widget_profile_in_use`) until removed from the dashboard
first — the same explicit-rejection discipline Stage 18A already applies
to goal deletion.

## §10. Public route, transport, and DTO

Still exactly one generic route: `GET /api/public/widgets/{slug}/config`
and `GET /api/public/widgets/{slug}/stream`. No per-kind route is ever
added. The stream keeps Stage 18A's exact poll-and-diff mechanism (one
`widget.reset` on connect, a fresh full snapshot only when a 1.5s poll
detects a real change) for every kind, including `dashboard` — a
dashboard's own fingerprint incorporates every child's own fingerprint,
so a child's runtime update is reflected within one poll interval.
**Decision: no push/broadcast notifier is added.** A ticker/latest-item
update becoming visible within ≤1.5s is imperceptible for an OBS overlay
use case, and the existing, already-tested poll mechanism is provably
sufficient — adding a broadcast channel would be complexity with no real
product need (governing task §28's own "do not build an elaborate replay
ring unless there is a real need").

The public DTO becomes a discriminated union by `kind` (`goal` unchanged
for backward compatibility; eight new kinds each get their own bounded
`presentation` shape; `dashboard`'s `presentation` is an array of its
children's own presentation DTOs, keyed by a runtime-generated
presentation id — **never the child's real widget-profile id**). Every
new DTO field is presentation-only: no `providerEventId`, no account/
source id, no user id, no internal DB id, no raw `engagement.Event`, no
provider-specific payload. A message is present only when the profile's
own `showMessage` is true. Empty projections render a defined empty state
(§12), never a 404 — a validly configured widget with nothing observed
yet is still a valid widget, exactly like Stage 18A's own "no goal
progress yet" case.

## §11. Public presentation builder (one place, not scattered JSX)

One provider-independent Go builder
(`internal/supporterwidgets/presentation.go`) turns one matched
`engagement.Event` into one bounded presentation row for whichever kinds
it's eligible for. It never fabricates a placeholder for an unavailable
field (no display name → omit the field, render "Anonymous"/generic
label client-side) and never invents a stable user id — the runtime
presentation item id (§12) is generated fresh, uncorrelated with any
provider identifier.

## §12. Runtime presentation item ids

Every list/latest/largest row needs a stable React key. Generated fresh
per runtime item (crypto-random, mirroring `internal/audio.NewItemID`'s
own approach) — never `providerEventId`, never persisted, never
correlated to a provider's own identifier.

## §13. Session counter metrics (closed set)

`follows`, `new_subscriptions`, `resubscriptions`, `gifted_subscriptions`,
`raids`, `bits_quantity`, `support_event_count`, `support_amount`. The
first six are plain observed counts; `bits_quantity` sums `Quantity`;
`support_amount` requires a configured `Currency` and sums `AmountMicros`
for matching-currency `donation`/`youtube.super_chat`/`youtube.
super_sticker` events only (no FX, no float, exact micros - identical
rule to Stage 18A's own donation-goal currency match). No formula
language, no scripting, no arbitrary field selection — a metric is one of
these eight strings or the request is rejected.

## §14. Runtime reset and restart

`POST /api/widget-profiles/{id}/reset-runtime`: clears that profile's own
runtime projection only (latest→empty, largest→empty, recent
supporters→empty list, ticker→empty list, counter→0). Never publishes an
Engagement Event, never touches Stage 18A goal state, alerts, TTS,
provider connections, or the profile's own persisted configuration.
Restart has the identical clearing effect for every profile
simultaneously, plus Stage 18A goals surviving unchanged (§3) — tested
end to end.

## §15. Filtering and source deletion

Reuses Stage 18A's exact provider/account validation
(`AccountLookup`/`SourceLookupAdapter`) — a Stage 18B profile's `Accounts`
filter must reference a real connected account or donation source at
save time, identical to a goal's own filter validation. **Deleting a
connected account or donation source still referenced by a Stage 18B
profile's filter is rejected** (mirrors the account-deletion-in-use
convention this codebase already applies elsewhere) rather than silently
widening the filter to "any" — silently broadening a filter is a privacy/
correctness regression this task explicitly forbids.

## §16. Profile edit semantics

**Presentation-only** (title, colors/style, orientation, show/hide
toggles, `MaxItems` lowered — existing retained rows are truncated, never
an error): keeps the profile's current runtime projection.
**Semantic** (`Kind`, `Providers`, `Accounts`, `EventTypes`, `Currency`,
`Metric`, dashboard `Children`/`Columns`): the HTTP layer detects the
change (comparing old vs. new profile before/after `UpdateWidgetProfile`)
and calls the runtime manager's `ResetRuntime` for that profile id —
incompatible old projection state is never kept across a semantic edit.

## §17. Ordering, dedupe, and concurrency

Event Bus arrival order (`Sequence`) is authoritative for "newest" in
every list — never a provider timestamp, which can be delayed or skewed.
No second dedupe ledger: Stage 18B trusts the same connector/Bus dedupe
Stage 18A already trusts, plus one in-manager guard (a per-profile "last
applied Bus sequence" check) so an internal retry can never double-apply
the same event to the same profile. Runtime state is memory-only and
resets on restart, so no cross-restart ledger is needed. One event may
legitimately update several profiles independently (a donation can update
`latest_donation`, `largest_donation`, `recent_supporters`, a `donation`-
type `event_ticker`, a `support_amount` counter, *and* a Stage 18A
donation goal, all at once) — no global consume-once behavior.

## §18. SQLite migration (`0026_supporter_widgets.sql`)

Widens `widget_profiles` (SQLite "recreate table" pattern, since SQLite
cannot drop a `NOT NULL`/add a `CHECK` via plain `ALTER TABLE`): adds
`kind` (`NOT NULL DEFAULT 'goal'`, `CHECK` over the 9 kinds), makes
`goal_id` nullable, adds `show_provider`, `show_time`, `show_message`,
`max_items`, `currency`, `metric` (nullable, `CHECK` over the 8 metrics),
`columns`. Adds four new tables: `widget_profile_providers`, `widget_
profile_accounts` (mirror `goal_providers`/`goal_accounts` exactly, same
"no FK on account_id" reasoning as `goal_accounts`), `widget_profile_
event_types` (event_ticker's own allowlist), and `widget_profile_
dashboard_children` (`dashboard_id`, `child_id`, `position`, bounded
`column_start`/`column_span`/`row_start`/`row_span`, `FK child_id →
widget_profiles(id)` with no `ON DELETE` — the repository explicitly
checks for referencing dashboards before allowing a delete, exactly like
goal deletion already does for widget profiles). No column or table in
this migration ever stores event-derived content (§3). A migration-
preservation test proves existing Stage 18A rows survive with `kind =
'goal'` and every new column at its safe default.

## §19. Management API

Extends the existing `/api/widget-profiles` routes rather than adding
seven new route families — the request/response DTO becomes kind-aware
(only the fields relevant to the selected kind are meaningful; the
backend is authoritative regardless of what the frontend sends for an
irrelevant field). Two new routes:

```
POST /api/widget-profiles/{id}/reset-runtime   -> clears runtime only (§14)
GET  /api/widget-profiles/{id}/runtime-status  -> operator-only, current
                                                    runtime projection
```

`runtime-status` is private/operator-side (reachable only from the
authenticated management API, never the public route) but still carries
no raw provider payload/private field beyond what the public DTO would
eventually show — it exists so the operator can see "what will render"
without opening the public URL. Existing error codes/conventions
(`404`/`405`+`Allow`/`409`/`422`/sanitized `500`, strict unknown-field
rejection, bounded bodies) are reused unchanged; `widget_profile_in_use`
(409) is the new dashboard-reference-integrity code, alongside the
existing `goal_in_use`.

## §20. Frontend

The existing "Goals" navigation destination gains **Goals / Widgets /
Dashboards** sections internally; no new top-level nav item. One shared,
kind-dispatching `GoalWidgetRenderer` (renamed in spirit, not necessarily
in code, to reflect it now dispatches by kind) is reused for management
preview and the real public route — never two implementations. Every
"latest"/"largest"/"recent"/counter UI states its runtime-only, restart-
clearing nature in plain, honest copy (§3). Event ticker/session counter
option lists (event types, metrics) are exposed by the backend rather
than duplicated by hand in multiple components. `prefers-reduced-motion`
disables non-essential ticker movement without hiding content or pausing
updates; the ticker itself is a bounded, deterministic item list, never
an unbounded marquee.

## §21. Integration script

`scripts/verify-supporter-widgets.mjs` (the 22nd script) — drives real
events through the same existing Twitch/YouTube/StreamElements fakes,
exactly like `verify-goals-widgets.mjs` already does; no new fake event
source. Full teardown discipline in its own `finally` (every SSE
iterator, every fake server `close()`d, backend process stopped and
waited for, temp directory removed) — the two Stage 18A closing-
regression defects (a stale-event race, and unclosed fake servers
preventing a clean process exit) must never repeat here; verified during
development by direct process inspection (no lingering `node`/
`testserver`/fake-provider process after a standalone run).

## §22. What Stage 18B does not implement

No free-form widget designer, no widget template/package format, no
`visualdesign.Document` involvement, no nested dashboards, no push/
broadcast SSE notifier, no second dedupe ledger, no cross-currency
conversion anywhere, no persisted event-derived content, no per-kind
public route. TikTok LIVE (Stage 19) and the updater (Stage 20) are
untouched.
