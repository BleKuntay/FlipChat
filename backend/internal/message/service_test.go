package message_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/BleKuntay/FlipChat/backend/internal/message"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ── mocks ─────────────────────────────────────────────────────────────────────

type mockRepo struct{ mock.Mock }

func (m *mockRepo) MarkAsRead(ctx context.Context, msg *message.Message) error {
	return m.Called(ctx, msg).Error(0)
}

func (m *mockRepo) Create(ctx context.Context, msg *message.Message) error {
	return m.Called(ctx, msg).Error(0)
}

func (m *mockRepo) GetByID(ctx context.Context, messageID string) (*message.Message, error) {
	args := m.Called(ctx, messageID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*message.Message), args.Error(1)
}

func (m *mockRepo) ListByConversation(ctx context.Context, conversationID, cursor string, limit int) ([]*message.Message, error) {
	args := m.Called(ctx, conversationID, cursor, limit)
	return args.Get(0).([]*message.Message), args.Error(1)
}

func (m *mockRepo) EditMessage(ctx context.Context, msg *message.Message) error {
	return m.Called(ctx, msg).Error(0)
}

func (m *mockRepo) Delete(ctx context.Context, msg *message.Message) error {
	return m.Called(ctx, msg).Error(0)
}

type mockConvStore struct{ mock.Mock }

func (m *mockConvStore) GetParticipants(ctx context.Context, conversationID string) (string, string, error) {
	args := m.Called(ctx, conversationID)
	return args.String(0), args.String(1), args.Error(2)
}

type mockBlockChecker struct{ mock.Mock }

func (m *mockBlockChecker) IsBlockedEitherWay(ctx context.Context, a, b string) (bool, error) {
	args := m.Called(ctx, a, b)
	return args.Bool(0), args.Error(1)
}

type mockPublisher struct{ mock.Mock }

func (m *mockPublisher) FanOutToConversation(ctx context.Context, senderID, recipientID string, eventType string, payload any) {
	m.Called(ctx, senderID, recipientID, eventType, payload)
}

type mockObjectDeleter struct{ mock.Mock }

func (m *mockObjectDeleter) DeleteObject(ctx context.Context, objectKey string) error {
	return m.Called(ctx, objectKey).Error(0)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newTestService(repo *mockRepo, convs *mockConvStore, blks *mockBlockChecker, opts ...message.Option) *message.Service {
	publisher := new(mockPublisher)
	publisher.On("FanOutToConversation", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	return message.NewService(repo, convs, blks, publisher, opts...)
}

func strPtr(s string) *string { return &s }

func stubMsg(id, convoID, senderID string) *message.Message {
	return &message.Message{
		ID:             id,
		ConversationID: convoID,
		SenderID:       senderID,
		Content:        strPtr("hello"),
		CreatedAt:      time.Now(),
	}
}

func newTestServiceWithObjects(repo *mockRepo, convs *mockConvStore, blks *mockBlockChecker, objects *mockObjectDeleter, opts ...message.Option) *message.Service {
	publisher := new(mockPublisher)
	publisher.On("FanOutToConversation", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	return message.NewService(repo, convs, blks, publisher, append(opts, message.WithObjectDeleter(objects))...)
}

func int64Ptr(n int64) *int64 { return &n }

func makeRawMetadata(objectKey string) *json.RawMessage {
	b, _ := json.Marshal(map[string]any{
		"attachment_id": "att-123",
		"object_key":    objectKey,
		"filename":      "photo.jpg",
		"mime_type":     "image/jpeg",
		"size":          int64(1024),
		"uploader_id":   "user-high",
	})
	raw := json.RawMessage(b)
	return &raw
}

// ── SendMessage ───────────────────────────────────────────────────────────────

func TestService_SendMessage_HappyPath(t *testing.T) {
	ctx := context.Background()

	const (
		senderID = "user-high"
		otherID  = "user-low"
		convoID  = "conv-1"
		content  = "halo!"
	)

	fixedID := "msg-abc"
	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	repo := new(mockRepo)
	convs := new(mockConvStore)
	blks := new(mockBlockChecker)

	convs.On("GetParticipants", ctx, convoID).Return(otherID, senderID, nil)
	blks.On("IsBlockedEitherWay", ctx, senderID, otherID).Return(false, nil)
	repo.On("Create", ctx, mock.MatchedBy(func(m *message.Message) bool {
		return m.ConversationID == convoID &&
			m.SenderID == senderID &&
			m.Content != nil && *m.Content == content &&
			m.ReplyToID == nil &&
			m.ID == fixedID &&
			m.CreatedAt.Equal(fixedTime)
	})).Return(nil)

	svc := newTestService(repo, convs, blks,
		message.WithIDGen(func() string { return fixedID }),
		message.WithClock(func() time.Time { return fixedTime }),
	)

	resp, err := svc.SendMessage(ctx, senderID, convoID, message.SendRequest{Content: content})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, fixedID, resp.ID)
	assert.Equal(t, convoID, resp.ConversationID)
	assert.Equal(t, senderID, resp.SenderID)
	assert.Equal(t, content, *resp.Content)
	assert.Nil(t, resp.ReplyToID)
	assert.Equal(t, fixedTime, resp.CreatedAt)
	assert.False(t, resp.IsEdited)

	repo.AssertExpectations(t)
	convs.AssertExpectations(t)
	blks.AssertExpectations(t)
}

func TestService_SendMessage_NonParticipant_ReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	svc := newTestService(repo, convs, blks)

	resp, err := svc.SendMessage(ctx, "outsider", "conv-1", message.SendRequest{Content: "halo!"})

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, apperr.ErrNotFound)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestService_SendMessage_Blocked_ReturnsForbidden(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	blks.On("IsBlockedEitherWay", ctx, "user-high", "user-low").Return(true, nil)
	svc := newTestService(repo, convs, blks)

	resp, err := svc.SendMessage(ctx, "user-high", "conv-1", message.SendRequest{Content: "halo!"})

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, apperr.ErrNotFound)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestService_SendMessage_EmptyContent_ReturnsBadRequest(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	blks.On("IsBlockedEitherWay", ctx, "user-high", "user-low").Return(false, nil)
	svc := newTestService(repo, convs, blks)

	for _, content := range []string{"", "   ", "\t", "\n"} {
		resp, err := svc.SendMessage(ctx, "user-high", "conv-1", message.SendRequest{Content: content})
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, apperr.ErrBadRequest)
	}
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestService_SendMessage_ReplyToOtherConversation_ReturnsForbidden(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	blks.On("IsBlockedEitherWay", ctx, "user-high", "user-low").Return(false, nil)
	repo.On("GetByID", ctx, "msg-other").Return(&message.Message{ID: "msg-other", ConversationID: "conv-2"}, nil)
	svc := newTestService(repo, convs, blks)

	replyTo := "msg-other"
	resp, err := svc.SendMessage(ctx, "user-high", "conv-1", message.SendRequest{Content: "halo!", ReplyToID: &replyTo})

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, apperr.ErrForbidden)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestService_SendMessage_ReplyToNonExistent_ReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	blks.On("IsBlockedEitherWay", ctx, "user-high", "user-low").Return(false, nil)
	repo.On("GetByID", ctx, "ghost").Return(nil, apperr.ErrNotFound)
	svc := newTestService(repo, convs, blks)

	replyTo := "ghost"
	resp, err := svc.SendMessage(ctx, "user-high", "conv-1", message.SendRequest{Content: "halo!", ReplyToID: &replyTo})

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestService_SendMessage_RepoCreateFails(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	dbErr := errors.New("connection refused")
	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	blks.On("IsBlockedEitherWay", ctx, "user-high", "user-low").Return(false, nil)
	repo.On("Create", ctx, mock.AnythingOfType("*message.Message")).Return(dbErr)
	svc := newTestService(repo, convs, blks)

	resp, err := svc.SendMessage(ctx, "user-high", "conv-1", message.SendRequest{Content: "halo!"})

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, dbErr)
}

// ── ListMessages ──────────────────────────────────────────────────────────────

func TestService_ListMessages_HappyPath(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	msgs := []*message.Message{
		{ID: "msg-3", ConversationID: "conv-1", SenderID: "user-high"},
		{ID: "msg-2", ConversationID: "conv-1", SenderID: "user-low"},
		{ID: "msg-1", ConversationID: "conv-1", SenderID: "user-high"},
	}
	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	repo.On("ListByConversation", ctx, "conv-1", "", message.DefaultLimit+1).Return(msgs, nil)
	svc := newTestService(repo, convs, blks)

	resp, err := svc.ListMessages(ctx, "user-high", "conv-1", message.ListQuery{})

	require.NoError(t, err)
	assert.Len(t, resp.Data, 3)
	assert.False(t, resp.HasMore)
	assert.Nil(t, resp.NextCursor)
}

func TestService_ListMessages_NonParticipant_ReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	svc := newTestService(repo, convs, blks)

	resp, err := svc.ListMessages(ctx, "outsider", "conv-1", message.ListQuery{})

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, apperr.ErrNotFound)
	repo.AssertNotCalled(t, "ListByConversation", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestService_ListMessages_HasMore_ReturnsCursor(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	msgs := []*message.Message{
		{ID: "msg-4", ConversationID: "conv-1"},
		{ID: "msg-3", ConversationID: "conv-1"},
		{ID: "msg-2", ConversationID: "conv-1"},
		{ID: "msg-1", ConversationID: "conv-1"}, // extra row
	}
	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	repo.On("ListByConversation", ctx, "conv-1", "", 4).Return(msgs, nil)
	svc := newTestService(repo, convs, blks)

	resp, err := svc.ListMessages(ctx, "user-high", "conv-1", message.ListQuery{Limit: 3})

	require.NoError(t, err)
	assert.Len(t, resp.Data, 3)
	assert.True(t, resp.HasMore)
	require.NotNil(t, resp.NextCursor)
	assert.Equal(t, "msg-2", *resp.NextCursor)
}

func TestService_ListMessages_ZeroLimit_UsesDefault(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	repo.On("ListByConversation", ctx, "conv-1", "", message.DefaultLimit+1).Return([]*message.Message{}, nil)
	svc := newTestService(repo, convs, blks)

	resp, err := svc.ListMessages(ctx, "user-high", "conv-1", message.ListQuery{Limit: 0})

	require.NoError(t, err)
	assert.Empty(t, resp.Data)
	repo.AssertExpectations(t)
}

// ── EditMessage ───────────────────────────────────────────────────────────────

func TestService_EditMessage_HappyPath(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)

	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	msg := stubMsg("msg-1", "conv-1", "user-high")
	msg.CreatedAt = fixedTime.Add(-5 * time.Minute) // within edit window

	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	repo.On("GetByID", ctx, "msg-1").Return(msg, nil)
	repo.On("EditMessage", ctx, mock.MatchedBy(func(m *message.Message) bool {
		return m.ID == "msg-1" && m.IsEdited && m.Content != nil && *m.Content == "updated"
	})).Return(nil)

	svc := newTestService(repo, convs, blks, message.WithClock(func() time.Time { return fixedTime }))

	resp, err := svc.EditMessage(ctx, "user-high", "conv-1", "msg-1", message.EditRequest{Content: "updated"})

	require.NoError(t, err)
	assert.True(t, resp.IsEdited)
	assert.Equal(t, "updated", *resp.Content)
	repo.AssertExpectations(t)
}

func TestService_EditMessage_NonParticipant_ReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	svc := newTestService(repo, convs, blks)

	resp, err := svc.EditMessage(ctx, "outsider", "conv-1", "msg-1", message.EditRequest{Content: "x"})

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, apperr.ErrNotFound)
	repo.AssertNotCalled(t, "GetByID", mock.Anything, mock.Anything)
}

func TestService_EditMessage_NotSender_ReturnsForbidden(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	msg := stubMsg("msg-1", "conv-1", "user-high") // sender is user-high
	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	repo.On("GetByID", ctx, "msg-1").Return(msg, nil)
	svc := newTestService(repo, convs, blks)

	// user-low tries to edit user-high's message
	resp, err := svc.EditMessage(ctx, "user-low", "conv-1", "msg-1", message.EditRequest{Content: "x"})

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, apperr.ErrForbidden)
	repo.AssertNotCalled(t, "EditMessage", mock.Anything, mock.Anything)
}

func TestService_EditMessage_WrongConversation_ReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	msg := stubMsg("msg-1", "conv-other", "user-high") // belongs to different conversation
	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	repo.On("GetByID", ctx, "msg-1").Return(msg, nil)
	svc := newTestService(repo, convs, blks)

	resp, err := svc.EditMessage(ctx, "user-high", "conv-1", "msg-1", message.EditRequest{Content: "x"})

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestService_EditMessage_AlreadyDeleted_ReturnsForbidden(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	now := time.Now()
	msg := stubMsg("msg-1", "conv-1", "user-high")
	msg.DeletedAt = &now
	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	repo.On("GetByID", ctx, "msg-1").Return(msg, nil)
	svc := newTestService(repo, convs, blks)

	resp, err := svc.EditMessage(ctx, "user-high", "conv-1", "msg-1", message.EditRequest{Content: "x"})

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, apperr.ErrForbidden)
}

func TestService_EditMessage_OutsideWindow_ReturnsForbidden(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)

	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	msg := stubMsg("msg-1", "conv-1", "user-high")
	msg.CreatedAt = fixedTime.Add(-20 * time.Minute) // outside 15-minute window

	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	repo.On("GetByID", ctx, "msg-1").Return(msg, nil)
	svc := newTestService(repo, convs, blks, message.WithClock(func() time.Time { return fixedTime }))

	resp, err := svc.EditMessage(ctx, "user-high", "conv-1", "msg-1", message.EditRequest{Content: "x"})

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, apperr.ErrForbidden)
	repo.AssertNotCalled(t, "EditMessage", mock.Anything, mock.Anything)
}

// ── DeleteMessage ─────────────────────────────────────────────────────────────

func TestService_DeleteMessage_HappyPath(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	msg := stubMsg("msg-1", "conv-1", "user-high")

	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	repo.On("GetByID", ctx, "msg-1").Return(msg, nil)
	repo.On("Delete", ctx, mock.MatchedBy(func(m *message.Message) bool {
		return m.ID == "msg-1" &&
			m.Content == nil &&
			m.DeletedAt != nil &&
			m.DeletedBy != nil && *m.DeletedBy == "user-high"
	})).Return(nil)

	svc := newTestService(repo, convs, blks)
	resp, err := svc.DeleteMessage(ctx, "user-high", "conv-1", "msg-1")

	require.NoError(t, err)
	assert.True(t, resp.IsDeleted)
	assert.Nil(t, resp.Content)
	repo.AssertExpectations(t)
}

func TestService_DeleteMessage_NonParticipant_ReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	svc := newTestService(repo, convs, blks)

	resp, err := svc.DeleteMessage(ctx, "outsider", "conv-1", "msg-1")

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, apperr.ErrNotFound)
	repo.AssertNotCalled(t, "GetByID", mock.Anything, mock.Anything)
}

func TestService_DeleteMessage_NotSender_ReturnsForbidden(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	msg := stubMsg("msg-1", "conv-1", "user-high")
	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	repo.On("GetByID", ctx, "msg-1").Return(msg, nil)
	svc := newTestService(repo, convs, blks)

	resp, err := svc.DeleteMessage(ctx, "user-low", "conv-1", "msg-1")

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, apperr.ErrForbidden)
	repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}

func TestService_DeleteMessage_WrongConversation_ReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	msg := stubMsg("msg-1", "conv-other", "user-high")
	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	repo.On("GetByID", ctx, "msg-1").Return(msg, nil)
	svc := newTestService(repo, convs, blks)

	resp, err := svc.DeleteMessage(ctx, "user-high", "conv-1", "msg-1")

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestService_DeleteMessage_AlreadyDeleted_ReturnsConflict(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	now := time.Now()
	msg := stubMsg("msg-1", "conv-1", "user-high")
	msg.DeletedAt = &now
	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	repo.On("GetByID", ctx, "msg-1").Return(msg, nil)
	svc := newTestService(repo, convs, blks)

	resp, err := svc.DeleteMessage(ctx, "user-high", "conv-1", "msg-1")

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, apperr.ErrConflict)
	repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}

// ── SendMessage — attachment cases ───────────────────────────────────────────

func TestService_SendMessage_AttachmentOnly_NoContent(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)

	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	blks.On("IsBlockedEitherWay", ctx, "user-high", "user-low").Return(false, nil)
	repo.On("Create", ctx, mock.MatchedBy(func(m *message.Message) bool {
		return m.Content == nil && m.Metadata != nil
	})).Return(nil)

	svc := newTestService(repo, convs, blks)

	resp, err := svc.SendMessage(ctx, "user-high", "conv-1", message.SendRequest{
		AttachmentID: strPtr("att-abc"),
	})

	require.NoError(t, err)
	assert.Nil(t, resp.Content)
	assert.NotNil(t, resp.Metadata)
	repo.AssertExpectations(t)
}

func TestService_SendMessage_ContentAndAttachment(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)

	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	blks.On("IsBlockedEitherWay", ctx, "user-high", "user-low").Return(false, nil)
	repo.On("Create", ctx, mock.MatchedBy(func(m *message.Message) bool {
		return m.Content != nil && *m.Content == "check this out" && m.Metadata != nil
	})).Return(nil)

	svc := newTestService(repo, convs, blks)

	resp, err := svc.SendMessage(ctx, "user-high", "conv-1", message.SendRequest{
		Content:      "check this out",
		AttachmentID: strPtr("att-abc"),
	})

	require.NoError(t, err)
	assert.Equal(t, "check this out", *resp.Content)
	assert.NotNil(t, resp.Metadata)
	repo.AssertExpectations(t)
}

func TestService_SendMessage_NoContentNoAttachment_ReturnsBadRequest(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)

	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	blks.On("IsBlockedEitherWay", ctx, "user-high", "user-low").Return(false, nil)

	svc := newTestService(repo, convs, blks)

	resp, err := svc.SendMessage(ctx, "user-high", "conv-1", message.SendRequest{})

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, apperr.ErrBadRequest)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestService_SendMessage_WhitespaceContentWithAttachment_ContentNil(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)

	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	blks.On("IsBlockedEitherWay", ctx, "user-high", "user-low").Return(false, nil)
	repo.On("Create", ctx, mock.MatchedBy(func(m *message.Message) bool {
		return m.Content == nil && m.Metadata != nil
	})).Return(nil)

	svc := newTestService(repo, convs, blks)

	resp, err := svc.SendMessage(ctx, "user-high", "conv-1", message.SendRequest{
		Content:      "   ",
		AttachmentID: strPtr("att-123"),
	})

	require.NoError(t, err)
	assert.Nil(t, resp.Content)
	assert.NotNil(t, resp.Metadata)
	repo.AssertExpectations(t)
}

func TestService_SendMessage_AttachmentMetadata_FieldsCorrect(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)

	const (
		senderID = "user-high"
		otherID  = "user-low"
		convoID  = "conv-1"
		attID    = "att-xyz"
	)

	convs.On("GetParticipants", ctx, convoID).Return(otherID, senderID, nil)
	blks.On("IsBlockedEitherWay", ctx, senderID, otherID).Return(false, nil)
	repo.On("Create", ctx, mock.MatchedBy(func(m *message.Message) bool {
		if m.Metadata == nil {
			return false
		}
		var meta map[string]any
		if err := json.Unmarshal(*m.Metadata, &meta); err != nil {
			return false
		}
		return meta["attachment_id"] == attID &&
			meta["object_key"] == "attachments/"+attID &&
			meta["filename"] == "photo.jpg" &&
			meta["mime_type"] == "image/jpeg" &&
			meta["uploader_id"] == senderID
	})).Return(nil)

	svc := newTestService(repo, convs, blks)

	resp, err := svc.SendMessage(ctx, senderID, convoID, message.SendRequest{
		AttachmentID: strPtr(attID),
		Filename:     strPtr("photo.jpg"),
		MIMEType:     strPtr("image/jpeg"),
		Size:         int64Ptr(2048),
	})

	require.NoError(t, err)
	assert.NotNil(t, resp.Metadata)
	repo.AssertExpectations(t)
}

func TestService_SendMessage_AttachmentNilOptionalFields_NoNilPanic(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)

	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	blks.On("IsBlockedEitherWay", ctx, "user-high", "user-low").Return(false, nil)
	repo.On("Create", ctx, mock.MatchedBy(func(m *message.Message) bool {
		if m.Metadata == nil {
			return false
		}
		var meta map[string]any
		if err := json.Unmarshal(*m.Metadata, &meta); err != nil {
			return false
		}

		return meta["filename"] == "" &&
			meta["mime_type"] == "" &&
			meta["size"] == float64(0)
	})).Return(nil)

	svc := newTestService(repo, convs, blks)

	resp, err := svc.SendMessage(ctx, "user-high", "conv-1", message.SendRequest{
		AttachmentID: strPtr("att-xyz"),
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
	repo.AssertExpectations(t)
}

func TestService_SendMessage_ReplyToDeletedMessage_Succeeds(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)

	const replyID = "msg-deleted"
	now := time.Now()
	deletedMsg := stubMsg(replyID, "conv-1", "user-low")
	deletedMsg.DeletedAt = &now

	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	blks.On("IsBlockedEitherWay", ctx, "user-high", "user-low").Return(false, nil)
	repo.On("GetByID", ctx, replyID).Return(deletedMsg, nil)
	repo.On("Create", ctx, mock.MatchedBy(func(m *message.Message) bool {
		return m.ReplyToID != nil && *m.ReplyToID == replyID
	})).Return(nil)

	svc := newTestService(repo, convs, blks)

	resp, err := svc.SendMessage(ctx, "user-high", "conv-1", message.SendRequest{
		Content:   "quoting a deleted message",
		ReplyToID: strPtr(replyID),
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
	repo.AssertExpectations(t)
}

// ── DeleteMessage — kasus attachment + MinIO cleanup ─────────────────────────

func TestService_DeleteMessage_WithAttachment_DeleteObjectCalled(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	objects := new(mockObjectDeleter)

	const objectKey = "attachments/att-123"

	msg := stubMsg("msg-1", "conv-1", "user-high")
	msg.Metadata = makeRawMetadata(objectKey)

	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	repo.On("GetByID", ctx, "msg-1").Return(msg, nil)
	objects.On("DeleteObject", ctx, objectKey).Return(nil)
	repo.On("Delete", ctx, mock.Anything).Return(nil)

	svc := newTestServiceWithObjects(repo, convs, blks, objects)
	resp, err := svc.DeleteMessage(ctx, "user-high", "conv-1", "msg-1")

	require.NoError(t, err)
	assert.True(t, resp.IsDeleted)
	objects.AssertCalled(t, "DeleteObject", ctx, objectKey)
	repo.AssertExpectations(t)
}

func TestService_DeleteMessage_WithoutAttachment_DeleteObjectNotCalled(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	objects := new(mockObjectDeleter)

	// Pesan tanpa attachment (Metadata nil)
	msg := stubMsg("msg-1", "conv-1", "user-high")

	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	repo.On("GetByID", ctx, "msg-1").Return(msg, nil)
	repo.On("Delete", ctx, mock.Anything).Return(nil)

	svc := newTestServiceWithObjects(repo, convs, blks, objects)
	resp, err := svc.DeleteMessage(ctx, "user-high", "conv-1", "msg-1")

	require.NoError(t, err)
	assert.True(t, resp.IsDeleted)
	objects.AssertNotCalled(t, "DeleteObject", mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
}

func TestService_DeleteMessage_EmptyObjectKey_DeleteObjectNotCalled(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	objects := new(mockObjectDeleter)

	msg := stubMsg("msg-1", "conv-1", "user-high")
	msg.Metadata = makeRawMetadata("") // object_key kosong

	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	repo.On("GetByID", ctx, "msg-1").Return(msg, nil)
	repo.On("Delete", ctx, mock.Anything).Return(nil)

	svc := newTestServiceWithObjects(repo, convs, blks, objects)
	resp, err := svc.DeleteMessage(ctx, "user-high", "conv-1", "msg-1")

	require.NoError(t, err)
	assert.True(t, resp.IsDeleted)
	objects.AssertNotCalled(t, "DeleteObject", mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
}

func TestService_DeleteMessage_InvalidMetadataJSON_SkipsMinIODeleteSucceeds(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	objects := new(mockObjectDeleter)

	invalidJSON := json.RawMessage(`{invalid json}`)
	msg := stubMsg("msg-1", "conv-1", "user-high")
	msg.Metadata = &invalidJSON

	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	repo.On("GetByID", ctx, "msg-1").Return(msg, nil)
	repo.On("Delete", ctx, mock.Anything).Return(nil)

	svc := newTestServiceWithObjects(repo, convs, blks, objects)
	resp, err := svc.DeleteMessage(ctx, "user-high", "conv-1", "msg-1")

	// deleteAttachmentObject gracefully skip if JSON invalid
	require.NoError(t, err)
	assert.True(t, resp.IsDeleted)
	objects.AssertNotCalled(t, "DeleteObject", mock.Anything, mock.Anything)
	repo.AssertExpectations(t)
}

func TestService_DeleteMessage_DeleteObjectFails_ReturnsError(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)
	objects := new(mockObjectDeleter)

	const objectKey = "attachments/att-123"

	msg := stubMsg("msg-1", "conv-1", "user-high")
	msg.Metadata = makeRawMetadata(objectKey)

	storageErr := errors.New("MinIO unavailable")

	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	repo.On("GetByID", ctx, "msg-1").Return(msg, nil)
	objects.On("DeleteObject", ctx, objectKey).Return(storageErr)

	svc := newTestServiceWithObjects(repo, convs, blks, objects)
	resp, err := svc.DeleteMessage(ctx, "user-high", "conv-1", "msg-1")

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, storageErr)
	repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
	objects.AssertExpectations(t)
}

func TestService_DeleteMessage_ObjectsNilWithAttachment_NoPanic(t *testing.T) {
	ctx := context.Background()
	repo, convs, blks := new(mockRepo), new(mockConvStore), new(mockBlockChecker)

	msg := stubMsg("msg-1", "conv-1", "user-high")
	msg.Metadata = makeRawMetadata("attachments/att-123")

	convs.On("GetParticipants", ctx, "conv-1").Return("user-low", "user-high", nil)
	repo.On("GetByID", ctx, "msg-1").Return(msg, nil)
	repo.On("Delete", ctx, mock.Anything).Return(nil)

	// newTestService without WithObjectDeleter → s.objects == nil
	svc := newTestService(repo, convs, blks)
	resp, err := svc.DeleteMessage(ctx, "user-high", "conv-1", "msg-1")

	require.NoError(t, err)
	assert.True(t, resp.IsDeleted)
	repo.AssertExpectations(t)
}
