-- Migration 011: Gym & Organization Settings
--
-- Adds JSONB settings columns to locations and organizations for configurable
-- grading, circuit colors, display preferences, and org-level permission controls.

-- ── Location Settings (gym-level, managed by head_setter) ────────────

ALTER TABLE locations
ADD COLUMN IF NOT EXISTS settings_json JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Default settings structure (for reference — not enforced by DB):
-- {
--   "grading": {
--     "boulder_method": "v_scale",          -- "v_scale" | "circuit" | "both"
--     "route_grade_format": "plus_minus",    -- "plus_minus" | "letter"
--     "show_grades_on_circuit": false,       -- show V-grade alongside circuit color
--     "v_scale_range": ["VB","V0","V1","V2","V3","V4","V5","V6","V7","V8","V9","V10","V11","V12"],
--     "yds_range": ["5.5","5.6","5.7","5.8-","5.8","5.8+","5.9-","5.9","5.9+","5.10-","5.10","5.10+","5.11-","5.11","5.11+","5.12-","5.12","5.12+","5.13-","5.13","5.13+","5.14-","5.14"]
--   },
--   "circuits": {
--     "colors": [
--       {"name":"red","hex":"#e53935","sort_order":0},
--       {"name":"orange","hex":"#fc5200","sort_order":1},
--       ...
--     ]
--   },
--   "hold_colors": [
--     {"name":"Red","hex":"#e53935","sort_order":0},
--     ...
--   ],
--   "display": {
--     "show_setter_name": true,
--     "show_route_age": true,
--     "show_difficulty_consensus": true,
--     "default_strip_age_days": 90
--   },
--   "sessions": {
--     "default_playbook_enabled": true,
--     "require_route_photo": false
--   }
-- }

-- ── Organization Settings (org-level, managed by org_admin) ──────────

ALTER TABLE organizations
ADD COLUMN IF NOT EXISTS settings_json JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Org settings control what head setters can change at gym level:
-- {
--   "permissions": {
--     "head_setter_can_edit_grading": true,
--     "head_setter_can_edit_circuits": true,
--     "head_setter_can_edit_hold_colors": true,
--     "head_setter_can_edit_display": true,
--     "head_setter_can_edit_sessions": true
--   },
--   "defaults": {
--     "boulder_method": "v_scale",
--     "route_grade_format": "plus_minus",
--     "show_grades_on_circuit": false
--   }
-- }
