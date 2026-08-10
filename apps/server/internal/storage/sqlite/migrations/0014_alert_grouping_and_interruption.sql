-- Stage 12B: bounded alert grouping and deterministic mid-alert
-- preemption - the two capabilities Stage 12A's own migration
-- (0013_alerts.sql) deliberately deferred. See docs/progress.md's
-- Stage 12B entries for the full capability matrix and semantics, and
-- internal/domain/alerts/grouping.go for the per-event-type reasoning.
--
-- Every new column defaults to Stage 12A's own non-grouping,
-- non-preempting behavior, so every existing persisted rule migrates
-- safely with zero behavior change until an operator explicitly opts in
-- (Stage 12B task's own Part 29).

-- allow_grouping: whether this rule's own newly matched alerts may merge
-- into a compatible, still-queued alert instead of always becoming a new
-- queue entry. Rejected at the application layer (never at this CHECK)
-- unless the rule's own event_type has a real, safe grouping strategy -
-- see internal/domain/alerts/grouping.go's own GroupingCapabilities table.
ALTER TABLE alert_rules ADD COLUMN allow_grouping INTEGER NOT NULL DEFAULT 0 CHECK (allow_grouping IN (0, 1));

-- group_window_ms: how long, anchored to the *first* member's own
-- queued_at, a later compatible event may still join the same group -
-- never a sliding/extending window (Stage 12B task Part 5). Always
-- validated within bounds regardless of allow_grouping - a single
-- unconditional CHECK is simpler than one that reads allow_grouping, and
-- the value is simply inert while grouping is disabled.
ALTER TABLE alert_rules ADD COLUMN group_window_ms INTEGER NOT NULL DEFAULT 5000 CHECK (group_window_ms BETWEEN 1000 AND 30000);

-- interrupt_mode: whether THIS rule's own alert, when newly matched, may
-- interrupt whatever is currently playing. 'never' (the default)
-- preserves every existing rule's Stage 12A playback behavior exactly -
-- only 'lower_priority' ever allows preemption, and only of a strictly
-- lower-priority current alert (see internal/alerts/playback.go).
ALTER TABLE alert_rules ADD COLUMN interrupt_mode TEXT NOT NULL DEFAULT 'never' CHECK (interrupt_mode IN ('never', 'lower_priority'));

-- interruptible: whether THIS rule's own alert, while it is the one
-- currently playing, may itself be interrupted by a later, eligible,
-- strictly-higher-priority alert. Default true (the Stage 12B task's own
-- recommended default) has no observable effect on its own - it only
-- matters once some *other* rule's interrupt_mode is 'lower_priority'.
ALTER TABLE alert_rules ADD COLUMN interruptible INTEGER NOT NULL DEFAULT 1 CHECK (interruptible IN (0, 1));
