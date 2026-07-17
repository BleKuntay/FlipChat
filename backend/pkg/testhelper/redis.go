//go:build integration

package testhelper

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
)

func NewTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	ctx := context.Background()

	container, err := tcredis.Run(ctx,
		"redis:8-alpine",
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err, "failed to start redis container")

	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("warn: failed to terminate redis container: %v", err)
		}
	})

	connStr, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	opts, err := redis.ParseURL(connStr)
	if err != nil {
		t.Fatal(err)
	}

	rdb := redis.NewClient(opts)

	t.Cleanup(func() { _ = rdb.Close() })

	require.NoError(t, rdb.Ping(ctx).Err(), "failed to ping redis container")

	return rdb
}
