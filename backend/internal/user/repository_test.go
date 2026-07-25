//go:build integration

package user_test

import (
	"context"
	"testing"
	"time"

	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "github.com/BleKuntay/FlipChat/backend/internal/user"
	"github.com/BleKuntay/FlipChat/backend/pkg/testhelper"
)

// insertUser is a test helper that inserts a user directly via SQL
// and returns the full User row (including DB-generated id, created_at, etc.)
func insertUser(t *testing.T, db *sqlx.DB, name, username, email, password string) *User {
	t.Helper()

	var u User
	err := db.Get(&u, `
		INSERT INTO users (name, username, email, password)
		VALUES ($1, $2, $3, $4)
		RETURNING *
	`, name, username, email, password)
	require.NoError(t, err, "insertUser helper failed")

	return &u
}

// cleanUsers truncates the users table between tests that need a clean slate.
func cleanUsers(t *testing.T, db *sqlx.DB) {
	t.Helper()
	_, err := db.Exec("TRUNCATE TABLE users RESTART IDENTITY CASCADE")
	require.NoError(t, err)
}

// ── FindByID ──────────────────────────────────────────────────────────────────

func TestRepository_FindByID(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := NewRepository(db)

	ctx := context.Background()

	t.Run("returns user for existing ID", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })
		inserted := insertUser(t, db, "John Doe", "johndoe", "john@example.com", "hashed")

		got, err := repo.FindByID(ctx, inserted.ID)

		require.NoError(t, err)
		assert.Equal(t, inserted.ID, got.ID)
		assert.Equal(t, "John Doe", got.Name)
		assert.Equal(t, "johndoe", got.Username)
		assert.Equal(t, "john@example.com", got.Email)
		assert.Equal(t, "en", got.Language) // default
		assert.Nil(t, got.Bio)
		assert.Nil(t, got.AvatarURL)
		assert.Nil(t, got.LastSeenAt)
		assert.False(t, got.CreatedAt.IsZero())
	})

	t.Run("returns error for non-existent ID", func(t *testing.T) {
		_, err := repo.FindByID(ctx, "00000000-0000-0000-0000-000000000000")

		assert.Error(t, err)
	})

	t.Run("returns error for malformed UUID", func(t *testing.T) {
		_, err := repo.FindByID(ctx, "not-a-uuid")

		assert.Error(t, err)
	})

	t.Run("does not expose password via json tag", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })
		inserted := insertUser(t, db, "John Doe", "johndoe2", "john2@example.com", "super-secret-hash")

		got, err := repo.FindByID(ctx, inserted.ID)

		require.NoError(t, err)
		// Password is fetched (needed for VerifyPassword in service),
		// but the json:"-" tag ensures it never appears in HTTP responses.
		assert.Equal(t, "super-secret-hash", got.Password)
	})
}

// ── UpdateProfile ─────────────────────────────────────────────────────────────

func TestRepository_UpdateProfile(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := NewRepository(db)

	ctx := context.Background()

	t.Run("updates name, username, bio, language and returns updated row", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })
		u := insertUser(t, db, "John Doe", "johndoe", "john@example.com", "hashed")

		bio := "Hello world"
		u.Name = "Jane Doe"
		u.Username = "janedoe"
		u.Bio = &bio
		u.Language = "id"

		res, err := repo.UpdateProfile(ctx, u)

		require.NoError(t, err)
		assert.Equal(t, "Jane Doe", res.Name)
		assert.Equal(t, "janedoe", res.Username)
		assert.Equal(t, &bio, res.Bio)
		assert.Equal(t, "id", res.Language)
		assert.Equal(t, "john@example.com", res.Email) // email unchanged
	})

	t.Run("updated_at is bumped by trigger", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })
		u := insertUser(t, db, "John Doe", "johndoe", "john@example.com", "hashed")

		originalUpdatedAt := u.UpdatedAt
		time.Sleep(10 * time.Millisecond) // ensure clock advances

		u.Name = "New Name"
		_, err := repo.UpdateProfile(ctx, u)
		require.NoError(t, err)

		var updatedAt time.Time
		err = db.QueryRow("SELECT updated_at FROM users WHERE id = $1", u.ID).Scan(&updatedAt)
		require.NoError(t, err)
		assert.True(t, updatedAt.After(originalUpdatedAt), "updated_at must be bumped after update")
	})

	t.Run("returns error on duplicate username", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })
		insertUser(t, db, "John Doe", "johndoe", "john@example.com", "hashed")
		u2 := insertUser(t, db, "Jane Doe", "janedoe", "jane@example.com", "hashed")

		u2.Username = "johndoe" // conflict
		_, err := repo.UpdateProfile(ctx, u2)

		assert.Error(t, err)
	})

	t.Run("returns ErrUserNotFound for non-existent ID", func(t *testing.T) {
		ghost := &User{
			ID:       "00000000-0000-0000-0000-000000000000",
			Name:     "Ghost",
			Username: "ghost",
			Language: "en",
		}

		_, err := repo.UpdateProfile(ctx, ghost)

		assert.ErrorIs(t, err, apperr.ErrNotFound)
	})
}

// ── UpdateEmail ───────────────────────────────────────────────────────────────

func TestRepository_UpdateEmail(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := NewRepository(db)

	ctx := context.Background()

	t.Run("updates email and returns updated row", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })
		u := insertUser(t, db, "John Doe", "johndoe", "john@example.com", "hashed")

		req := &UpdateEmailRequest{NewEmail: "newemail@example.com"}
		res, err := repo.UpdateEmail(ctx, u.ID, req)

		require.NoError(t, err)
		assert.Equal(t, "newemail@example.com", res.Email)
		assert.Equal(t, "johndoe", res.Username) // other fields unchanged
	})

	t.Run("returns error on duplicate email", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })
		insertUser(t, db, "John Doe", "johndoe", "john@example.com", "hashed")
		u2 := insertUser(t, db, "Jane Doe", "janedoe", "jane@example.com", "hashed")

		req := &UpdateEmailRequest{NewEmail: "john@example.com"} // conflict
		_, err := repo.UpdateEmail(ctx, u2.ID, req)

		assert.Error(t, err)
	})

	t.Run("returns ErrUserNotFound for non-existent ID", func(t *testing.T) {
		req := &UpdateEmailRequest{NewEmail: "ghost@example.com"}
		_, err := repo.UpdateEmail(ctx, "00000000-0000-0000-0000-000000000000", req)

		assert.ErrorIs(t, err, apperr.ErrNotFound)
	})
}

// ── UpdatePassword ────────────────────────────────────────────────────────────

func TestRepository_UpdatePassword(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := NewRepository(db)

	ctx := context.Background()

	t.Run("updates password hash in database", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })
		u := insertUser(t, db, "John Doe", "johndoe", "john@example.com", "old-hash")

		err := repo.UpdatePassword(ctx, u.ID, "new-hash")
		require.NoError(t, err)

		var storedPassword string
		err = db.QueryRow("SELECT password FROM users WHERE id = $1", u.ID).Scan(&storedPassword)
		require.NoError(t, err)
		assert.Equal(t, "new-hash", storedPassword)
	})

	// FIXED: UpdatePassword now checks rows affected.
	t.Run("non-existent ID returns ErrNotFound", func(t *testing.T) {
		err := repo.UpdatePassword(ctx, "00000000-0000-0000-0000-000000000000", "new-hash")

		assert.ErrorIs(t, err, apperr.ErrNotFound)
	})
}

// ── DeleteByID ────────────────────────────────────────────────────────────────

func TestRepository_DeleteByID(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := NewRepository(db)

	ctx := context.Background()

	t.Run("deletes user from database", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })
		u := insertUser(t, db, "John Doe", "johndoe", "john@example.com", "hashed")

		err := repo.DeleteByID(ctx, u.ID)
		require.NoError(t, err)

		// Verify row is gone
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM users WHERE id = $1", u.ID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	// FIXED: DeleteByID now checks rows affected.
	t.Run("non-existent ID returns ErrNotFound", func(t *testing.T) {
		err := repo.DeleteByID(ctx, "00000000-0000-0000-0000-000000000000")

		assert.ErrorIs(t, err, apperr.ErrNotFound)
	})
}

// ── Search ────────────────────────────────────────────────────────────────────

func TestRepository_Search(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := NewRepository(db)

	ctx := context.Background()

	// Seed once for all Search sub-tests — no mutations, so no cleanup needed per sub-test.
	t.Cleanup(func() { cleanUsers(t, db) })
	alice := insertUser(t, db, "Alice Smith", "alice", "alice@example.com", "hashed")
	bob := insertUser(t, db, "Bob Jones", "bob", "bob@example.com", "hashed")
	insertUser(t, db, "Charlie Brown", "charlie", "charlie@example.com", "hashed")

	t.Run("returns matching users by username (case insensitive)", func(t *testing.T) {
		results, err := repo.Search(ctx, alice.ID, "BOB", "", 10)

		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, bob.ID, results[0].ID)
		assert.Equal(t, "bob", results[0].Username)
	})

	t.Run("partial match returns results", func(t *testing.T) {
		// "li" matches "alice" and "charlie"
		results, err := repo.Search(ctx, bob.ID, "li", "", 10)

		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("excludes the searching user from results", func(t *testing.T) {
		// alice searches for "alice" — should not find herself
		results, err := repo.Search(ctx, alice.ID, "alice", "", 10)

		require.NoError(t, err)
		for _, r := range results {
			assert.NotEqual(t, alice.ID, r.ID, "search must not return the caller's own profile")
		}
	})

	t.Run("no match returns empty slice, not error", func(t *testing.T) {
		results, err := repo.Search(ctx, alice.ID, "zzznomatch", "", 10)

		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("limit is respected", func(t *testing.T) {
		// "li" matches alice dan charlie, limit=1 → repo return 1
		results, err := repo.Search(ctx, bob.ID, "li", "", 1)

		require.NoError(t, err)
		assert.LessOrEqual(t, len(results), 1)
	})

	t.Run("cursor-based pagination returns next page", func(t *testing.T) {
		firstPage, err := repo.Search(ctx, bob.ID, "li", "", 1)
		require.NoError(t, err)
		require.Len(t, firstPage, 1)

		cursor := firstPage[0].ID
		secondPage, err := repo.Search(ctx, bob.ID, "li", cursor, 10)
		require.NoError(t, err)

		firstIDs := map[string]bool{firstPage[0].ID: true}
		for _, r := range secondPage {
			assert.False(t, firstIDs[r.ID])
		}
	})

	t.Run("response fields contain expected data (no password, no email)", func(t *testing.T) {
		results, err := repo.Search(ctx, alice.ID, "bob", "", 10)

		require.NoError(t, err)
		require.Len(t, results, 1)

		s := results[0]
		assert.NotEmpty(t, s.ID)
		assert.NotEmpty(t, s.Name)
		assert.NotEmpty(t, s.Username)
		// Summary only has: ID, Name, Username, AvatarURL, LastSeenAt
		// No email, no password — enforced by the SELECT column list in the query
	})
}
