package web

import (
	"io/fs"
	"testing"
)

// ── TemplateFS ─────────────────────────────────────────────────────

func TestTemplateFS_ContainsBaseTemplate(t *testing.T) {
	data, err := TemplateFS.ReadFile("templates/base.html")
	if err != nil {
		t.Fatalf("base.html not embedded: %v", err)
	}
	if len(data) == 0 {
		t.Error("base.html is empty")
	}
}

func TestTemplateFS_ContainsErrorTemplate(t *testing.T) {
	data, err := TemplateFS.ReadFile("templates/error.html")
	if err != nil {
		t.Fatalf("error.html not embedded: %v", err)
	}
	if len(data) == 0 {
		t.Error("error.html is empty")
	}
}

func TestTemplateFS_ContainsSubdirectories(t *testing.T) {
	dirs := []string{"templates/auth", "templates/climber", "templates/setter", "templates/partials"}
	for _, dir := range dirs {
		entries, err := fs.ReadDir(TemplateFS, dir)
		if err != nil {
			t.Errorf("directory %q not embedded: %v", dir, err)
			continue
		}
		if len(entries) == 0 {
			t.Errorf("directory %q is empty", dir)
		}
	}
}

func TestTemplateFS_AuthTemplatesPresent(t *testing.T) {
	entries, err := fs.ReadDir(TemplateFS, "templates/auth")
	if err != nil {
		t.Fatalf("auth dir: %v", err)
	}

	// Should have at least login and register templates
	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name()] = true
	}

	for _, expected := range []string{"login.html", "register.html"} {
		if !names[expected] {
			t.Errorf("auth/ missing %q", expected)
		}
	}
}

// ── StaticFS ───────────────────────────────────────────────────────

func TestStaticFS_ContainsCSS(t *testing.T) {
	entries, err := fs.ReadDir(StaticFS, "static/css")
	if err != nil {
		t.Fatalf("static/css not embedded: %v", err)
	}
	if len(entries) == 0 {
		t.Error("static/css is empty")
	}

	// Should contain the main stylesheet
	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name()] = true
	}
	if !names["routewerk.css"] {
		t.Error("static/css/ missing routewerk.css")
	}
}

func TestStaticFS_ContainsJS(t *testing.T) {
	entries, err := fs.ReadDir(StaticFS, "static/js")
	if err != nil {
		t.Fatalf("static/js not embedded: %v", err)
	}
	if len(entries) == 0 {
		t.Error("static/js is empty")
	}
}

func TestStaticFS_ContainsFonts(t *testing.T) {
	entries, err := fs.ReadDir(StaticFS, "static/fonts")
	if err != nil {
		t.Fatalf("static/fonts not embedded: %v", err)
	}
	if len(entries) == 0 {
		t.Error("static/fonts is empty")
	}
}

func TestStaticFS_CSSNotEmpty(t *testing.T) {
	data, err := StaticFS.ReadFile("static/css/routewerk.css")
	if err != nil {
		t.Fatalf("routewerk.css read: %v", err)
	}
	// Should be a non-trivial CSS file
	if len(data) < 1000 {
		t.Errorf("routewerk.css is suspiciously small: %d bytes", len(data))
	}
}

// ── Template file format verification ──────────────────────────────

func TestTemplateFS_AllHTMLFiles(t *testing.T) {
	count := 0
	err := fs.WalkDir(TemplateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			count++
			// All template files should be .html
			name := d.Name()
			if len(name) < 5 || name[len(name)-5:] != ".html" {
				t.Errorf("non-HTML file in templates: %s", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk templates: %v", err)
	}
	if count == 0 {
		t.Error("no template files found")
	}
	// Sanity check: should have at least 10 templates
	if count < 10 {
		t.Errorf("only %d templates found — expected at least 10", count)
	}
}
