package miniodb

import (
	"context"
	"github.com/BleKuntay/FlipChat/backend/internal/config"
	"github.com/minio/minio-go/v7"
	"io"
)

type Store struct {
	client *minio.Client
	bucket string
}

func NewStore(client *minio.Client) *Store {
	return &Store{
		client: client,
		bucket: config.App.MinioBucket,
	}
}

func (s *Store) PutObject(ctx context.Context, objectKey string, reader io.Reader, size int64, mimeType string) error {
	opts := minio.PutObjectOptions{
		ContentType: mimeType,
	}

	_, err := s.client.PutObject(ctx, s.bucket, objectKey, reader, size, opts)
	return err
}

func (s *Store) GetObject(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	object, err := s.client.GetObject(ctx, s.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}

	return object, nil
}

func (s *Store) DeleteObject(ctx context.Context, objectKey string) error {
	return s.client.RemoveObject(ctx, s.bucket, objectKey, minio.RemoveObjectOptions{})
}
