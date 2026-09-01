-- Stage 25: stream setup profiles (docs/stream-setup-profiles.md). A
-- reusable LOCAL preparation of the app for a particular kind of show
-- - which destinations are intended, and optionally which Stage 22
-- metadata preset to apply. Never a credential, never a generic
-- arbitrary-configuration snapshot (that is Stage 23's own, different,
-- purpose).
--
-- Scope note: only destinations and a metadata-preset reference are
-- included, per the contract's own §1 audit - every other "profile-
-- shaped" domain in this codebase (alerts, chat overlays, goals/
-- widgets, chat automation, audio/TTS) either has no real "active"
-- concept to reference (every enabled row already runs simultaneously)
-- or is a true singleton with nothing to select. No stream key, OAuth
-- token, client secret, or other credential is ever stored here -
-- structurally, not merely by convention: no column exists that could
-- hold one.
--
-- No seed data: a fresh database starts with zero setup profiles.

CREATE TABLE stream_setup_profiles (
    id                 TEXT NOT NULL PRIMARY KEY,
    name               TEXT NOT NULL,
    note               TEXT NOT NULL DEFAULT '',
    -- SET NULL, never CASCADE: deleting a metadata preset must not
    -- delete a setup profile that references it - the profile survives
    -- and reports the reference as missing.
    metadata_preset_id TEXT
                           REFERENCES metadata_presets (id) ON DELETE SET NULL,
    -- A snapshot of the referenced preset's own name, taken when the
    -- reference is set - never cleared by the FK's own SET NULL action,
    -- so "never referenced a preset" (both columns empty) stays
    -- distinguishable from "referenced one that was later deleted"
    -- (metadata_preset_id NULL, this column still holds the old name).
    metadata_preset_name TEXT NOT NULL DEFAULT '',
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_stream_setup_profiles_name ON stream_setup_profiles (name COLLATE NOCASE);

CREATE TABLE stream_setup_profile_destinations (
    profile_id    TEXT NOT NULL
                       REFERENCES stream_setup_profiles (id) ON DELETE CASCADE,
    position      INTEGER NOT NULL,
    -- SET NULL, never CASCADE, mirroring stream_session_destinations'
    -- own pattern (Stage 24) exactly: deleting a destination must not
    -- delete the setup profile's own membership row - it survives and
    -- reports "destination missing". provider_id/display_name are a
    -- snapshot taken when this row is written, purely for display once
    -- platform_id is NULL - never authoritative over the live platform
    -- row while it still exists.
    platform_id   TEXT
                       REFERENCES platforms (id) ON DELETE SET NULL,
    provider_id   TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    PRIMARY KEY (profile_id, position)
);
