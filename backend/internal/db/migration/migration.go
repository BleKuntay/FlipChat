package migration

import (
	"fmt"
	"github.com/BleKuntay/FlipChat/backend/internal/config"
	"github.com/BleKuntay/FlipChat/backend/pkg/logger"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

type MigrateConfig struct {
	DBUrl          string
	MigrationsPath string
}

func DatabaseURL() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		config.App.PostgresUser,
		config.App.PostgresPassword,
		config.App.PostgresHost,
		config.App.PostgresPort,
		config.App.PostgresDatabase,
		config.App.PostgresSSLMode,
	)
}

func NewMigrate(config MigrateConfig) (*migrate.Migrate, error) {
	m, err := migrate.New(
		fmt.Sprintf("file://%s", config.MigrationsPath),
		config.DBUrl,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create migrate instance: %s", err)
	}

	return m, nil
}

func RunMigrations(m *migrate.Migrate) error {
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration failed: %w", err)
	}

	logger.Info("migration applied successfully")

	return nil
}
