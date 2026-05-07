package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

type WallHandler struct {
	walls *repository.WallRepo
	audit *service.AuditService
}

func NewWallHandler(walls *repository.WallRepo, audit *service.AuditService) *WallHandler {
	return &WallHandler{walls: walls, audit: audit}
}

type createWallRequest struct {
	Name         string   `json:"name"`
	WallType     string   `json:"wall_type"`
	Angle        *string  `json:"angle,omitempty"`
	HeightMeters *float64 `json:"height_meters,omitempty"`
	NumAnchors   *int     `json:"num_anchors,omitempty"`
	SurfaceType  *string  `json:"surface_type,omitempty"`
	SortOrder    int      `json:"sort_order"`
	MapX         *float64 `json:"map_x,omitempty"`
	MapY         *float64 `json:"map_y,omitempty"`
	MapWidth     *float64 `json:"map_width,omitempty"`
	MapHeight    *float64 `json:"map_height,omitempty"`
}

func (h *WallHandler) Create(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")

	var req createWallRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		Error(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.WallType != "boulder" && req.WallType != "route" {
		Error(w, http.StatusBadRequest, "wall_type must be 'boulder' or 'route'")
		return
	}

	wall := &model.Wall{
		LocationID:   locationID,
		Name:         req.Name,
		WallType:     req.WallType,
		Angle:        req.Angle,
		HeightMeters: req.HeightMeters,
		NumAnchors:   req.NumAnchors,
		SurfaceType:  req.SurfaceType,
		SortOrder:    req.SortOrder,
		MapX:         req.MapX,
		MapY:         req.MapY,
		MapWidth:     req.MapWidth,
		MapHeight:    req.MapHeight,
	}

	if err := h.walls.Create(r.Context(), wall); err != nil {
		Error(w, http.StatusInternalServerError, "failed to create wall")
		return
	}

	h.audit.Record(r, service.AuditWallCreate, "wall", wall.ID, "", map[string]interface{}{
		"location_id": locationID,
		"name":        wall.Name,
		"wall_type":   wall.WallType,
	})

	JSON(w, http.StatusCreated, wall)
}

func (h *WallHandler) List(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")

	includeArchived := r.URL.Query().Get("include_archived") == "true"

	var walls []model.Wall
	var err error
	if includeArchived {
		walls, err = h.walls.ListByLocationAll(r.Context(), locationID)
	} else {
		walls, err = h.walls.ListByLocation(r.Context(), locationID)
	}
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	if walls == nil {
		walls = []model.Wall{}
	}

	JSON(w, http.StatusOK, walls)
}

func (h *WallHandler) Get(w http.ResponseWriter, r *http.Request) {
	wall, ok := h.resolveWall(w, r)
	if !ok {
		return
	}
	JSON(w, http.StatusOK, wall)
}

func (h *WallHandler) Update(w http.ResponseWriter, r *http.Request) {
	wallID := chi.URLParam(r, "wallID")
	wall, ok := h.resolveWall(w, r)
	if !ok {
		return
	}

	var req createWallRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != "" {
		wall.Name = req.Name
	}
	if req.WallType != "" {
		wall.WallType = req.WallType
	}
	if req.Angle != nil {
		wall.Angle = req.Angle
	}
	if req.HeightMeters != nil {
		wall.HeightMeters = req.HeightMeters
	}
	if req.NumAnchors != nil {
		wall.NumAnchors = req.NumAnchors
	}
	if req.SurfaceType != nil {
		wall.SurfaceType = req.SurfaceType
	}
	wall.SortOrder = req.SortOrder
	if req.MapX != nil {
		wall.MapX = req.MapX
	}
	if req.MapY != nil {
		wall.MapY = req.MapY
	}
	if req.MapWidth != nil {
		wall.MapWidth = req.MapWidth
	}
	if req.MapHeight != nil {
		wall.MapHeight = req.MapHeight
	}

	if err := h.walls.Update(r.Context(), wall); err != nil {
		Error(w, http.StatusInternalServerError, "failed to update wall")
		return
	}

	h.audit.Record(r, service.AuditWallUpdate, "wall", wallID, "", map[string]interface{}{
		"name": wall.Name,
	})

	JSON(w, http.StatusOK, wall)
}

func (h *WallHandler) Delete(w http.ResponseWriter, r *http.Request) {
	wallID := chi.URLParam(r, "wallID")
	wall, ok := h.resolveWall(w, r)
	if !ok {
		return
	}

	if err := h.walls.Delete(r.Context(), wallID); err != nil {
		Error(w, http.StatusInternalServerError, "failed to delete wall")
		return
	}

	h.audit.Record(r, service.AuditWallDelete, "wall", wallID, "", map[string]interface{}{
		"name": wall.Name,
	})

	JSON(w, http.StatusNoContent, nil)
}

// Archive sets archived_at on the wall (soft hide). Mirrors the HTMX
// POST /walls/{id}/archive at handler/web/walls.go::WallArchive.
// Routes on the wall stay intact; archived walls don't show up in
// climber browsers but still resolve via direct URL for staff.
func (h *WallHandler) Archive(w http.ResponseWriter, r *http.Request) {
	h.setArchived(w, r, true)
}

// Unarchive clears archived_at, restoring the wall to climber views.
func (h *WallHandler) Unarchive(w http.ResponseWriter, r *http.Request) {
	h.setArchived(w, r, false)
}

func (h *WallHandler) setArchived(w http.ResponseWriter, r *http.Request, archive bool) {
	wallID := chi.URLParam(r, "wallID")
	if _, ok := h.resolveWall(w, r); !ok {
		return
	}

	var err error
	if archive {
		err = h.walls.Archive(r.Context(), wallID)
	} else {
		err = h.walls.Unarchive(r.Context(), wallID)
	}
	if err != nil {
		Error(w, http.StatusInternalServerError, "failed to update wall")
		return
	}

	// Re-fetch so the response reflects the new archived_at.
	wall, _ := h.walls.GetByID(r.Context(), wallID)
	JSON(w, http.StatusOK, wall)
}

// resolveWall fetches the wall by ID and verifies it belongs to the
// URL's locationID, writing 404 on mismatch. Centralizes the
// cross-tenant guard that every by-id wall handler needs — without
// it a setter at gym A could read/edit/delete a wall at gym B (within
// orgs they share, or across orgs entirely).
func (h *WallHandler) resolveWall(w http.ResponseWriter, r *http.Request) (*model.Wall, bool) {
	locationID := chi.URLParam(r, "locationID")
	wallID := chi.URLParam(r, "wallID")
	wall, err := h.walls.GetByID(r.Context(), wallID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return nil, false
	}
	if wall == nil || wall.LocationID != locationID {
		Error(w, http.StatusNotFound, "wall not found")
		return nil, false
	}
	return wall, true
}
