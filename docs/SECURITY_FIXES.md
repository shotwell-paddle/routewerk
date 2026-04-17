# Security & Performance Fixes

Prioritized punch list from the April 2026 audit. Each item lists file:line, the root cause, the concrete fix (with code), and a test to lock the fix in. Work top-to-bottom; criticals first.

Legend: `[ ]` not started · `[~]` in progress · `[x]` done

---

## Contents

- [Critical](#critical)
  - [C1 — SMTP header injection](#c1--smtp-header-injection)
  - [C2 — Image decompression bomb](#c2--image-decompression-bomb)
  - [C3 — JWT issuer + audience not enforced](#c3--jwt-issuer--audience-not-enforced)
  - [C4 — Refresh tokens stored with bcrypt](#c4--refresh-tokens-stored-with-bcrypt)
- [High](#high)
  - [H1 — Cross-tenant data leak in route tag/rating lookups](#h1--cross-tenant-data-leak-in-route-tagrating-lookups)
  - [H2 — CSRF token never rotates](#h2--csrf-token-never-rotates)
  - [H3 — View-as role stored only in a cookie](#h3--view-as-role-stored-only-in-a-cookie)
  - [H4 — Audit log is fire-and-forget](#h4--audit-log-is-fire-and-forget)
  - [H5 — Image upload trusts client Content-Type](#h5--image-upload-trusts-client-content-type)
  - [H6 — Storage.Delete takes a URL, not a key](#h6--storagedelete-takes-a-url-not-a-key)
  - [H7 — Confirm API surface has no cookie-auth fallback](#h7--confirm-api-surface-has-no-cookie-auth-fallback)
- [Medium](#medium)
- [Low / hygiene](#low--hygiene)
- [Migration hazards](#migration-hazards)
- [Suggested order of operations](#suggested-order-of-operations)

---

## Critical

### C1 — SMTP header injection

- **Status:** `[ ]`
- **File:** `internal/service/email.go:159-169`
- **Problem:** `buildMIME` concatenates `from`, `to`, `subject` directly into SMTP headers with no CR/LF filtering. User-controlled fields (e.g. the display name that gets interpolated into templates, or any future user-editable subject) can inject additional headers (`Bcc:`, `Cc:`) or split the body.

**Fix.** Replace `buildMIME`:

```go
// sanitizeHeader removes CR and LF so user-controlled values cannot
// inject additional SMTP headers or split the body.
func sanitizeHeader(s string) string {
    return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

func buildMIME(from, to, subject, htmlBody string) []byte {
    var buf bytes.Buffer
    buf.WriteString("From: " + sanitizeHeader(from) + "\r\n")
    buf.WriteString("To: " + sanitizeHeader(to) + "\r\n")
    buf.WriteString("Subject: " + sanitizeHeader(subject) + "\r\n")
    buf.WriteString("MIME-Version: 1.0\r\n")
    buf.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
    buf.WriteString("\r\n")
    buf.WriteString(htmlBody)
    return buf.Bytes()
}
```

Validate the recipient before sending. In `Send`, just above `msg := buildMIME(...)`:

```go
if _, err := mail.ParseAddress(to); err != nil {
    return fmt.Errorf("invalid recipient %q: %w", to, err)
}
```

Add `"net/mail"` to the import block.

**Test (`internal/service/email_test.go`):**

```go
func TestBuildMIME_StripsCRLFFromHeaders(t *testing.T) {
    tests := []struct {
        name, subject   string
        wantContains    []string
        wantNotContains []string
    }{
        {
            name:            "injection via subject",
            subject:         "Hello\r\nBcc: attacker@example.com",
            wantContains:    []string{"Subject: HelloBcc: attacker@example.com\r\n"},
            wantNotContains: []string{"\r\nBcc:"},
        },
        {
            name:         "plain subject unchanged",
            subject:      "Reset your password",
            wantContains: []string{"Subject: Reset your password\r\n"},
        },
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            got := string(buildMIME("no-reply@routewerk.app", "user@example.com", tc.subject, "<p>hi</p>"))
            for _, s := range tc.wantContains {
                if !strings.Contains(got, s) {
                    t.Errorf("missing %q in\n%s", s, got)
                }
            }
            for _, s := range tc.wantNotContains {
                if strings.Contains(got, s) {
                    t.Errorf("unexpected %q in\n%s", s, got)
                }
            }
        })
    }
}
```

---

### C2 — Image decompression bomb

- **Status:** `[ ]`
- **File:** `internal/service/imageproc.go:40-44`
- **Problem:** `image.Decode(src)` is called with no byte cap and no pre-check on declared dimensions. A 5 MB upload can claim 65535×65535 pixels and allocate tens of GB during decode.

**Fix.** Decode the config first (reads just the header), reject oversized declared dimensions, then decode against an in-memory capped buffer. Replace `ProcessImage`:

```go
const (
    // maxInputBytes bounds the uploaded payload before decode.
    // Must match or exceed the handler's multipart cap.
    maxInputBytes = 5 * 1024 * 1024

    // maxInputPixels bounds the decoded image size to prevent
    // decompression bombs. 40 MP accommodates modern phone cameras
    // while rejecting pathological 65535x65535 declarations.
    maxInputPixels = 40 * 1000 * 1000
)

// ProcessImage decodes an uploaded image, resizes it if larger than the max
// dimensions, and re-encodes as JPEG (for JPEG/WebP inputs) or PNG (for PNG
// inputs with transparency). Rejects oversized payloads and decompression
// bombs before allocating pixel memory.
func ProcessImage(src io.Reader, contentType string) (*ProcessedImage, error) {
    raw, err := io.ReadAll(io.LimitReader(src, maxInputBytes+1))
    if err != nil {
        return nil, fmt.Errorf("read image: %w", err)
    }
    if len(raw) > maxInputBytes {
        return nil, fmt.Errorf("image exceeds %d bytes", maxInputBytes)
    }

    // Peek at declared dimensions before allocating pixel buffers.
    cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
    if err != nil {
        return nil, fmt.Errorf("decode image config: %w", err)
    }
    if int64(cfg.Width)*int64(cfg.Height) > maxInputPixels {
        return nil, fmt.Errorf("image dimensions %dx%d exceed maximum pixels", cfg.Width, cfg.Height)
    }

    img, _, err := image.Decode(bytes.NewReader(raw))
    if err != nil {
        return nil, fmt.Errorf("decode image: %w", err)
    }

    // … existing resize + re-encode code unchanged from bounds := img.Bounds() onwards …
}
```

**Test:**

```go
func TestProcessImage_RejectsOversizePayload(t *testing.T) {
    big := bytes.Repeat([]byte{0x00}, maxInputBytes+1)
    _, err := ProcessImage(bytes.NewReader(big), "image/jpeg")
    if err == nil || !strings.Contains(err.Error(), "exceeds") {
        t.Errorf("expected size error, got %v", err)
    }
}

// Craft a minimal PNG with a 65535x65535 IHDR. The decoder reads the
// header (cheap) and we reject before allocating pixels.
func TestProcessImage_RejectsDeclaredBomb(t *testing.T) {
    bomb := makeBombPNGHeader(65535, 65535)
    _, err := ProcessImage(bytes.NewReader(bomb), "image/png")
    if err == nil || !strings.Contains(err.Error(), "exceed maximum pixels") {
        t.Errorf("wrong error: %v", err)
    }
}
```

---

### C3 — JWT issuer + audience not enforced

- **Status:** `[ ]`
- **File:** `internal/auth/jwt.go:62-80` (also `jwt.go:86-112`)
- **Problem:** `ValidateAccessToken` verifies only the HMAC signature. It does not check `iss` or `aud`. If `JWT_SECRET` is ever shared across environments/services, tokens cross surfaces.

**Fix.** Add an audience, enforce issuer + audience + method on every parse.

```go
const (
    jwtIssuer   = "routewerk"
    jwtAudience = "routewerk-api"
)

func GenerateAccessToken(userID, email, secret string, expiry time.Duration) (string, time.Time, error) {
    expiresAt := time.Now().Add(expiry)

    claims := Claims{
        UserID: userID,
        Email:  email,
        RegisteredClaims: jwt.RegisteredClaims{
            Subject:   userID,
            Audience:  jwt.ClaimStrings{jwtAudience},
            ExpiresAt: jwt.NewNumericDate(expiresAt),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
            NotBefore: jwt.NewNumericDate(time.Now()),
            Issuer:    jwtIssuer,
        },
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    signed, err := token.SignedString([]byte(secret))
    if err != nil {
        return "", time.Time{}, fmt.Errorf("sign token: %w", err)
    }
    return signed, expiresAt, nil
}

func ValidateAccessToken(tokenStr, secret string) (*Claims, error) {
    token, err := jwt.ParseWithClaims(
        tokenStr,
        &Claims{},
        func(t *jwt.Token) (interface{}, error) {
            if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
                return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
            }
            return []byte(secret), nil
        },
        jwt.WithIssuer(jwtIssuer),
        jwt.WithAudience(jwtAudience),
        jwt.WithExpirationRequired(),
        jwt.WithValidMethods([]string{"HS256"}),
    )
    if err != nil {
        return nil, fmt.Errorf("parse token: %w", err)
    }
    claims, ok := token.Claims.(*Claims)
    if !ok || !token.Valid {
        return nil, fmt.Errorf("invalid token claims")
    }
    return claims, nil
}
```

`ParseExpiredClaims` uses `WithoutClaimsValidation`, so move the audience check into the post-parse block:

```go
func ParseExpiredClaims(tokenStr, secret string) (*Claims, error) {
    token, err := jwt.ParseWithClaims(
        tokenStr,
        &Claims{},
        func(t *jwt.Token) (interface{}, error) {
            if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
                return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
            }
            return []byte(secret), nil
        },
        jwt.WithValidMethods([]string{"HS256"}),
        jwt.WithExpirationRequired(),
        jwt.WithoutClaimsValidation(),
    )
    if err != nil {
        return nil, fmt.Errorf("parse token: %w", err)
    }
    claims, ok := token.Claims.(*Claims)
    if !ok {
        return nil, fmt.Errorf("invalid token claims")
    }
    if claims.Issuer != jwtIssuer {
        return nil, fmt.Errorf("invalid token issuer")
    }
    hasAud := false
    for _, a := range claims.Audience {
        if a == jwtAudience {
            hasAud = true
            break
        }
    }
    if !hasAud {
        return nil, fmt.Errorf("invalid token audience")
    }
    if claims.UserID == "" {
        return nil, fmt.Errorf("missing user_id in token")
    }
    return claims, nil
}
```

**Test:**

```go
func TestValidateAccessToken_RejectsWrongIssuer(t *testing.T) {
    claims := Claims{
        UserID: "u1", Email: "a@b.c",
        RegisteredClaims: jwt.RegisteredClaims{
            Issuer:    "attacker",
            Audience:  jwt.ClaimStrings{jwtAudience},
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
        },
    }
    tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("secret"))
    if _, err := ValidateAccessToken(tok, "secret"); err == nil {
        t.Fatal("expected issuer rejection")
    }
}

func TestValidateAccessToken_RejectsWrongAudience(t *testing.T) {
    claims := Claims{
        UserID: "u1", Email: "a@b.c",
        RegisteredClaims: jwt.RegisteredClaims{
            Issuer:    jwtIssuer,
            Audience:  jwt.ClaimStrings{"wrong-service"},
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
        },
    }
    tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("secret"))
    if _, err := ValidateAccessToken(tok, "secret"); err == nil {
        t.Fatal("expected audience rejection")
    }
}

func TestValidateAccessToken_RejectsAlgNone(t *testing.T) {
    tok := jwt.NewWithClaims(jwt.SigningMethodNone, Claims{
        UserID: "u1",
        RegisteredClaims: jwt.RegisteredClaims{
            Issuer:    jwtIssuer,
            Audience:  jwt.ClaimStrings{jwtAudience},
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
        },
    })
    s, _ := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
    if _, err := ValidateAccessToken(s, "secret"); err == nil {
        t.Fatal("expected method rejection")
    }
}
```

**Rollout — must not break live sessions.** Existing access tokens don't carry `aud`. Ship this change behind a flag:

1. Add a config field `EnforceJWTAudience bool` (default `false`, override via `JWT_ENFORCE_AUDIENCE=true`).
2. In `ValidateAccessToken`, build the parse-options slice conditionally — only append `jwt.WithAudience(jwtAudience)` when the flag is set. Always write the audience claim in `GenerateAccessToken` (safe regardless of the flag).
3. Deploy with the flag **off**. All newly-issued tokens get the audience claim.
4. Wait at least `JWT_EXPIRY` (15 min) + one refresh round-trip (≈ 20 min in practice) so every in-flight access token has rotated.
5. Flip `JWT_ENFORCE_AUDIENCE=true` via `fly secrets set` and restart. No user sees a 401 if timing is respected.

Code skeleton for the flagged parse:

```go
func ValidateAccessToken(tokenStr, secret string, enforceAudience bool) (*Claims, error) {
    opts := []jwt.ParserOption{
        jwt.WithIssuer(jwtIssuer),
        jwt.WithExpirationRequired(),
        jwt.WithValidMethods([]string{"HS256"}),
    }
    if enforceAudience {
        opts = append(opts, jwt.WithAudience(jwtAudience))
    }
    token, err := jwt.ParseWithClaims(tokenStr, &Claims{},
        func(t *jwt.Token) (interface{}, error) {
            if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
                return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
            }
            return []byte(secret), nil
        },
        opts...,
    )
    if err != nil {
        return nil, fmt.Errorf("parse token: %w", err)
    }
    claims, ok := token.Claims.(*Claims)
    if !ok || !token.Valid {
        return nil, fmt.Errorf("invalid token claims")
    }
    return claims, nil
}
```

Update the auth middleware call site to pass `cfg.EnforceJWTAudience`. Remember to add `WithIssuer` unconditionally — the previous code wasn't checking it at all, so flipping it on for everyone is the actual security win even before the audience enforcement lands.

---

### C4 — Refresh tokens stored with bcrypt

- **Status:** `[ ]`
- **Files:** `internal/auth/jwt.go:123-127`, `internal/service/auth.go:163-200`, `internal/repository/user.go`
- **Problem:** `HashRefreshToken` is commented "SHA-256 hash" but actually bcrypts. Because bcrypt salts every hash, lookup degrades into "load all of a user's active refresh tokens, run bcrypt.Compare on each". Default cost is ~100 ms per comparison, so a user with N devices pays N × 100 ms per refresh. Plus the name/comment are lying.

**Fix.** Switch to keyed HMAC-SHA256 using `JWT_SECRET` as the pepper. Deterministic, so you can `WHERE token_hash = $1`. Tokens are already 256-bit CSPRNG, so bcrypt's cost factor was wasted.

Replace the two functions in `internal/auth/jwt.go`:

```go
import (
    "crypto/hmac"
    "crypto/sha256"
    "crypto/subtle"
    // ... keep existing imports
)

// HashRefreshToken returns a keyed HMAC-SHA256 of the plaintext refresh
// token for indexed lookup. The secret acts as a pepper so a leaked DB
// alone cannot be used to forge lookups.
func HashRefreshToken(token, secret string) string {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(token))
    return hex.EncodeToString(mac.Sum(nil))
}

// CheckRefreshToken compares a plaintext refresh token against its stored
// HMAC hash using constant-time comparison.
func CheckRefreshToken(token, hash, secret string) bool {
    expected := HashRefreshToken(token, secret)
    return subtle.ConstantTimeCompare([]byte(expected), []byte(hash)) == 1
}
```

In `internal/service/auth.go`, rewrite `Refresh`:

```go
func (s *AuthService) Refresh(ctx context.Context, userID, refreshToken string) (*AuthResult, error) {
    tokenHash := auth.HashRefreshToken(refreshToken, s.jwtSecret)

    // Atomically revoke on match. Returns false if not found or already consumed.
    revoked, err := s.users.RevokeRefreshTokenByHash(ctx, userID, tokenHash)
    if err != nil {
        return nil, err
    }
    if !revoked {
        return nil, ErrInvalidRefresh
    }

    u, err := s.users.GetByID(ctx, userID)
    if err != nil {
        return nil, err
    }
    if u == nil {
        return nil, ErrUserNotFound
    }
    return s.generateResult(ctx, u)
}
```

And wherever `generateResult` creates a refresh token:

```go
refreshHash := auth.HashRefreshToken(refreshPlain, s.jwtSecret)
if err := s.users.SaveRefreshToken(ctx, u.ID, refreshHash, refreshExpiry); err != nil {
    return nil, err
}
```

In `internal/repository/user.go`, add:

```go
// RevokeRefreshTokenByHash marks a single refresh token as revoked if it
// exists, belongs to userID, and is not already revoked. Returns true if
// a row was updated.
func (r *UserRepo) RevokeRefreshTokenByHash(ctx context.Context, userID, tokenHash string) (bool, error) {
    tag, err := r.db.Exec(ctx, `
        UPDATE refresh_tokens
        SET revoked_at = NOW()
        WHERE user_id = $1
          AND token_hash = $2
          AND revoked_at IS NULL
          AND expires_at > NOW()`,
        userID, tokenHash)
    if err != nil {
        return false, fmt.Errorf("revoke refresh token: %w", err)
    }
    return tag.RowsAffected() == 1, nil
}
```

Tighten the loose signature (L2):

```go
func (r *UserRepo) SaveRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
    _, err := r.db.Exec(ctx, `
        INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
        VALUES ($1, $2, $3)`,
        userID, tokenHash, expiresAt)
    if err != nil {
        return fmt.Errorf("save refresh token: %w", err)
    }
    return nil
}
```

Remove `GetActiveRefreshTokens` and the original `RevokeRefreshToken(hash string)` once no callers remain.

**Migration.** Live production — cannot wipe `refresh_tokens`. Roll out over two deploys with a dual-scheme column.

*Deploy 1 — writes in HMAC, reads either scheme.*

`internal/database/migrations/000017_refresh_token_hash_scheme.up.sql`:

```sql
ALTER TABLE refresh_tokens ADD COLUMN hash_scheme TEXT NOT NULL DEFAULT 'bcrypt';
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_hash
  ON refresh_tokens (token_hash)
  WHERE revoked_at IS NULL AND hash_scheme = 'hmac';
```

Down:

```sql
DROP INDEX IF EXISTS idx_refresh_tokens_hash;
ALTER TABLE refresh_tokens DROP COLUMN hash_scheme;
```

Store `hash_scheme = 'hmac'` on every new refresh token. Read path in `AuthService.Refresh`:

```go
func (s *AuthService) Refresh(ctx context.Context, userID, refreshToken string) (*AuthResult, error) {
    // Fast path: HMAC lookup (deterministic, indexed).
    tokenHash := auth.HashRefreshToken(refreshToken, s.jwtSecret)
    revoked, err := s.users.RevokeRefreshTokenByHashScheme(ctx, userID, tokenHash, "hmac")
    if err != nil {
        return nil, err
    }
    if !revoked {
        // Slow path: bcrypt fallback for pre-migration tokens. Remove after
        // all bcrypt tokens have expired (RefreshTokenExpiry = 30 d).
        bcryptHashes, err := s.users.GetActiveRefreshTokenHashesByScheme(ctx, userID, "bcrypt")
        if err != nil {
            return nil, err
        }
        var matched string
        for _, h := range bcryptHashes {
            if auth.CheckRefreshTokenBcrypt(refreshToken, h) {
                matched = h
                break
            }
        }
        if matched == "" {
            return nil, ErrInvalidRefresh
        }
        ok, err := s.users.RevokeRefreshTokenByHashScheme(ctx, userID, matched, "bcrypt")
        if err != nil {
            return nil, err
        }
        if !ok {
            return nil, ErrInvalidRefresh
        }
    }

    u, err := s.users.GetByID(ctx, userID)
    if err != nil {
        return nil, err
    }
    if u == nil {
        return nil, ErrUserNotFound
    }
    return s.generateResult(ctx, u)
}
```

Keep `CheckRefreshTokenBcrypt` (the old behaviour) around only for the fallback path — rename the function in `internal/auth/jwt.go` to make it obvious it's legacy:

```go
// CheckRefreshTokenBcrypt compares a plaintext refresh token against a legacy
// bcrypt hash. Only used for tokens issued before the HMAC migration.
// Remove after the migration window (RefreshTokenExpiry) passes.
func CheckRefreshTokenBcrypt(token, hash string) bool {
    return bcrypt.CompareHashAndPassword([]byte(hash), []byte(token)) == nil
}
```

Repo additions:

```go
func (r *UserRepo) RevokeRefreshTokenByHashScheme(ctx context.Context, userID, tokenHash, scheme string) (bool, error) {
    tag, err := r.db.Exec(ctx, `
        UPDATE refresh_tokens
        SET revoked_at = NOW()
        WHERE user_id = $1
          AND token_hash = $2
          AND hash_scheme = $3
          AND revoked_at IS NULL
          AND expires_at > NOW()`,
        userID, tokenHash, scheme)
    if err != nil {
        return false, fmt.Errorf("revoke refresh token: %w", err)
    }
    return tag.RowsAffected() == 1, nil
}

func (r *UserRepo) GetActiveRefreshTokenHashesByScheme(ctx context.Context, userID, scheme string) ([]string, error) {
    rows, err := r.db.Query(ctx, `
        SELECT token_hash FROM refresh_tokens
        WHERE user_id = $1
          AND hash_scheme = $2
          AND revoked_at IS NULL
          AND expires_at > NOW()`,
        userID, scheme)
    if err != nil {
        return nil, fmt.Errorf("list active refresh tokens: %w", err)
    }
    defer rows.Close()
    var hashes []string
    for rows.Next() {
        var h string
        if err := rows.Scan(&h); err != nil {
            return nil, err
        }
        hashes = append(hashes, h)
    }
    return hashes, rows.Err()
}
```

Write path in `generateResult`:

```go
refreshHash := auth.HashRefreshToken(refreshPlain, s.jwtSecret)
if err := s.users.SaveRefreshTokenWithScheme(ctx, u.ID, refreshHash, "hmac", refreshExpiry); err != nil {
    return nil, err
}
```

```go
func (r *UserRepo) SaveRefreshTokenWithScheme(ctx context.Context, userID, tokenHash, scheme string, expiresAt time.Time) error {
    _, err := r.db.Exec(ctx, `
        INSERT INTO refresh_tokens (user_id, token_hash, hash_scheme, expires_at)
        VALUES ($1, $2, $3, $4)`,
        userID, tokenHash, scheme, expiresAt)
    if err != nil {
        return fmt.Errorf("save refresh token: %w", err)
    }
    return nil
}
```

*Observation window.* Track `audit_log` or a counter metric for "bcrypt refresh path taken" vs "hmac refresh path taken". Once the bcrypt counter stops incrementing for ≥ `RefreshTokenExpiry` (30 d by default), schedule deploy 2.

*Deploy 2 — remove the fallback.*

`internal/database/migrations/000018_refresh_token_remove_bcrypt.up.sql`:

```sql
DELETE FROM refresh_tokens WHERE hash_scheme = 'bcrypt';
-- optional: drop the column once the fallback code path is removed
-- ALTER TABLE refresh_tokens DROP COLUMN hash_scheme;
```

Remove `CheckRefreshTokenBcrypt`, `GetActiveRefreshTokenHashesByScheme`, and the bcrypt branch of `Refresh` in the same release.

**Test:**

```go
func TestHashRefreshToken_DeterministicAndKeyed(t *testing.T) {
    h1 := HashRefreshToken("abc", "secret1")
    h2 := HashRefreshToken("abc", "secret1")
    h3 := HashRefreshToken("abc", "secret2")
    if h1 != h2 {
        t.Error("HMAC should be deterministic")
    }
    if h1 == h3 {
        t.Error("different secrets should produce different hashes")
    }
}

func TestCheckRefreshToken_Constant(t *testing.T) {
    h := HashRefreshToken("token", "s")
    if !CheckRefreshToken("token", h, "s") {
        t.Error("valid token should match")
    }
    if CheckRefreshToken("wrong", h, "s") {
        t.Error("wrong token should not match")
    }
    if CheckRefreshToken("token", h, "other-secret") {
        t.Error("wrong secret should not match")
    }
}
```

See [Migration hazards](#migration-hazards) for the dual-deploy rollout — you cannot drop the bcrypt path immediately without logging out every active user.

---

## High

### H1 — Cross-tenant data leak in route tag/rating lookups

- **Status:** `[ ]`
- **Files:** `internal/repository/route.go:444-466`, `internal/repository/rating.go:59-100`
- **Problem:** `GetTags(routeID)` and `ListByRoute(routeID, ...)` return rows for any route id, with no tenant/org check. Callers are expected to do their own `checkLocationOwnership` first — defence-by-convention.

**Fix.** Require a scoping identifier in the signature so the query can enforce it:

```go
// GetTags returns tags for a route scoped to the caller's location, returning
// an empty slice if the route is not in that location (or doesn't exist).
func (r *RouteRepo) GetTags(ctx context.Context, locationID, routeID string) ([]model.Tag, error) {
    query := `
        SELECT t.id, t.org_id, t.category, t.name, t.color
        FROM tags t
        JOIN route_tags rt ON rt.tag_id = t.id
        JOIN routes r ON r.id = rt.route_id
        WHERE rt.route_id = $1
          AND r.location_id = $2
          AND r.deleted_at IS NULL`
    rows, err := r.db.Query(ctx, query, routeID, locationID)
    // … same scanning loop
}
```

Do the same for `RatingRepo.ListByRoute`. Every caller already has `locationID` in scope (from `checkLocationOwnership`), so plumbing is minimal.

**Migration plan.**
1. Add the new methods (`GetTagsForLocation`, etc.) next to the old ones.
2. Migrate callers one by one, verifying each compiles and tests pass.
3. Delete the old signatures.

**Defensive sweep.** Grep for other repo methods that take an ID without a scoping parameter:

```
grep -n 'ctx context.Context, [a-zA-Z]*ID string)' internal/repository/*.go
```

Review each hit — anything that returns rows keyed off that ID without a join to a tenant-scoping table is a candidate.

---

### H2 — CSRF token never rotates

- **Status:** `[ ]`
- **File:** `internal/middleware/csrf.go:66-73`
- **Problem:** Token is set on first request and never rotated. Any XSS/DOM leak gives an attacker persistent CSRF power across the session lifetime. The `HttpOnly: false` cookie is a deliberate choice for HTMX, so the defense has to be rotation, not confidentiality.

**Fix.** Rotate the token on privilege boundaries: login, logout, password change, role change. Expose a helper:

```go
// Rotate generates a fresh CSRF token and rewrites the cookie. Call on
// login/logout/password-change/role-change so a leaked token becomes stale.
func (c *CSRFProtection) Rotate(w http.ResponseWriter) (string, error) {
    token, err := generateCSRFToken()
    if err != nil {
        return "", err
    }
    http.SetCookie(w, &http.Cookie{
        Name:     csrfCookieName,
        Value:    token,
        Path:     "/",
        HttpOnly: false,
        Secure:   c.secure,
        SameSite: http.SameSiteStrictMode,
    })
    return token, nil
}
```

Call it from:
- `LoginSubmit` after creating the session.
- `LogoutSubmit` before redirecting.
- `ChangePassword` handler after the DB update.
- Any role-change handler in `handler/web/org_settings.go`.

**Stronger alternative.** Drop the double-submit cookie, store the canonical token on the session row, emit it via `<meta name="csrf-token">` for HTMX to read. Removes the XSS-accessible cookie entirely. Larger change — file as a follow-up.

**Test:** hit `/login`, capture the cookie, log in, confirm cookie changed.

---

### H3 — View-as role stored only in a cookie

- **Status:** `[ ]`
- **File:** `internal/middleware/websession.go:311-327`
- **Problem:** Admins use "view as" to downgrade role for testing. Selection lives in a cookie: clearing it silently escalates to real role, and any XSS can toggle it. Worse, audit entries don't record that a write happened *while* in a pretended role.

**Fix.** Persist `view_as_role` on `web_sessions`, read it from the DB on each request, stamp it into audit entries.

Migration `000018_web_sessions_view_as_role.up.sql`:

```sql
ALTER TABLE web_sessions ADD COLUMN view_as_role TEXT;
```

Down:

```sql
ALTER TABLE web_sessions DROP COLUMN view_as_role;
```

In `applyViewAsOverride`, read from `session.ViewAsRole` instead of the cookie. Provide a setter `SetViewAsRole(ctx, sessionID, role)` on `WebSessionRepo` and expose it via the admin "view as" endpoint.

In `AuditService.Record`, add `view_as_role` to the meta map whenever it's set.

---

### H4 — Audit log is fire-and-forget

- **Status:** `[ ]`
- **File:** `internal/service/audit.go:44-46`
- **Problem:** Entries go onto a background goroutine with `context.Background()`. SIGKILL or an unclean shutdown drops them. Audit is the one thing you need to survive a crash.

**Fix (minimum).** Write synchronously for state-changing actions:

```go
func (s *AuditService) Record(r *http.Request, action, resource, resourceID, orgID string, meta map[string]interface{}) {
    actorID := middleware.GetUserID(r.Context())
    if actorID == "" {
        actorID = "system"
    }
    entry := repository.AuditEntry{ /* ... */ }

    // Sync — if audit DB is unavailable, surface the error.
    if err := s.repo.Log(r.Context(), entry); err != nil {
        slog.Error("audit log failed", "action", action, "resource", resource, "error", err)
    }
}
```

**Fix (better).** Keep async for high-volume read-path telemetry, sync for writes. Take an explicit `Sync bool` parameter or split into `Record` vs `RecordAsync`.

**Fix (if you keep async).** Add a bounded channel + `sync.WaitGroup`, drain it from `main.go` during `srv.Shutdown` with the same 15 s deadline.

---

### H5 — Image upload trusts client Content-Type

- **Status:** `[ ]`
- **File:** `internal/handler/web/photos.go:71-75`
- **Problem:** The allow-list check uses the multipart header's Content-Type, which is attacker-controlled. `ProcessImage` re-encodes so embedded payloads are mostly neutered, but arbitrary binary still reaches `image.Decode` — see C2.

**Fix.** Sniff the first 512 bytes:

```go
// Read the first 512 bytes for content sniffing without consuming the reader.
sniff := make([]byte, 512)
n, _ := io.ReadFull(file, sniff)
sniff = sniff[:n]
if _, err := file.Seek(0, io.SeekStart); err != nil {
    http.Error(w, "failed to rewind upload", http.StatusInternalServerError)
    return
}

detected := http.DetectContentType(sniff)
if !allowedImageTypes[detected] {
    http.Error(w, "Only JPEG, PNG, and WebP images are allowed", http.StatusBadRequest)
    return
}
// pass `detected` to ProcessImage instead of the client header
```

The multipart File returned by `r.FormFile` is a `multipart.File` which supports `Seek` — so this works without extra buffering.

---

### H6 — Storage.Delete takes a URL, not a key

- **Status:** `[ ]`
- **File:** `internal/service/storage.go:74-91`
- **Problem:** `Delete(photoURL)` derives the S3 key by trimming a URL prefix. Currently the URL comes from the DB (safe), but the signature invites future misuse (e.g. a "delete-by-URL" endpoint where a crafted Referer could escape the prefix check).

**Fix.** Change the contract:

```go
// Delete removes an object by its storage key. Callers should pass the key
// persisted alongside the photo row rather than reconstructing it from a URL.
func (s *StorageService) Delete(ctx context.Context, key string) error {
    if !strings.HasPrefix(key, "photos/") {
        return fmt.Errorf("invalid key: must be under photos/")
    }
    if strings.Contains(key, "..") {
        return fmt.Errorf("invalid key: path traversal")
    }
    _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
        Bucket: aws.String(s.bucket),
        Key:    aws.String(key),
    })
    return err
}
```

This requires storing the object key on `route_photos` rows (if not already). If the DB currently stores only the URL, add a migration `000019_route_photos_storage_key.up.sql`:

```sql
ALTER TABLE route_photos ADD COLUMN storage_key TEXT;
UPDATE route_photos
SET storage_key = substring(url from 'photos/.*$')
WHERE storage_key IS NULL;
```

---

### H7 — Confirm API surface has no cookie-auth fallback

- **Status:** `[ ]`
- **Scope:** verification only, no code change unless you find a fallback.
- **Checks to run:**
  - `grep -n 'GetWebUser\|web_session' internal/handler/*.go` — API handlers (not `handler/web/`) should never call these.
  - `grep -n 'r.Cookie(' internal/middleware/auth.go` — the API auth middleware should read only `Authorization:` headers.
  - CORS in `internal/router/router.go:63` — `AllowedOrigins` must be a concrete list, not `["*"]`, when `AllowCredentials: true`.
  - If the future native app is Capacitor/TWA and *does* use cookies, you'll need CSRF on the API too. Note it in the roadmap.

---

## Medium

Short entries — full fix code lives with each as needed.

### M1 — `Validate()` skips all checks in dev
`internal/config/config.go:95-97`. Always run a non-emptiness check on `DatabaseURL`, `JWTSecret`, `SessionSecret`, `FrontendURL`; gate the strict rules on `Env == "production"` instead of returning early on `IsDev()`.

### M2 — Event bus detaches request context
`internal/event/memory.go:56-69`. Wrap async handlers with `context.WithTimeout(context.Background(), 30*time.Second)`, register a `sync.WaitGroup`, drain it from `bus.Shutdown(ctx)` in `main.go`.

### M3 — Job queue shutdown not coordinated
`cmd/api/main.go:130-138`, `internal/jobs/queue.go`. Expose `queue.Shutdown(ctx)` that cancels intake and waits on an in-flight `WaitGroup`. Share the 15 s deadline with `srv.Shutdown`.

### M4 — Analytics query deadline re-wrapping
`internal/repository/analytics.go:21-60`. Take `ctx` as-is and never re-wrap inside the method; the caller owns the budget. (Re-verify by tracing every call path; the current code looks OK but future refactors can regress.)

### M5 — No per-route rate limits on reset / upload
Add stricter limiters than the global 120 req/min:

- Password reset request: 5/hour keyed by email.
- Photo upload: 10/min keyed by user id.

Wire them in `internal/router/router.go` alongside the existing `authLimiter`.

### M6 — Recovery middleware always logs stack traces
`internal/middleware/recovery.go:20-35`. Emit `stack` only at debug log level in production, or ship to a dedicated error sink.

### M7 — Session not bound to any device signal
`internal/middleware/websession.go:114-135`. On each request, compare `X-Forwarded-For` against the stored IP with a grace window; revoke on mismatch. Optional: bind a hashed User-Agent as well.

### M8 — Profanity filter runs on every render
`internal/service/profanity.go:49-56`. Move the check to write time (on route/tag/wall create/update), store `name_safe` or a `flagged_at` timestamp. Read path becomes a pure column lookup.

---

## Low / hygiene

- **L1** Rename `HashRefreshToken` comment and method name — already addressed by C4.
- **L2** `SaveRefreshToken(..., expiresAt interface{})` → `time.Time` — already addressed by C4.
- **L3** `.env.example` literal password → change to `CHANGE_ME_LOCAL_ONLY` with a prominent comment.
- **L4** `Dockerfile` pins `alpine:3.19`; move to `alpine:3.19.x` and add Dependabot/Renovate for base images.
- **L5** Repo-layer errors (`fmt.Errorf("count routes: %w", ...)`) are fine for logs but shouldn't reach the client verbatim — audit `handler/apperror.go` usage and make sure every JSON error response goes through the sanitizer.

---

## Migration hazards

**Deployment context: live production with real users.** Hard cutovers that invalidate sessions or refresh tokens are unacceptable. Both criticals below use flagged / dual-scheme rollouts.

**C3 — JWT audience.** Full rollout plan is in the C3 section. Summary:

1. Deploy 1: write `aud` on new tokens, keep validation unflagged (issuer-only).
2. Wait > 15 min (access token TTL) so every in-flight token is either re-minted or expired.
3. Deploy 2 (or `fly secrets set JWT_ENFORCE_AUDIENCE=true` + restart): flip the flag. No user sees a 401.

Also unconditionally add `jwt.WithIssuer(jwtIssuer)` in step 1 — that closes the current gap (issuer is not enforced at all today) without breaking anyone.

**C4 — Refresh token format.** Full rollout plan is in the C4 section. Summary:

1. Deploy 1: add `hash_scheme` column, write `'hmac'` for all new tokens, keep `'bcrypt'` fallback for reads.
2. Observe the "bcrypt path taken" metric. It should trend toward zero as old refresh tokens expire (30 d max).
3. Once that metric has been zero for ≥ a week, Deploy 2: delete legacy rows, remove fallback code, optionally drop the column.

Expected total timeline: ~5 weeks (30 d to exhaust existing refresh tokens + observation). Resist the temptation to shorten by truncating `refresh_tokens` — that force-logs-out every user.

**H3 — View-as role column.** Pure additive migration (`ALTER TABLE ADD COLUMN`). Handler code keeps reading the cookie until Deploy 1 ships, then transitions to DB. No data loss either way since the cookie is ephemeral.

**H6 — Storage key column.** Additive migration with a backfill `UPDATE` from the existing URL. Since the URL is deterministic (`https://{endpoint}/{bucket}/photos/{id}.{ext}`), the regex extract is safe. Run the backfill in a transaction and verify row counts match before switching the delete path.

---

## Suggested order of operations

Each step is shippable on its own; none depend on later steps.

1. **C1 SMTP sanitize** — ~30 min, zero migration risk.
2. **C2 image bomb guard** — ~1 hour with tests.
3. **H5 content-type sniff** — pairs naturally with C2.
4. **H1 tenant scoping** — a few hours; also sweep for similar methods.
5. **H4 audit sync** — ~1 hour; behaviour change is visible if the audit DB hiccups, so watch error rates post-deploy.
6. **C3 JWT aud/iss** — ship flag-off, verify in prod, flip flag. ~2 hours + 15-minute verification window.
7. **C4 refresh token HMAC** + index migration + truncate `refresh_tokens`. Schedule during a low-traffic window. ~3 hours.
8. **H6 storage-by-key** — needs a migration and a caller sweep. ~2 hours.
9. **H2 CSRF rotation** — ~1 hour.
10. **H3 view-as persistence** — ~2 hours (schema + handler + audit hook).
11. **H7 confirm API has no cookie fallback** — read-only audit, ~30 min.
12. Mediums/Lows as they fit into sprints.

---

## Changelog

| Date | Item | Who |
|------|------|-----|
| 2026-04-17 | Initial audit | Claude |
