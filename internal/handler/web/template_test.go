package webhandler

import (
	"html/template"
	"io/fs"
	"testing"

	"github.com/shotwell-paddle/routewerk/web"
)

// TestTemplatesParseWithoutError verifies that every HTML template parses
// correctly with the registered funcMap. This catches syntax errors, missing
// template functions, and broken includes at test time rather than at startup.
func TestTemplatesParseWithoutError(t *testing.T) {
	tFS, err := fs.Sub(web.TemplateFS, "templates")
	if err != nil {
		t.Fatalf("cannot access template FS: %v", err)
	}

	baseBytes := mustRead(tFS, "base.html")
	sidebarBytes := mustRead(tFS, "partials/sidebar.html")
	routeCardBytes := mustRead(tFS, "partials/route-card.html")
	shared := string(baseBytes) + "\n" + string(sidebarBytes) + "\n" + string(routeCardBytes)

	// Layout-wrapped pages
	pages := []string{
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
		"setter/settings.html",
		"setter/org-settings.html",
		"setter/team.html",
		"setter/org-team.html",
		"setter/gym-new.html",
		"setter/gym-edit.html",
		"climber/routes.html",
		"climber/route-detail.html",
		"climber/walls.html",
		"climber/profile.html",
		"climber/profile-settings.html",
		"climber/join-gym.html",
		"error.html",
	}

	for _, page := range pages {
		t.Run(page, func(t *testing.T) {
			pageBytes := mustRead(tFS, page)

			// Two-step parse: shared layout first, then page (allows block overrides)
			tmpl, err := template.New(page).Funcs(funcMap).Parse(shared)
			if err != nil {
				t.Fatalf("failed to parse shared layout for %s: %v", page, err)
			}
			if _, err = tmpl.Parse(string(pageBytes)); err != nil {
				t.Fatalf("failed to parse template %s: %v", page, err)
			}
		})
	}

	// Standalone pages (no shared layout)
	standalone := []string{
		"auth/login.html",
		"auth/register.html",
	}
	for _, page := range standalone {
		t.Run(page, func(t *testing.T) {
			pageBytes := mustRead(tFS, page)
			_, err := template.New(page).Funcs(funcMap).Parse(string(pageBytes))
			if err != nil {
				t.Fatalf("failed to parse template %s: %v", page, err)
			}
		})
	}
}

// TestFuncMapHasAllRequiredFunctions verifies the funcMap contains all
// functions referenced in templates.
func TestFuncMapHasAllRequiredFunctions(t *testing.T) {
	required := []string{
		"deref", "derefFloat", "derefInt",
		"title", "reltime", "abs", "seq", "printf",
		"staticPath", "safeCSS", "roleName",
		"add", "sub", "initial",
	}

	for _, fn := range required {
		if funcMap[fn] == nil {
			t.Errorf("funcMap missing required function %q", fn)
		}
	}
}

func TestFuncMap_Add(t *testing.T) {
	addFn := funcMap["add"].(func(int, int) int)
	if addFn(3, 4) != 7 {
		t.Error("add(3, 4) should be 7")
	}
	if addFn(-1, 1) != 0 {
		t.Error("add(-1, 1) should be 0")
	}
}

func TestFuncMap_Sub(t *testing.T) {
	subFn := funcMap["sub"].(func(int, int) int)
	if subFn(7, 3) != 4 {
		t.Error("sub(7, 3) should be 4")
	}
	if subFn(0, 5) != -5 {
		t.Error("sub(0, 5) should be -5")
	}
}

func TestFuncMap_Initial(t *testing.T) {
	initialFn := funcMap["initial"].(func(string) string)
	tests := map[string]string{
		"Chris":   "C",
		"alex":    "A",
		"":        "?",
		"Z":       "Z",
	}
	for input, want := range tests {
		if got := initialFn(input); got != want {
			t.Errorf("initial(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFuncMap_RoleName(t *testing.T) {
	roleNameFn := funcMap["roleName"].(func(string) string)
	if roleNameFn("org_admin") != "Admin" {
		t.Errorf("roleName(org_admin) = %q, want Admin", roleNameFn("org_admin"))
	}
	if roleNameFn("setter") != "Setter" {
		t.Errorf("roleName(setter) = %q, want Setter", roleNameFn("setter"))
	}
}

// DerefString and RelativeTime tests are in helpers_test.go
