//go:build integration

package message_test

import (
	"context"
	"testing"
	"time"

	"github.com/BleKuntay/FlipChat/backend/internal/message"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/BleKuntay/FlipChat/backend/pkg/testhelper"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func insertUser(t *testing.T, db *sqlx.DB) string {
	t.Helper()
	var id string
	err := db.QueryRow(`
        INSERT INTO users (name, username, email, password)
        VALUES ($1, $2, $3, $4)
        RETURNING id
    `, "Test User", "user_"+uuid.NewString()[:8], uuid.NewString()+"@example.com", "hashed").Scan(&id)
	require.NoError(t, err)
	return id
}

func insertConversation(t *testing.T, db *sqlx.DB, userLowID, userHighID string) string {
	t.Helper()
	if userLowID > userHighID {
		userLowID, userHighID = userHighID, userLowID
	}
	var id string
	err := db.QueryRow(`
        INSERT INTO conversations (id, user_low_id, user_high_id)
        VALUES ($1, $2, $3)
        RETURNING id
    `, uuid.New().String(), userLowID, userHighID).Scan(&id)
	require.NoError(t, err)
	return id
}

func insertMessage(t *testing.T, db *sqlx.DB, m *message.Message) {
	t.Helper()
	_, err := db.Exec(`
        INSERT INTO messages (id, conversation_id, sender_id, content, reply_to_id, metadata, is_edited, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
    `, m.ID, m.ConversationID, m.SenderID, m.Content, m.ReplyToID, m.Metadata, m.IsEdited, m.CreatedAt)
	require.NoError(t, err)
}

func strPtr(s string) *string { return &s }

// ── Create ────────────────────────────────────────────────────────────────────

func TestRepository_Create(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := message.NewRepository(db)
	ctx := context.Background()

	userA := insertUser(t, db)
	userB := insertUser(t, db)
	convoID := insertConversation(t, db, userA, userB)

	t.Run("inserts message and can be retrieved", func(t *testing.T) {
		v7, _ := uuid.NewV7()
		content := "halo!"
		m := &message.Message{
			ID:             v7.String(),
			ConversationID: convoID,
			SenderID:       userA,
			Content:        &content,
			CreatedAt:      time.Now().UTC().Truncate(time.Microsecond),
		}

		err := repo.Create(ctx, m)
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, m.ID)
		require.NoError(t, err)
		assert.Equal(t, m.ID, got.ID)
		assert.Equal(t, convoID, got.ConversationID)
		assert.Equal(t, userA, got.SenderID)
		assert.Equal(t, &content, got.Content)
		assert.False(t, got.IsEdited)
		assert.Nil(t, got.ReplyToID)
	})

	t.Run("inserts message with reply_to_id", func(t *testing.T) {
		v7a, _ := uuid.NewV7()
		v7b, _ := uuid.NewV7()
		content := "original"
		reply := "reply!"

		original := &message.Message{
			ID:             v7a.String(),
			ConversationID: convoID,
			SenderID:       userA,
			Content:        &content,
			CreatedAt:      time.Now().UTC().Truncate(time.Microsecond),
		}
		require.NoError(t, repo.Create(ctx, original))

		replyMsg := &message.Message{
			ID:             v7b.String(),
			ConversationID: convoID,
			SenderID:       userB,
			Content:        &reply,
			ReplyToID:      &original.ID,
			CreatedAt:      time.Now().UTC().Truncate(time.Microsecond),
		}
		require.NoError(t, repo.Create(ctx, replyMsg))

		got, err := repo.GetByID(ctx, replyMsg.ID)
		require.NoError(t, err)
		require.NotNil(t, got.ReplyToID)
		assert.Equal(t, original.ID, *got.ReplyToID)
	})
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func TestRepository_GetByID(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := message.NewRepository(db)
	ctx := context.Background()

	userA := insertUser(t, db)
	userB := insertUser(t, db)
	convoID := insertConversation(t, db, userA, userB)

	t.Run("returns ErrNotFound for non-existent ID", func(t *testing.T) {
		_, err := repo.GetByID(ctx, uuid.New().String())
		assert.ErrorIs(t, err, apperr.ErrNotFound)
	})

	t.Run("returns error for malformed UUID", func(t *testing.T) {
		_, err := repo.GetByID(ctx, "not-a-uuid")
		assert.Error(t, err)
	})

	t.Run("returns correct message for existing ID", func(t *testing.T) {
		v7, _ := uuid.NewV7()
		content := "test content"
		m := &message.Message{
			ID:             v7.String(),
			ConversationID: convoID,
			SenderID:       userA,
			Content:        &content,
			CreatedAt:      time.Now().UTC().Truncate(time.Microsecond),
		}
		insertMessage(t, db, m)

		got, err := repo.GetByID(ctx, m.ID)
		require.NoError(t, err)
		assert.Equal(t, m.ID, got.ID)
		assert.Equal(t, convoID, got.ConversationID)
	})
}

// ── ListByConversation ────────────────────────────────────────────────────────

func TestRepository_ListByConversation(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := message.NewRepository(db)
	ctx := context.Background()

	userA := insertUser(t, db)
	userB := insertUser(t, db)
	convoID := insertConversation(t, db, userA, userB)

	// Seed 5 messages in order
	var msgIDs []string
	for i := 0; i < 5; i++ {
		v7, _ := uuid.NewV7()
		content := "msg"
		m := &message.Message{
			ID:             v7.String(),
			ConversationID: convoID,
			SenderID:       userA,
			Content:        &content,
			CreatedAt:      time.Now().UTC().Truncate(time.Microsecond),
		}
		insertMessage(t, db, m)
		msgIDs = append(msgIDs, m.ID)
		time.Sleep(2 * time.Millisecond) // Ensure UUIDv7 ordering is deterministic
	}

	t.Run("no cursor returns newest first", func(t *testing.T) {
		msgs, err := repo.ListByConversation(ctx, convoID, "", 10)
		require.NoError(t, err)
		assert.Len(t, msgs, 5)
		// Newest first — last inserted message should be first in response
		assert.Equal(t, msgIDs[4], msgs[0].ID)
		assert.Equal(t, msgIDs[0], msgs[4].ID)
	})

	t.Run("limit respected", func(t *testing.T) {
		msgs, err := repo.ListByConversation(ctx, convoID, "", 3)
		require.NoError(t, err)
		assert.Len(t, msgs, 3)
	})

	t.Run("cursor returns only older messages", func(t *testing.T) {
		// cursor = msgIDs[3] → only return msgIDs[0..2]
		msgs, err := repo.ListByConversation(ctx, convoID, msgIDs[3], 10)
		require.NoError(t, err)
		assert.Len(t, msgs, 3)
		for _, m := range msgs {
			assert.Less(t, m.ID, msgIDs[3], "all results must be older than cursor")
		}
	})

	t.Run("empty conversation returns empty slice", func(t *testing.T) {
		userC := insertUser(t, db)
		userD := insertUser(t, db)
		emptyConvo := insertConversation(t, db, userC, userD)

		msgs, err := repo.ListByConversation(ctx, emptyConvo, "", 10)
		require.NoError(t, err)
		assert.Empty(t, msgs)
	})
}
