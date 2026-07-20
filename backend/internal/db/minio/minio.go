package miniodb

import (
	"context"
	"github.com/BleKuntay/FlipChat/backend/internal/config"
	"github.com/BleKuntay/FlipChat/backend/pkg/logger"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"
)

func Connect() *minio.Client {
	opts := minio.Options{
		Creds:  credentials.NewStaticV4(config.App.MinioAccessKey, config.App.MinioSecretKey, ""),
		Secure: config.App.MinioUseSSL,
	}

	client, err := minio.New(config.App.MinioEndpoint, &opts)
	if err != nil {
		logger.Fatal("could not create minio client", zap.Error(err))
	}

	if err := ensureBucket(client); err != nil {
		logger.Fatal("could not ensure minio bucket", zap.Error(err))
	}

	logger.Info("connected to minio",
		zap.String("endpoint", config.App.MinioEndpoint),
		zap.String("bucket", config.App.MinioBucket),
	)

	return client
}

func ensureBucket(client *minio.Client) error {
	ctx := context.Background()

	exists, err := client.BucketExists(ctx, config.App.MinioBucket)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	return client.MakeBucket(ctx, config.App.MinioBucket, minio.MakeBucketOptions{})
}
