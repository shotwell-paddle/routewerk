package webhandler

import (
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/web"
)

func (h *Handler) loadTemplates() {
	h.templates = make(map[string]*template.Template)

	tFS, err := fs.Sub(web.TemplateFS, "templates")
	if err != nil {
		panic("cannot access template FS: " + err.Error())
	}

	// Read shared layout files
	baseBytes := mustRead(tFS, "base.html")
	sidebarBytes := mustRead(tFS, "partials/sidebar.html")
	routeCardBytes := mustRead(tFS, "partials/route-card.html")

	shared := string(baseBytes) + "\n" + string(sidebarBytes) + "\n" + string(routeCardBytes)

	// Parse each page template with the shared layout
	pages := []string{
		"admin/health.html",
		"admin/metrics.html",
		"setter/dashboard.html",
		"setter/route-form.html",
		"setter/route-manage.html",
		"setter/walls.html",
		"setter/wall-form.html",
		"setter/wall-detail.html",
		"setter/sessions.html",
		"setter/session-detail.html",
		"setter/session-form.html",
		"setter/session-complete.html",
		"setter/session-photos.html",
		"setter/settings.html",
		"setter/org-settings.html",
		"setter/team.html",
		"setter/org-team.html",
		"setter/gym-new.html",
		"setter/gym-edit.html",
		"setter/progressions.html",
		"setter/domain-form.html",
		"setter/quest-form.html",
		"setter/badge-form.html",
		"setter/card-batches.html",
		"setter/card-batch-form.html",
		"setter/card-batch-detail.html",
		"climber/routes.html",
		"climber/archive.html",
		"climber/route-detail.html",
		"climber/walls.html",
		"climber/profile.html",
		"climber/profile-settings.html",
		"climber/join-gym.html",
		"climber/quests.html",
		"climber/quest-detail.html",
		"climber/my-quests.html",
		"climber/quest-activity.html",
		"climber/notifications.html",
		"error.html",
	}

	for _, page := range pages {
		pageBytes := mustRead(tFS, page)

		// Parse shared layout first, then the page template in a second Parse
		// call. This allows the page's {{define "title"}} to override the
		// {{block "title"}} default from base.html without a "multiple
		// definition" error (Go's template engine allows overrides across
		// separate Parse calls but not within a single one).
		tmpl, parseErr := template.New(page).Funcs(funcMap).Parse(shared)
		if parseErr != nil {
			panic(fmt.Sprintf("failed to parse shared layout for %s: %v", page, parseErr))
		}
		if _, parseErr = tmpl.Parse(string(pageBytes)); parseErr != nil {
			panic(fmt.Sprintf("failed to parse template %s: %v", page, parseErr))
		}
		h.templates[page] = tmpl
	}

	// Standalone pages (no sidebar/base layout)
	standalone := []string{
		"auth/login.html",
		"auth/register.html",
		"auth/setup.html",
	}
	for _, page := range standalone {
		pageBytes := mustRead(tFS, page)
		tmpl, parseErr := template.New(page).Funcs(funcMap).Parse(string(pageBytes))
		if parseErr != nil {
			panic(fmt.Sprintf("failed to parse template %s: %v", page, parseErr))
		}
		h.templates[page] = tmpl
	}
}

func mustRead(fsys fs.FS, name string) []byte {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		panic(fmt.Sprintf("cannot read %s: %v", name, err))
	}
	return data
}

// render executes a page template. HTMX requests get just the content block;
// full page loads get the complete HTML shell.
func (h *Handler) render(w http.ResponseWriter, r *http.Request, page string, data *PageData) {
	tmpl, ok := h.templates[page]
	if !ok {
		slog.Error("template not found", "page", page)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Please try again later.")
		return
	}

	// Inject CSRF token into every render
	data.CSRFToken = middleware.TokenFromRequest(r)

	// Populate location and view-as data for the sidebar
	h.enrichTemplateData(r, &data.TemplateData)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	isHTMX := r.Header.Get("HX-Request") == "true"

	if isHTMX {
		// HTMX partial swap — render just the "content" block
		if err := tmpl.ExecuteTemplate(w, "content", data); err != nil {
			slog.Error("template render failed", "page", page, "error", err)
			http.Error(w, "Something went wrong", http.StatusInternalServerError)
		}
		return
	}

	// Full page — render base.html which includes sidebar + content
	if err := tmpl.ExecuteTemplate(w, page, data); err != nil {
		slog.Error("template render failed", "page", page, "error", err)
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
	}
}

// renderError renders a user-friendly error page. Does not expose internals.
func (h *Handler) renderError(w http.ResponseWriter, r *http.Request, code int, title, message string) {
	tmpl, ok := h.templates["error.html"]
	if !ok {
		// Last resort: plain text
		http.Error(w, title, code)
		return
	}

	data := &PageData{
		TemplateData: TemplateData{
			ActiveNav:   "",
			UserName:    "Guest",
			UserInitial: "?",
		},
		ErrorCode:    code,
		ErrorTitle:   title,
		ErrorMessage: message,
	}
	data.CSRFToken = middleware.TokenFromRequest(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)

	if err := tmpl.ExecuteTemplate(w, "error.html", data); err != nil {
		slog.Error("error template render failed", "error", err)
		http.Error(w, title, code)
	}
}

// checkLocationOwnership verifies a resource belongs to the user's active location.
// Returns true if the check passes (locations match or no location context).
// Returns false and renders a 404 if the resource belongs to a different location.
func (h *Handler) checkLocationOwnership(w http.ResponseWriter, r *http.Request, resourceLocationID string) bool {
	locationID := middleware.GetWebLocationID(r.Context())
	if locationID != "" && resourceLocationID != locationID {
		h.renderError(w, r, http.StatusNotFound, "Not found", "This resource doesn't exist.")
		return false
	}
	return true
}
