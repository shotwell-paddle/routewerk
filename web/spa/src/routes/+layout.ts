// Pure SPA: no SSR, no prerendering. The static adapter writes a single
// fallback HTML and the client takes over for all routing.
//
// trailingSlash defaults to 'never' — matches Routewerk's existing HTMX
// route style (/dashboard, /routes). The Go router serves the SPA fallback
// at both /spa-test and /spa-test/* so reloading the URL with or without
// the trailing slash both hit the SPA cleanly.
export const ssr = false;
export const prerender = false;
