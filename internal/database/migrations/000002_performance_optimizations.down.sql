-- Reverse of 000002: drop performance triggers, functions, indexes, and column.

DROP TRIGGER IF EXISTS trg_rating_avg ON route_ratings;
DROP TRIGGER IF EXISTS trg_ascent_insert_counts ON ascents;

DROP FUNCTION IF EXISTS update_route_avg_rating();
DROP FUNCTION IF EXISTS increment_route_ascent_counts();

DROP INDEX IF EXISTS idx_routes_location_date;
DROP INDEX IF EXISTS idx_route_ratings_route_avg;
DROP INDEX IF EXISTS idx_ascents_user_type;
DROP INDEX IF EXISTS idx_ascents_route_climbed;
DROP INDEX IF EXISTS idx_setting_session_assignments_session;
DROP INDEX IF EXISTS idx_training_plan_items_plan;
DROP INDEX IF EXISTS idx_follows_follower;
DROP INDEX IF EXISTS idx_route_tags_route;

ALTER TABLE routes DROP COLUMN IF EXISTS rating_count;
