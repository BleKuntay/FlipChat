package presence

import (
	"context"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"time"
)

type Store struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewStore(rdb *redis.Client, ttl time.Duration) *Store {
	return &Store{rdb: rdb, ttl: ttl}
}

func key(userID string) string {
	return fmt.Sprintf("online:%s", userID)
}

func (s *Store) SetOnline(ctx context.Context, userID string) error {
	return s.rdb.Set(ctx, key(userID), 1, s.ttl).Err()
}

func (s *Store) SetOffline(ctx context.Context, userID string) error {
	return s.rdb.Del(ctx, key(userID)).Err()
}

func (s *Store) IsOnline(ctx context.Context, userID string) (bool, error) {
	err := s.rdb.Get(ctx, key(userID)).Err()
	if err == nil {
		return true, nil
	}
	if errors.Is(err, redis.Nil) {
		return false, nil
	}

	return false, err
}
