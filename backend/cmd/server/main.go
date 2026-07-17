package main

import (
	"github.com/BleKuntay/FlipChat/backend/internal/auth"
	"github.com/BleKuntay/FlipChat/backend/internal/block"
	"github.com/BleKuntay/FlipChat/backend/internal/config"
	"github.com/BleKuntay/FlipChat/backend/internal/conversation"
	"github.com/BleKuntay/FlipChat/backend/internal/db/migration"
	"github.com/BleKuntay/FlipChat/backend/internal/db/postgres"
	rdb "github.com/BleKuntay/FlipChat/backend/internal/db/redis"
	"github.com/BleKuntay/FlipChat/backend/internal/friend"
	"github.com/BleKuntay/FlipChat/backend/internal/message"
	"github.com/BleKuntay/FlipChat/backend/internal/presence"
	"github.com/BleKuntay/FlipChat/backend/internal/user"
	"github.com/BleKuntay/FlipChat/backend/internal/ws"
	"github.com/BleKuntay/FlipChat/backend/pkg/jwt"
	"github.com/BleKuntay/FlipChat/backend/pkg/logger"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	fiberLogger "github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func main() {
	config.LoadConfig()
	setupLogger()

	if err := setupJWT(); err != nil {
		logger.Fatal("JWT init failed", zap.Error(err))
	}

	db := setupPostgres()
	applyMigration()

	redisClient := rdb.Connect()

	app := setupApp()
	registerRoutes(app, db, redisClient)

	logger.Info("server starting",
		zap.String("env", config.App.AppEnv),
		zap.String("port", config.App.AppPort),
	)

	if err := app.Listen(":" + config.App.AppPort); err != nil {
		logger.Fatal("server failed to start", zap.Error(err))
	}
}

func registerRoutes(app *fiber.App, db *sqlx.DB, redisClient *redis.Client) {
	// ── infrastructure ────────────────────────────────────────────────────────
	presenceStore := presence.NewStore(redisClient, config.App.PresenceTTL)

	// ── repositories ─────────────────────────────────────────────────────────
	authRepo := auth.NewRepository(db, redisClient, config.App.RefreshTokenExpiry)
	blockRepo := block.NewRepository(db)
	hub := ws.NewHub(blockRepo)
	userRepo := user.NewRepository(db)
	friendRepo := friend.NewRepository(db)
	conversationRepo := conversation.NewRepository(db)
	messageRepo := message.NewRepository(db)

	// ── services ──────────────────────────────────────────────────────────────
	authSvc := auth.NewService(authRepo)
	blockSvc := block.NewService(blockRepo)
	userSvc := user.NewService(userRepo, blockSvc, presenceStore)
	friendSvc := friend.NewService(friendRepo, blockSvc)
	conversationSvc := conversation.NewService(conversationRepo, blockSvc)
	messageSvc := message.NewService(messageRepo, conversationRepo, blockSvc, hub)

	// ── handlers ──────────────────────────────────────────────────────────────
	authHandler := auth.NewHandler(authSvc)
	blockHandler := block.NewHandler(blockSvc)
	userHandler := user.NewHandler(userSvc)
	friendHandler := friend.NewHandler(friendSvc)
	conversationHandler := conversation.NewHandler(conversationSvc)
	messageHandler := message.NewHandler(messageSvc)
	wsHandler := ws.NewHandler(hub, presenceStore, conversationRepo, userRepo)

	// ── routes ────────────────────────────────────────────────────────────────
	v1 := app.Group("/v1")

	authHandler.RegisterRoutes(v1.Group("/auth"))
	wsHandler.RegisterRoute(v1)

	protected := v1.Use(jwt.Protected())

	userHandler.RegisterRoutes(protected.Group("/users"))
	friendHandler.RegisterRoutes(protected.Group("/friends"))
	blockHandler.RegisterRoutes(protected.Group("/blocks"))

	conv := protected.Group("/conversations")
	conversationHandler.RegisterRoute(conv)
	messageHandler.RegisterRoutes(conv)
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
