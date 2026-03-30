-- Reverse of 000001: drop everything from the initial schema.
-- Order: triggers → function → tables (reverse dependency) → enums → extensions.

-- Drop all updated_at triggers
DROP TRIGGER IF EXISTS trg_device_tokens_updated_at ON device_tokens;
DROP TRIGGER IF EXISTS trg_training_plans_updated_at ON training_plans;
DROP TRIGGER IF EXISTS trg_setter_labor_logs_updated_at ON setter_labor_logs;
DROP TRIGGER IF EXISTS trg_setting_sessions_updated_at ON setting_sessions;
DROP TRIGGER IF EXISTS trg_route_ratings_updated_at ON route_ratings;
DROP TRIGGER IF EXISTS trg_routes_updated_at ON routes;
DROP TRIGGER IF EXISTS trg_walls_updated_at ON walls;
DROP TRIGGER IF EXISTS trg_user_memberships_updated_at ON user_memberships;
DROP TRIGGER IF EXISTS trg_users_updated_at ON users;
DROP TRIGGER IF EXISTS trg_locations_updated_at ON locations;
DROP TRIGGER IF EXISTS trg_organizations_updated_at ON organizations;

DROP FUNCTION IF EXISTS update_updated_at();

-- Drop tables in reverse dependency order
DROP TABLE IF EXISTS device_tokens;
DROP TABLE IF EXISTS partner_profiles;
DROP TABLE IF EXISTS training_plan_items;
DROP TABLE IF EXISTS training_plans;
DROP TABLE IF EXISTS user_achievements;
DROP TABLE IF EXISTS achievement_definitions;
DROP TABLE IF EXISTS follows;
DROP TABLE IF EXISTS route_ratings;
DROP TABLE IF EXISTS ascents;
DROP TABLE IF EXISTS setter_labor_logs;
DROP TABLE IF EXISTS setter_pay_rates;
DROP TABLE IF EXISTS setting_session_assignments;
DROP TABLE IF EXISTS setting_sessions;
DROP TABLE IF EXISTS route_photos;
DROP TABLE IF EXISTS route_tags;
DROP TABLE IF EXISTS routes;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS walls;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS user_memberships;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS locations;
DROP TABLE IF EXISTS organizations;

-- Drop enum types
DROP TYPE IF EXISTS grading_system;
DROP TYPE IF EXISTS ascent_type;
DROP TYPE IF EXISTS route_status;
DROP TYPE IF EXISTS route_type;
DROP TYPE IF EXISTS user_role;

-- Drop extensions (only if no other schemas need them)
DROP EXTENSION IF EXISTS "pgcrypto";
DROP EXTENSION IF EXISTS "uuid-ossp";
