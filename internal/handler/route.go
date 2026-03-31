package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

type RouteHandler struct {
	routes *repository.RouteRepo
	audit  *service.AuditService
}

func NewRouteHandler(routes *repository.RouteRepo, audit *service.AuditService) *RouteHandler {
	return &RouteHandler{routes: routes, audit: audit}
}

type createRouteRequest struct {
	WallID             string   `json:"wall_id"`
	RouteType          string   `json:"route_type"`
	GradingSystem      string   `json:"grading_system"`
	Grade              string   `json:"grade"`
	GradeLow           *string  `json:"grade_low,omitempty"`
	GradeHigh          *string  `json:"grade_high,omitempty"`
	CircuitColor       *string  `json:"circuit_color,omitempty"`
	Name               *string  `json:"name,omitempty"`
	Color              string   `json:"color"`
	Description        *string  `json:"description,omitempty"`
	PhotoURL           *string  `json:"photo_url,omitempty"`
	DateSet            *string  `json:"date_set,omitempty"`
	ProjectedStripDate *string  `json:"projected_strip_date,omitempty"`
	TagIDs             []string `json:"tag_ids,omitempty"`
}

func (h *RouteHandler) Create(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	setterID := middleware.GetUserID(r.Context())

	var req createRouteRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.WallID == "" || req.RouteType == "" || req.Grade == "" || req.Color == "" || req.GradingSystem == "" {
		Error(w, http.StatusBadRequest, "wall_id, route_type, grading_system, grade, and color are required")
		return
	}

	dateSet := time.Now()
	if req.DateSet != nil {
		parsed, err := time.Parse("2006-01-02", *req.DateSet)
		if err == nil {
			dateSet = parsed
		}
	}

	var projectedStrip *time.Time
	if req.ProjectedStripDate != nil {
		parsed, err := time.Parse("2006-01-02", *req.ProjectedStripDate)
		if err == nil {
			projectedStrip = &parsed
		}
	}

	rt := &model.Route{
		LocationID:         locationID,
		WallID:             req.WallID,
		SetterID:           &setterID,
		RouteType:          req.RouteType,
		Status:             "active",
		GradingSystem:      req.GradingSystem,
		Grade:              req.Grade,
		GradeLow:           req.GradeLow,
		GradeHigh:          req.GradeHigh,
		CircuitColor:       req.CircuitColor,
		Name:               req.Name,
		Color:              req.Color,
		Description:        req.Description,
		PhotoURL:           req.PhotoURL,
		DateSet:            dateSet,
		ProjectedStripDate: projectedStrip,
	}

	if err := h.routes.CreateWithTags(r.Context(), rt, req.TagIDs); err != nil {
		Error(w, http.StatusInternalServerError, "failed to create route")
		return
	}

	// Reload to include tags in response
	if len(req.TagIDs) > 0 {
		rt, _ = h.routes.GetByID(r.Context(), rt.ID)
	}

	h.audit.Record(r, service.AuditRouteCreate, "route", rt.ID, "", map[string]interface{}{
		"location_id": locationID,
		"grade":       rt.Grade,
		"route_type":  rt.RouteType,
	})

	JSON(w, http.StatusCreated, rt)
}

func (h *RouteHandler) List(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	filter := repository.RouteFilter{
		LocationID: locationID,
		WallID:     r.URL.Query().Get("wall_id"),
		Status:     r.URL.Query().Get("status"),
		RouteType:  r.URL.Query().Get("route_type"),
		Grade:      r.URL.Query().Get("grade"),
		SetterID:   r.URL.Query().Get("setter_id"),
		Limit:      limit,
		Offset:     offset,
	}

	routes, total, err := h.routes.List(r.Context(), filter)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	if routes == nil {
		routes = []model.Route{}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"routes": routes,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

func (h *RouteHandler) Get(w http.ResponseWriter, r *http.Request) {
	routeID := chi.URLParam(r, "routeID")

	rt, err := h.routes.GetByID(r.Context(), routeID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if rt == nil {
		Error(w, http.StatusNotFound, "route not found")
		return
	}

	JSON(w, http.StatusOK, rt)
}

func (h *RouteHandler) Update(w http.ResponseWriter, r *http.Request) {
	routeID := chi.URLParam(r, "routeID")

	rt, err := h.routes.GetByID(r.Context(), routeID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if rt == nil {
		Error(w, http.StatusNotFound, "route not found")
		return
	}

	var req createRouteRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.WallID != "" {
		rt.WallID = req.WallID
	}
	if req.RouteType != "" {
		rt.RouteType = req.RouteType
	}
	if req.GradingSystem != "" {
		rt.GradingSystem = req.GradingSystem
	}
	if req.Grade != "" {
		rt.Grade = req.Grade
	}
	if req.GradeLow != nil {
		rt.GradeLow = req.GradeLow
	}
	if req.GradeHigh != nil {
		rt.GradeHigh = req.GradeHigh
	}
	if req.CircuitColor != nil {
		rt.CircuitColor = req.CircuitColor
	}
	if req.Name != nil {
		rt.Name = req.Name
	}
	if req.Color != "" {
		rt.Color = req.Color
	}
	if req.Description != nil {
		rt.Description = req.Description
	}
	if req.PhotoURL != nil {
		rt.PhotoURL = req.PhotoURL
	}
	if req.ProjectedStripDate != nil {
		parsed, err := time.Parse("2006-01-02", *req.ProjectedStripDate)
		if err == nil {
			rt.ProjectedStripDate = &parsed
		}
	}

	if err := h.routes.Update(r.Context(), rt); err != nil {
		Error(w, http.StatusInternalServerError, "failed to update route")
		return
	}

	if len(req.TagIDs) > 0 {
		h.routes.SetTags(r.Context(), rt.ID, req.TagIDs)
	}

	// Only reload if tags changed; otherwise return what we have
	if len(req.TagIDs) > 0 {
		rt, _ = h.routes.GetByID(r.Context(), rt.ID)
	}

	h.audit.Record(r, service.AuditRouteUpdate, "route", rt.ID, "", map[string]interface{}{
		"grade":      rt.Grade,
		"route_type": rt.RouteType,
	})

	JSON(w, http.StatusOK, rt)
}

type updateStatusRequest struct {
	Status string `json:"status"`
}

func (h *RouteHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	routeID := chi.URLParam(r, "routeID")

	var req updateStatusRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Status != "active" && req.Status != "flagged" && req.Status != "archived" {
		Error(w, http.StatusBadRequest, "status must be 'active', 'flagged', or 'archived'")
		return
	}

	if err := h.routes.UpdateStatus(r.Context(), routeID, req.Status); err != nil {
		Error(w, http.StatusInternalServerError, "failed to update status")
		return
	}

	h.audit.Record(r, service.AuditRouteStatusChange, "route", routeID, "", map[string]interface{}{
		"new_status": req.Status,
	})

	rt, _ := h.routes.GetByID(r.Context(), routeID)
	JSON(w, http.StatusOK, rt)
}

type bulkArchiveRequest struct {
	RouteIDs []string `json:"route_ids,omitempty"`
	WallID   string   `json:"wall_id,omitempty"`
}

func (h *RouteHandler) BulkArchive(w http.ResponseWriter, r *http.Request) {
	var req bulkArchiveRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var affected int
	var err error

	if req.WallID != "" {
		affected, err = h.routes.BulkArchiveByWall(r.Context(), req.WallID)
	} else if len(req.RouteIDs) > 0 {
		affected, err = h.routes.BulkArchive(r.Context(), req.RouteIDs)
	} else {
		Error(w, http.StatusBadRequest, "provide route_ids or wall_id")
		return
	}

	if err != nil {
		Error(w, http.StatusInternalServerError, "failed to archive routes")
		return
	}

	h.audit.Record(r, service.AuditRouteBulkArchive, "route", chi.URLParam(r, "locationID"), "", map[string]interface{}{
		"affected":  affected,
		"wall_id":   req.WallID,
		"route_ids": req.RouteIDs,
	})

	JSON(w, http.StatusOK, map[string]int{"archived": affected})
}
