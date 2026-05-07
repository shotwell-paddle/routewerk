# Routewerk SPA

SvelteKit single-page app, built to static assets and embedded into the Go
binary at compile time. Phase 0 of the competition-tracking work; see
`/competitions-handoff.md` for the larger plan.

## Quick start

From the repo root:

```
make spa-install   # one-time: npm install
make spa-build     # produces web/spa/build/, then go build embeds it
make build         # builds the Go binary including the embedded SPA
make run           # serves at http://localhost:8080
```

Smoke test URL: `http://localhost:8080/spa-test/` (Phase 0 only).

## SPA dev server with API proxy

```
make spa-dev       # vite dev on :5173, proxies /api → :8080
```

Run the Go API in another terminal (`make run`). Cookies and CSRF work because
Vite proxies `/api/*` to the API origin without rewriting Host.

## Conventions

- TypeScript strict mode, no `any`.
- Svelte 5 runes (`$state`, `$derived`, `$effect`).
- API access only through the generated client in `src/lib/api/` (added in
  Phase 1).
- Component files PascalCase, route files SvelteKit-conventional.

## How embedding works

`web/spa/embed.go` (package `spa`) does `//go:embed all:build`. A placeholder
`build/index.html` is committed so `go build` always succeeds; the real bundle
replaces it after `make spa-build` runs.
