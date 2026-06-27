//go:build integration

package block_test

import (
	"context"
	"testing"
	"time"

	"github.com/BleKuntay/FlipChat/backend/internal/block"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// ------------------------------------------------------------------ //
// Setup                                                                //
// ------------------------------------------------------------------ //

func setupDB(t *testing.T) *sqlx.DB {
	t.Helper()
	ctx := context.Background()

	pgc, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:16-alpine"),
		postgres.WithDatabase("flipchat_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pgc.Terminate(ctx) })

	dsn, err := pgc.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := sqlx.Connect("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	migrate(t, db)
	return db
}

func migrate(t *testing.T, db *sqlx.DB) {
	t.Helper()

	_, err := db.Exec(`
		CREATE EXTENSION IF NOT EXISTS "pgcrypto";

		CREATE TABLE IF NOT EXISTS users (
			id         TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
			username   TEXT NOT NULL UNIQUE,
			name       TEXT NOT NULL,
			email      TEXT NOT NULL UNIQUE,
			password   TEXT NOT NULL,
			avatar_url TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);

		CREATE TABLE IF NOT EXISTS friends (
			user_low_id  TEXT NOT NULL,
			user_high_id TEXT NOT NULL,
			requester_id TEXT NOT NULL,
			status       TEXT NOT NULL DEFAULT 'pending',
			created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (user_low_id, user_high_id),
			FOREIGN KEY (user_low_id)  REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (user_high_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (requester_id) REFERENCES users(id) ON DELETE CASCADE
		);

		CREATE TABLE IF NOT EXISTS blocks (
			blocker_id TEXT NOT NULL,
			blocked_id TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (blocker_id, blocked_id),
			FOREIGN KEY (blocker_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (blocked_id) REFERENCES users(id) ON DELETE CASCADE
		);
	`)
	require.NoError(t, err)
}

func seedUser(t *testing.T, db *sqlx.DB, username string) string {
	t.Helper()
	var id string
	err := db.QueryRowx(
		`INSERT INTO users (username, name, email, password)
		 VALUES ($1, $2, $3, 'hash')
		 RETURNING id`,
		username, username, username+"@test.com",
	).Scan(&id)
	require.NoError(t, err)
	return id
}

func seedFriendship(t *testing.T, db *sqlx.DB, aID, bID string) {
	t.Helper()
	low, high := aID, bID
	if aID > bID {
		low, high = bID, aID
	}
	_, err := db.Exec(
		`INSERT INTO friends (user_low_id, user_high_id, requester_id, status)
		 VALUES ($1, $2, $3, 'accepted')`,
		low, high, aID,
	)
	require.NoError(t, err)
}

// ------------------------------------------------------------------ //
// BlockUserAtomic                                                      //
// ------------------------------------------------------------------ //

func TestRepository_BlockUserAtomic(t *testing.T) {
	ctx := context.Background()

	t.Run("block user with no prior friendship", func(t *testing.T) {
		db := setupDB(t)
		repo := block.NewRepository(db)

		aID := seedUser(t, db, "alpha")
		bID := seedUser(t, db, "beta")

		low, high := aID, bID
		if aID > bID {
			low, high = bID, aID
		}

		req := block.Request{BlockerID: aID, BlockedID: bID}
		resp, err := repo.BlockUserAtomic(ctx, req, low, high)
		require.NoError(t, err)
		assert.Equal(t, aID, resp.BlockerID)
		assert.Equal(t, bID, resp.BlockedID)

		var count int
		err = db.QueryRowx("SELECT COUNT(*) FROM friends").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("block user who was previously friends - friendship deleted atomically", func(t *testing.T) {
		db := setupDB(t)
		repo := block.NewRepository(db)

		aID := seedUser(t, db, "alpha")
		bID := seedUser(t, db, "beta")
		seedFriendship(t, db, aID, bID)

		low, high := aID, bID
		if aID > bID {
			low, high = bID, aID
		}

		req := block.Request{BlockerID: aID, BlockedID: bID}
		resp, err := repo.BlockUserAtomic(ctx, req, low, high)
		require.NoError(t, err)
		assert.Equal(t, aID, resp.BlockerID)

		var count int
		err = db.QueryRowx("SELECT COUNT(*) FROM friends").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)

		err = db.QueryRowx("SELECT COUNT(*) FROM blocks").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("block twice - unique violation", func(t *testing.T) {
		db := setupDB(t)
		repo := block.NewRepository(db)

		aID := seedUser(t, db, "alpha")
		bID := seedUser(t, db, "beta")

		low, high := aID, bID
		if aID > bID {
			low, high = bID, aID
		}

		req := block.Request{BlockerID: aID, BlockedID: bID}
		_, err := repo.BlockUserAtomic(ctx, req, low, high)
		require.NoError(t, err)

		_, err = repo.BlockUserAtomic(ctx, req, low, high)
		assert.Error(t, err)
	})
}

// ------------------------------------------------------------------ //
// UnblockUser                                                          //
// ------------------------------------------------------------------ //

func TestRepository_UnblockUser(t *testing.T) {
	ctx := context.Background()

	t.Run("unblock successfully", func(t *testing.T) {
		db := setupDB(t)
		repo := block.NewRepository(db)

		aID := seedUser(t, db, "alpha")
		bID := seedUser(t, db, "beta")

		_, err := db.Exec(
			"INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2)",
			aID, bID,
		)
		require.NoError(t, err)

		req := block.Request{BlockerID: aID, BlockedID: bID}
		err = repo.UnblockUser(ctx, req)
		require.NoError(t, err)

		var count int
		err = db.QueryRowx("SELECT COUNT(*) FROM blocks").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("unblock user who was never blocked - no-op, no error", func(t *testing.T) {
		db := setupDB(t)
		repo := block.NewRepository(db)

		aID := seedUser(t, db, "alpha")
		bID := seedUser(t, db, "beta")

		req := block.Request{BlockerID: aID, BlockedID: bID}
		err := repo.UnblockUser(ctx, req)
		assert.NoError(t, err)
	})
}

// ------------------------------------------------------------------ //
// IsBlockedEitherWay                                                   //
// ------------------------------------------------------------------ //

func TestRepository_IsBlockedEitherWay(t *testing.T) {
	ctx := context.Background()

	t.Run("A blocks B - true from A perspective", func(t *testing.T) {
		db := setupDB(t)
		repo := block.NewRepository(db)

		aID := seedUser(t, db, "alpha")
		bID := seedUser(t, db, "beta")

		_, err := db.Exec(
			"INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2)",
			aID, bID,
		)
		require.NoError(t, err)

		ok, err := repo.IsBlockedEitherWay(ctx, aID, bID)
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("A blocks B - true from B perspective too (either way)", func(t *testing.T) {
		db := setupDB(t)
		repo := block.NewRepository(db)

		aID := seedUser(t, db, "alpha")
		bID := seedUser(t, db, "beta")

		_, err := db.Exec(
			"INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2)",
			aID, bID,
		)
		require.NoError(t, err)

		ok, err := repo.IsBlockedEitherWay(ctx, bID, aID)
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("no block - false", func(t *testing.T) {
		db := setupDB(t)
		repo := block.NewRepository(db)

		aID := seedUser(t, db, "alpha")
		bID := seedUser(t, db, "beta")

		ok, err := repo.IsBlockedEitherWay(ctx, aID, bID)
		require.NoError(t, err)
		assert.False(t, ok)
	})
}

// ------------------------------------------------------------------ //
// GetBlockList                                                         //
// ------------------------------------------------------------------ //

func TestRepository_GetBlockList(t *testing.T) {
	ctx := context.Background()

	t.Run("pagination - cursor cuts correctly", func(t *testing.T) {
		db := setupDB(t)
		repo := block.NewRepository(db)

		blockerID := seedUser(t, db, "blocker")

		var blockedIDs []string
		for _, name := range []string{"aaa", "bbb", "ccc"} {
			id := seedUser(t, db, name)
			blockedIDs = append(blockedIDs, id)
			_, err := db.Exec(
				"INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2)",
				blockerID, id,
			)
			require.NoError(t, err)
		}

		query := block.ListQuery{Limit: 2}
		records, err := repo.GetBlockList(ctx, blockerID, query)
		require.NoError(t, err)

		assert.Len(t, records, 3)
	})

	t.Run("cursor filters previous records", func(t *testing.T) {
		db := setupDB(t)
		repo := block.NewRepository(db)

		blockerID := seedUser(t, db, "blocker")

		var ids []string
		for _, name := range []string{"aaa", "bbb", "ccc"} {
			id := seedUser(t, db, name)
			ids = append(ids, id)
			_, err := db.Exec(
				"INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2)",
				blockerID, id,
			)
			require.NoError(t, err)
		}

		page1, err := repo.GetBlockList(ctx, blockerID, block.ListQuery{Limit: 2})
		require.NoError(t, err)
		require.Len(t, page1, 3)

		cursor := page1[1].UserID

		page2, err := repo.GetBlockList(ctx, blockerID, block.ListQuery{Cursor: cursor, Limit: 2})
		require.NoError(t, err)

		assert.Len(t, page2, 1)
		assert.NotEqual(t, cursor, page2[0].UserID)
	})

	t.Run("only blocker's blocks returned", func(t *testing.T) {
		db := setupDB(t)
		repo := block.NewRepository(db)

		aID := seedUser(t, db, "alpha")
		bID := seedUser(t, db, "beta")
		cID := seedUser(t, db, "gamma")

		_, err := db.Exec("INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2)", aID, cID)
		require.NoError(t, err)

		_, err = db.Exec("INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2)", bID, cID)
		require.NoError(t, err)

		records, err := repo.GetBlockList(ctx, aID, block.ListQuery{Limit: 10})
		require.NoError(t, err)

		assert.Len(t, records, 1)
		assert.Equal(t, cID, records[0].UserID)
	})
}
