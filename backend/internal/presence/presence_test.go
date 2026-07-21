//go:build integration

package presence_test

import (
	"context"
	"testing"
	"time"

	"github.com/BleKuntay/FlipChat/backend/internal/presence"
	"github.com/BleKuntay/FlipChat/backend/pkg/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── SetOnline tests ───────────────────────────────────────────────────────────

func TestStore_SetOnline_KeyCreated(t *testing.T) {
	ctx := context.Background()
	rdb := testhelper.NewTestRedis(t)
	store := presence.NewStore(rdb, 30*time.Second)

	const userID = "user-123"

	err := store.SetOnline(ctx, userID)
	require.NoError(t, err)

	// Verify key exists in Redis with correct format
	val, err := rdb.Get(ctx, "online:user-123").Result()
	require.NoError(t, err)
	assert.Equal(t, "1", val)
}

func TestStore_SetOnline_TTLSet(t *testing.T) {
	ctx := context.Background()
	rdb := testhelper.NewTestRedis(t)

	ttl := 35 * time.Second
	store := presence.NewStore(rdb, ttl)

	const userID = "user-456"

	err := store.SetOnline(ctx, userID)
	require.NoError(t, err)

	// Verify TTL is set approximately to the configured duration
	duration, err := rdb.TTL(ctx, "online:user-456").Result()
	require.NoError(t, err)

	// Allow 5 seconds margin for test execution time
	assert.True(t, duration > 0 && duration <= ttl, "TTL should be set and positive")
}

func TestStore_SetOnline_CalledTwice_TTLReset(t *testing.T) {
	ctx := context.Background()
	rdb := testhelper.NewTestRedis(t)

	ttl := 30 * time.Second
	store := presence.NewStore(rdb, ttl)

	const userID = "user-789"

	// Set online first time
	err := store.SetOnline(ctx, userID)
	require.NoError(t, err)

	ttl1, err := rdb.TTL(ctx, "online:user-789").Result()
	require.NoError(t, err)

	// Wait a bit
	time.Sleep(1 * time.Second)

	// Set online again
	err = store.SetOnline(ctx, userID)
	require.NoError(t, err)

	ttl2, err := rdb.TTL(ctx, "online:user-789").Result()
	require.NoError(t, err)

	// TTL should be reset (second call should have equal or slightly greater TTL than first call after 1s wait)
	assert.True(t, ttl2 >= ttl1, "TTL should be reset on second SetOnline call")
}

func TestStore_SetOnline_EmptyUserID_KeyCreatedWithEmptyUserID(t *testing.T) {
	ctx := context.Background()
	rdb := testhelper.NewTestRedis(t)
	store := presence.NewStore(rdb, 30*time.Second)

	const userID = ""

	err := store.SetOnline(ctx, userID)
	require.NoError(t, err)

	// Key "online:" will be created (no guard for empty userID)
	val, err := rdb.Get(ctx, "online:").Result()
	require.NoError(t, err)
	assert.Equal(t, "1", val)
}

// ── SetOffline tests ──────────────────────────────────────────────────────────

func TestStore_SetOffline_ExistingKey_Deleted(t *testing.T) {
	ctx := context.Background()
	rdb := testhelper.NewTestRedis(t)
	store := presence.NewStore(rdb, 30*time.Second)

	const userID = "user-abc"

	// First, set online
	err := store.SetOnline(ctx, userID)
	require.NoError(t, err)

	// Verify key exists
	val, err := rdb.Get(ctx, "online:user-abc").Result()
	require.NoError(t, err)
	assert.Equal(t, "1", val)

	// Now set offline
	err = store.SetOffline(ctx, userID)
	require.NoError(t, err)

	// Verify key is deleted
	val, err = rdb.Get(ctx, "online:user-abc").Result()
	assert.Error(t, err)
	assert.Empty(t, val)
}

func TestStore_SetOffline_NonExistentKey_NoError(t *testing.T) {
	ctx := context.Background()
	rdb := testhelper.NewTestRedis(t)
	store := presence.NewStore(rdb, 30*time.Second)

	const userID = "user-nonexistent"

	// Set offline without setting online first
	err := store.SetOffline(ctx, userID)
	require.NoError(t, err) // Should be idempotent

	// Verify key still doesn't exist
	val, err := rdb.Get(ctx, "online:user-nonexistent").Result()
	assert.Error(t, err)
	assert.Empty(t, val)
}

func TestStore_SetOffline_ExpiredKey_NoError(t *testing.T) {
	ctx := context.Background()
	rdb := testhelper.NewTestRedis(t)

	// Create store with very short TTL
	shortTTL := 100 * time.Millisecond
	store := presence.NewStore(rdb, shortTTL)

	const userID = "user-exp"

	// Set online
	err := store.SetOnline(ctx, userID)
	require.NoError(t, err)

	// Wait for key to expire
	time.Sleep(150 * time.Millisecond)

	// Set offline (key already expired)
	err = store.SetOffline(ctx, userID)
	require.NoError(t, err) // Should still succeed

	// Verify key is gone
	val, err := rdb.Get(ctx, "online:user-exp").Result()
	assert.Error(t, err)
	assert.Empty(t, val)
}

// ── IsOnline tests ────────────────────────────────────────────────────────────

func TestStore_IsOnline_KeyExists_ReturnsTrue(t *testing.T) {
	ctx := context.Background()
	rdb := testhelper.NewTestRedis(t)
	store := presence.NewStore(rdb, 30*time.Second)

	const userID = "user-online"

	err := store.SetOnline(ctx, userID)
	require.NoError(t, err)

	isOnline, err := store.IsOnline(ctx, userID)
	require.NoError(t, err)
	assert.True(t, isOnline)
}

func TestStore_IsOnline_KeyNotExists_ReturnsFalse(t *testing.T) {
	ctx := context.Background()
	rdb := testhelper.NewTestRedis(t)
	store := presence.NewStore(rdb, 30*time.Second)

	const userID = "user-offline"

	isOnline, err := store.IsOnline(ctx, userID)
	require.NoError(t, err)
	assert.False(t, isOnline)
}

func TestStore_IsOnline_KeyExpired_ReturnsFalse(t *testing.T) {
	ctx := context.Background()
	rdb := testhelper.NewTestRedis(t)

	// Create store with very short TTL
	shortTTL := 100 * time.Millisecond
	store := presence.NewStore(rdb, shortTTL)

	const userID = "user-expired"

	err := store.SetOnline(ctx, userID)
	require.NoError(t, err)

	// Wait for key to expire
	time.Sleep(150 * time.Millisecond)

	isOnline, err := store.IsOnline(ctx, userID)
	require.NoError(t, err)
	assert.False(t, isOnline)
}

func TestStore_IsOnline_RedisNil_DistinguishedFromError(t *testing.T) {
	ctx := context.Background()
	rdb := testhelper.NewTestRedis(t)
	store := presence.NewStore(rdb, 30*time.Second)

	const userID = "user-notfound"

	// IsOnline should return (false, nil) for non-existent key (redis.Nil)
	isOnline, err := store.IsOnline(ctx, userID)
	require.NoError(t, err)
	assert.False(t, isOnline)
}

func TestStore_IsOnline_RedisError_Propagated(t *testing.T) {
	ctx := context.Background()

	// Create a disconnected Redis client to force an error
	// Using a closed or invalid connection
	rdb := testhelper.NewTestRedis(t)

	store := presence.NewStore(rdb, 30*time.Second)

	// Close the connection to force an error
	rdb.Close()

	const userID = "user-123"

	// IsOnline should propagate Redis errors (not redis.Nil)
	isOnline, err := store.IsOnline(ctx, userID)
	assert.Error(t, err)
	assert.False(t, isOnline)
}

func TestStore_IsOnline_MultipleUsers_Isolated(t *testing.T) {
	ctx := context.Background()
	rdb := testhelper.NewTestRedis(t)
	store := presence.NewStore(rdb, 30*time.Second)

	const (
		user1 = "user-1"
		user2 = "user-2"
		user3 = "user-3"
	)

	// Set user1 and user3 online
	err := store.SetOnline(ctx, user1)
	require.NoError(t, err)

	err = store.SetOnline(ctx, user3)
	require.NoError(t, err)

	// Check status
	is1Online, err := store.IsOnline(ctx, user1)
	require.NoError(t, err)
	assert.True(t, is1Online)

	is2Online, err := store.IsOnline(ctx, user2)
	require.NoError(t, err)
	assert.False(t, is2Online)

	is3Online, err := store.IsOnline(ctx, user3)
	require.NoError(t, err)
	assert.True(t, is3Online)
}
