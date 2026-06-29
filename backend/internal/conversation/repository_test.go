//go:build integration

package conversation_test

import (
	"context"
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

	. "github.com/BleKuntay/FlipChat/backend/internal/conversation"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
)

var testDB *sqlx.DB

// ── setup ───────────────────────────────────────────────────────────────────

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

	mg, err := migrate.New(fmt.Sprintf("file://%s", migrationsPath), dsn)
	if err != nil {
		return fmt.Errorf("new migrate: %w", err)
	}

	if err := mg.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}

	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newID() string { return uuid.New().String() }

func insertUser(t *testing.T, id, username, name, email string) {
	t.Helper()
	_, err := testDB.ExecContext(context.Background(), `
		INSERT INTO users (id, username, name, email, password)
		VALUES ($1, $2, $3, $4, 'placeholder')
	`, id, username, name, email)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testDB.ExecContext(context.Background(),
			"DELETE FROM users WHERE id = $1", id)
	})
}

func insertConversation(t *testing.T, lowID, highID string) string {
	t.Helper()
	var id string
	err := testDB.QueryRowContext(context.Background(), `
		INSERT INTO conversations (user_low_id, user_high_id)
		VALUES ($1, $2)
		RETURNING id
	`, lowID, highID).Scan(&id)
	require.NoError(t, err)
	return id
}

func insertMessage(t *testing.T, conversationID, senderID string, content *string) string {
	newMessageID, _ := uuid.NewV7()

	t.Helper()
	var id string
	err := testDB.QueryRowContext(context.Background(), `
		INSERT INTO messages (id, conversation_id, sender_id, content)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, newMessageID, conversationID, senderID, content).Scan(&id)
	require.NoError(t, err)
	return id
}

func canonical(a, b string) (string, string) {
	if a < b {
		return a, b
	}
	return b, a
}

func strPtr(s string) *string { return &s }

// ── FindAllByUserID ───────────────────────────────────────────────────────────

func TestRepository_FindAllByUserID(t *testing.T) {
	repo := NewRepository(testDB)
	ctx := context.Background()

	t.Run("returns conversations for user as user_low", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-fau1", "Alice", "alice-fau1@test.com")
		insertUser(t, bID, "bob-fau1", "Bob", "bob-fau1@test.com")

		low, high := canonical(aID, bID)
		insertConversation(t, low, high)

		results, err := repo.FindAllByUserID(ctx, aID)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "Bob", results[0].Name)
		assert.Equal(t, "bob-fau1", results[0].Username)
	})

	t.Run("returns conversations for user as user_high", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-fau2", "Alice", "alice-fau2@test.com")
		insertUser(t, bID, "bob-fau2", "Bob", "bob-fau2@test.com")

		low, high := canonical(aID, bID)
		insertConversation(t, low, high)

		results, err := repo.FindAllByUserID(ctx, bID)
		require.NoError(t, err)
		require.Len(t, results, 1)
	})

	t.Run("resolves other participant name and username", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-fau3", "Alice Wonder", "alice-fau3@test.com")
		insertUser(t, bID, "bob-fau3", "Bob Tables", "bob-fau3@test.com")

		low, high := canonical(aID, bID)
		insertConversation(t, low, high)

		results, err := repo.FindAllByUserID(ctx, aID)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "Bob Tables", results[0].Name)
		assert.Equal(t, "bob-fau3", results[0].Username)
	})

	t.Run("last message preview is nil when no messages", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-fau4", "Alice", "alice-fau4@test.com")
		insertUser(t, bID, "bob-fau4", "Bob", "bob-fau4@test.com")

		low, high := canonical(aID, bID)
		insertConversation(t, low, high)

		results, err := repo.FindAllByUserID(ctx, aID)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Nil(t, results[0].LastMessagePreview)
		assert.Nil(t, results[0].LastMessageAt)
	})

	t.Run("last message preview reflects most recent message", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-fau5", "Alice", "alice-fau5@test.com")
		insertUser(t, bID, "bob-fau5", "Bob", "bob-fau5@test.com")

		low, high := canonical(aID, bID)
		convID := insertConversation(t, low, high)
		insertMessage(t, convID, aID, strPtr("hello"))
		time.Sleep(10 * time.Millisecond)
		insertMessage(t, convID, bID, strPtr("world"))

		results, err := repo.FindAllByUserID(ctx, aID)
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.NotNil(t, results[0].LastMessagePreview)
		assert.Equal(t, "world", *results[0].LastMessagePreview)
	})

	t.Run("returns empty slice when user has no conversations", func(t *testing.T) {
		aID := newID()
		insertUser(t, aID, "alice-fau6", "Alice", "alice-fau6@test.com")

		results, err := repo.FindAllByUserID(ctx, aID)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("ordered by last message at desc nulls last", func(t *testing.T) {
		ownerID := newID()
		b1ID, b2ID, b3ID := newID(), newID(), newID()
		insertUser(t, ownerID, "owner-fau7", "Owner", "owner-fau7@test.com")
		insertUser(t, b1ID, "b1-fau7", "B1", "b1-fau7@test.com")
		insertUser(t, b2ID, "b2-fau7", "B2", "b2-fau7@test.com")
		insertUser(t, b3ID, "b3-fau7", "B3", "b3-fau7@test.com")

		// conv with no messages
		low1, high1 := canonical(ownerID, b1ID)
		insertConversation(t, low1, high1)

		// conv with older message
		low2, high2 := canonical(ownerID, b2ID)
		conv2 := insertConversation(t, low2, high2)
		insertMessage(t, conv2, ownerID, strPtr("older"))
		time.Sleep(10 * time.Millisecond)

		// conv with latest message
		low3, high3 := canonical(ownerID, b3ID)
		conv3 := insertConversation(t, low3, high3)
		insertMessage(t, conv3, ownerID, strPtr("newest"))

		results, err := repo.FindAllByUserID(ctx, ownerID)
		require.NoError(t, err)
		require.Len(t, results, 3)

		// newest message first, no-message conversation last
		assert.NotNil(t, results[0].LastMessagePreview)
		assert.Equal(t, "newest", *results[0].LastMessagePreview)
		assert.NotNil(t, results[1].LastMessagePreview)
		assert.Equal(t, "older", *results[1].LastMessagePreview)
		assert.Nil(t, results[2].LastMessagePreview)
	})
}

// ── FindByID ──────────────────────────────────────────────────────────────────

func TestRepository_FindByID(t *testing.T) {
	repo := NewRepository(testDB)
	ctx := context.Background()

	t.Run("returns conversation for participant", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-fbi1", "Alice", "alice-fbi1@test.com")
		insertUser(t, bID, "bob-fbi1", "Bob", "bob-fbi1@test.com")

		low, high := canonical(aID, bID)
		convID := insertConversation(t, low, high)

		result, err := repo.FindByID(ctx, aID, convID)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, convID, result.ConversationID)
		assert.Equal(t, "Bob", result.Name)
		assert.Equal(t, "bob-fbi1", result.Username)
	})

	t.Run("returns ErrNotFound for non-participant", func(t *testing.T) {
		aID, bID, cID := newID(), newID(), newID()
		insertUser(t, aID, "alice-fbi2", "Alice", "alice-fbi2@test.com")
		insertUser(t, bID, "bob-fbi2", "Bob", "bob-fbi2@test.com")
		insertUser(t, cID, "charlie-fbi2", "Charlie", "charlie-fbi2@test.com")

		low, high := canonical(aID, bID)
		convID := insertConversation(t, low, high)

		_, err := repo.FindByID(ctx, cID, convID)
		assert.ErrorIs(t, err, apperr.ErrNotFound)
	})

	t.Run("returns ErrNotFound for non-existent conversation", func(t *testing.T) {
		aID := newID()
		insertUser(t, aID, "alice-fbi3", "Alice", "alice-fbi3@test.com")

		_, err := repo.FindByID(ctx, aID, newID())
		assert.ErrorIs(t, err, apperr.ErrNotFound)
	})

	t.Run("resolves other participant correctly for both sides", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-fbi4", "Alice Wonder", "alice-fbi4@test.com")
		insertUser(t, bID, "bob-fbi4", "Bob Tables", "bob-fbi4@test.com")

		low, high := canonical(aID, bID)
		convID := insertConversation(t, low, high)

		// from A's perspective: sees B
		fromA, err := repo.FindByID(ctx, aID, convID)
		require.NoError(t, err)
		assert.Equal(t, "Bob Tables", fromA.Name)

		// from B's perspective: sees A
		fromB, err := repo.FindByID(ctx, bID, convID)
		require.NoError(t, err)
		assert.Equal(t, "Alice Wonder", fromB.Name)
	})

	t.Run("last message preview populated when messages exist", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-fbi5", "Alice", "alice-fbi5@test.com")
		insertUser(t, bID, "bob-fbi5", "Bob", "bob-fbi5@test.com")

		low, high := canonical(aID, bID)
		convID := insertConversation(t, low, high)
		insertMessage(t, convID, aID, strPtr("hi there"))

		result, err := repo.FindByID(ctx, aID, convID)
		require.NoError(t, err)
		require.NotNil(t, result.LastMessagePreview)
		assert.Equal(t, "hi there", *result.LastMessagePreview)
	})
}

// ── FindByPair ────────────────────────────────────────────────────────────────

func TestRepository_FindByPair(t *testing.T) {
	repo := NewRepository(testDB)
	ctx := context.Background()

	t.Run("returns conversation for existing pair", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-fbp1", "Alice", "alice-fbp1@test.com")
		insertUser(t, bID, "bob-fbp1", "Bob", "bob-fbp1@test.com")

		low, high := canonical(aID, bID)
		convID := insertConversation(t, low, high)

		result, err := repo.FindByPair(ctx, low, high)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, convID, result.ID)
	})

	t.Run("returns nil for non-existent pair", func(t *testing.T) {
		aID, bID := newID(), newID()
		low, high := canonical(aID, bID)

		result, err := repo.FindByPair(ctx, low, high)
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("returns nil when queried in wrong order (high, low)", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-fbp2", "Alice", "alice-fbp2@test.com")
		insertUser(t, bID, "bob-fbp2", "Bob", "bob-fbp2@test.com")

		low, high := canonical(aID, bID)
		insertConversation(t, low, high)

		result, err := repo.FindByPair(ctx, high, low)
		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestRepository_Create(t *testing.T) {
	repo := NewRepository(testDB)
	ctx := context.Background()

	t.Run("creates conversation and returns response with other participant info", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-c1", "Alice Wonder", "alice-c1@test.com")
		insertUser(t, bID, "bob-c1", "Bob Tables", "bob-c1@test.com")

		low, high := canonical(aID, bID)

		result, err := repo.Create(ctx, aID, low, high)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.NotEmpty(t, result.ConversationID)
		assert.Equal(t, "Bob Tables", result.Name)
		assert.Equal(t, "bob-c1", result.Username)
		assert.Nil(t, result.LastMessagePreview)
		assert.Nil(t, result.LastMessageAt)
	})

	t.Run("resolves other participant correctly from requester perspective", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-c2", "Alice", "alice-c2@test.com")
		insertUser(t, bID, "bob-c2", "Bob", "bob-c2@test.com")

		low, high := canonical(aID, bID)

		// from B's perspective
		result, err := repo.Create(ctx, bID, low, high)
		require.NoError(t, err)
		assert.Equal(t, "Alice", result.Name)
	})

	t.Run("duplicate pair returns error (unique constraint)", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-c3", "Alice", "alice-c3@test.com")
		insertUser(t, bID, "bob-c3", "Bob", "bob-c3@test.com")

		low, high := canonical(aID, bID)

		_, err := repo.Create(ctx, aID, low, high)
		require.NoError(t, err)

		_, err = repo.Create(ctx, aID, low, high)
		assert.Error(t, err, "duplicate pair should fail on unique constraint")
	})

	t.Run("created conversation is findable via FindByPair", func(t *testing.T) {
		aID, bID := newID(), newID()
		insertUser(t, aID, "alice-c4", "Alice", "alice-c4@test.com")
		insertUser(t, bID, "bob-c4", "Bob", "bob-c4@test.com")

		low, high := canonical(aID, bID)

		created, err := repo.Create(ctx, aID, low, high)
		require.NoError(t, err)

		found, err := repo.FindByPair(ctx, low, high)
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, created.ConversationID, found.ID)
	})
}
