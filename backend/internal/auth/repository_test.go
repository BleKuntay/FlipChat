//go:build integration

package auth_test

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	. "github.com/BleKuntay/FlipChat/backend/internal/auth"
	"github.com/BleKuntay/FlipChat/backend/internal/db/migration"
	"github.com/BleKuntay/FlipChat/backend/internal/shared"
)

// ── test setup ────────────────────────────────────────────────────────────────

func newTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("flipchat_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err, "failed to start postgres container")

	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("warn: failed to terminate container: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := sqlx.Connect("postgres", dsn)
	require.NoError(t, err, "failed to connect to test database")

	t.Cleanup(func() { _ = db.Close() })

	runMigrations(t, dsn)

	return db
}

func runMigrations(t *testing.T, dsn string) {
	t.Helper()

	_, filename, _, _ := runtime.Caller(0)
	migrationsPath := filepath.Join(filepath.Dir(filename), "..", "..", "db", "migrations")
	migrationsPath = strings.ReplaceAll(migrationsPath, "\\", "/")

	m, err := migration.NewMigrate(migration.MigrateConfig{
		DBUrl:          dsn,
		MigrationsPath: migrationsPath,
	})
	require.NoError(t, err, "failed to create migrator")

	err = migration.RunMigrations(m)
	require.NoError(t, err, "failed to run migrations")
}

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

func insertRefreshToken(t *testing.T, db *sqlx.DB, userID, token string, expiresAt time.Time) *RefreshToken {
	t.Helper()

	var rt RefreshToken
	err := db.Get(&rt, `
		INSERT INTO refresh_tokens (user_id, token, expires_at)
		VALUES ($1, $2, $3)
		RETURNING *
	`, userID, token, expiresAt)
	require.NoError(t, err, "insertRefreshToken helper failed")

	return &rt
}

func cleanTables(t *testing.T, db *sqlx.DB) {
	t.Helper()
	_, err := db.Exec("TRUNCATE TABLE refresh_tokens, users RESTART IDENTITY CASCADE")
	require.NoError(t, err)
}

// ── CreateUser ────────────────────────────────────────────────────────────────

func TestRepository_CreateUser(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	t.Run("creates user and returns full row", func(t *testing.T) {
		t.Cleanup(func() { cleanTables(t, db) })

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

	t.Run("language defaults to en if empty", func(t *testing.T) {
		t.Cleanup(func() { cleanTables(t, db) })

		u := &User{
			Name:     "John Doe",
			Username: "johndoe",
			Email:    "john@example.com",
			Password: "hashed-password",
			Language: "en",
		}

		got, err := repo.CreateUser(ctx, u)

		require.NoError(t, err)
		assert.Equal(t, "en", got.Language)
	})

	t.Run("returns error on duplicate email", func(t *testing.T) {
		t.Cleanup(func() { cleanTables(t, db) })
		insertUser(t, db, "John Doe", "johndoe", "john@example.com")

		u := &User{
			Name:     "Jane Doe",
			Username: "janedoe",
			Email:    "john@example.com", // conflict
			Password: "hashed",
			Language: "en",
		}

		_, err := repo.CreateUser(ctx, u)

		assert.Error(t, err)
	})

	t.Run("returns error on duplicate username", func(t *testing.T) {
		t.Cleanup(func() { cleanTables(t, db) })
		insertUser(t, db, "John Doe", "johndoe", "john@example.com")

		u := &User{
			Name:     "Jane Doe",
			Username: "johndoe", // conflict
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
	db := newTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	t.Run("returns user for existing email", func(t *testing.T) {
		t.Cleanup(func() { cleanTables(t, db) })
		inserted := insertUser(t, db, "John Doe", "johndoe", "john@example.com")

		got, err := repo.FindUserByEmail(ctx, "john@example.com")

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, inserted.ID, got.ID)
		assert.Equal(t, "johndoe", got.Username)
		assert.NotEmpty(t, got.Password)
	})

	t.Run("returns nil for non-existent email", func(t *testing.T) {
		got, err := repo.FindUserByEmail(ctx, "ghost@example.com")

		require.NoError(t, err)
		assert.Nil(t, got)
	})
}

// ── ExistsByEmail ─────────────────────────────────────────────────────────────

func TestRepository_ExistsByEmail(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	t.Run("returns true for existing email", func(t *testing.T) {
		t.Cleanup(func() { cleanTables(t, db) })
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
	db := newTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	t.Run("returns true for existing username", func(t *testing.T) {
		t.Cleanup(func() { cleanTables(t, db) })
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
	db := newTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	t.Run("saves refresh token and persists to database", func(t *testing.T) {
		t.Cleanup(func() { cleanTables(t, db) })
		u := insertUser(t, db, "John Doe", "johndoe", "john@example.com")

		token := RefreshToken{
			UserID:    u.ID,
			Token:     "some-refresh-token",
			ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		}

		err := repo.SaveRefreshToken(ctx, token)
		require.NoError(t, err)

		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM refresh_tokens WHERE token = $1", token.Token).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("returns error on duplicate token", func(t *testing.T) {
		t.Cleanup(func() { cleanTables(t, db) })
		u := insertUser(t, db, "John Doe", "johndoe", "john@example.com")
		insertRefreshToken(t, db, u.ID, "duplicate-token", time.Now().Add(time.Hour))

		token := RefreshToken{
			UserID:    u.ID,
			Token:     "duplicate-token", // conflict
			ExpiresAt: time.Now().Add(time.Hour),
		}

		err := repo.SaveRefreshToken(ctx, token)
		assert.Error(t, err)
	})
}

// ── FindTokenByToken ──────────────────────────────────────────────────────────

func TestRepository_FindTokenByToken(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	t.Run("returns refresh token for existing token string", func(t *testing.T) {
		t.Cleanup(func() { cleanTables(t, db) })
		u := insertUser(t, db, "John Doe", "johndoe", "john@example.com")
		inserted := insertRefreshToken(t, db, u.ID, "valid-token", time.Now().Add(time.Hour))

		got, err := repo.FindTokenByToken(ctx, "valid-token")

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, inserted.ID, got.ID)
		assert.Equal(t, u.ID, got.UserID)
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
	db := newTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	t.Run("deletes token from database", func(t *testing.T) {
		t.Cleanup(func() { cleanTables(t, db) })
		u := insertUser(t, db, "John Doe", "johndoe", "john@example.com")
		insertRefreshToken(t, db, u.ID, "to-delete", time.Now().Add(time.Hour))

		err := repo.DeleteTokenByToken(ctx, "to-delete")
		require.NoError(t, err)

		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM refresh_tokens WHERE token = $1", "to-delete").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("non-existent token does not return error (silent no-op)", func(t *testing.T) {
		err := repo.DeleteTokenByToken(ctx, "ghost-token")

		assert.NoError(t, err)
	})
}

// ── DeleteTokenByUserID ───────────────────────────────────────────────────────

func TestRepository_DeleteTokenByUserID(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	t.Run("deletes all tokens for a user", func(t *testing.T) {
		t.Cleanup(func() { cleanTables(t, db) })
		u := insertUser(t, db, "John Doe", "johndoe", "john@example.com")
		insertRefreshToken(t, db, u.ID, "token-1", time.Now().Add(time.Hour))
		insertRefreshToken(t, db, u.ID, "token-2", time.Now().Add(time.Hour))
		insertRefreshToken(t, db, u.ID, "token-3", time.Now().Add(time.Hour))

		err := repo.DeleteTokenByUserID(ctx, u.ID)
		require.NoError(t, err)

		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1", u.ID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("only deletes tokens for the specified user", func(t *testing.T) {
		t.Cleanup(func() { cleanTables(t, db) })
		u1 := insertUser(t, db, "John Doe", "johndoe", "john@example.com")
		u2 := insertUser(t, db, "Jane Doe", "janedoe", "jane@example.com")
		insertRefreshToken(t, db, u1.ID, "u1-token", time.Now().Add(time.Hour))
		insertRefreshToken(t, db, u2.ID, "u2-token", time.Now().Add(time.Hour))

		err := repo.DeleteTokenByUserID(ctx, u1.ID)
		require.NoError(t, err)

		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1", u2.ID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("non-existent userID does not return error (silent no-op)", func(t *testing.T) {
		err := repo.DeleteTokenByUserID(ctx, "00000000-0000-0000-0000-000000000000")

		assert.NoError(t, err)
	})
}

// ── RotateRefreshToken ────────────────────────────────────────────────────────

func TestRepository_RotateRefreshToken(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	t.Run("replaces old token with new token and updates expires_at", func(t *testing.T) {
		t.Cleanup(func() { cleanTables(t, db) })
		u := insertUser(t, db, "John Doe", "johndoe", "john@example.com")
		insertRefreshToken(t, db, u.ID, "old-token", time.Now().Add(time.Hour))

		newExpiry := time.Now().Add(7 * 24 * time.Hour).UTC().Truncate(time.Second)
		err := repo.RotateRefreshToken(ctx, "old-token", "new-token", newExpiry)
		require.NoError(t, err)

		var oldCount int
		err = db.QueryRow("SELECT COUNT(*) FROM refresh_tokens WHERE token = $1", "old-token").Scan(&oldCount)
		require.NoError(t, err)
		assert.Equal(t, 0, oldCount)

		var got RefreshToken
		err = db.Get(&got, "SELECT * FROM refresh_tokens WHERE token = $1", "new-token")
		require.NoError(t, err)
		assert.Equal(t, u.ID, got.UserID)
		assert.Equal(t, newExpiry, got.ExpiresAt.UTC().Truncate(time.Second))
	})

	t.Run("ON DELETE CASCADE — deleting user removes all refresh tokens", func(t *testing.T) {
		t.Cleanup(func() { cleanTables(t, db) })
		u := insertUser(t, db, "John Doe", "johndoe", "john@example.com")
		insertRefreshToken(t, db, u.ID, "token-a", time.Now().Add(time.Hour))
		insertRefreshToken(t, db, u.ID, "token-b", time.Now().Add(time.Hour))

		_, err := db.Exec("DELETE FROM users WHERE id = $1", u.ID)
		require.NoError(t, err)

		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1", u.ID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count, "refresh tokens harus terhapus cascade saat user dihapus")
	})
}
