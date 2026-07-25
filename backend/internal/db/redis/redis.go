package redis

import (
	"context"
	"github.com/BleKuntay/FlipChat/backend/internal/config"
	"github.com/BleKuntay/FlipChat/backend/pkg/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func Connect() *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     config.App.RedisAddr,
		Password: config.App.RedisPassword,
		DB:       config.App.RedisDB,
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		logger.Fatal("could not connect to redis", zap.Error(err))
	}

	logger.Info("connected to redis", zap.String("addr", config.App.RedisAddr))

	return rdb
}
