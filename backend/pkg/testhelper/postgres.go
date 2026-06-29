//go:build integration

package testhelper

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/BleKuntay/FlipChat/backend/internal/db/migration"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func NewTestDB(t *testing.T) *sqlx.DB {
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
