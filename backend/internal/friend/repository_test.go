//go:build integration

package friend_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	. "github.com/BleKuntay/FlipChat/backend/internal/friend"
)

var testDB *sqlx.DB

func TestMain(m *testing.M) {
	ctx := context.Background()

	ctr, err := postgres.Run(ctx,
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
	if err != nil {
		panic(fmt.Sprintf("start postgres container: %v", err))
	}
	defer func() { _ = ctr.Terminate(ctx) }()

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(fmt.Sprintf("get connection string: %v", err))
	}

	testDB, err = sqlx.Open("postgres", dsn)
	if err != nil {
		panic(fmt.Sprintf("open db: %v", err))
	}
	defer testDB.Close()

	if err := runMigrations(dsn); err != nil {
		panic(fmt.Sprintf("run migrations: %v", err))
	}

	m.Run()
}

func runMigrations(dsn string) error {
	_, filename, _, _ := runtime.Caller(0)

	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..")
	migrationsPath := filepath.Join(projectRoot, "db", "migrations")

	migrationsPath = strings.ReplaceAll(migrationsPath, "\\", "/")
	migrationsURL := fmt.Sprintf("file://%s", migrationsPath)

	mg, err := migrate.New(migrationsURL, dsn)
	if err != nil {
		return fmt.Errorf("new migrate: %w", err)
	}

	if err := mg.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}

	return nil
}

func newID() string { return uuid.New().String() }

func insertUser(t *testing.T, id, username, name, email string) {
	t.Helper()
	_, err := testDB.ExecContext(context.Background(), `
		INSERT INTO users (id, username, name, email, password)
		VALUES ($1, $2, $3, $4, 'placeholder')
	`, id, username, name, email)
	require.NoError(t, err, "insertUser")
	t.Cleanup(func() {
		_, _ = testDB.ExecContext(context.Background(),
			"DELETE FROM users WHERE id = $1", id)
	})
}

func insertFriend(t *testing.T, aID, bID, requesterID string, status Status) {
	t.Helper()
	lowID, highID := aID, bID
	if aID > bID {
		lowID, highID = bID, aID
	}

	var statusStr string
	switch status {
	case StatusPending:
		statusStr = "pending"
	case StatusAccepted:
		statusStr = "accepted"
	default:
		t.Fatalf("insertFriend: unknown status %v", status)
	}

	_, err := testDB.ExecContext(context.Background(), `
		INSERT INTO friends (user_low_id, user_high_id, requester_id, status)
		VALUES ($1, $2, $3, $4)
	`, lowID, highID, requesterID, statusStr)
	require.NoError(t, err, "insertFriend")
}

func TestRepository_FindByPair(t *testing.T) {
	repo := NewRepository(testDB)
	ctx := context.Background()

	t.Run("returns nil when pair does not exist", func(t *testing.T) {
		got, err := repo.FindByPair(ctx, newID(), newID())
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("finds existing pair in canonical order", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-fbp1", "Alice", "alice-fbp1@test.com")
		insertUser(t, bID, "bob-fbp1", "Bob", "bob-fbp1@test.com")
		insertFriend(t, aID, bID, aID, StatusPending)

		lowID, highID := aID, bID
		if aID > bID {
			lowID, highID = bID, aID
		}

		got, err := repo.FindByPair(ctx, lowID, highID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, aID, got.RequesterID)
		assert.Equal(t, StatusPending, got.Status)
	})

	t.Run("returns nil for wrong order (high, low)", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-fbp2", "Alice", "alice-fbp2@test.com")
		insertUser(t, bID, "bob-fbp2", "Bob", "bob-fbp2@test.com")
		insertFriend(t, aID, bID, aID, StatusPending)

		lowID, highID := aID, bID
		if aID > bID {
			lowID, highID = bID, aID
		}

		got, err := repo.FindByPair(ctx, highID, lowID)
		require.NoError(t, err)
		assert.Nil(t, got)
	})
}

func TestRepository_ExistsByUserID(t *testing.T) {
	repo := NewRepository(testDB)
	ctx := context.Background()

	t.Run("returns true for existing user", func(t *testing.T) {
		id := newID()
		insertUser(t, id, "alice-exists", "Alice", "alice-exists@test.com")

		got, err := repo.ExistsByUserID(ctx, id)
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("returns false for non-existent user", func(t *testing.T) {
		got, err := repo.ExistsByUserID(ctx, newID())
		require.NoError(t, err)
		assert.False(t, got)
	})
}

func TestRepository_UpsertFriend(t *testing.T) {
	repo := NewRepository(testDB)
	ctx := context.Background()

	t.Run("inserts new pending request", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-ups1", "Alice", "alice-ups1@test.com")
		insertUser(t, bID, "bob-ups1", "Bob", "bob-ups1@test.com")

		lowID, highID := aID, bID
		if aID > bID {
			lowID, highID = bID, aID
		}

		rec, err := repo.UpsertFriend(ctx, lowID, highID, aID)
		require.NoError(t, err)
		require.NotNil(t, rec)

		assert.Equal(t, bID, rec.UserID)
		assert.Equal(t, StatusPending, rec.Status)
		assert.Equal(t, aID, rec.RequesterID)
	})

	t.Run("mutual accept: second party upserts → status becomes accepted", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-ups2", "Alice", "alice-ups2@test.com")
		insertUser(t, bID, "bob-ups2", "Bob", "bob-ups2@test.com")

		lowID, highID := aID, bID
		if aID > bID {
			lowID, highID = bID, aID
		}

		_, err := repo.UpsertFriend(ctx, lowID, highID, aID)
		require.NoError(t, err)

		rec, err := repo.UpsertFriend(ctx, lowID, highID, bID)
		require.NoError(t, err)
		require.NotNil(t, rec)
		assert.Equal(t, StatusAccepted, rec.Status)
	})

	t.Run("duplicate insert (same requester) returns ErrNoRows — no conflict update", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-ups3", "Alice", "alice-ups3@test.com")
		insertUser(t, bID, "bob-ups3", "Bob", "bob-ups3@test.com")

		lowID, highID := aID, bID
		if aID > bID {
			lowID, highID = bID, aID
		}

		_, err := repo.UpsertFriend(ctx, lowID, highID, aID)
		require.NoError(t, err)

		_, err = repo.UpsertFriend(ctx, lowID, highID, aID)
		assert.Error(t, err, "second upsert by same requester should fail")
	})
}

func TestRepository_FindAll(t *testing.T) {
	repo := NewRepository(testDB)
	ctx := context.Background()

	t.Run("returns accepted friends from both directions", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-fa1", "Alice", "alice-fa1@test.com")
		insertUser(t, bID, "bob-fa1", "Bob", "bob-fa1@test.com")
		insertFriend(t, aID, bID, aID, StatusAccepted)

		gotA, err := repo.FindAll(ctx, aID, ListQuery{Limit: 20})
		require.NoError(t, err)
		require.Len(t, gotA, 1)
		assert.Equal(t, bID, gotA[0].UserID)

		gotB, err := repo.FindAll(ctx, bID, ListQuery{Limit: 20})
		require.NoError(t, err)
		require.Len(t, gotB, 1)
		assert.Equal(t, aID, gotB[0].UserID)
	})

	t.Run("pending requests do not appear in friends list", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-fa2", "Alice", "alice-fa2@test.com")
		insertUser(t, bID, "bob-fa2", "Bob", "bob-fa2@test.com")
		insertFriend(t, aID, bID, aID, StatusPending)

		got, err := repo.FindAll(ctx, aID, ListQuery{Limit: 20})
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("search by username (partial, case-insensitive)", func(t *testing.T) {
		aID, bID, cID := newID(), newID(), newID()
		insertUser(t, aID, "alice-fa3", "Alice", "alice-fa3@test.com")
		insertUser(t, bID, "bobby-fa3", "Bobby", "bobby-fa3@test.com")
		insertUser(t, cID, "charlie-fa3", "Charlie", "charlie-fa3@test.com")
		insertFriend(t, aID, bID, aID, StatusAccepted)
		insertFriend(t, aID, cID, aID, StatusAccepted)

		got, err := repo.FindAll(ctx, aID, ListQuery{Q: "BOBBY", Limit: 20})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, bID, got[0].UserID)
	})

	t.Run("search by name (partial, case-insensitive)", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-fa4", "Alice Wonder", "alice-fa4@test.com")
		insertUser(t, bID, "bob-fa4", "Bobby Tables", "bob-fa4@test.com")
		insertFriend(t, aID, bID, aID, StatusAccepted)

		got, err := repo.FindAll(ctx, aID, ListQuery{Q: "tables", Limit: 20})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, bID, got[0].UserID)
	})

	t.Run("cursor pagination: second page excludes first-page items", func(t *testing.T) {
		ownerID := newID()
		insertUser(t, ownerID, "owner-fa5", "Owner", "owner-fa5@test.com")

		friendIDs := make([]string, 3)
		for i := range friendIDs {
			friendIDs[i] = newID()
			insertUser(t, friendIDs[i],
				fmt.Sprintf("friend-fa5-%d", i),
				fmt.Sprintf("Friend %d", i),
				fmt.Sprintf("friend-fa5-%d@test.com", i),
			)
			insertFriend(t, ownerID, friendIDs[i], ownerID, StatusAccepted)
		}

		page1, err := repo.FindAll(ctx, ownerID, ListQuery{Limit: 2})
		require.NoError(t, err)
		require.LessOrEqual(t, len(page1), 3, "repo returns at most Limit+1 rows")
		require.GreaterOrEqual(t, len(page1), 2, "at least Limit rows expected")

		trimmed1 := page1[:2]
		cursor := trimmed1[len(trimmed1)-1].UserID

		page2, err := repo.FindAll(ctx, ownerID, ListQuery{Cursor: cursor, Limit: 2})
		require.NoError(t, err)
		assert.NotEmpty(t, page2, "page 2 should have remaining records")

		page1IDs := make(map[string]bool)
		for _, r := range trimmed1 {
			page1IDs[r.UserID] = true
		}
		for _, r := range page2 {
			assert.False(t, page1IDs[r.UserID], "record %s appears on both pages", r.UserID)
		}
	})
}

func TestRepository_FindAllRequests(t *testing.T) {
	repo := NewRepository(testDB)
	ctx := context.Background()

	t.Run("shows sent request to requester", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-far1", "Alice", "alice-far1@test.com")
		insertUser(t, bID, "bob-far1", "Bob", "bob-far1@test.com")
		insertFriend(t, aID, bID, aID, StatusPending)

		got, err := repo.FindAllRequests(ctx, aID, RequestListQuery{Limit: 20})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, bID, got[0].UserID)
		assert.Equal(t, aID, got[0].RequesterID)
		assert.Equal(t, StatusPending, got[0].Status)
	})

	t.Run("shows received request to recipient", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-far2", "Alice", "alice-far2@test.com")
		insertUser(t, bID, "bob-far2", "Bob", "bob-far2@test.com")
		insertFriend(t, aID, bID, aID, StatusPending)

		got, err := repo.FindAllRequests(ctx, bID, RequestListQuery{Limit: 20})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, aID, got[0].UserID)
		assert.Equal(t, aID, got[0].RequesterID)
	})

	t.Run("accepted friends do not appear in requests list", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-far3", "Alice", "alice-far3@test.com")
		insertUser(t, bID, "bob-far3", "Bob", "bob-far3@test.com")
		insertFriend(t, aID, bID, aID, StatusAccepted)

		got, err := repo.FindAllRequests(ctx, aID, RequestListQuery{Limit: 20})
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("cursor pagination: second page excludes first-page items", func(t *testing.T) {
		ownerID := newID()
		insertUser(t, ownerID, "owner-far4", "Owner", "owner-far4@test.com")

		senderIDs := make([]string, 3)
		for i := range senderIDs {
			senderIDs[i] = newID()
			insertUser(t, senderIDs[i],
				fmt.Sprintf("sender-far4-%d", i),
				fmt.Sprintf("Sender %d", i),
				fmt.Sprintf("sender-far4-%d@test.com", i),
			)
			insertFriend(t, ownerID, senderIDs[i], senderIDs[i], StatusPending)
		}

		page1, err := repo.FindAllRequests(ctx, ownerID, RequestListQuery{Limit: 2})
		require.NoError(t, err)
		require.LessOrEqual(t, len(page1), 3, "repo returns at most Limit+1 rows")
		require.GreaterOrEqual(t, len(page1), 2, "at least Limit rows expected")

		trimmed1 := page1[:2]
		cursor := trimmed1[len(trimmed1)-1].UserID

		page2, err := repo.FindAllRequests(ctx, ownerID, RequestListQuery{Cursor: cursor, Limit: 2})
		require.NoError(t, err)
		assert.NotEmpty(t, page2)

		page1IDs := make(map[string]bool)
		for _, r := range trimmed1 {
			page1IDs[r.UserID] = true
		}
		for _, r := range page2 {
			assert.False(t, page1IDs[r.UserID], "record %s appears on both pages", r.UserID)
		}
	})
}

func TestRepository_DeleteByPair(t *testing.T) {
	repo := NewRepository(testDB)
	ctx := context.Background()

	t.Run("deletes existing pair", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-dbp1", "Alice", "alice-dbp1@test.com")
		insertUser(t, bID, "bob-dbp1", "Bob", "bob-dbp1@test.com")
		insertFriend(t, aID, bID, aID, StatusPending)

		lowID, highID := aID, bID
		if aID > bID {
			lowID, highID = bID, aID
		}

		err := repo.DeleteByPair(ctx, lowID, highID)
		require.NoError(t, err)

		pair, err := repo.FindByPair(ctx, lowID, highID)
		require.NoError(t, err)
		assert.Nil(t, pair)
	})

	t.Run("deleting non-existent pair is a no-op (no error)", func(t *testing.T) {
		err := repo.DeleteByPair(ctx, newID(), newID())
		require.NoError(t, err)
	})
}

func TestRepository_UpsertFriend_DataIntegrity(t *testing.T) {
	repo := NewRepository(testDB)
	ctx := context.Background()

	t.Run("persisted row is visible via FindByPair after upsert", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-di1", "Alice", "alice-di1@test.com")
		insertUser(t, bID, "bob-di1", "Bob", "bob-di1@test.com")

		lowID, highID := aID, bID
		if aID > bID {
			lowID, highID = bID, aID
		}

		_, err := repo.UpsertFriend(ctx, lowID, highID, aID)
		require.NoError(t, err)

		pair, err := repo.FindByPair(ctx, lowID, highID)
		require.NoError(t, err)
		require.NotNil(t, pair)
		assert.Equal(t, aID, pair.RequesterID)
		assert.Equal(t, StatusPending, pair.Status)
		assert.False(t, pair.CreatedAt.IsZero())
	})

	t.Run("accepted pair no longer appears in FindAllRequests", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-di2", "Alice", "alice-di2@test.com")
		insertUser(t, bID, "bob-di2", "Bob", "bob-di2@test.com")

		lowID, highID := aID, bID
		if aID > bID {
			lowID, highID = bID, aID
		}

		_, err := repo.UpsertFriend(ctx, lowID, highID, aID)
		require.NoError(t, err)

		_, err = repo.UpsertFriend(ctx, lowID, highID, bID)
		require.NoError(t, err)

		requests, err := repo.FindAllRequests(ctx, aID, RequestListQuery{Limit: 20})
		require.NoError(t, err)
		for _, r := range requests {
			assert.NotEqual(t, bID, r.UserID, "accepted friend should not appear in pending requests")
		}

		friends, err := repo.FindAll(ctx, aID, ListQuery{Limit: 20})
		require.NoError(t, err)
		found := false
		for _, f := range friends {
			if f.UserID == bID {
				found = true
				break
			}
		}
		assert.True(t, found, "accepted friend should appear in FindAll")
	})

	t.Run("deleted pair not returned by FindAll", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-di3", "Alice", "alice-di3@test.com")
		insertUser(t, bID, "bob-di3", "Bob", "bob-di3@test.com")
		insertFriend(t, aID, bID, aID, StatusAccepted)

		lowID, highID := aID, bID
		if aID > bID {
			lowID, highID = bID, aID
		}

		err := repo.DeleteByPair(ctx, lowID, highID)
		require.NoError(t, err)

		friends, err := repo.FindAll(ctx, aID, ListQuery{Limit: 20})
		require.NoError(t, err)
		for _, f := range friends {
			assert.NotEqual(t, bID, f.UserID)
		}
	})
}

func TestRepository_EdgeCases(t *testing.T) {
	repo := NewRepository(testDB)
	ctx := context.Background()

	t.Run("empty database returns empty slice from FindAll", func(t *testing.T) {
		got, err := repo.FindAll(ctx, newID(), ListQuery{Limit: 20})
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("empty database returns empty slice from FindAllRequests", func(t *testing.T) {
		got, err := repo.FindAllRequests(ctx, newID(), RequestListQuery{Limit: 20})
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("context cancellation is propagated", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.FindAll(ctx, newID(), ListQuery{Limit: 20})
		assert.Error(t, err, "cancelled context should produce an error")
	})

	t.Run("FindAll with limit=1 returns at most 1 record", func(t *testing.T) {
		aID, bID, cID := newID(), newID(), newID()
		insertUser(t, aID, "alice-ec1", "Alice", "alice-ec1@test.com")
		insertUser(t, bID, "bob-ec1", "Bob", "bob-ec1@test.com")
		insertUser(t, cID, "charlie-ec1", "Charlie", "charlie-ec1@test.com")
		insertFriend(t, aID, bID, aID, StatusAccepted)
		insertFriend(t, aID, cID, aID, StatusAccepted)

		got, err := repo.FindAll(ctx, aID, ListQuery{Limit: 1})
		require.NoError(t, err)

		assert.LessOrEqual(t, len(got), 2, "repo returns at most limit+1 rows")
		assert.GreaterOrEqual(t, len(got), 1, "at least 1 row expected")
	})

	t.Run("FindAll empty search query returns all friends", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-ec2", "Alice", "alice-ec2@test.com")
		insertUser(t, bID, "bob-ec2", "Bob", "bob-ec2@test.com")
		insertFriend(t, aID, bID, aID, StatusAccepted)

		got, err := repo.FindAll(ctx, aID, ListQuery{Q: "", Limit: 20})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, bID, got[0].UserID)
	})

	t.Run("context deadline is propagated", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		time.Sleep(1 * time.Millisecond)

		_, err := repo.FindAll(ctx, newID(), ListQuery{Limit: 20})
		assert.Error(t, err)

		var pgErr interface{ Error() string }
		if errors.As(err, &pgErr) {
			_ = pgErr
		}
		assert.NotNil(t, ctx.Err())
	})
}

var _ = sql.ErrNoRows
