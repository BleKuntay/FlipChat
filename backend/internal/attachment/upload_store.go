package attachment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/redis/go-redis/v9"
	"time"
)

const UploadRecordTTL = time.Hour

// UploadRecord holds trusted metadata written by Upload and consumed
// exactly once by SendMessage. TTL acts as automatic orphan cleanup.
type UploadRecord struct {
	UploaderID string `json:"uploader_id"`
	ObjectKey  string `json:"object_key"`
	MIMEType   string `json:"mime_type"`
	Filename   string `json:"filename"`
	Size       int64  `json:"size"`
}

// UploadStore is the interface consumed by both Service (write) and
// the message package (read+delete).
type UploadStore interface {
	SaveUploadRecord(ctx context.Context, attachmentID string, record UploadRecord) error
	PopUploadRecord(ctx context.Context, attachmentID string) (*UploadRecord, error)
}

type redisUploadStore struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewRedisUploadStore(rdb *redis.Client) UploadStore {
	return &redisUploadStore{
		rdb: rdb,
		ttl: UploadRecordTTL,
	}
}

func uploadKey(attachmentID string) string {
	return fmt.Sprintf("upload:%s", attachmentID)
}

func (s *redisUploadStore) SaveUploadRecord(ctx context.Context, attachmentID string, record UploadRecord) error {
	b, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, uploadKey(attachmentID), b, s.ttl).Err()
}

// PopUploadRecord atomically fetches and deletes the record.
// Returns apperr.ErrNotFound if the record does not exist or has expired.
func (s *redisUploadStore) PopUploadRecord(ctx context.Context, attachmentID string) (*UploadRecord, error) {
	key := uploadKey(attachmentID)

	pipe := s.rdb.TxPipeline()
	getCmd := pipe.Get(ctx, key)
	pipe.Del(ctx, key)

	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	val, err := getCmd.Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, apperr.ErrNotFound
		}
		return nil, err
	}

	var record UploadRecord
	if err := json.Unmarshal([]byte(val), &record); err != nil {
		return nil, err
	}

	return &record, nil
}
