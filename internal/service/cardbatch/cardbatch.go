// Package cardbatch exposes the service layer that resolves a list of route
// IDs into full CardData and drives the sheet composer to render an 8-up
// print-and-cut PDF. It lives in its own package (rather than inside the
// generic service package) because it depends on cardsheet, which in turn
// depends on service — keeping cardbatch at the leaf of the service tree
// breaks that import cycle.
package cardbatch

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
	"github.com/shotwell-paddle/routewerk/internal/service/cardsheet"
)

// MaxBatchCards caps the number of routes that can go into a single print
// batch. Each card drives a QR render + a per-page content stream inside
// gofpdf, and gofpdf buffers the whole PDF in memory before Output() — so
// batch size is effectively a linear driver of peak RSS at render time.
//
// 200 cards = 25 letter sheets, which is already a full day's print job for
// any gym we've talked to. On the 256MB Fly VM (see fly.toml) this leaves
// comfortable headroom for Go runtime + concurrent requests; without the cap
// a "print every active route at the gym" submission would OOM the instance
// (previously triggered on routewerk-dev with ~800+ routes).
//
// Setters who hit the cap should split into multiple batches; that's also
// how they'll want to cut them physically anyway (25 sheets is about the
// stack the Silhouette can take without re-feeding).
const MaxBatchCards = 200

// Service composes route cards into a multi-card print-and-cut sheet PDF.
// It owns the "resolve a route ID into full CardData" step so handlers don't
// have to reimplement the wall / setter / QR-URL wiring that the single-card
// endpoints already do.
//
// The service is stateless apart from its dependencies — every call re-reads
// routes fresh from the DB so reprints always reflect current route state.
// We deliberately don't cache rendered PDFs here; that's the job of the
// (future) storage-backed cache layer once it lands.
type Service struct {
	routes    *repository.RouteRepo
	walls     *repository.WallRepo
	locations *repository.LocationRepo
	users     *repository.UserRepo
	composer  *cardsheet.Composer
	cards     *service.CardGenerator
}

// NewService wires a cardbatch.Service against existing repos.
// composer should be the same *cardsheet.Composer the app uses elsewhere —
// it wraps the single-card renderer, so we pass cards in separately to
// handle the QR-URL formatting without re-plumbing it through Composer.
func NewService(
	routes *repository.RouteRepo,
	walls *repository.WallRepo,
	locations *repository.LocationRepo,
	users *repository.UserRepo,
	composer *cardsheet.Composer,
	cards *service.CardGenerator,
) *Service {
	return &Service{
		routes:    routes,
		walls:     walls,
		locations: locations,
		users:     users,
		composer:  composer,
		cards:     cards,
	}
}

// RenderBatch resolves routeIDs into CardData and writes an 8-up print-and-cut
// PDF to w. Card order on the sheet matches the order of routeIDs. Routes that
// can't be resolved — deleted, in a different location, or DB errors on a
// single row — are silently skipped so setters can reprint older batches
// after some of the original routes have aged out.
//
// Returns the number of cards actually rendered (may be less than len(routeIDs))
// plus any hard render error from the composer.
func (s *Service) RenderBatch(
	ctx context.Context,
	locationID string,
	routeIDs []string,
	cfg cardsheet.SheetConfig,
	w io.Writer,
) (int, error) {
	if len(routeIDs) == 0 {
		return 0, fmt.Errorf("cardbatch: no routes")
	}
	if len(routeIDs) > MaxBatchCards {
		// Hard cap — see MaxBatchCards comment. Returning an error here keeps
		// the Render OOM from reaching the process; handlers surface this to
		// the user as a "split into smaller batches" message.
		return 0, fmt.Errorf("cardbatch: too many routes (%d, max %d) — split into multiple batches",
			len(routeIDs), MaxBatchCards)
	}

	// Resolve the location once — name never changes mid-render.
	var locationName string
	if loc, err := s.locations.GetByID(ctx, locationID); err == nil && loc != nil {
		locationName = loc.Name
	}

	// Single batch lookup instead of N round-trips. Pre-#000037 this
	// loop did one DB round-trip per route — at MaxBatchCards=200 cards
	// × 50 gyms × hourly print runs that's ~10k round-trips per hour
	// just resolving routes. ListByIDs returns the set in one query;
	// indexing by id lets us still walk routeIDs in order so the sheet
	// layout matches what the setter selected.
	rows, err := s.routes.ListByIDs(ctx, locationID, routeIDs)
	if err != nil {
		return 0, fmt.Errorf("cardbatch: load routes: %w", err)
	}
	routeByID := make(map[string]model.Route, len(rows))
	for _, rt := range rows {
		routeByID[rt.ID] = rt
	}

	// Cache wall names + setter display names across the batch. Most
	// batches come from a single wall or two, so this still avoids the
	// per-route wall/setter round-trip; the inner GetByIDs are bounded
	// by len(distinct walls + distinct setters), which is tiny.
	wallNames := map[string]string{}
	setterNames := map[string]string{}

	cards := make([]service.CardData, 0, len(routeIDs))
	for _, id := range routeIDs {
		rt, ok := routeByID[id]
		if !ok {
			// Missing or cross-location route — silently skipped (as
			// before). The location filter is enforced inside ListByIDs
			// so cross-tenant IDs simply don't appear in the result.
			continue
		}
		wname, seen := wallNames[rt.WallID]
		if !seen {
			if wall, wErr := s.walls.GetByID(ctx, rt.WallID); wErr == nil && wall != nil {
				wname = wall.Name
			}
			wallNames[rt.WallID] = wname
		}
		sname := ""
		if rt.SetterID != nil {
			cached, ok := setterNames[*rt.SetterID]
			if !ok {
				if u, uErr := s.users.GetByID(ctx, *rt.SetterID); uErr == nil && u != nil {
					cached = u.DisplayName
				}
				setterNames[*rt.SetterID] = cached
			}
			sname = cached
		}
		cards = append(cards, service.CardData{
			Route:        &rt,
			WallName:     wname,
			LocationName: locationName,
			SetterName:   sname,
			QRTargetURL:  s.cards.RouteURL(locationID, rt.ID),
		})
	}

	if len(cards) == 0 {
		return 0, fmt.Errorf("cardbatch: no renderable routes (requested %d)", len(routeIDs))
	}
	if err := s.composer.Render(w, cards, cfg); err != nil {
		return 0, fmt.Errorf("cardbatch: render sheet: %w", err)
	}
	return len(cards), nil
}

// RenderPreviewPNG renders the first resolvable route in routeIDs as a digital
// card PNG and writes it to w. This powers the "preview thumbnail" on the
// batch detail page so setters can eyeball what will print without pulling
// the full PDF.
//
// We deliberately use the digital (social) card rather than the print card:
// it's higher resolution, has a cleaner composition for on-screen viewing,
// and is already what setters see when they share a single card. The thumb
// is bandwidth-friendly enough that we don't cache it here — the browser's
// own cache plus the handler's short Cache-Control is sufficient.
func (s *Service) RenderPreviewPNG(
	ctx context.Context,
	locationID string,
	routeIDs []string,
	w io.Writer,
) error {
	if len(routeIDs) == 0 {
		return fmt.Errorf("cardbatch: no routes")
	}

	var locationName string
	if loc, err := s.locations.GetByID(ctx, locationID); err == nil && loc != nil {
		locationName = loc.Name
	}

	// Try each route until we successfully render one. A single broken route
	// (deleted, moved, transient DB blip) shouldn't break the whole thumbnail
	// — setters lean on the preview to spot-check the output, so we prefer a
	// "good enough" card from further down the list over an unhelpful broken
	// image. This mirrors RenderBatch's silent-skip policy on missing routes.
	//
	// We log each skip at Warn so that when previews fail in prod we can see
	// *which* route failed first and whether the rest were poisoned by a
	// cascading context deadline. Without this, only the final "last error"
	// surfaces in logs, which hides the actual trigger.
	var lastErr error
	for _, id := range routeIDs {
		rt, err := s.routes.GetByID(ctx, id)
		if err != nil {
			slog.Warn("cardbatch preview: skip route (load failed)", "route_id", id, "error", err)
			lastErr = fmt.Errorf("load route %s: %w", id, err)
			continue
		}
		if rt == nil || rt.LocationID != locationID {
			slog.Warn("cardbatch preview: skip route (missing or cross-location)", "route_id", id)
			continue
		}

		wname := ""
		if wall, wErr := s.walls.GetByID(ctx, rt.WallID); wErr == nil && wall != nil {
			wname = wall.Name
		}
		sname := ""
		if rt.SetterID != nil {
			if u, uErr := s.users.GetByID(ctx, *rt.SetterID); uErr == nil && u != nil {
				sname = u.DisplayName
			}
		}

		png, err := s.cards.GenerateDigitalPNG(service.CardData{
			Route:        rt,
			WallName:     wname,
			LocationName: locationName,
			SetterName:   sname,
			QRTargetURL:  s.cards.RouteURL(locationID, rt.ID),
		})
		if err != nil {
			slog.Warn("cardbatch preview: skip route (render failed)", "route_id", id, "error", err)
			lastErr = fmt.Errorf("render route %s: %w", id, err)
			continue
		}
		if _, err := w.Write(png); err != nil {
			return fmt.Errorf("cardbatch: write preview: %w", err)
		}
		return nil
	}

	if lastErr != nil {
		return fmt.Errorf("cardbatch: no renderable routes for preview (last error: %w)", lastErr)
	}
	return fmt.Errorf("cardbatch: no renderable routes for preview")
}

// ValidateRouteIDs filters routeIDs to those that resolve to a route in the
// given location. Used at batch-create time to reject bogus IDs before
// committing a row.
//
// Returns the subset in their original order, or a hard error on DB trouble.
// A route that's been deleted or moved to another location is simply dropped
// from the result without an error — the caller decides whether an empty
// result should be a user-visible problem.
func (s *Service) ValidateRouteIDs(
	ctx context.Context,
	locationID string,
	routeIDs []string,
) ([]string, error) {
	// Single batch lookup. The location filter inside ListByIDs takes
	// care of the cross-tenant skip — anything that doesn't belong to
	// this location simply doesn't appear in the result.
	rows, err := s.routes.ListByIDs(ctx, locationID, routeIDs)
	if err != nil {
		return nil, fmt.Errorf("cardbatch: validate routes: %w", err)
	}
	present := make(map[string]struct{}, len(rows))
	for _, rt := range rows {
		present[rt.ID] = struct{}{}
	}
	// Walk the input slice so we preserve the caller's intended order.
	valid := make([]string, 0, len(rows))
	for _, id := range routeIDs {
		if _, ok := present[id]; ok {
			valid = append(valid, id)
		}
	}
	return valid, nil
}
