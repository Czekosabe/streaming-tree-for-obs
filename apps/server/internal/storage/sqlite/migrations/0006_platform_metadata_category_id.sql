-- Adds a provider-stable category identifier alongside the existing
-- free-text category display name.
--
-- category (existing) stays the user-facing display text. category_id is
-- new: the provider's own stable remote identifier for that category
-- (Twitch: Get/Search Categories "id", a game/category ID), needed because
-- publishing a category to a real provider API requires its ID, not a
-- guess derived from arbitrary display text. NULL means "no remote category
-- selected" - every existing row gets NULL here, since no ID was ever
-- captured before this migration; the display text itself is untouched.

ALTER TABLE platform_metadata ADD COLUMN category_id TEXT;
