-- Route card print batches: groups of routes printed together as an 8-up
-- print-and-cut sheet. Batches do NOT store the rendered PDF bytes — the PDF
-- is regenerated from current route data each time it's downloaded, so
-- reprints always reflect the latest grade / setter / name / QR target.
CREATE TABLE route_card_batches (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    location_id     UUID NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    created_by      UUID NOT NULL REFERENCES users(id),
    route_ids       UUID[] NOT NULL,
    theme           TEXT NOT NULL DEFAULT 'trading_card'
                    CHECK (theme IN ('block_and_info', 'full_color', 'minimal', 'trading_card')),
    cutter_profile  TEXT NOT NULL DEFAULT 'silhouette_type2'
                    CHECK (cutter_profile IN ('silhouette_type2')),
    storage_key     TEXT,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'ready', 'failed')),
    error_message   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_route_card_batches_location
    ON route_card_batches (location_id, created_at DESC);

CREATE INDEX idx_route_card_batches_pending
    ON route_card_batches (status, created_at)
    WHERE status = 'pending';
