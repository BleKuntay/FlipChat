package postgres

import (
	"github.com/BleKuntay/FlipChat/backend/internal/config"
	"github.com/BleKuntay/FlipChat/backend/pkg/logger"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	_ "github.com/lib/pq"
)

func ConnectDB() *sqlx.DB {
	db, err := sqlx.Connect("postgres", config.App.DSN())
	if err != nil {
		logger.Fatal("could not connect to database", zap.Error(err))
	}

	db.SetMaxOpenConns(config.App.PostgresMaxOpenConn)
	db.SetMaxIdleConns(config.App.PostgresMaxIdleConn)

	if err := db.Ping(); err != nil {
		logger.Fatal("could not ping database", zap.Error(err))
	}

	logger.Info("connected to database",
		zap.String("host", config.App.PostgresHost),
		zap.String("db", config.App.PostgresDatabase),
		zap.Int("max_open_conn", config.App.PostgresMaxOpenConn),
		zap.Int("max_idle_conn", config.App.PostgresMaxIdleConn),
	)

	return db
}
