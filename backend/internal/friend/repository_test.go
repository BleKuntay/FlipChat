//go:build integration

package friend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var testDB *sqlx.DB

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "postgres:16-alpine",
			Env: map[string]string{
				"POSTGRES_DB":       "flipchat_test",
				"POSTGRES_USER":     "test",
				"POSTGRES_PASSWORD": "test",
			},
			ExposedPorts: []string{"5432/tcp"},
			WaitingFor:   wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		},
		Started: true,
	})
	if err != nil {
		panic(fmt.Sprintf("failed to start container: %v", err))
	}
	defer container.Terminate(ctx)

	host, err := container.Host(ctx)
	if err != nil {
		panic(err)
	}
	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		panic(err)
	}

	dsn := fmt.Sprintf("postgres://test:test@%s:%s/flipchat_test?sslmode=disable", host, port.Port())

	testDB, err = sqlx.Connect("postgres", dsn)
	if err != nil {
		panic(fmt.Sprintf("failed to connect: %v", err))
	}
	defer testDB.Close()

	// path relatif dari file ini ke migration directory
	_, filename, _, _ := runtime.Caller(0)
	migrationsPath := filepath.Join(filepath.Dir(filename), "..", "..", "db", "migrations")

	mig, err := migrate.New("file://"+migrationsPath, dsn)
	if err != nil {
		panic(fmt.Sprintf("failed to init migrator: %v", err))
	}
	if err := mig.Up(); err != nil && err != migrate.ErrNoChange {
		panic(fmt.Sprintf("migrations failed: %v", err))
	}

	os.Exit(m.Run())
}

// ------------------------------------------------------------------ //
// Helpers                                                              //
// ------------------------------------------------------------------ //

// insertUser memasukkan test user ke DB.
// Sesuaikan kolom dengan schema users table yang ada di proyekmu.
func insertUser(t *testing.T, id, username, name, email string) {
	t.Helper()
	_, err := testDB.Exec(`
		INSERT INTO users (id, username, name, email, password_hash)
		VALUES ($1, $2, $3, $4, 'dummy_hash')
	`, id, username, name, email)
	require.NoError(t, err)

	t.Cleanup(func() {
		// ON DELETE CASCADE di friends akan ikut membersihkan
		testDB.Exec("DELETE FROM users WHERE id = $1", id)
	})
}

func insertFriend(t *testing.T, lowID, highID, requesterID string, status Status) {
	t.Helper()
	_, err := testDB.Exec(`
		INSERT INTO friends (user_low_id, user_high_id, requester_id, status)
		VALUES ($1, $2, $3, $4)
	`, lowID, highID, requesterID, string(status))
	require.NoError(t, err)

	t.Cleanup(func() {
		testDB.Exec(
			"DELETE FROM friends WHERE user_low_id = $1 AND user_high_id = $2",
			lowID, highID,
		)
	})
}

// ------------------------------------------------------------------ //
// Tests                                                                 //
// ------------------------------------------------------------------ //

func TestRepository_ExistsByUserID(t *testing.T) {
	repo := NewRepository(testDB)
	ctx := context.Background()

	t.Run("user ada returns true", func(t *testing.T) {
		insertUser(t, userA, "alice", "Alice", "alice@test.com")

		got, err := repo.ExistsByUserID(ctx, userA)

		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("user tidak ada returns false", func(t *testing.T) {
		got, err := repo.ExistsByUserID(ctx, "00000000-0000-0000-0000-000000000099")

		require.NoError(t, err)
		assert.False(t, got)
	})
}

func TestRepository_FindByPair(t *testing.T) {
	repo := NewRepository(testDB)
	ctx := context.Background()

	t.Run("pair tidak ada returns nil", func(t *testing.T) {
		got, err := repo.FindByPair(ctx, userA, userB)

		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("pair ada returns Friend", func(t *testing.T) {
		insertUser(t, userA, "alice", "Alice", "alice@test.com")
		insertUser(t, userB, "bob", "Bob", "bob@test.com")
		insertFriend(t, userA, userB, userA, StatusPending)

		got, err := repo.FindByPair(ctx, userA, userB)

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, StatusPending, got.Status)
		assert.Equal(t, userA, got.RequesterID)
	})
}

func TestRepository_AddFriend(t *testing.T) {
	repo := NewRepository(testDB)
	ctx := context.Background()

	t.Run("berhasil insert dan return profil target", func(t *testing.T) {
		insertUser(t, userA, "alice", "Alice", "alice@test.com")
		insertUser(t, userB, "bob", "Bob", "bob@test.com")

		got, err := repo.AddFriend(ctx, userA, userB, userA)

		require.NoError(t, err)
		require.NotNil(t, got)
		// harus return profil target (bukan requester)
		assert.Equal(t, userB, got.UserID)
		assert.Equal(t, "bob", got.Username)
		assert.Equal(t, "Bob", got.FullName)
		assert.Equal(t, userA, got.RequesterID)
	})
}

func TestRepository_AcceptFriend(t *testing.T) {
	repo := NewRepository(testDB)
	ctx := context.Background()

	t.Run("berhasil update status dan return profil requester", func(t *testing.T) {
		insertUser(t, userA, "alice", "Alice", "alice@test.com")
		insertUser(t, userB, "bob", "Bob", "bob@test.com")
		insertFriend(t, userA, userB, userA, StatusPending)

		got, err := repo.AcceptFriend(ctx, userA, userB)

		require.NoError(t, err)
		require.NotNil(t, got)
		// harus return profil requester (A yang kirim request)
		assert.Equal(t, userA, got.UserID)
		assert.Equal(t, "alice", got.Username)

		// verifikasi status berubah di DB
		pair, err := repo.FindByPair(ctx, userA, userB)
		require.NoError(t, err)
		assert.Equal(t, StatusAccepted, pair.Status)
	})
}

func TestRepository_FindAll(t *testing.T) {
	repo := NewRepository(testDB)
	ctx := context.Background()

	t.Run("returns semua teman yang accepted", func(t *testing.T) {
		insertUser(t, userA, "alice", "Alice", "alice@test.com")
		insertUser(t, userB, "bob", "Bob", "bob@test.com")
		insertFriend(t, userA, userB, userA, StatusAccepted)

		got, err := repo.FindAll(ctx, userA, ListQuery{Limit: 20})

		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, userB, got[0].UserID)
		assert.Equal(t, "bob", got[0].Username)
	})

	t.Run("pending request tidak muncul di list teman", func(t *testing.T) {
		insertUser(t, userA, "alice", "Alice", "alice2@test.com")
		insertUser(t, userB, "bob", "Bob", "bob2@test.com")
		insertFriend(t, userA, userB, userA, StatusPending)

		got, err := repo.FindAll(ctx, userA, ListQuery{Limit: 20})

		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

func TestRepository_FindAllRequests(t *testing.T) {
	repo := NewRepository(testDB)
	ctx := context.Background()

	t.Run("returns pending requests dari kedua arah", func(t *testing.T) {
		insertUser(t, userA, "alice", "Alice", "alice@test.com")
		insertUser(t, userB, "bob", "Bob", "bob@test.com")
		// A kirim request ke B
		insertFriend(t, userA, userB, userA, StatusPending)

		// dari sudut pandang A: harus ada 1 request (sent)
		gotA, err := repo.FindAllRequests(ctx, userA, RequestListQuery{Limit: 20})
		require.NoError(t, err)
		require.Len(t, gotA, 1)
		assert.Equal(t, userB, gotA[0].UserID)
		assert.Equal(t, userA, gotA[0].RequesterID)

		// dari sudut pandang B: harus ada 1 request (received)
		gotB, err := repo.FindAllRequests(ctx, userB, RequestListQuery{Limit: 20})
		require.NoError(t, err)
		require.Len(t, gotB, 1)
		assert.Equal(t, userA, gotB[0].UserID)
		assert.Equal(t, userA, gotB[0].RequesterID)
	})
}

func TestRepository_DeleteByPair(t *testing.T) {
	repo := NewRepository(testDB)
	ctx := context.Background()

	t.Run("berhasil hapus pair", func(t *testing.T) {
		insertUser(t, userA, "alice", "Alice", "alice@test.com")
		insertUser(t, userB, "bob", "Bob", "bob@test.com")
		insertFriend(t, userA, userB, userA, StatusPending)

		err := repo.DeleteByPair(ctx, userA, userB)
		require.NoError(t, err)

		pair, err := repo.FindByPair(ctx, userA, userB)
		require.NoError(t, err)
		assert.Nil(t, pair)
	})
}
