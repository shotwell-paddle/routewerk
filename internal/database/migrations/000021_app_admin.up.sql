-- App admin flag — separate from org-level roles.
-- App admins can access health, metrics, and diagnostics regardless of org membership.
ALTER TABLE users ADD COLUMN is_app_admin BOOLEAN NOT NULL DEFAULT false;
