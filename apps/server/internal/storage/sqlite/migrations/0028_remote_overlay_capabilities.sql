-- Stage 20D2C: shared remote-overlay capability mapping
-- (docs/remote-ingest.md §12). One table backs every overlay domain
-- (chat overlay, alert profile, audio, supporter widget) rather than a
-- duplicated remote-capability column in each domain's own schema -
-- see docs/remote-ingest.md §12's own "implementation simplification"
-- note.
--
-- A row's mere existence for (domain, local_slug) means remote access
-- is enabled for that profile; no row means disabled. token is the
-- capability itself - a 256-bit random value, base64url-no-padding
-- encoded by the application, never generated or interpreted by
-- SQLite. local_slug is the existing domain's own publicSlug value,
-- never a foreign key into a domain-specific table: this package is
-- deliberately domain-agnostic, so it does not depend on any of the
-- four overlay domains' own schemas.

CREATE TABLE remote_overlay_capabilities (
    token       TEXT PRIMARY KEY,
    domain      TEXT NOT NULL CHECK (domain IN ('chat-overlay', 'alert-profile', 'audio', 'widget')),
    local_slug  TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    -- Also serves as the (domain, local_slug) lookup index Resolve's
    -- own reverse lookup and Get/Issue/Revoke all need - no separate
    -- CREATE INDEX is needed alongside a UNIQUE constraint.
    UNIQUE (domain, local_slug)
);
