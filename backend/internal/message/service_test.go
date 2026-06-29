// internal/message/service_test.go
package message_test

import (
	"context"
	"errors"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"testing"
	"time"

	"github.com/BleKuntay/FlipChat/backend/internal/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- mocks ---

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

// --- helper ---

func newTestService(
	repo *mockRepo,
	convs *mockConvStore,
	blks *mockBlockChecker,
	opts ...message.Option,
) *message.Service {
	return message.NewService(repo, convs, blks, opts...)
}

// --- Send tests ---

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

	const (
		senderID   = "user-high"
		otherID    = "user-low"
		outsiderID = "user-outsider"
		convoID    = "conv-1"
	)

	repo := new(mockRepo)
	convs := new(mockConvStore)
	blks := new(mockBlockChecker)

	convs.On("GetParticipants", ctx, convoID).Return(otherID, senderID, nil)

	svc := newTestService(repo, convs, blks)

	resp, err := svc.SendMessage(ctx, outsiderID, convoID, message.SendRequest{Content: "halo!"})

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, apperr.ErrNotFound)

	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	blks.AssertNotCalled(t, "IsBlockedEitherWay", mock.Anything, mock.Anything, mock.Anything)
}

func TestService_SendMessage_BlockerSends_ReturnsForbidden(t *testing.T) {
	ctx := context.Background()

	const (
		blockerID = "user-high"
		blockedID = "user-low"
		convoID   = "conv-1"
	)

	repo := new(mockRepo)
	convs := new(mockConvStore)
	blks := new(mockBlockChecker)

	convs.On("GetParticipants", ctx, convoID).Return(blockedID, blockerID, nil)
	blks.On("IsBlockedEitherWay", ctx, blockerID, blockedID).Return(true, nil)

	svc := newTestService(repo, convs, blks)

	resp, err := svc.SendMessage(ctx, blockerID, convoID, message.SendRequest{Content: "halo!"})

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, apperr.ErrForbidden)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestService_SendMessage_BlockedSends_ReturnsForbidden(t *testing.T) {
	ctx := context.Background()

	const (
		blockerID = "user-high"
		blockedID = "user-low"
		convoID   = "conv-1"
	)

	repo := new(mockRepo)
	convs := new(mockConvStore)
	blks := new(mockBlockChecker)

	convs.On("GetParticipants", ctx, convoID).Return(blockedID, blockerID, nil)
	blks.On("IsBlockedEitherWay", ctx, blockedID, blockerID).Return(true, nil)

	svc := newTestService(repo, convs, blks)

	resp, err := svc.SendMessage(ctx, blockedID, convoID, message.SendRequest{Content: "halo!"})

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, apperr.ErrForbidden)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestService_SendMessage_EmptyContent_ReturnsBadRequest(t *testing.T) {
	ctx := context.Background()

	const (
		senderID = "user-high"
		otherID  = "user-low"
		convoID  = "conv-1"
	)

	repo := new(mockRepo)
	convs := new(mockConvStore)
	blks := new(mockBlockChecker)

	convs.On("GetParticipants", ctx, convoID).Return(otherID, senderID, nil)
	blks.On("IsBlockedEitherWay", ctx, senderID, otherID).Return(false, nil)

	svc := newTestService(repo, convs, blks)

	cases := []struct {
		name    string
		content string
	}{
		{"empty string", ""},
		{"whitespace only", "   "},
		{"tab only", "\t"},
		{"newline only", "\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := svc.SendMessage(ctx, senderID, convoID, message.SendRequest{Content: tc.content})
			assert.Nil(t, resp)
			assert.ErrorIs(t, err, apperr.ErrBadRequest)
		})
	}

	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestService_SendMessage_ReplyToMessageInOtherConversation_ReturnsForbidden(t *testing.T) {
	ctx := context.Background()

	const (
		senderID     = "user-high"
		otherID      = "user-low"
		convoID      = "conv-1"
		otherConvoID = "conv-2"
		replyToID    = "msg-in-other-conv"
	)

	repo := new(mockRepo)
	convs := new(mockConvStore)
	blks := new(mockBlockChecker)

	convs.On("GetParticipants", ctx, convoID).Return(otherID, senderID, nil)
	blks.On("IsBlockedEitherWay", ctx, senderID, otherID).Return(false, nil)

	repo.On("GetByID", ctx, replyToID).Return(&message.Message{
		ID:             replyToID,
		ConversationID: otherConvoID,
	}, nil)

	replyTo := replyToID
	svc := newTestService(repo, convs, blks)

	resp, err := svc.SendMessage(ctx, senderID, convoID, message.SendRequest{
		Content:   "halo!",
		ReplyToID: &replyTo,
	})

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, apperr.ErrForbidden)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestService_SendMessage_ReplyToNonExistentMessage_ReturnsNotFound(t *testing.T) {
	ctx := context.Background()

	const (
		senderID  = "user-high"
		otherID   = "user-low"
		convoID   = "conv-1"
		replyToID = "msg-ghost"
	)

	repo := new(mockRepo)
	convs := new(mockConvStore)
	blks := new(mockBlockChecker)

	convs.On("GetParticipants", ctx, convoID).Return(otherID, senderID, nil)
	blks.On("IsBlockedEitherWay", ctx, senderID, otherID).Return(false, nil)
	repo.On("GetByID", ctx, replyToID).Return(nil, apperr.ErrNotFound)

	replyTo := replyToID
	svc := newTestService(repo, convs, blks)

	resp, err := svc.SendMessage(ctx, senderID, convoID, message.SendRequest{
		Content:   "halo!",
		ReplyToID: &replyTo,
	})

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, apperr.ErrNotFound)
	repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestService_SendMessage_RepoCreateFails_ReturnsError(t *testing.T) {
	ctx := context.Background()

	const (
		senderID = "user-high"
		otherID  = "user-low"
		convoID  = "conv-1"
	)

	repo := new(mockRepo)
	convs := new(mockConvStore)
	blks := new(mockBlockChecker)

	dbErr := errors.New("connection refused")

	convs.On("GetParticipants", ctx, convoID).Return(otherID, senderID, nil)
	blks.On("IsBlockedEitherWay", ctx, senderID, otherID).Return(false, nil)
	repo.On("Create", ctx, mock.AnythingOfType("*message.Message")).Return(dbErr)

	svc := newTestService(repo, convs, blks)

	resp, err := svc.SendMessage(ctx, senderID, convoID, message.SendRequest{Content: "halo!"})

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, dbErr)

	repo.AssertExpectations(t)
}

// --- list message test ---

func TestService_ListMessages_HappyPath_NoCursor(t *testing.T) {
	ctx := context.Background()

	const (
		userID  = "user-high"
		otherID = "user-low"
		convoID = "conv-1"
	)

	repo := new(mockRepo)
	convs := new(mockConvStore)
	blks := new(mockBlockChecker)

	msgs := []*message.Message{
		{ID: "msg-3", ConversationID: convoID, SenderID: userID},
		{ID: "msg-2", ConversationID: convoID, SenderID: otherID},
		{ID: "msg-1", ConversationID: convoID, SenderID: userID},
	}

	convs.On("GetParticipants", ctx, convoID).Return(otherID, userID, nil)
	// limit+1 = defaultLimit+1 = 51
	repo.On("ListByConversation", ctx, convoID, "", message.DefaultLimit+1).Return(msgs, nil)

	svc := newTestService(repo, convs, blks)

	resp, err := svc.ListMessages(ctx, userID, convoID, message.ListQuery{})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Data, 3)
	assert.False(t, resp.HasMore)
	assert.Nil(t, resp.NextCursor)
	assert.Equal(t, "msg-3", resp.Data[0].ID)
	assert.Equal(t, "msg-1", resp.Data[2].ID)
}

func TestService_ListMessages_NonParticipant_ReturnsNotFound(t *testing.T) {
	ctx := context.Background()

	const (
		userID     = "user-high"
		otherID    = "user-low"
		outsiderID = "user-outsider"
		convoID    = "conv-1"
	)

	repo := new(mockRepo)
	convs := new(mockConvStore)
	blks := new(mockBlockChecker)

	convs.On("GetParticipants", ctx, convoID).Return(otherID, userID, nil)

	svc := newTestService(repo, convs, blks)

	resp, err := svc.ListMessages(ctx, outsiderID, convoID, message.ListQuery{})

	assert.Nil(t, resp)
	assert.ErrorIs(t, err, apperr.ErrNotFound)
	repo.AssertNotCalled(t, "ListByConversation", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestService_ListMessages_ZeroLimit_UsesDefaultLimit(t *testing.T) {
	ctx := context.Background()

	const (
		userID  = "user-high"
		otherID = "user-low"
		convoID = "conv-1"
	)

	repo := new(mockRepo)
	convs := new(mockConvStore)
	blks := new(mockBlockChecker)

	convs.On("GetParticipants", ctx, convoID).Return(otherID, userID, nil)
	repo.On("ListByConversation", ctx, convoID, "", message.DefaultLimit+1).Return([]*message.Message{}, nil)

	svc := newTestService(repo, convs, blks)

	resp, err := svc.ListMessages(ctx, userID, convoID, message.ListQuery{Limit: 0})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Data)
	assert.False(t, resp.HasMore)

	repo.AssertExpectations(t)
}

func TestService_ListMessages_HasMore_ReturnsCursor(t *testing.T) {
	ctx := context.Background()

	const (
		userID  = "user-high"
		otherID = "user-low"
		convoID = "conv-1"
		limit   = 3
	)

	repo := new(mockRepo)
	convs := new(mockConvStore)
	blks := new(mockBlockChecker)

	msgs := []*message.Message{
		{ID: "msg-4", ConversationID: convoID, SenderID: userID},
		{ID: "msg-3", ConversationID: convoID, SenderID: otherID},
		{ID: "msg-2", ConversationID: convoID, SenderID: userID},
		{ID: "msg-1", ConversationID: convoID, SenderID: otherID}, // row 4 must be discarded
	}

	convs.On("GetParticipants", ctx, convoID).Return(otherID, userID, nil)
	repo.On("ListByConversation", ctx, convoID, "", limit+1).Return(msgs, nil)

	svc := newTestService(repo, convs, blks)

	resp, err := svc.ListMessages(ctx, userID, convoID, message.ListQuery{Limit: limit})

	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Len(t, resp.Data, limit)
	assert.True(t, resp.HasMore)
	require.NotNil(t, resp.NextCursor)

	assert.Equal(t, "msg-2", *resp.NextCursor)

	ids := make([]string, 0, len(resp.Data))
	for _, m := range resp.Data {
		ids = append(ids, m.ID)
	}
	assert.NotContains(t, ids, "msg-1")

	repo.AssertExpectations(t)
}

func TestService_ListMessages_LessThanLimit_NoNextCursor(t *testing.T) {
	ctx := context.Background()

	const (
		userID  = "user-high"
		otherID = "user-low"
		convoID = "conv-1"
		limit   = 10
	)

	repo := new(mockRepo)
	convs := new(mockConvStore)
	blks := new(mockBlockChecker)

	msgs := []*message.Message{
		{ID: "msg-3", ConversationID: convoID, SenderID: userID},
		{ID: "msg-2", ConversationID: convoID, SenderID: otherID},
		{ID: "msg-1", ConversationID: convoID, SenderID: userID},
	}

	convs.On("GetParticipants", ctx, convoID).Return(otherID, userID, nil)
	repo.On("ListByConversation", ctx, convoID, "", limit+1).Return(msgs, nil)

	svc := newTestService(repo, convs, blks)

	resp, err := svc.ListMessages(ctx, userID, convoID, message.ListQuery{Limit: limit})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Data, 3)
	assert.False(t, resp.HasMore)
	assert.Nil(t, resp.NextCursor)

	repo.AssertExpectations(t)
}
