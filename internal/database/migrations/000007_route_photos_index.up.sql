-- Add index on route_photos(route_id) for efficient lookups.
CREATE INDEX IF NOT EXISTS idx_route_photos_route_id ON route_photos(route_id);

-- Add index on route_photos(uploaded_by) for user-scoped queries.
CREATE INDEX IF NOT EXISTS idx_route_photos_uploaded_by ON route_photos(uploaded_by);
