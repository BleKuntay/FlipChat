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

func newMsg(convoID, senderID string) *message.Message {
	v7, _ := uuid.NewV7()
	return &message.Message{
		ID:             v7.String(),
		ConversationID: convoID,
		SenderID:       senderID,
		Content:        strPtr("hello"),
		CreatedAt:      time.Now().UTC().Truncate(time.Microsecond),
	}
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestRepository_Create(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := message.NewRepository(db)
	ctx := context.Background()

	userA := insertUser(t, db)
	userB := insertUser(t, db)
	convoID := insertConversation(t, db, userA, userB)

	t.Run("inserts message and can be retrieved", func(t *testing.T) {
		m := newMsg(convoID, userA)
		require.NoError(t, repo.Create(ctx, m))

		got, err := repo.GetByID(ctx, m.ID)
		require.NoError(t, err)
		assert.Equal(t, m.ID, got.ID)
		assert.Equal(t, convoID, got.ConversationID)
		assert.Equal(t, userA, got.SenderID)
		assert.Equal(t, m.Content, got.Content)
		assert.False(t, got.IsEdited)
		assert.Nil(t, got.ReplyToID)
		assert.Nil(t, got.DeletedAt)
		assert.Nil(t, got.UpdatedAt)
	})

	t.Run("inserts message with reply_to_id", func(t *testing.T) {
		original := newMsg(convoID, userA)
		require.NoError(t, repo.Create(ctx, original))

		reply := newMsg(convoID, userB)
		reply.ReplyToID = &original.ID
		require.NoError(t, repo.Create(ctx, reply))

		got, err := repo.GetByID(ctx, reply.ID)
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

	t.Run("returns correct message with all lifecycle columns", func(t *testing.T) {
		m := newMsg(convoID, userA)
		insertMessage(t, db, m)

		got, err := repo.GetByID(ctx, m.ID)
		require.NoError(t, err)
		assert.Equal(t, m.ID, got.ID)
		assert.Equal(t, convoID, got.ConversationID)
		assert.Nil(t, got.UpdatedAt)
		assert.Nil(t, got.ReadAt)
		assert.Nil(t, got.DeletedAt)
		assert.Nil(t, got.DeletedBy)
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

	var msgIDs []string
	for i := 0; i < 5; i++ {
		m := newMsg(convoID, userA)
		insertMessage(t, db, m)
		msgIDs = append(msgIDs, m.ID)
		time.Sleep(2 * time.Millisecond)
	}

	t.Run("no cursor returns newest first", func(t *testing.T) {
		msgs, err := repo.ListByConversation(ctx, convoID, "", 10)
		require.NoError(t, err)
		assert.Len(t, msgs, 5)
		assert.Equal(t, msgIDs[4], msgs[0].ID)
		assert.Equal(t, msgIDs[0], msgs[4].ID)
	})

	t.Run("limit respected", func(t *testing.T) {
		msgs, err := repo.ListByConversation(ctx, convoID, "", 3)
		require.NoError(t, err)
		assert.Len(t, msgs, 3)
	})

	t.Run("cursor returns only older messages", func(t *testing.T) {
		msgs, err := repo.ListByConversation(ctx, convoID, msgIDs[3], 10)
		require.NoError(t, err)
		assert.Len(t, msgs, 3)
		for _, m := range msgs {
			assert.Less(t, m.ID, msgIDs[3])
		}
	})

	t.Run("deleted messages are included with is_deleted true", func(t *testing.T) {
		userC := insertUser(t, db)
		userD := insertUser(t, db)
		emptyConvo := insertConversation(t, db, userC, userD)

		m := newMsg(emptyConvo, userC)
		insertMessage(t, db, m)

		// soft delete directly via SQL
		_, err := db.Exec(`UPDATE messages SET content = NULL, deleted_at = now(), deleted_by = $1 WHERE id = $2`, userC, m.ID)
		require.NoError(t, err)

		msgs, err := repo.ListByConversation(ctx, emptyConvo, "", 10)
		require.NoError(t, err)
		require.Len(t, msgs, 1)
		assert.Nil(t, msgs[0].Content)
		assert.NotNil(t, msgs[0].DeletedAt)
	})

	t.Run("empty conversation returns empty slice", func(t *testing.T) {
		userE := insertUser(t, db)
		userF := insertUser(t, db)
		emptyConvo := insertConversation(t, db, userE, userF)

		msgs, err := repo.ListByConversation(ctx, emptyConvo, "", 10)
		require.NoError(t, err)
		assert.Empty(t, msgs)
	})
}

// ── EditMessage ───────────────────────────────────────────────────────────────

func TestRepository_EditMessage(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := message.NewRepository(db)
	ctx := context.Background()

	userA := insertUser(t, db)
	userB := insertUser(t, db)
	convoID := insertConversation(t, db, userA, userB)

	t.Run("updates content, sets is_edited and updated_at", func(t *testing.T) {
		m := newMsg(convoID, userA)
		insertMessage(t, db, m)

		now := time.Now().UTC().Truncate(time.Microsecond)
		m.Content = strPtr("edited content")
		m.IsEdited = true
		m.UpdatedAt = &now

		require.NoError(t, repo.EditMessage(ctx, m))

		got, err := repo.GetByID(ctx, m.ID)
		require.NoError(t, err)
		require.NotNil(t, got.Content)
		assert.Equal(t, "edited content", *got.Content)
		assert.True(t, got.IsEdited)
		require.NotNil(t, got.UpdatedAt)
	})

	t.Run("returns ErrNotFound for non-existent message", func(t *testing.T) {
		v7, _ := uuid.NewV7()
		now := time.Now()
		ghost := &message.Message{
			ID:        v7.String(),
			Content:   strPtr("x"),
			IsEdited:  true,
			UpdatedAt: &now,
		}
		err := repo.EditMessage(ctx, ghost)
		assert.ErrorIs(t, err, apperr.ErrNotFound)
	})
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestRepository_Delete(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := message.NewRepository(db)
	ctx := context.Background()

	userA := insertUser(t, db)
	userB := insertUser(t, db)
	convoID := insertConversation(t, db, userA, userB)

	t.Run("sets content null, deleted_at, deleted_by", func(t *testing.T) {
		m := newMsg(convoID, userA)
		insertMessage(t, db, m)

		now := time.Now().UTC().Truncate(time.Microsecond)
		m.Content = nil
		m.DeletedAt = &now
		m.DeletedBy = &userA

		require.NoError(t, repo.Delete(ctx, m))

		got, err := repo.GetByID(ctx, m.ID)
		require.NoError(t, err)
		assert.Nil(t, got.Content)
		require.NotNil(t, got.DeletedAt)
		require.NotNil(t, got.DeletedBy)
		assert.Equal(t, userA, *got.DeletedBy)
	})

	t.Run("returns ErrNotFound for non-existent message", func(t *testing.T) {
		v7, _ := uuid.NewV7()
		now := time.Now()
		ghost := &message.Message{
			ID:        v7.String(),
			DeletedAt: &now,
			DeletedBy: &userA,
		}
		err := repo.Delete(ctx, ghost)
		assert.ErrorIs(t, err, apperr.ErrNotFound)
	})
}
