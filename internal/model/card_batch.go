package model

import "time"

// ============================================================
// Route Card Print Batches
// ============================================================

// CardBatchStatus values. Kept as constants to centralise the valid set and
// mirror the CHECK constraint in migrations/000029_route_card_batches.up.sql.
const (
	CardBatchStatusPending = "pending"
	CardBatchStatusReady   = "ready"
	CardBatchStatusFailed  = "failed"
)

// Card design themes. The migration's CHECK constraint is the source of truth;
// these match 1:1 with the 4 design variants reviewed in tmp/card-designs.
const (
	CardThemeBlockAndInfo = "block_and_info"
	CardThemeFullColor    = "full_color"
	CardThemeMinimal      = "minimal"
	CardThemeTradingCard  = "trading_card"
)

// Cutter profiles. We only support Silhouette Cameo Type 2 today; the enum
// exists so future cutters can be added without a schema migration.
const (
	CutterSilhouetteType2 = "silhouette_type2"
)

// CardBatch is a set of routes a setter has queued up for print-and-cut.
//
// Batches store the list of route IDs and rendering metadata but NOT the
// rendered PDF bytes. The PDF is regenerated from live route data on each
// download so reprints always reflect the current grade / setter / name.
// StorageKey points at the most recently rendered copy in Tigris, which can
// be invalidated (set to nil) when the batch needs re-rendering.
type CardBatch struct {
	ID             string    `json:"id"`
	LocationID     string    `json:"location_id"`
	CreatedBy      string    `json:"created_by"`
	RouteIDs       []string  `json:"route_ids"`
	Theme          string    `json:"theme"`
	CutterProfile  string    `json:"cutter_profile"`
	StorageKey     *string   `json:"storage_key,omitempty"`
	Status         string    `json:"status"`
	ErrorMessage   *string   `json:"error_message,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// IsReady reports whether the batch has a rendered PDF ready to download.
func (b CardBatch) IsReady() bool {
	return b.Status == CardBatchStatusReady && b.StorageKey != nil
}

// IsPending reports whether the batch is still being rendered.
func (b CardBatch) IsPending() bool {
	return b.Status == CardBatchStatusPending
}

// IsFailed reports whether rendering failed; ErrorMessage carries the reason.
func (b CardBatch) IsFailed() bool {
	return b.Status == CardBatchStatusFailed
}
