-- Performance optimizations: missing indexes, trigger-based counters

-- ============================================================
-- MISSING INDEXES
-- ============================================================

-- route_tags: route_id lookups (every GetByID loads tags)
CREATE INDEX idx_route_tags_route ON route_tags(route_id);

-- follows: follower_id lookups (Following list, ActivityFeed)
CREATE INDEX idx_follows_follower ON follows(follower_id);

-- training_plan_items: plan_id lookups
CREATE INDEX idx_training_plan_items_plan ON training_plan_items(plan_id, sort_order);

-- setting_session_assignments: session_id lookups
CREATE INDEX idx_setting_session_assignments_session ON setting_session_assignments(session_id);

-- ascents: route + date for analytics engagement queries
CREATE INDEX idx_ascents_route_climbed ON ascents(route_id, climbed_at DESC);

-- ascents: user + type for grade pyramid stats
CREATE INDEX idx_ascents_user_type ON ascents(user_id, ascent_type);

-- route_ratings: route_id for avg calculation
CREATE INDEX idx_route_ratings_route_avg ON route_ratings(route_id, rating);

-- routes: location + date_set for analytics
CREATE INDEX idx_routes_location_date ON routes(location_id, date_set DESC) WHERE deleted_at IS NULL;

-- ============================================================
-- TRIGGER-BASED COUNTER UPDATES (replace COUNT subqueries)
-- ============================================================

-- Ascent counters: increment on insert instead of COUNT(*)
CREATE OR REPLACE FUNCTION increment_route_ascent_counts()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE routes SET
        ascent_count = ascent_count + CASE WHEN NEW.ascent_type IN ('send', 'flash') THEN 1 ELSE 0 END,
        attempt_count = attempt_count + 1
    WHERE id = NEW.route_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_ascent_insert_counts
    AFTER INSERT ON ascents
    FOR EACH ROW
    EXECUTE FUNCTION increment_route_ascent_counts();

-- Rating avg: incremental update instead of full AVG(*)
-- Stores count in a helper column for O(1) recalculation
ALTER TABLE routes ADD COLUMN IF NOT EXISTS rating_count INTEGER DEFAULT 0;

CREATE OR REPLACE FUNCTION update_route_avg_rating()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        UPDATE routes SET
            avg_rating = (avg_rating * rating_count + NEW.rating) / (rating_count + 1),
            rating_count = rating_count + 1
        WHERE id = NEW.route_id;
    ELSIF TG_OP = 'UPDATE' THEN
        UPDATE routes SET
            avg_rating = avg_rating + (NEW.rating - OLD.rating)::numeric / GREATEST(rating_count, 1)
        WHERE id = NEW.route_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_rating_avg
    AFTER INSERT OR UPDATE OF rating ON route_ratings
    FOR EACH ROW
    EXECUTE FUNCTION update_route_avg_rating();
