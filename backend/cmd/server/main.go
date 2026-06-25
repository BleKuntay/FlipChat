package main

import (
	"github.com/BleKuntay/FlipChat/backend/internal/config"
	"github.com/BleKuntay/FlipChat/backend/internal/db/migration"
	"github.com/BleKuntay/FlipChat/backend/internal/db/postgres"
	"github.com/BleKuntay/FlipChat/backend/pkg/jwt"
	"github.com/BleKuntay/FlipChat/backend/pkg/logger"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	fiberLogger "github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

func main() {
	config.LoadConfig()

	setupLogger()

	db := setupPostgres()
	applyMigration()

	_ = db // nanti dipakai untuk inject ke repository

	if err := setupJWT(); err != nil {
		logger.Fatal("JWT init failed", zap.Error(err))
	}

	app := setupApp()

	// register routes di sini nanti
	// v1 := app.Group("/v1")
	// user.NewHandler(...).RegisterRoutes(v1.Group("/users"))

	logger.Info("server starting",
		zap.String("env", config.App.AppEnv),
		zap.String("port", config.App.AppPort),
	)

	if err := app.Listen(":" + config.App.AppPort); err != nil {
		logger.Fatal("server failed to start", zap.Error(err))
	}
}

func setupLogger() {
	logger.Init(config.App.AppEnv)
	defer logger.Sync()
}

func setupPostgres() *sqlx.DB {
	return postgres.ConnectDB()
}

func setupJWT() error {
	return jwt.Init(config.App.JWTSecret)
}

func setupApp() *fiber.App {
	app := fiber.New()

	app.Use(fiberLogger.New())
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     config.App.AllowedOrigins,
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowCredentials: true,
	}))

	return app
}

func applyMigration() {
	cfg := migration.MigrateConfig{
		DBUrl:          migration.DatabaseURL(),
		MigrationsPath: "db/migrations",
	}

	m, err := migration.NewMigrate(cfg)
	if err != nil {
		logger.Fatal("migration NewMigrations failed", zap.Error(err))
	}
	defer m.Close()

	if err := migration.RunMigrations(m); err != nil {
		logger.Fatal("migration Up failed", zap.Error(err))
	}
}
