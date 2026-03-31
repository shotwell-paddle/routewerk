package repository

import (
	"context"
	"testing"

	"github.com/shotwell-paddle/routewerk/internal/model"
)

func TestUserRepo_CreateAndGetByID(t *testing.T) {
	pool := testDB(t)
	repo := NewUserRepo(pool)
	ctx := context.Background()

	u := &model.User{
		Email:        "test@example.com",
		PasswordHash: "$2a$10$fakehashfortest",
		DisplayName:  "Test User",
	}

	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if u.ID == "" {
		t.Fatal("Create should populate ID")
	}
	if u.CreatedAt.IsZero() {
		t.Error("Create should populate CreatedAt")
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID returned nil for existing user")
	}
	if got.Email != "test@example.com" {
		t.Errorf("Email = %q, want %q", got.Email, "test@example.com")
	}
	if got.DisplayName != "Test User" {
		t.Errorf("DisplayName = %q, want %q", got.DisplayName, "Test User")
	}
	// Password hash should not be returned as empty (it's stored)
	if got.PasswordHash == "" {
		t.Error("PasswordHash should be populated from DB")
	}
}

func TestUserRepo_GetByID_NotFound(t *testing.T) {
	pool := testDB(t)
	repo := NewUserRepo(pool)
	ctx := context.Background()

	got, err := repo.GetByID(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got != nil {
		t.Error("GetByID should return nil for nonexistent user")
	}
}

func TestUserRepo_GetByEmail(t *testing.T) {
	pool := testDB(t)
	repo := NewUserRepo(pool)
	ctx := context.Background()

	u := &model.User{
		Email:        "find@example.com",
		PasswordHash: "$2a$10$fakehash",
		DisplayName:  "Findable User",
	}
	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByEmail(ctx, "find@example.com")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if got == nil {
		t.Fatal("GetByEmail returned nil for existing user")
	}
	if got.ID != u.ID {
		t.Errorf("ID = %q, want %q", got.ID, u.ID)
	}

	// Not found
	got, err = repo.GetByEmail(ctx, "nobody@example.com")
	if err != nil {
		t.Fatalf("GetByEmail (not found): %v", err)
	}
	if got != nil {
		t.Error("GetByEmail should return nil for nonexistent email")
	}
}

func TestUserRepo_CreateDuplicateEmail(t *testing.T) {
	pool := testDB(t)
	repo := NewUserRepo(pool)
	ctx := context.Background()

	u1 := &model.User{
		Email:        "dupe@example.com",
		PasswordHash: "$2a$10$hash1",
		DisplayName:  "User One",
	}
	if err := repo.Create(ctx, u1); err != nil {
		t.Fatalf("Create first: %v", err)
	}

	u2 := &model.User{
		Email:        "dupe@example.com",
		PasswordHash: "$2a$10$hash2",
		DisplayName:  "User Two",
	}
	err := repo.Create(ctx, u2)
	if err == nil {
		t.Error("Create should fail for duplicate email")
	}
}

func TestUserRepo_Update(t *testing.T) {
	pool := testDB(t)
	repo := NewUserRepo(pool)
	ctx := context.Background()

	u := &model.User{
		Email:        "update@example.com",
		PasswordHash: "$2a$10$hash",
		DisplayName:  "Original Name",
	}
	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("Create: %v", err)
	}

	u.DisplayName = "Updated Name"
	bio := "I climb V10"
	u.Bio = &bio
	if err := repo.Update(ctx, u); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.DisplayName != "Updated Name" {
		t.Errorf("DisplayName = %q, want %q", got.DisplayName, "Updated Name")
	}
	if got.Bio == nil || *got.Bio != "I climb V10" {
		t.Errorf("Bio = %v, want %q", got.Bio, "I climb V10")
	}
}

func TestUserRepo_UpdatePassword(t *testing.T) {
	pool := testDB(t)
	repo := NewUserRepo(pool)
	ctx := context.Background()

	u := &model.User{
		Email:        "pwchange@example.com",
		PasswordHash: "$2a$10$oldhash",
		DisplayName:  "PW Test",
	}
	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.UpdatePassword(ctx, u.ID, "$2a$10$newhash"); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.PasswordHash != "$2a$10$newhash" {
		t.Errorf("PasswordHash = %q, want %q", got.PasswordHash, "$2a$10$newhash")
	}
}
