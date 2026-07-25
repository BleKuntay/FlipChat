package main

import (
	"context"
	"github.com/BleKuntay/FlipChat/backend/internal/attachment"
	"github.com/BleKuntay/FlipChat/backend/internal/auth"
	"github.com/BleKuntay/FlipChat/backend/internal/block"
	"github.com/BleKuntay/FlipChat/backend/internal/config"
	"github.com/BleKuntay/FlipChat/backend/internal/conversation"
	"github.com/BleKuntay/FlipChat/backend/internal/db/migration"
	miniodb "github.com/BleKuntay/FlipChat/backend/internal/db/minio"
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
	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	config.LoadConfig()
	setupLogger()
	defer logger.Sync()

	if err := setupJWT(); err != nil {
		logger.Fatal("JWT init failed", zap.Error(err))
	}

	db := setupPostgres()
	applyMigration()

	redisClient := rdb.Connect()
	minioClient := miniodb.Connect()

	app := setupApp()
	registerRoutes(app, db, redisClient, minioClient)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("server starting",
			zap.String("env", config.App.AppEnv),
			zap.String("port", config.App.AppPort),
		)
		if err := app.Listen(":" + config.App.AppPort); err != nil {
			logger.Fatal("server failed to start", zap.Error(err))
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received, starting graceful termination...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		logger.Fatal("server forced to shutdown due to error", zap.Error(err))
	}

	logger.Info("server stopped gracefully")
}

func registerRoutes(app *fiber.App, db *sqlx.DB, redisClient *redis.Client, minioClient *minio.Client) {
	// ── infrastructure ────────────────────────────────────────────────────────
	presenceStore := presence.NewStore(redisClient, config.App.PresenceTTL)
	minioStore := miniodb.NewStore(minioClient)
	uploadStore := attachment.NewRedisUploadStore(redisClient)

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
	userSvc := user.NewService(userRepo, blockSvc, presenceStore, authRepo)
	friendSvc := friend.NewService(friendRepo, blockSvc)
	conversationSvc := conversation.NewService(conversationRepo, blockSvc)
	attachmentSvc := attachment.NewService(minioStore, uploadStore, messageRepo, conversationRepo)
	messageSvc := message.NewService(
		messageRepo,
		conversationRepo,
		blockSvc,
		hub,
		message.WithObjectDeleter(minioStore),
		message.WithAttachmentStore(uploadStore),
	)
	// ── handlers ──────────────────────────────────────────────────────────────
	authHandler := auth.NewHandler(authSvc)
	blockHandler := block.NewHandler(blockSvc)
	userHandler := user.NewHandler(userSvc)
	friendHandler := friend.NewHandler(friendSvc)
	conversationHandler := conversation.NewHandler(conversationSvc)
	attachmentHandler := attachment.NewHandler(attachmentSvc)
	messageHandler := message.NewHandler(messageSvc)
	wsHandler := ws.NewHandler(hub, presenceStore, conversationRepo, userRepo)

	// ── routes ────────────────────────────────────────────────────────────────
	v1 := app.Group("/v1")

	authHandler.RegisterRoutes(v1.Group("/auth"))
	wsHandler.RegisterRoute(v1)

	protected := v1.Group("", jwt.Protected())

	userHandler.RegisterRoutes(protected.Group("/users"))
	friendHandler.RegisterRoutes(protected.Group("/friends"))
	blockHandler.RegisterRoutes(protected.Group("/blocks"))
	attachmentHandler.RegisterRoutes(protected.Group("/attachments"))

	conv := protected.Group("/conversations")
	conversationHandler.RegisterRoute(conv)
	messageHandler.RegisterRoutes(conv)
}

func setupLogger() {
	logger.Init(config.App.AppEnv)
}

func setupPostgres() *sqlx.DB {
	return postgres.ConnectDB()
}

func setupJWT() error {
	return jwt.Init(config.App.JWTSecret)
}

func setupApp() *fiber.App {
	app := fiber.New(fiber.Config{
		// Must be larger than MaxFileSize (5MB) to account for multipart overhead.
		BodyLimit: 6 * 1024 * 1024,
	})

	app.Use(fiberLogger.New())
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     config.App.AllowedOrigins,
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
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
