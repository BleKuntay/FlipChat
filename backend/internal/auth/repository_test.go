//go:build integration

package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "github.com/BleKuntay/FlipChat/backend/internal/auth"
	"github.com/BleKuntay/FlipChat/backend/internal/shared"
	"github.com/BleKuntay/FlipChat/backend/pkg/testhelper"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func insertUser(t *testing.T, db *sqlx.DB, name, username, email string) *User {
	t.Helper()

	hashed, err := shared.HashPassword("Secret123")
	require.NoError(t, err)

	var u User
	err = db.Get(&u, `
		INSERT INTO users (name, username, email, password)
		VALUES ($1, $2, $3, $4)
		RETURNING *
	`, name, username, email, hashed)
	require.NoError(t, err, "insertUser helper failed")

	return &u
}

func cleanUsers(t *testing.T, db *sqlx.DB) {
	t.Helper()
	_, err := db.Exec("TRUNCATE TABLE users RESTART IDENTITY CASCADE")
	require.NoError(t, err)
}

// ── CreateUser ────────────────────────────────────────────────────────────────

func TestRepository_CreateUser(t *testing.T) {
	db := testhelper.NewTestDB(t)
	rdb := testhelper.NewTestRedis(t)
	repo := NewRepository(db, rdb, 7*24*time.Hour)
	ctx := context.Background()

	t.Run("creates user and returns full row", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })

		u := &User{
			Name:     "John Doe",
			Username: "johndoe",
			Email:    "john@example.com",
			Password: "hashed-password",
			Language: "en",
		}

		got, err := repo.CreateUser(ctx, u)

		require.NoError(t, err)
		assert.NotEmpty(t, got.ID)
		assert.Equal(t, "John Doe", got.Name)
		assert.Equal(t, "johndoe", got.Username)
		assert.Equal(t, "john@example.com", got.Email)
		assert.Equal(t, "en", got.Language)
		assert.False(t, got.CreatedAt.IsZero())
		assert.Nil(t, got.Bio)
		assert.Nil(t, got.AvatarURL)
	})

	t.Run("returns error on duplicate email", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })
		insertUser(t, db, "John Doe", "johndoe", "john@example.com")

		u := &User{
			Name:     "Jane Doe",
			Username: "janedoe",
			Email:    "john@example.com",
			Password: "hashed",
			Language: "en",
		}

		_, err := repo.CreateUser(ctx, u)
		assert.Error(t, err)
	})

	t.Run("returns error on duplicate username", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })
		insertUser(t, db, "John Doe", "johndoe", "john@example.com")

		u := &User{
			Name:     "Jane Doe",
			Username: "johndoe",
			Email:    "jane@example.com",
			Password: "hashed",
			Language: "en",
		}

		_, err := repo.CreateUser(ctx, u)
		assert.Error(t, err)
	})
}

// ── FindUserByEmail ───────────────────────────────────────────────────────────

func TestRepository_FindUserByEmail(t *testing.T) {
	db := testhelper.NewTestDB(t)
	rdb := testhelper.NewTestRedis(t)
	repo := NewRepository(db, rdb, 7*24*time.Hour)
	ctx := context.Background()

	t.Run("returns user for existing email", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })
		inserted := insertUser(t, db, "John Doe", "johndoe", "john@example.com")

		got, err := repo.FindUserByEmail(ctx, "john@example.com")

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, inserted.ID, got.ID)
		assert.Equal(t, "johndoe", got.Username)
	})

	t.Run("returns nil for non-existent email", func(t *testing.T) {
		got, err := repo.FindUserByEmail(ctx, "ghost@example.com")

		require.NoError(t, err)
		assert.Nil(t, got)
	})
}

// ── ExistsByEmail ─────────────────────────────────────────────────────────────

func TestRepository_ExistsByEmail(t *testing.T) {
	db := testhelper.NewTestDB(t)
	rdb := testhelper.NewTestRedis(t)
	repo := NewRepository(db, rdb, 7*24*time.Hour)
	ctx := context.Background()

	t.Run("returns true for existing email", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })
		insertUser(t, db, "John Doe", "johndoe", "john@example.com")

		exists, err := repo.ExistsByEmail(ctx, "john@example.com")

		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("returns false for non-existent email", func(t *testing.T) {
		exists, err := repo.ExistsByEmail(ctx, "ghost@example.com")

		require.NoError(t, err)
		assert.False(t, exists)
	})
}

// ── ExistsByUsername ──────────────────────────────────────────────────────────

func TestRepository_ExistsByUsername(t *testing.T) {
	db := testhelper.NewTestDB(t)
	rdb := testhelper.NewTestRedis(t)
	repo := NewRepository(db, rdb, 7*24*time.Hour)
	ctx := context.Background()

	t.Run("returns true for existing username", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })
		insertUser(t, db, "John Doe", "johndoe", "john@example.com")

		exists, err := repo.ExistsByUsername(ctx, "johndoe")

		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("returns false for non-existent username", func(t *testing.T) {
		exists, err := repo.ExistsByUsername(ctx, "ghost")

		require.NoError(t, err)
		assert.False(t, exists)
	})
}

// ── SaveRefreshToken ──────────────────────────────────────────────────────────

func TestRepository_SaveRefreshToken(t *testing.T) {
	db := testhelper.NewTestDB(t)
	rdb := testhelper.NewTestRedis(t)
	repo := NewRepository(db, rdb, 7*24*time.Hour)
	ctx := context.Background()

	t.Run("saves token and can be found by token string", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })
		u := insertUser(t, db, "John Doe", "johndoe", "john@example.com")

		token := RefreshToken{
			UserID:    u.ID,
			Token:     "some-refresh-token",
			ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		}

		err := repo.SaveRefreshToken(ctx, token)
		require.NoError(t, err)

		got, err := repo.FindTokenByToken(ctx, token.Token)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, u.ID, got.UserID)
		assert.Equal(t, token.Token, got.Token)
	})

	t.Run("returns error when token already expired", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })
		u := insertUser(t, db, "John Doe2", "johndoe2", "john2@example.com")

		token := RefreshToken{
			UserID:    u.ID,
			Token:     "expired-token",
			ExpiresAt: time.Now().Add(-1 * time.Hour), // already expired
		}

		err := repo.SaveRefreshToken(ctx, token)
		assert.Error(t, err)
	})
}

// ── FindTokenByToken ──────────────────────────────────────────────────────────

func TestRepository_FindTokenByToken(t *testing.T) {
	db := testhelper.NewTestDB(t)
	rdb := testhelper.NewTestRedis(t)
	repo := NewRepository(db, rdb, 7*24*time.Hour)
	ctx := context.Background()

	t.Run("returns token for existing token string", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })
		u := insertUser(t, db, "John Doe", "johndoe", "john@example.com")

		token := RefreshToken{
			UserID:    u.ID,
			Token:     "valid-token",
			ExpiresAt: time.Now().Add(time.Hour),
		}
		require.NoError(t, repo.SaveRefreshToken(ctx, token))

		got, err := repo.FindTokenByToken(ctx, "valid-token")

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, u.ID, got.UserID)
		assert.Equal(t, "valid-token", got.Token)
		assert.False(t, got.ExpiresAt.IsZero())
	})

	t.Run("returns nil for non-existent token", func(t *testing.T) {
		got, err := repo.FindTokenByToken(ctx, "ghost-token")

		require.NoError(t, err)
		assert.Nil(t, got)
	})
}

// ── DeleteTokenByToken ────────────────────────────────────────────────────────

func TestRepository_DeleteTokenByToken(t *testing.T) {
	db := testhelper.NewTestDB(t)
	rdb := testhelper.NewTestRedis(t)
	repo := NewRepository(db, rdb, 7*24*time.Hour)
	ctx := context.Background()

	t.Run("deletes token — no longer findable", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })
		u := insertUser(t, db, "John Doe", "johndoe", "john@example.com")

		token := RefreshToken{
			UserID:    u.ID,
			Token:     "to-delete",
			ExpiresAt: time.Now().Add(time.Hour),
		}
		require.NoError(t, repo.SaveRefreshToken(ctx, token))

		err := repo.DeleteTokenByToken(ctx, "to-delete")
		require.NoError(t, err)

		got, err := repo.FindTokenByToken(ctx, "to-delete")
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("non-existent token is idempotent", func(t *testing.T) {
		err := repo.DeleteTokenByToken(ctx, "ghost-token")
		assert.NoError(t, err)
	})
}

// ── DeleteTokenByUserID ───────────────────────────────────────────────────────

func TestRepository_DeleteTokenByUserID(t *testing.T) {
	db := testhelper.NewTestDB(t)
	rdb := testhelper.NewTestRedis(t)
	repo := NewRepository(db, rdb, 7*24*time.Hour)
	ctx := context.Background()

	t.Run("deletes all tokens for a user", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })
		u := insertUser(t, db, "John Doe", "johndoe", "john@example.com")

		for _, tk := range []string{"token-1", "token-2", "token-3"} {
			require.NoError(t, repo.SaveRefreshToken(ctx, RefreshToken{
				UserID:    u.ID,
				Token:     tk,
				ExpiresAt: time.Now().Add(time.Hour),
			}))
		}

		err := repo.DeleteTokenByUserID(ctx, u.ID)
		require.NoError(t, err)

		for _, tk := range []string{"token-1", "token-2", "token-3"} {
			got, err := repo.FindTokenByToken(ctx, tk)
			require.NoError(t, err)
			assert.Nil(t, got, "token %s should be deleted", tk)
		}
	})

	t.Run("only deletes tokens for the specified user", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })
		u1 := insertUser(t, db, "John Doe", "johndoe", "john@example.com")
		u2 := insertUser(t, db, "Jane Doe", "janedoe", "jane@example.com")

		require.NoError(t, repo.SaveRefreshToken(ctx, RefreshToken{
			UserID: u1.ID, Token: "u1-token", ExpiresAt: time.Now().Add(time.Hour),
		}))
		require.NoError(t, repo.SaveRefreshToken(ctx, RefreshToken{
			UserID: u2.ID, Token: "u2-token", ExpiresAt: time.Now().Add(time.Hour),
		}))

		err := repo.DeleteTokenByUserID(ctx, u1.ID)
		require.NoError(t, err)

		got, err := repo.FindTokenByToken(ctx, "u2-token")
		require.NoError(t, err)
		assert.NotNil(t, got, "u2 token must still exist")
	})

	t.Run("non-existent userID is idempotent", func(t *testing.T) {
		err := repo.DeleteTokenByUserID(ctx, "00000000-0000-0000-0000-000000000000")
		assert.NoError(t, err)
	})
}

// ── RotateRefreshToken ────────────────────────────────────────────────────────

func TestRepository_RotateRefreshToken(t *testing.T) {
	db := testhelper.NewTestDB(t)
	rdb := testhelper.NewTestRedis(t)
	repo := NewRepository(db, rdb, 7*24*time.Hour)
	ctx := context.Background()

	t.Run("old token removed, new token findable", func(t *testing.T) {
		t.Cleanup(func() { cleanUsers(t, db) })
		u := insertUser(t, db, "John Doe", "johndoe", "john@example.com")

		require.NoError(t, repo.SaveRefreshToken(ctx, RefreshToken{
			UserID: u.ID, Token: "old-token", ExpiresAt: time.Now().Add(time.Hour),
		}))

		newExpiry := time.Now().Add(7 * 24 * time.Hour)
		err := repo.RotateRefreshToken(ctx, "old-token", "new-token", newExpiry)
		require.NoError(t, err)

		old, err := repo.FindTokenByToken(ctx, "old-token")
		require.NoError(t, err)
		assert.Nil(t, old, "old token must be gone")

		got, err := repo.FindTokenByToken(ctx, "new-token")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, u.ID, got.UserID)
	})

	t.Run("rotating non-existent token returns ErrNotFound", func(t *testing.T) {
		err := repo.RotateRefreshToken(ctx, "ghost-token", "new-token", time.Now().Add(time.Hour))
		assert.Error(t, err)
	})
}
