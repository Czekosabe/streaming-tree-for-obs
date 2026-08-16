# Stage 18A — persistent goals/counters foundation + core OBS goal widgets

This is the canonical Stage 18A contract, written before any product code in
this milestone, exactly as `docs/alert-audio.md` preceded Stage 17B. It
records the primary-source audit this milestone's design rests on (real
`internal/domain/engagement` model, real Twitch/YouTube/StreamElements
normalizers, real Event Bus semantics, real persistence/overlay
conventions), the exact split between Stage 18A and Stage 18B, and the
runtime/data contracts every later commit in this milestone implements
against.

Stage 18A closes with Stage 18 as a whole still **incomplete** - Stage 18B
is deliberately not started by this milestone. See §0.2.

## 0.1 Research method

Every design decision below was checked against this repository's own real,
already-shipped source - never against an assumed or aspirational event
model. Specifically read in full before writing this contract:
`internal/domain/engagement/{event,types,money,user,message,validation}.go`,
`internal/engagement/{bus,dedupe,subscription,buffer}.go`,
`internal/provider/twitch/eventsub_normalize.go`,
`internal/provider/youtube/livechat_normalize.go`,
`internal/provider/streamelements/normalize.go`,
`internal/domain/alerts/{model,capability,validation}.go` and
`internal/alerts/wiring.go` (the provider/source-filter and
capability-table precedent this milestone reuses), `internal/domain/
chatoverlay/model.go` (the closed-style-enum precedent for a
non-designer overlay), the SQLite migration/repository conventions
(`internal/storage/sqlite/migrations/0013_alerts.sql`,
`0018_visual_assets.sql`, `0020_donation_sources.sql`,
`0022_audio_assets.sql`, `database.go`'s pragma configuration), and the
per-slug public SSE pattern in `internal/httpapi/chatoverlay.go`. Where a
fact below is a genuine, non-obvious finding from that reading (not
something the aspirational planning docs already stated), it is cited by
file and reasoning, not just asserted.

## 0.2 Stage 18 split

**Stage 18A** (this milestone):

- one provider-independent, persistent goal/counter accumulation engine
  consuming the existing Engagement Event Bus at current position;
- four closed goal kinds: followers, subscriptions, donations, Bits;
- explicit operator baseline/current-value management (§2);
- persisted accumulated goal state, surviving backend restarts;
- one provider-independent contribution capability table (§4) - the only
  place contribution rules are decided;
- durable, per-goal duplicate-contribution protection (§8);
- real public OBS Browser Source goal-widget profiles, one generic
  `/overlay/widgets/{publicSlug}` route (§12);
- a real Goals management page;
- the 21st integration script, `scripts/verify-goals-widgets.mjs`.

**Stage 18B** (deliberately not started here):

- latest follower / latest subscriber / latest donation / largest
  donation widgets;
- recent-supporters list, event ticker;
- richer/platform-specific counters;
- multi-widget composition/dashboards;
- any dedicated visual designer or template-package integration for
  widgets, if justified once the foundation in this document exists.

**Why the split exists**: mirrors Stage 17A/17B exactly - a widget that
merely *displays the last event* is a different, simpler problem than a
widget that *accumulates state durably and safely across a backend
restart*. Building the accumulation foundation and exactly the four
goal kinds that need it first means Stage 18B's "latest X" widgets (which
need no persistent accumulator at all, only the existing Event Bus tail)
can be built later without having to retrofit dedupe/persistence/
concurrency correctness onto something already shipped.

## 1. Fundamental semantic rule: observed goals, not provider totals

Streaming Tree is a local-first application that only ever sees the
Engagement Event Bus's own normalized event stream (§5). It never has,
and Stage 18A never adds, a provider API client that fetches "current
follower count" or "lifetime subscriber count" as a single authoritative
number. Therefore every Stage 18A goal means exactly:

**"events this application has observed since this goal's own configured
baseline/start"**

and never:

- current total followers on Twitch;
- total active subscriptions on YouTube;
- lifetime donation amount reported by a provider dashboard;
- any provider-canonical total.

No hidden provider API fetch, scraping, or retrospective provider-history
request is introduced anywhere in this milestone. Every UI label and every
doc sentence describing a goal's `current` value uses honest wording
("observed progress", "events observed since baseline" - never "current
followers" or "total subscribers").

An operator who already knows their real current standing (e.g. "the
channel has 825 followers today") sets that as the goal's own
**baseline/current** value at creation time (§7). The application never
invents this number itself - see §2's worked example, and §7 for the
persisted model.

## 2. Goal kinds

Stage 18A supports exactly four closed goal kinds. No arbitrary formula
language, no expression parser, no user scripting, ever. An unknown future
kind is rejected by validation, not silently accepted.

```go
type Kind string

const (
    KindFollowers     Kind = "followers"
    KindSubscriptions Kind = "subscriptions"
    KindDonations     Kind = "donations"
    KindBits          Kind = "bits"
)
```

Worked example (follower goal): an operator knows the channel currently
has 825 followers. They create a follower goal with `target = 1000` and
set `baseline = current = 825` (§7, §9). From that moment, every newly
observed real `follow` event increments `current` by 1. The application
never fabricates the initial 825 and never reaches out to Twitch to
confirm it.

## 3. Contribution capability table

One provider-independent table, keyed by `internal/domain/goals`'s own
`Type` enum - a small, closed set of string constants mirroring, as plain
literals, the real `internal/domain/engagement.Type` values this
application's connectors actually produce (`goals.TypeFollow =
"follow"`, and so on). This is not a stylistic choice: it follows this
codebase's own explicit, already-established architectural rule, stated
verbatim in `internal/domain/alerts/model.go`'s own package doc comment -
*"This package never imports internal/domain/engagement,
internal/provider/twitch, or any other domain package's concrete types -
it declares its own narrow, primitive-typed ProviderID/EventType, exactly
like every other domain package in this project... no provider-id or
event-type type is shared across domain packages here."* `internal/
domain/goals` follows the identical rule for the identical reason -
`internal/domain/alerts`, `internal/domain/chatoverlay`, and
`internal/domain/chatautomation` all already do this, and Stage 18A does
not introduce the first exception. Only the runtime layer,
`internal/goals.Manager` (which - like `internal/alerts` and
`internal/audio` - does import `internal/domain/engagement`, since it is
the thing that actually reads events off the Bus), maps a real
`engagement.Event.Type` to this package's own `goals.Type` before
calling `goals.ContributionFor`.

`internal/domain/goals.ContributionFor(Type) Contribution` mirrors
`internal/domain/alerts.CapabilityFor` exactly in spirit: never scattered
contribution logic in HTTP/frontend/provider code, and an unknown/future
`Type` value contributes nothing (the zero `Contribution`) until this
table is explicitly extended - a future provider event can never
unexpectedly change persisted goal state.

```go
type Contribution struct {
    Followers     bool  // contributes exactly 1 to a follower goal
    Subscriptions bool  // contributes exactly 1 to a subscription goal
    Money         bool  // contributes evt.Money.AmountMicros to a donation goal (same currency only)
    Bits          bool  // contributes evt.Quantity to a Bits goal
    // QuantityIsCount: when Bits is true, whether evt.Quantity is itself
    // the exact contribution (always true for TypeBits today - reserved
    // so a future Bits-adjacent type cannot be added without an explicit
    // decision here).
}
```

Full table, decided from the real normalizers audited in §0.1 (only the
four goal-relevant columns are meaningful; every `goals.Type` value below
mirrors the identically-named real `engagement.Type` value as a plain
string literal, per §3's own architectural note; every other
`engagement.Type` not listed here has no `goals.Type` counterpart at all
and contributes nothing to any Stage 18A goal kind, including
`chat.message`, `chat.message_deleted`, `chat.cleared`, `moderation`,
`raid`, `channel_point_redemption`, `stream.online`, `stream.offline`):

| `goals.Type` | Followers | Subscriptions | Money | Bits | Reasoning |
| --- | --- | --- | --- | --- | --- |
| `follow` | **yes (+1)** | | | | The only follow-shaped event; see §5. |
| `subscription` | | **yes (+1)** | | | New, non-gift subscription (Twitch `channel.subscribe`, `is_gift=false`). |
| `resubscription` | | **no** | | | Deliberately excluded - see §6.1. |
| `gifted_subscription` | | **yes (+1)** | | | One gift *recipient* (Twitch `channel.subscribe` with `is_gift=true`, or YouTube `giftMembershipReceivedEvent`) - counted per recipient, never per batch. |
| `subscription_gift_batch` | | **no** | | | Deliberately excluded - counting this AND the batch's own individual `gifted_subscription` events would double-count the same gift operation. See §6.2. |
| `bits` | | | | **yes** (`evt.Quantity`) | Twitch `channel.cheer`'s exact Bits count. |
| `youtube.membership` | | **yes (+1)** | | | YouTube's direct non-gift new-membership event (`newSponsorEvent`) - the YouTube equivalent of `subscription`. |
| `youtube.membership_milestone` | | **no** | | | Deliberately excluded - an *existing* member's milestone chat, not a new subscription event. Same reasoning as `resubscription` (§6.1). |
| `youtube.super_chat` | | | **yes** (`evt.Money`) | | Real provider-reported monetary amount. |
| `youtube.super_sticker` | | | **yes** (`evt.Money`) | | Real provider-reported monetary amount. |
| `donation` | | | **yes** (`evt.Money`) | | External donation (StreamElements today). |

Every event type in this table that contributes `Money` requires the
event's own `evt.Money.Currency` to case-sensitively equal (after the
engagement model's own uppercase normalization, §0.1) the goal's single
configured currency (§7.2) - a different-currency event contributes zero,
never a converted amount (§7.2, no FX anywhere in this codebase).

## 4. Follower semantics

- `follow` contributes exactly `+1`.
- A synthetic event (`evt.Synthetic == true`) never contributes (§10).
- No `User`/`ProviderUserID` is required for a follow event to
  contribute - `normalizeFollow` (`internal/provider/twitch/
  eventsub_normalize.go`) always populates `User`, but the goals manager
  does not require it as a precondition.
- **No unfollow subtraction.** `engagement.KnownTypes` (`internal/domain/
  engagement/types.go`) has no unfollow-shaped type today, and no
  connector normalizes one. A follower goal's `current` value is honestly
  "observed follow events since baseline," never "current follower
  count," and every UI label says so (§1).

## 5. Subscription semantics

### 5.1 Continuing subscriptions are excluded

`resubscription` (Twitch `channel.subscription.message`) and
`youtube.membership_milestone` (YouTube `memberMilestoneChatEvent`) are
both **excluded** from subscription-goal contribution. Both represent an
**already-counted, continuing** subscription/membership being reaffirmed
- not a new incremental subscription event. Counting them would inflate
an "observed subscription events" goal in a way that does not match the
common streamer mental model of a sub goal (new + gifted subs count,
renewals do not), and there is no reliable way to tell "this resub is
from someone already counted toward this goal" from "this resub is from
someone who subscribed before this goal's own baseline" without a user-
identity ledger this milestone deliberately does not build (§8 dedupe is
about not-double-counting *the same event*, not about tracking every
distinct subscriber identity per goal). This is a documented, deliberate
choice, not an oversight.

### 5.2 Gift batch vs individual gift: the no-double-count rule

**Confirmed in this repository's own canonical architecture doc**
(`docs/engagement-architecture.md`, §5.4 mapping table): *"`subscription_
gift_batch` — A gifter giving N subs at once — kept distinct from N
individual `gifted_subscription` events so a rule can target 'gifted 5
subs' as one occurrence, not five."* This is an explicit statement that
Twitch (and, via the intentional type reuse in `docs/provider-
integrations/youtube-engagement.md`, YouTube's `membershipGiftingEvent`/
`giftMembershipReceivedEvent` pair) delivers **both** the batch summary
event **and** one individual event per recipient, for the same underlying
gift operation.

Per this task's own instruction ("prefer a rule that cannot double-count
even if that means intentionally ignoring a summary form"), Stage 18A's
canonical, safe rule is:

**A subscription goal counts only individual-recipient events
(`gifted_subscription`, `+1` each) - it never adds
`subscription_gift_batch`'s own `Quantity`.** The batch event contributes
zero. This can never double-count, because it only ever reads one event
family for gifted subs, never two representations of the same operation.
The one accepted cost: if a connector or provider behavior ever changed
such that a batch event arrived **without** its individual recipient
events, those gifted subs would not be counted - an intentional,
documented trade-off in favor of correctness over completeness, exactly
as this task's instructions require.

### 5.3 Full subscription contribution summary

| Event | Contributes? | Amount |
| --- | --- | --- |
| `subscription` (new, non-gift) | yes | `+1` |
| `resubscription` | no | - (§5.1) |
| `gifted_subscription` (one recipient) | yes | `+1` |
| `subscription_gift_batch` (summary) | no | - (§5.2, prevents double-count) |
| `youtube.membership` (new, non-gift) | yes | `+1` |
| `youtube.membership_milestone` | no | - (§5.1) |

Tests (§17) prove directly: a Twitch gift-batch-of-5 scenario (one
`subscription_gift_batch` with `Quantity=5` plus five `gifted_
subscription` events) increments a subscription goal by exactly 5, never
10 and never 0.

## 6. Donation semantics

Eligible event families (§3): `donation` (StreamElements today),
`youtube.super_chat`, `youtube.super_sticker` - the only three
`engagement.Type` values that `internal/provider/*/*.go` ever attaches a
real `Money` value to (confirmed directly by grepping every `.Money =`
assignment across every provider package, §0.1). No event is included
merely because its name sounds monetary (e.g. `channel_point_redemption`
is never eligible - it carries a points cost, never a `Money` value).

Each donation goal has **exactly one configured currency** (an uppercase,
provider-style code, e.g. `USD`), set at creation:

- a same-currency monetary event contributes its exact
  `AmountMicros` (integer, never rounded, never floated);
- a different-currency event contributes **zero** - never converted, never
  summed as if currencies were interchangeable ("USD and EUR are both 5"
  is explicitly rejected by this task's own instruction and by this
  codebase's own standing rule, `internal/domain/engagement/money.go`:
  *"This application never converts between currencies... There is no
  exchange-rate table anywhere in this codebase, and none should ever be
  added"*);
- the goal's manual baseline/current (§9) is set in the same exact
  currency - the API rejects a mismatched currency on a manual-value
  request;
- **a goal's currency cannot silently change** once created. Changing it
  requires an explicit reset/reconfigure flow: `PUT /api/goals/{id}`
  rejects a currency change unless the request also resets `current` to
  the new baseline in the new currency in the same call (§9.3) - a goal
  can never end up with a `current` value whose currency provenance is
  ambiguous.

## 7. Bits semantics

`bits` (Twitch `channel.cheer`) contributes its exact `evt.Quantity` -
an integer count of Bits, never converted to money, never assumed to be
"$0.01 per Bit" or any other cents-per-Bit rate (Twitch's own Bits-to-
payout rate is not published to a broadcaster's API consumer and this
application never guesses one). `currentBits`/`targetBits` are both plain
integers.

## 8. Baseline/current/target model

```go
type Goal struct {
    ID        string
    Name      string   // 1-80 code points, mirrors alerts.MaxNameCodePoints
    Kind      Kind
    Enabled   bool

    Target    int64    // > 0, required; see bounds below
    Current   int64    // accumulated observed value; may exceed Target (§13)
    Baseline  int64    // the value Current was set to at creation/last reset

    Currency  string   // required and immutable-except-via-reset when Kind == KindDonations; empty otherwise

    Providers []engagement.ProviderID // empty == any provider
    Accounts  []string                // empty == any account/source; connected_account id or donationsource id (§14)

    CreatedAt time.Time
    UpdatedAt time.Time   // bumped by ANY change: config edit or contribution
    StartedAt time.Time   // set at creation, refreshed by an explicit Reset (§9.2) - never by restart

    ConfigRevision int64  // optimistic concurrency for PUT edits only (§8.1)
}
```

### 8.1 Two independent revision-like fields, on purpose

`ConfigRevision` guards only operator *configuration* edits (`PUT
/api/goals/{id}`: name/target/kind/currency/filters/enabled) via
optimistic concurrency (mirrors `visual_designs.revision`,
`internal/storage/sqlite/visualdesign_repository.go`). It is **never**
touched by contribution application. This is a deliberate design choice:
if contribution application also bumped/required `ConfigRevision`, a
real-time event arriving while an operator's edit form was open could
spuriously conflict with, or silently invalidate, that in-flight edit's
optimistic-concurrency check. `UpdatedAt` still changes on every
contribution (for observability/"last updated" display), but no revision
counter is checked or incremented for it.

### 8.2 Bounds

- `Target`: `1 <= Target <= MaxGoalCountValue` (count goals) or `1 <=
  Target <= MaxGoalAmountMicros` (donation goals). Zero and negative are
  both rejected (§13, §27 of the task spec: "target cannot be zero").
- `Current`/`Baseline`: `0 <= value <= MaxGoalCountValue` /
  `MaxGoalAmountMicros`. Never negative (an operator cannot set a goal
  "behind zero").
- `MaxGoalCountValue = 100_000_000` (100 million) - for followers,
  subscriptions, and Bits goals. Astronomically above any realistic
  target while leaving vast headroom under both Go's `int64` and
  JavaScript's `Number.MAX_SAFE_INTEGER` (9,007,199,254,740,991) - see
  §8.3.
- `MaxGoalAmountMicros = 100_000_000_000_000` (1e14 micros = 100,000,000
  major currency units) - mirrors `engagement.maxAmountMicros`'s own
  reasoning (`money.go`: "well above any realistic amount... while still
  leaving vast headroom") applied to an *accumulated* total rather than a
  single event. ~90x headroom remains under
  `Number.MAX_SAFE_INTEGER` even at this bound.

### 8.3 JavaScript safe-integer transport

This codebase's own established convention (`internal/httpapi/{alerts,
engagement,chatoverlay,operatorchat}.go`) already transports every
`AmountMicros`/quantity field as a **plain JSON number** (`*int64`),
never a string, and already documents why that is safe: every persisted
per-event bound (`engagement.maxAmountMicros = 1_000_000_000_000`) sits
far under `Number.MAX_SAFE_INTEGER`. Stage 18A follows the exact same
convention for `Target`/`Current`/`Baseline`, and picks the bounds in
§8.2 specifically so the *accumulated* values stay just as safely inside
that limit - `MaxGoalAmountMicros` (1e14) leaves ~90x headroom, and
`MaxGoalCountValue` (1e8) leaves ~9x10^7 headroom. No new string-integer
transport convention is introduced; this document records the audit
explicitly rather than silently relying on it (per this task's own
instruction).

## 9. Manual adjustment

Because Streaming Tree cannot discover a complete historical baseline
automatically (§1), the operator needs explicit management actions.

### 9.1 Set current value

`POST /api/goals/{id}/set-current {"current": <int64>}` (and, for a
donation goal, the request's implied currency must match the goal's own -
no currency field is accepted here, since it can never change outside
§9.3). Persists immediately. Does not change `Baseline` or `Target`.

### 9.2 Reset progress

`POST /api/goals/{id}/reset` sets `Current = Baseline` (the value most
recently established at creation or by the last explicit reset/
reconfigure) and refreshes `StartedAt` to now. Does not change `Target`.

### 9.3 Reconfigure (baseline + currency change)

`PUT /api/goals/{id}` may set a **new** `Baseline` and, for a donation
goal, a new `Currency` in the same request - in that case `Current` is
also set to the new `Baseline` atomically in the same transaction (§8.2's
"cannot silently change" rule), and `StartedAt` refreshes. A `PUT` that
does not touch `Baseline`/`Currency` leaves `Current` untouched.

### 9.4 What manual actions never do

- never publish a fake Engagement Event (a manual action is data-layer
  only - it never re-enters the Bus, so it can never be mistaken for a
  real observed contribution later);
- never mutate any provider/connector state;
- never create an alert or a TTS event (Stage 18A intentionally does not
  wire goal completion to alerts/audio - see §13 and the task's own §48
  exclusion list: "goal completion alerts" is explicitly Stage 18B-or-
  later, not decided by this milestone at all);
- a goal continues accumulating real observed contributions immediately
  afterward, exactly as before the manual action.

## 10. Event Bus subscription semantics

The goals manager (`internal/goals.Manager`) subscribes to the Event Bus
at **current position, zero replay** - by snapshotting the bus's own
`NewestSequence` first and calling `Subscribe(snap.NewestSequence)`,
exactly mirroring `internal/alerts.Manager`'s own identical reconnect
logic. This is not a stylistic choice: `Bus.Subscribe(after)` calls
`ring.after(after)` **unconditionally**, and `ring.after(0)` returns
every retained event with `Sequence > 0` - i.e. everything still in the
ring - so `Subscribe(0)` actually **replays retained history**, the
opposite of "current position." This was confirmed the hard way, by a
manager test that published an event before the manager ever
subscribed and asserted it was never applied - the test genuinely
failed against a first implementation that called `Subscribe(0)`
directly, which is exactly why this document now states the mechanism precisely
rather than the shorthand "Subscribe(after=0)" an earlier draft of this
section used. There is no historical catch-up in Stage 18A.

This matters for two concrete, tested scenarios:

- **Backend/manager restart never double-applies an already-observed
  event.** The Bus itself is confirmed in-memory-only and lost on
  restart (`internal/engagement/bus.go`'s own package doc: *"Nothing in
  this package ever writes to SQLite, and every retained event is lost,
  by design, on a backend restart"*) - so after a restart there is
  nothing retained to replay even if the manager asked for it. The
  manager's own durable dedupe ledger (§11) is what actually protects
  against a *provider* redelivering an event after restart, not Bus
  replay.
- **A goal created while the server is already running never
  retroactively consumes an event published before its own creation.**
  The manager matches every incoming live event against the *current*
  set of enabled goals at the moment the event arrives - a goal that did
  not exist yet was never in that set.

## 11. Durable duplicate protection

### 11.1 What identifier is actually available, audited per provider

This is the single most important, non-obvious finding of this
milestone's audit (§0.1), and it directly shapes the dedupe design:

| Provider | Goal-relevant `engagement.Type` | `ProviderEventID` | `DedupeKey` |
| --- | --- | --- | --- |
| Twitch | `follow`, `subscription`, `gifted_subscription`, `subscription_gift_batch`, `bits` | **always empty** - `internal/provider/twitch/eventsub_normalize.go`'s `base()` never sets it; only `channel.chat.message` and the channel-point-redemption normalizer set it, neither goal-relevant | always set, to the EventSub delivery's own `metadata.message_id` |
| YouTube | `youtube.membership`, `gifted_subscription` (gift-received), `subscription_gift_batch` (gift-batch), `youtube.super_chat`, `youtube.super_sticker` | **always set** - `internal/provider/youtube/livechat_normalize.go`'s `base()` sets `ProviderEventID: msg.ID` for every event | always set, to the same `msg.ID` |
| StreamElements | `donation` | **always set** - `internal/provider/streamelements/normalize.go` sets `ProviderEventID: tip.ID` | always set, to the same `tip.ID` |

So: YouTube and StreamElements goal-relevant events already carry a
genuine, stable, provider-assigned business-event identifier. **Twitch's
own goal-relevant events do not** - only the EventSub *delivery's*
`message_id` is available, which identifies "this specific redelivered
notification," not necessarily "this specific underlying business event"
in every conceivable edge case (see §11.3).

### 11.2 The durable dedupe key

```go
func dedupeIdentity(evt engagement.Event) (key string, ok bool) {
    id := evt.ProviderEventID
    if id == "" {
        id = evt.DedupeKey
    }
    if id == "" {
        return "", false
    }
    return string(evt.ProviderID) + "|" + evt.ConnectedAccountID + "|" + id, true
}
```

Persisted per accepted contribution, scoped **per goal** (§11.4, never
globally): `(goal_id, provider_id, account_id, provider_event_key)` is
the ledger's primary key. Never dedupe on display name, message text,
amount, timestamp, donor email, or any hash of a guessed field
combination - every one of those is explicitly forbidden by this task's
own instruction, and none of them is a provider-assigned identity.

### 11.3 Residual limitation (documented honestly, never hidden)

For Twitch's goal-relevant events, the durable key is the EventSub
delivery's own `message_id`, not a guaranteed-unique-forever business-
event id. Twitch's own EventSub contract redelivers a *failed* delivery
with the *same* `message_id` (which is exactly what this key protects
against), but this application has no official guarantee that an
extremely delayed re-subscription after a very long outage could never
produce a fresh `message_id` for what Twitch itself would consider "the
same" underlying follow/subscription/gift/cheer. This is a real,
documented residual limitation - not a heuristic fingerprint pretending
to be exact, and not silently ignored. YouTube and StreamElements do not
share this limitation, since their own `ProviderEventID` is a genuine
business-event id, not a delivery id.

### 11.4 Dedupe is per-goal, never global

Two independently configured goals may legitimately both match the same
real event (§15). Dedupe granularity is `(goal_id, ...)`, so goal A's own
duplicate-application check never suppresses goal B's independent,
equally-legitimate first application of the same event. See §12 for the
transaction shape that keeps this correct under concurrency.

### 11.5 Bounded retention

The applied-event ledger is pruned, never allowed to grow forever.
Retention bound: rows older than the same 5-minute window the Bus's own
in-memory publish-time dedupe already uses as its worst-case redelivery
assumption (`internal/engagement/bus.go`'s `defaultDedupeTTL`) would be
too short for a *durable*, cross-restart guarantee, so Stage 18A uses a
longer, still-bounded retention: **30 days**, pruned lazily (on manager
startup and periodically) by `applied_at < now - 30d`. This comfortably
outlasts any realistic connector reconnect/backoff window while keeping
the table's growth bounded by time rather than unbounded by event count.

## 12. Atomic persistence

Applying one accepted event to the goals it matches is transactional,
per matching goal:

```
BEGIN
  INSERT INTO goal_applied_events (goal_id, provider_id, account_id, provider_event_key, applied_at)
    VALUES (...) -- fails with a unique-constraint violation if already applied for this goal
  UPDATE goals SET current_value = current_value + ?, updated_at = ? WHERE id = ?
COMMIT
```

A unique-constraint violation on the ledger insert means "already
applied for this goal" - the transaction is rolled back cleanly (no
partial state: never a ledger row without a matching increment, and
never an increment without its ledger row, satisfying this task's own
crash-safety requirement). One real event that matches N enabled goals
runs N independent such transactions, so one goal's own dedupe outcome
never affects another goal's independent application (§11.4).

**Concurrency correctness**: the goals manager runs a single dedicated
consumer goroutine draining its own Bus subscription channel, applying
one event at a time, in order - so contribution application is already
serialized at the application level. The atomic `current_value =
current_value + ?` SQL form (never a Go-level read-then-write) additionally
guarantees correctness even if a manual `set-current`/`reset` API call
(§9) races a concurrent contribution from a different goroutine - SQLite's
own WAL mode + `busy_timeout` (`internal/storage/sqlite/database.go`)
serializes the two writers safely rather than losing one. Tests (§17)
prove exact totals under concurrent contribution application.

## 13. Goal completion

A goal reaching or exceeding `Target`:

- keeps accumulating - `Current` is never clamped to `Target` in the
  persisted value;
- never auto-deletes, auto-resets, or triggers any provider action;
- never triggers an alert or TTS event (out of scope for this milestone,
  §9.4).

Public presentation (§16) exposes `completed: true` once `Current >=
Target`. The **visual progress bar** may clamp its own rendered fill to
100% while the **textual current value** still shows the real,
potentially-over-target number - a presentation-only clamp, never a
persisted one.

## 14. Provider/source filters

Reuses `internal/alerts/wiring.go`'s own `AccountLookupAdapter` pattern
exactly (§0.1): `Goal.Accounts` may hold either a `connected_accounts` id
or a `donationsource` id, both validated for existence by a small
`SourceLookupAdapter{ Accounts *account.Service; DonationSources
*donationsource.Service }` combined adapter with the same
`AccountExists(ctx, id) (bool, error)` shape.

SQLite: `goal_accounts(goal_id, account_id)` carries **no table-level
foreign key** on `account_id` - mirroring `internal/storage/sqlite/
migrations/0020_donation_sources.sql`'s own correction to
`alert_rule_accounts` for exactly the same reason (an id may reference
either of two different tables; only application-layer validation, never
a single-table SQL foreign key, can express that). `goal_providers
(goal_id, provider_id)` has no foreign key either, mirroring
`alert_rule_providers`.

An empty `Providers`/`Accounts` list means "any" (matches every enabled
goal against every provider/account by default) - never "matches
nothing," mirroring `alert_rule_providers`'s own documented convention.
The frontend never decides provider/account compatibility on its own;
`internal/domain/goals`'s own validation is authoritative, exactly like
`internal/domain/alerts.ValidateAccounts`/`ValidateProviders`.

## 15. Multiple matching goals

One real event may legitimately contribute to several independently
configured goals at once (e.g. two active USD donation goals with
different targets both increment from the same donation). Goal ids fully
isolate persisted state and dedupe ledgers (§11.4); there is no global
"consume once and stop" behavior anywhere in this design.

## 16. Synthetic events

`evt.Synthetic == true` is always ignored - checked first, before
matching against any goal or writing anything. Per this milestone's own
audit (§0.1): **no current code path actually publishes a
`Synthetic: true` event to the real Event Bus.** Both existing "test"
triggers construct their own synthetic `engagement.Event` value but never
call `Bus.Publish` with it:

- Alert "Test Rule" (`internal/alerts/testevents.go`'s
  `buildFixtureEvent`, doc comment: *"never published to the real
  Engagement Event Bus"*) feeds its fixture directly into the alert
  matcher;
- TTS "Test Speak" (`internal/audio/manager.go`) sets `Synthetic: true`
  only on its own internal `audio.Item`, an entirely different type that
  never touches `engagement.Event` or the Bus at all.

The goals manager's own `Synthetic` check is therefore genuinely
defense-in-depth against a **future** code path (the `engagement.Event.
Synthetic` field's own doc comment already anticipates one: *"exists so
the model is ready for the preview/test events... without requiring a
second, parallel event type later"*) - not something reachable today.
Tests (§17) still cover it directly, by publishing a `Synthetic: true`
event to a real `Bus` in a unit test and asserting no goal moves, exactly
because the check must hold regardless of whether today's specific
Test-Rule/Test-Speak code paths happen to reach it.

## 17. Test plan summary

Full enumeration lives in the implementation commits' own test files;
this section records the shape only (exhaustive Go unit-test scenarios
stay in Go tests, the 21st integration script stays a representative
end-to-end subset - the same "representative subset, not literal
transcription" precedent `scripts/verify-tts-audio.mjs` already
established). Coverage spans: goal validation (all four kinds, bounds,
currency rules), the contribution table (every `engagement.Type`, both
included and explicitly-excluded ones), the gift no-double-count proof
(§5.2/5.3), YouTube membership behavior, exact-micros donation handling
(including cross-currency rejection), Bits exact quantity, synthetic
isolation, persistence/restart survival, concurrent-contribution
exactness, durable dedupe (in-process duplicate, cross-restart duplicate
where a stable id exists, two genuinely different provider ids both
counting), current-position-only Bus subscription (no retroactive
consumption), provider/source filtering, multiple-goal fan-out, goal
completion/over-target retention, and the full public widget-profile/SSE
surface (§18-§20) including privacy-boundary leak scans.

## 18. Public widget profile model

```go
type WidgetProfile struct {
    ID         string
    GoalID     string
    Name       string  // 1-80 code points
    Enabled    bool
    PublicSlug string  // high-entropy, rotatable - mirrors every existing overlay slug (chat, alert, audio)

    TitleOverride string // optional; falls back to the goal's own Name when empty
    ShowCurrent   bool
    ShowTarget    bool
    ShowPercent   bool

    Orientation HorizontalOrientation // closed enum: horizontal | vertical
    TextAlign   TextAlignment         // closed enum: left | center | right
    FontFamily  FontFamily            // closed enum: sans_serif | serif | monospace | rounded - own small type, mirrors internal/domain/chatoverlay.FontFamily's identical closed-list precedent rather than importing that unrelated domain

    BackgroundColor string // bounded hex color
    ForegroundColor string // bounded hex color
    FillColor       string // progress-bar fill, bounded hex color
    BorderColor     string // bounded hex color
    BorderRadiusPx  int    // 0-32
    Opacity         float64 // 0.0-1.0

    CreatedAt time.Time
    UpdatedAt time.Time
}
```

One goal may have zero or more widget profiles. No arbitrary CSS, no
custom font upload, no remote URL, no uploaded image/video/audio, no
`visualdesign.Document` involvement, and no bump to
`visualdesign.Document.Version` - Stage 18A deliberately stays outside
the free-form designer entirely (task §28). Stage 18B may decide whether
generic widget design/template reuse is worth adding once real widgets
exist in the product.

**Goal deletion policy**: deleting a goal that still has one or more
widget profiles is **rejected** (`409`, `goal_in_use`) rather than
cascading - chosen explicitly over cascade delete because a widget
profile is itself a *published, embedded-in-OBS* artifact; silently
invalidating a live Browser Source URL as a side effect of an unrelated
goal-management action is a worse operator surprise than an explicit,
actionable error asking them to delete the widget profile(s) first. This
mirrors the same tension `docs/alert-audio.md` and `docs/visual-
template-packages.md` resolve the same way for their own reference-
tracked resources; tests (§17) prove the rejection.

## 19. Public route and protocol

One generic, future-compatible route: `GET /overlay/widgets/{publicSlug}`
(never a new route shape per Stage 18B widget kind later - a `kind` field
in the config payload distinguishes them). Stage 18A's own `WidgetProfile.
Kind` is always `"goal"` today; a future Stage 18B kind extends the same
enum, never a parallel route.

```
GET /api/public/widgets/{slug}/config   -- presentation config snapshot (not SSE)
GET /api/public/widgets/{slug}/stream   -- SSE: goal snapshot only
```

**Implementation note (simplified from an earlier draft of this
section):** an earlier draft of this section described reusing
`internal/httpapi/chatoverlay.go`'s full Last-Event-ID/ring-buffer/gap
machinery verbatim. That machinery exists to let a chat-overlay client
replay a *sequence of discrete items* (upserts/removes) it may have
missed. A goal widget never has that problem - §19 already establishes
that this stream carries **only the current snapshot, never a delta
sequence or event history** - so replaying "missed" snapshots is
meaningless: the only snapshot that ever matters is the latest one, and
a fresh connection always gets exactly that. Implemented instead as: on
every fresh connection, send one `widget.reset` with the goal's current
snapshot immediately; a lightweight internal poll (~1.5s) re-reads the
goal and its widget profile and sends a new `widget.reset` (with an
incrementing, connection-local `revision`) only when something actually
changed; periodic SSE keepalive comments; and a bounded live-client
count per slug (mirrors `maxChatOverlaySSEClientsPerOverlay`). No
Last-Event-ID handling and no `widget.gap` event - there is nothing to
gap on when every message is already a complete, self-sufficient
snapshot. Rotating a profile's `publicSlug` invalidates the old URL
immediately - mirrors every existing overlay's rotation behavior.

**Stage 18A's own stream sends the current goal snapshot only** - never
raw Engagement Events, never a queue, never event history. A widget
client that reconnects gets a fresh `widget.reset` with the goal's
current state; it never needs to reconstruct history from a sequence of
deltas.

The public page itself (`PublicWidgetPage`, no `AppShell`, no
navigation, no management controls): no arbitrary HTML/CSS/JS, no
provider credentials, no internal database ids beyond what's unavoidable
for the DTO shape (§20), no source/account ids, no `providerEventId`, no
user identity anywhere in a Stage 18A goal-bar payload.

## 20. Public goal DTO and privacy boundary

```json
{
  "revision": 42,
  "kind": "goal",
  "goalKind": "donations",
  "title": "New PC Fund",
  "currency": "USD",
  "current": 15230000,
  "target": 50000000,
  "progressBasisPoints": 3046,
  "completed": false,
  "presentation": { "showCurrent": true, "showTarget": true, "showPercent": true,
                     "orientation": "horizontal", "textAlign": "center",
                     "fontFamily": "sans_serif", "backgroundColor": "#00000080",
                     "foregroundColor": "#ffffff", "fillColor": "#7c3aed",
                     "borderColor": "#ffffff33", "borderRadiusPx": 12, "opacity": 1.0 }
}
```

Never exposed: `providerEventId`, any account/source id, any provider
user id, any internal dedupe-ledger content, database file paths, applied-
event history, or any private user/donor data. `revision` is the widget
profile's own SSE sequence number (mirrors every other overlay's
`Sequence`), not a database row id.

Money values in the public DTO stay exact integer micros (`current`/
`target` above are `AmountMicros` for a donation goal, plain integer
counts for the other three kinds) - never a floating-point-derived
display string computed by summing floats, and never re-derived by any
downstream renderer from anything other than the exact persisted
integer.

## 21. Progress representation

Server-side arithmetic stays integer throughout - no persisted float
percentage anywhere. `progressBasisPoints` is `0..10000` representing
`0.00%..100.00%` (and may report **more than 10000** while `Current >
Target`, letting the client decide whether to clamp its own rendered bar
- see §13). Computed as `min(Current, MaxGoalAmountMicros or
MaxGoalCountValue) * 10000 / Target` using integer division, with
`Target` already guaranteed `> 0` by validation (§8.2), so no division-
by-zero path exists.

## 22-23. Visual scope and fixed widget layout

Deliberately bounded, per task §28-§29: no free-form designer, no
`visualdesign.Document` bump, no widget template/package this stage. One
robust, responsive, transparent-background goal presentation: title line,
current/target line, a progress bar, an optional percentage, a distinct
completed visual state (e.g. a filled/glowing bar rather than a second
layout), exact money formatting (integer micros formatted client-side
for display only, using the same formatting convention already
established for Money display elsewhere in the frontend - never re-
derived server-side as a float). No animation beyond a subtle progress-
bar fill transition, disabled under `prefers-reduced-motion` exactly like
every other overlay renderer in this codebase already does.

## 24. Management API

Follows this codebase's existing REST conventions exactly (bounded
bodies, strict unknown-field rejection, trailing-JSON rejection, `404`
unknown resource, `405` + `Allow`, `409` real reference/revision conflict,
`422` validation, sanitized `500`, no raw SQL/storage errors, no raw
event contents):

```
GET    /api/goals
POST   /api/goals
GET    /api/goals/{id}
PUT    /api/goals/{id}
DELETE /api/goals/{id}
POST   /api/goals/{id}/set-current
POST   /api/goals/{id}/reset

GET    /api/widget-profiles
POST   /api/widget-profiles
GET    /api/widget-profiles/{id}
PUT    /api/widget-profiles/{id}
DELETE /api/widget-profiles/{id}
POST   /api/widget-profiles/{id}/rotate-public-slug
```

## 25. SQLite

Highest existing migration is `0024_visual_template_audio.sql` (§0.1) -
Stage 18A's own migrations begin at `0025`. Tables:

- `goals` - one row per goal (§8's fields, plus `config_revision`).
- `goal_providers` / `goal_accounts` - filter child tables, no table-
  level FK on `account_id` (§14).
- `goal_applied_events` - the durable dedupe ledger (§11.2), primary key
  `(goal_id, provider_id, account_id, provider_event_key)`, indexed for
  the 30-day retention prune (§11.5).
- `widget_profiles` - one row per widget profile (§18's fields),
  `goal_id` references `goals(id)` and blocks delete-while-referenced at
  the application layer (§18's rejection policy, not a `RESTRICT` FK
  surprise with an opaque SQLite error).
- Index on `widget_profiles.public_slug` (unique) for the public-route
  lookup, mirroring every other overlay's own slug index.

A repository test proves migrating a pre-Stage-18 database (seeded with
existing alert/audio/visual-template rows from an earlier migration
state) up through the new Stage 18A migrations preserves every existing
table and row unchanged - mirroring this codebase's own established
migration-preservation testing shape (`internal/storage/sqlite/
platform_repository_test.go`'s `TestDeletedSeedDataIsNotRecreated`).

## 26. Runtime manager

`internal/goals.Manager` - provider-independent, imports no Twitch/
YouTube/StreamElements provider package (provider-specific facts arrive
only through normalized `engagement.Event`, exactly like `internal/
alerts` and `internal/audio` already do). Owns: the Bus subscription
(§10), the contribution table lookup (§3), matching against currently-
enabled goals, atomic apply (§12), and notifying the public widget
projection/SSE layer of the new snapshot.

## 27. Frontend

New canonical nav item **Goals** (`/goals`, no `planned: true` - a real
page from the day this milestone ships), plus the public route `/overlay/
widgets/:publicSlug` (no `AppShell`, mirrors every existing public overlay
route in `App.tsx`). The Goals page manages goal CRUD, kind-specific
create/edit forms (integer input for followers/subscriptions/Bits; an
exact decimal-string-to-micros parser - never `parseFloat` - for
donations, paired with a currency code field), Set current/Reset actions,
completion/over-target display, the provider/source filter UI (reusing
`RuleManager.tsx`'s own `useAccountsQuery` + donation-source combined-
picker pattern, §0.1), and per-goal widget-profile management (add/edit/
delete, Copy URL, Rotate URL, the bounded style controls from §18, and an
in-app preview using the exact same `GoalWidgetRenderer` component the
public route renders - never a second, visually-different preview
implementation). Every new user-facing string ships with both English and
Polish translations; `npm run i18n:check` passing is part of every
frontend commit's own focused validation, not deferred to the end.

## 28. What Stage 18A does not implement

Exactly the Stage 18B list from §0.2, restated for closing-audit
convenience: latest follower/subscriber/donation widgets, largest-
donation widget, recent-supporters list, event ticker, richer/platform-
specific counters, arbitrary multi-widget dashboards, a free-form widget
designer, a widget template/package format, a `visualdesign.Document`-
owned widget, uploaded widget graphics, widget-specific audio,
achievement animations, goal-completion alerts, provider-total polling,
API-based follower-count synchronization, automatic provider-history
import, automatic daily/session reset, and FX conversion. None of these
is decided, scaffolded, or partially built by this milestone.
