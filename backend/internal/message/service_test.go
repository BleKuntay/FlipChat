package message_test

import (
	"context"
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

// ── helpers ───────────────────────────────────────────────────────────────────

func newTestService(repo *mockRepo, convs *mockConvStore, blks *mockBlockChecker, opts ...message.Option) *message.Service {
	return message.NewService(repo, convs, blks, opts...)
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
			m.CreatedAt == fixedTime
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
	assert.ErrorIs(t, err, apperr.ErrForbidden)
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
