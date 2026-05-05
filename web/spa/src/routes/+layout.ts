// Pure SPA: no SSR, no prerendering. The static adapter writes a single
// fallback HTML and the client takes over for all routing.
export const ssr = false;
export const prerender = false;
export const trailingSlash = 'never';
