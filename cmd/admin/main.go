package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/auth"
	"github.com/shotwell-paddle/routewerk/internal/config"
	"github.com/shotwell-paddle/routewerk/internal/database"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"

	// Register pgx5 driver for golang-migrate
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cfg := config.Load()
	ctx := context.Background()

	// Migration commands don't need a connection pool — they manage their own.
	switch os.Args[1] {
	case "migrate":
		migrateUp(cfg)
		return
	case "migrate-down":
		migrateDown(cfg)
		return
	case "migrate-version":
		migrateVersion(cfg)
		return
	case "migrate-force":
		migrateForce(cfg, os.Args[2:])
		return
	case "backup":
		// Manual one-off of the same pipeline the in-app scheduler runs
		// nightly (pg_dump → object storage → prune). Run on the server:
		//   fly ssh console -a routewerk -C "/app/admin backup"
		runBackup(ctx, cfg)
		return
	}

	db, err := database.Connect(cfg.DatabaseURL, cfg.IsDev())
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	switch os.Args[1] {
	case "create-org":
		createOrg(ctx, db, os.Args[2:])
	case "add-member":
		addMember(ctx, db, os.Args[2:])
	case "remove-member":
		removeMember(ctx, db, os.Args[2:])
	case "set-role":
		setRole(ctx, db, os.Args[2:])
	case "list-members":
		listMembers(ctx, db, os.Args[2:])
	case "list-orgs":
		listOrgs(ctx, db)
	case "reset-password":
		resetPassword(ctx, db, os.Args[2:])
	case "set-domain":
		setDomain(ctx, db, os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `routewerk-admin — platform administration tool

Usage:
  routewerk-admin <command> [arguments]

Database:
  migrate          Apply all pending database migrations.
  migrate-down     Roll back the last applied migration.
  migrate-version  Show current migration version.
  migrate-force <version>  Force migration version and clear dirty flag.
  backup           Take a database backup now (pg_dump → object storage).
                   Same pipeline the API runs nightly; run on the server:
                   fly ssh console -a routewerk -C "/app/admin backup"

Organizations:
  create-org   --name <name> --slug <slug> --owner-email <email>
               Create an organization and assign the owner (org_admin).

  add-member   --org <slug-or-id> --email <email> --role <role> [--location-id <id>]
               Add a user to an organization. Roles: org_admin, head_setter, setter, climber

  remove-member --org <slug-or-id> --email <email>
               Remove a user's membership from an organization.

  set-role     --org <slug-or-id> --email <email> --role <role>
               Change a user's role in an organization. Account-recovery
               tool: e.g. restore a head_setter who self-demoted. Errors
               if the user has multiple memberships in the org.
               Roles: org_admin, gym_manager, head_setter, setter, climber

  list-members --org <slug-or-id>
               List all members of an organization.

  list-orgs    List all organizations.

Users:
  reset-password --email <email> --password <new-password>
               Reset a user's password (admin override, no old password required).

Locations:
  set-domain   --location-id <id> --domain <hostname>
               Set the custom_domain for a location (e.g. routes.mosaicclimbing.com).
               Pass --domain "" to clear.

Environment:
  DATABASE_URL   PostgreSQL connection string (required)`)
}

// ============================================================
// migrate
// ============================================================

func migrateUp(cfg *config.Config) {
	fmt.Println("applying migrations...")
	if err := database.Migrate(cfg.DatabaseURL); err != nil {
		log.Fatalf("migration failed: %v", err)
	}
	fmt.Println("migrations applied successfully")
}

func migrateDown(cfg *config.Config) {
	fmt.Println("rolling back last migration...")
	if err := database.MigrateDown(cfg.DatabaseURL); err != nil {
		log.Fatalf("rollback failed: %v", err)
	}
	fmt.Println("rollback complete")
}

func migrateVersion(cfg *config.Config) {
	version, dirty, err := database.MigrateVersion(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to get version: %v", err)
	}
	state := "clean"
	if dirty {
		state = "DIRTY"
	}
	fmt.Printf("migration version: %d (%s)\n", version, state)
}

func migrateForce(cfg *config.Config, args []string) {
	if len(args) < 1 {
		log.Fatal("usage: migrate-force <version>")
	}
	version, err := strconv.Atoi(args[0])
	if err != nil {
		log.Fatalf("invalid version number: %s", args[0])
	}
	fmt.Printf("forcing migration version to %d...\n", version)
	if err := database.MigrateForce(cfg.DatabaseURL, version); err != nil {
		log.Fatalf("force failed: %v", err)
	}
	fmt.Println("done — dirty flag cleared")
}

// ============================================================
// create-org
// ============================================================

func createOrg(ctx context.Context, db *pgxpool.Pool, args []string) {
	var name, slug, ownerEmail string
	for i := 0; i < len(args)-1; i += 2 {
		switch args[i] {
		case "--name":
			name = args[i+1]
		case "--slug":
			slug = args[i+1]
		case "--owner-email":
			ownerEmail = args[i+1]
		}
	}

	if name == "" || ownerEmail == "" {
		fmt.Fprintln(os.Stderr, "error: --name and --owner-email are required")
		os.Exit(1)
	}
	if slug == "" {
		slug = slugify(name)
	}

	orgRepo := repository.NewOrgRepo(db)
	userRepo := repository.NewUserRepo(db)

	// Verify owner exists
	owner, err := userRepo.GetByEmail(ctx, strings.ToLower(strings.TrimSpace(ownerEmail)))
	if err != nil {
		log.Fatalf("database error: %v", err)
	}
	if owner == nil {
		log.Fatalf("user not found: %s (they must register first)", ownerEmail)
	}

	// Check slug uniqueness
	existing, err := orgRepo.GetBySlug(ctx, slug)
	if err != nil {
		log.Fatalf("database error: %v", err)
	}
	if existing != nil {
		log.Fatalf("org slug already taken: %s", slug)
	}

	// Create org
	org := &model.Organization{Name: name, Slug: slug}
	if err := orgRepo.Create(ctx, org); err != nil {
		log.Fatalf("failed to create org: %v", err)
	}

	// Assign owner as org_admin
	membership := &model.UserMembership{
		UserID: owner.ID,
		OrgID:  org.ID,
		Role:   "org_admin",
	}
	if err := orgRepo.AddMember(ctx, membership); err != nil {
		log.Fatalf("failed to add owner: %v", err)
	}

	fmt.Printf("created org %q (id=%s, slug=%s)\n", org.Name, org.ID, org.Slug)
	fmt.Printf("assigned %s as org_admin\n", ownerEmail)
}

// ============================================================
// add-member
// ============================================================

func addMember(ctx context.Context, db *pgxpool.Pool, args []string) {
	var orgRef, email, role, locationID string
	for i := 0; i < len(args)-1; i += 2 {
		switch args[i] {
		case "--org":
			orgRef = args[i+1]
		case "--email":
			email = args[i+1]
		case "--role":
			role = args[i+1]
		case "--location-id":
			locationID = args[i+1]
		}
	}

	if orgRef == "" || email == "" || role == "" {
		fmt.Fprintln(os.Stderr, "error: --org, --email, and --role are required")
		os.Exit(1)
	}

	validRoles := map[string]bool{"org_admin": true, "head_setter": true, "setter": true, "climber": true}
	if !validRoles[role] {
		log.Fatalf("invalid role: %s (valid: org_admin, head_setter, setter, climber)", role)
	}

	orgRepo := repository.NewOrgRepo(db)
	userRepo := repository.NewUserRepo(db)

	org := resolveOrg(ctx, orgRepo, orgRef)
	user, err := userRepo.GetByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		log.Fatalf("database error: %v", err)
	}
	if user == nil {
		log.Fatalf("user not found: %s", email)
	}

	m := &model.UserMembership{
		UserID: user.ID,
		OrgID:  org.ID,
		Role:   role,
	}
	if locationID != "" {
		m.LocationID = &locationID
	}

	if err := orgRepo.AddMember(ctx, m); err != nil {
		log.Fatalf("failed to add member: %v", err)
	}

	fmt.Printf("added %s to %q as %s\n", email, org.Name, role)
}

// ============================================================
// remove-member
// ============================================================

func removeMember(ctx context.Context, db *pgxpool.Pool, args []string) {
	var orgRef, email string
	for i := 0; i < len(args)-1; i += 2 {
		switch args[i] {
		case "--org":
			orgRef = args[i+1]
		case "--email":
			email = args[i+1]
		}
	}

	if orgRef == "" || email == "" {
		fmt.Fprintln(os.Stderr, "error: --org and --email are required")
		os.Exit(1)
	}

	orgRepo := repository.NewOrgRepo(db)
	userRepo := repository.NewUserRepo(db)

	org := resolveOrg(ctx, orgRepo, orgRef)
	user, err := userRepo.GetByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		log.Fatalf("database error: %v", err)
	}
	if user == nil {
		log.Fatalf("user not found: %s", email)
	}

	query := `UPDATE user_memberships SET deleted_at = NOW() WHERE user_id = $1 AND org_id = $2 AND deleted_at IS NULL`
	tag, err := db.Exec(ctx, query, user.ID, org.ID)
	if err != nil {
		log.Fatalf("database error: %v", err)
	}
	if tag.RowsAffected() == 0 {
		fmt.Printf("%s is not a member of %q\n", email, org.Name)
		return
	}

	fmt.Printf("removed %s from %q\n", email, org.Name)
}

// ============================================================
// set-role
// ============================================================

// setRole updates a user's role at their existing membership in an
// org. Account-recovery tool: when a head_setter self-demotes and
// locks themselves out of the team page, a peer manager can promote
// them back via the UI — but if no peer at the right level exists,
// this CLI is the escape hatch.
//
// The JSON team API enforces a privilege-escalation guard (caller
// must outrank target — see internal/handler/team.go); this CLI
// intentionally bypasses that since it's the rescue path used when
// no qualifying caller exists.
//
// Errors if the user has multiple active memberships in the org —
// the typical recovery case is a single membership and bulk-updating
// every membership row could destroy admin access (e.g. an org_admin
// who also holds a location-specific climber row).
func setRole(ctx context.Context, db *pgxpool.Pool, args []string) {
	var orgRef, email, role string
	for i := 0; i < len(args)-1; i += 2 {
		switch args[i] {
		case "--org":
			orgRef = args[i+1]
		case "--email":
			email = args[i+1]
		case "--role":
			role = args[i+1]
		}
	}

	if orgRef == "" || email == "" || role == "" {
		fmt.Fprintln(os.Stderr, "error: --org, --email, and --role are required")
		os.Exit(1)
	}

	validRoles := map[string]bool{"org_admin": true, "gym_manager": true, "head_setter": true, "setter": true, "climber": true}
	if !validRoles[role] {
		log.Fatalf("invalid role: %s (valid: org_admin, gym_manager, head_setter, setter, climber)", role)
	}

	orgRepo := repository.NewOrgRepo(db)
	userRepo := repository.NewUserRepo(db)

	org := resolveOrg(ctx, orgRepo, orgRef)
	user, err := userRepo.GetByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		log.Fatalf("database error: %v", err)
	}
	if user == nil {
		log.Fatalf("user not found: %s", email)
	}

	var memberCount int
	err = db.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_memberships WHERE user_id = $1 AND org_id = $2 AND deleted_at IS NULL`,
		user.ID, org.ID).Scan(&memberCount)
	if err != nil {
		log.Fatalf("database error: %v", err)
	}
	if memberCount == 0 {
		log.Fatalf("%s has no active membership in %q", email, org.Name)
	}
	if memberCount > 1 {
		log.Fatalf("%s has %d active memberships in %q — set-role does not support multi-membership users; patch the row directly via psql", email, memberCount, org.Name)
	}

	tag, err := db.Exec(ctx,
		`UPDATE user_memberships SET role = $1::user_role, updated_at = NOW() WHERE user_id = $2 AND org_id = $3 AND deleted_at IS NULL`,
		role, user.ID, org.ID,
	)
	if err != nil {
		log.Fatalf("database error: %v", err)
	}

	fmt.Printf("set role=%s for %s in %q (%d row updated)\n", role, email, org.Name, tag.RowsAffected())
}

// ============================================================
// list-members
// ============================================================

func listMembers(ctx context.Context, db *pgxpool.Pool, args []string) {
	var orgRef string
	for i := 0; i < len(args)-1; i += 2 {
		if args[i] == "--org" {
			orgRef = args[i+1]
		}
	}

	if orgRef == "" {
		fmt.Fprintln(os.Stderr, "error: --org is required")
		os.Exit(1)
	}

	orgRepo := repository.NewOrgRepo(db)
	org := resolveOrg(ctx, orgRepo, orgRef)

	query := `
		SELECT u.email, um.role, um.location_id, um.created_at
		FROM user_memberships um
		JOIN users u ON u.id = um.user_id
		WHERE um.org_id = $1 AND um.deleted_at IS NULL
		ORDER BY um.role, u.email`

	rows, err := db.Query(ctx, query, org.ID)
	if err != nil {
		log.Fatalf("database error: %v", err)
	}
	defer rows.Close()

	fmt.Printf("Members of %q (%s):\n", org.Name, org.ID)
	fmt.Printf("%-35s %-14s %-38s %s\n", "EMAIL", "ROLE", "LOCATION_ID", "JOINED")
	fmt.Println(strings.Repeat("-", 100))

	for rows.Next() {
		var email, role string
		var locationID *string
		var createdAt interface{}
		if err := rows.Scan(&email, &role, &locationID, &createdAt); err != nil {
			log.Fatalf("scan error: %v", err)
		}
		locStr := "(org-wide)"
		if locationID != nil {
			locStr = *locationID
		}
		fmt.Printf("%-35s %-14s %-38s %v\n", email, role, locStr, createdAt)
	}
}

// ============================================================
// list-orgs
// ============================================================

func listOrgs(ctx context.Context, db *pgxpool.Pool) {
	query := `
		SELECT o.id, o.name, o.slug, COUNT(um.id) as member_count
		FROM organizations o
		LEFT JOIN user_memberships um ON um.org_id = o.id AND um.deleted_at IS NULL
		WHERE o.deleted_at IS NULL
		GROUP BY o.id, o.name, o.slug
		ORDER BY o.name`

	rows, err := db.Query(ctx, query)
	if err != nil {
		log.Fatalf("database error: %v", err)
	}
	defer rows.Close()

	fmt.Printf("%-38s %-30s %-20s %s\n", "ID", "NAME", "SLUG", "MEMBERS")
	fmt.Println(strings.Repeat("-", 95))

	for rows.Next() {
		var id, name, slug string
		var count int
		if err := rows.Scan(&id, &name, &slug, &count); err != nil {
			log.Fatalf("scan error: %v", err)
		}
		fmt.Printf("%-38s %-30s %-20s %d\n", id, name, slug, count)
	}
}

// ============================================================
// Helpers
// ============================================================

func resolveOrg(ctx context.Context, orgRepo *repository.OrgRepo, ref string) *model.Organization {
	// Pick lookup path by format: only attempt GetByID for inputs that
	// are shaped like a UUID. The previous "try ID first, fall through
	// to slug" approach crashed Postgres with `invalid input syntax for
	// type uuid` on any non-UUID input (the SELECT on a uuid column
	// fails before the caller can fall through), making `--org <slug>`
	// unusable despite the help text advertising it.
	var org *model.Organization
	var err error
	if looksLikeUUID(ref) {
		org, err = orgRepo.GetByID(ctx, ref)
	} else {
		org, err = orgRepo.GetBySlug(ctx, ref)
	}
	if err != nil {
		log.Fatalf("database error: %v", err)
	}
	if org == nil {
		log.Fatalf("org not found: %s", ref)
	}
	return org
}

// looksLikeUUID returns true for canonical 36-char UUIDs with the
// 8-4-4-4-12 hex layout (e.g. "f5994c22-d690-4718-bc09-1bde813f7f98").
// Used by resolveOrg to dispatch between GetByID and GetBySlug
// without making Postgres parse a non-UUID into a uuid column.
func looksLikeUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
		if !isHex {
			return false
		}
	}
	return true
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	result := make([]byte, 0, len(s))
	lastDash := false
	for _, b := range []byte(s) {
		if (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') {
			result = append(result, b)
			lastDash = false
		} else if !lastDash && len(result) > 0 {
			result = append(result, '-')
			lastDash = true
		}
	}
	return strings.Trim(string(result), "-")
}

func resetPassword(ctx context.Context, db *pgxpool.Pool, args []string) {
	var email, password string
	for i := 0; i < len(args)-1; i += 2 {
		switch args[i] {
		case "--email":
			email = args[i+1]
		case "--password":
			password = args[i+1]
		}
	}

	if email == "" || password == "" {
		log.Fatal("usage: reset-password --email <email> --password <new-password>")
	}

	if len(password) < 8 {
		log.Fatal("password must be at least 8 characters")
	}
	if len(password) > 72 {
		log.Fatal("password must be 72 characters or fewer")
	}

	userRepo := repository.NewUserRepo(db)
	user, err := userRepo.GetByEmail(ctx, email)
	if err != nil {
		log.Fatalf("failed to look up user: %v", err)
	}
	if user == nil {
		log.Fatalf("no user found with email: %s", email)
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		log.Fatalf("failed to hash password: %v", err)
	}

	if err := userRepo.UpdatePassword(ctx, user.ID, hash); err != nil {
		log.Fatalf("failed to update password: %v", err)
	}

	fmt.Printf("Password reset for %s (%s)\n", user.DisplayName, user.Email)
}

func setDomain(ctx context.Context, db *pgxpool.Pool, args []string) {
	var locationID, domain string
	domainSet := false
	for i := 0; i < len(args)-1; i += 2 {
		switch args[i] {
		case "--location-id":
			locationID = args[i+1]
		case "--domain":
			domain = args[i+1]
			domainSet = true
		}
	}

	if locationID == "" || !domainSet {
		log.Fatal("usage: set-domain --location-id <id> --domain <hostname>")
	}

	locationRepo := repository.NewLocationRepo(db)
	loc, err := locationRepo.GetByID(ctx, locationID)
	if err != nil {
		log.Fatalf("database error: %v", err)
	}
	if loc == nil {
		log.Fatalf("location not found: %s", locationID)
	}

	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		loc.CustomDomain = nil
		fmt.Printf("clearing custom_domain for %q (%s)\n", loc.Name, loc.ID)
	} else {
		loc.CustomDomain = &domain
		fmt.Printf("setting custom_domain=%s for %q (%s)\n", domain, loc.Name, loc.ID)
	}

	if err := locationRepo.Update(ctx, loc); err != nil {
		log.Fatalf("failed to update location: %v", err)
	}

	fmt.Println("done")
}

// runBackup takes one backup with the same service the API's nightly
// scheduler uses. Needs pg_dump on PATH (present in the app image) and
// the STORAGE_* env — i.e. run it on the server via fly ssh console.
func runBackup(ctx context.Context, cfg *config.Config) {
	svc := service.NewBackupService(cfg)
	if svc == nil {
		log.Fatal("backup: object storage not configured (STORAGE_ENDPOINT/STORAGE_ACCESS_KEY)")
	}
	key, size, err := svc.RunOnce(ctx)
	if err != nil {
		log.Fatalf("backup failed: %v", err)
	}
	fmt.Printf("backup complete: %s (%d bytes)\n", key, size)
}
