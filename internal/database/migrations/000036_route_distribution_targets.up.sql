-- Route distribution targets: per-location goals for "how many routes
-- should we have at each grade / circuit". Head-setters and gym
-- managers configure these to plan setting work; the dashboard
-- distribution charts overlay actual vs target so the gap is obvious.
--
-- Shape:
--   - route_type narrows the bucket: 'boulder' (V-scale grades),
--     'route' (YDS grades), or 'circuit' (circuit color name).
--   - grade is the bucket label — V0, 5.10a, "red", etc. Matches the
--     same string the routes table stores in `grade` (or in
--     `circuit_color` for circuit-graded routes), so the join is a
--     direct equality.
--   - target_count is what the gym wants to maintain; the chart
--     compares to the actual COUNT(*) on routes.
--
-- The unique key (location_id, route_type, grade) means every bucket
-- has at most one target row. Editing is upsert: PUT replaces the
-- caller-supplied set, deleting any rows the caller didn't include.

CREATE TABLE route_distribution_targets (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  location_id  uuid NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
  route_type   text NOT NULL CHECK (route_type IN ('boulder', 'route', 'circuit')),
  grade        text NOT NULL,
  target_count integer NOT NULL CHECK (target_count >= 0),
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (location_id, route_type, grade)
);

CREATE INDEX route_distribution_targets_location_idx
  ON route_distribution_targets(location_id);
