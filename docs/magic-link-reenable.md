# Magic-link sign-in — parked, and how to re-enable

**Status: parked (disabled) as of PR #110.** Password login is the primary
and only exposed auth path on both the HTMX and SPA surfaces.

## Why it's off

The passwordless magic-link flow was wired end-to-end in code (generate
token → store hash → enqueue `email.magic_link` job → `EmailService.send`
→ SMTP) but **no email delivery was ever configured** — no `SMTP_*`
secrets on either Fly app, and nothing in `.env.example`. Because
`EmailService.send` used to log `"dev mode, not sent"` and return success
when SMTP was unset, requesting a link returned `202 "check your email"`
while nothing was delivered. The SPA auth gate routed every
unauthenticated user straight into that broken flow.

PR #110 made password primary and gated magic-link behind a flag rather
than deleting it. The `magic_link_tokens` table and all the
service/repo/handler code remain in place and tested — nothing was
removed, so re-enabling is config + two small UI reverts.

## The flag and the guardrails

- `MAGIC_LINK_ENABLED` (env, default `false`). When off:
  - `POST /api/v1/auth/magic/request` and `GET /verify-magic` are **not
    registered** (request 404s, so the SPA can't show a false "check your
    inbox"; a stale verify link just loads the SPA, which gates to
    `/login`).
- When `MAGIC_LINK_ENABLED=true`, two guardrails make silent failure
  impossible:
  - `config.Validate()` refuses a **production** boot unless `SMTP_HOST`
    and `SMTP_FROM` are set.
  - `EmailService.send` returns an **error** in production when SMTP is
    unconfigured, so the job retries and dead-letters loudly instead of
    swallowing the mail. (Dev still logs and swallows.)

## Re-enable checklist

1. **Pick an SMTP provider** and verify your sending domain (Postmark,
   Resend, SES, Mailgun — the code uses plain `net/smtp` + `PlainAuth`,
   so any SMTP provider works; no code change for the provider choice).

2. **Set the Fly secrets** (prod shown; repeat for `routewerk-dev` to
   test on staging first):

   ```
   fly secrets set \
     SMTP_HOST=smtp.postmarkapp.com \
     SMTP_PORT=587 \
     SMTP_USERNAME=<api-key> \
     SMTP_PASSWORD=<api-token> \
     SMTP_FROM=noreply@routewerk.com \
     MAGIC_LINK_ENABLED=true \
     -a routewerk
   ```

   > Order doesn't matter, but the app will **refuse to boot** if
   > `MAGIC_LINK_ENABLED=true` lands without `SMTP_HOST`/`SMTP_FROM`.
   > Set them together (one `fly secrets set` applies atomically).

3. **Re-point the SPA auth gate** back to the magic-link page.
   In `web/spa/src/routes/(app)/+layout.svelte`, the gate currently does a
   full-page nav to the server-rendered password login:

   ```js
   window.location.href =
     '/login?next=' + encodeURIComponent(page.url.pathname + page.url.search);
   ```

   To prefer magic-link again, switch back to the client route:

   ```js
   goto('/sign-in?next=' + encodeURIComponent(page.url.pathname + page.url.search));
   ```

   (Or keep `/login` primary and surface `/sign-in` as a secondary option —
   your call. The SPA `/sign-in/+page.svelte` was left fully intact.)

4. **Restore the login footer link** (optional — only if you want a
   magic-link entry point on the HTMX password page). In
   `web/templates/auth/login.html`, after the "Sign up" footer:

   ```html
   <div class="auth-footer" style="margin-top: var(--sp-2);">
     Climbing in a competition? <a href="/sign-in">Sign in with email link</a>
   </div>
   ```

5. **Stage it first.** This touches auth + email + config, exactly the
   class CI can't exercise. Deploy to `routewerk-dev`, then walk the real
   flow: request a link → confirm the email actually arrives → click it →
   confirm `/verify-magic` mints a session and redirects to `next`. Check
   `fly logs -a routewerk-dev` for `"email sent"` (delivery) vs
   `"email not sent: SMTP not configured"` (misconfig).

## Relevant code

- Flag + guardrail: `internal/config/config.go` (`MagicLinkEnabled`,
  `Validate`)
- Send hardening: `internal/service/email.go` (`EmailService.send`,
  `requireDelivery`)
- Route gating: `internal/router/router.go` (`if cfg.MagicLinkEnabled`)
- Service/flow: `internal/service/magic_link.go`,
  `internal/handler/auth_magic.go` (request),
  `internal/handler/web/auth_magic.go` (verify + session mint)
- SPA entry: `web/spa/src/routes/sign-in/+page.svelte`
- Schema (dormant): `internal/database/migrations/000035_magic_link_tokens.*`
