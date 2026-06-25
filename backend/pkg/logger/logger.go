package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"sync"
)

var (
	instance *zap.Logger
	once     sync.Once
)

func Init(env string) {
	once.Do(func() {
		var cfg zap.Config
		var err error

		if env == "production" {
			cfg = zap.NewProductionConfig()
		} else {
			cfg = zap.NewDevelopmentConfig()
			cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		}

		instance, err = cfg.Build()
		if err != nil {
			panic(err)
		}
	})
}

func Get() *zap.Logger {
	if instance == nil {
		instance, _ = zap.NewDevelopment()
	}

	return instance
}

func Sync() error {
	return Get().Sync()
}

func Debug(msg string, fields ...zap.Field) {
	Get().Debug(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	Get().Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	Get().Warn(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	Get().Error(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	Get().Fatal(msg, fields...)
}
