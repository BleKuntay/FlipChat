package conversation_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	. "github.com/BleKuntay/FlipChat/backend/internal/conversation"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
)

// ── mock repository ───────────────────────────────────────────────────────────

type mockRepo struct{ mock.Mock }

func (m *mockRepo) FindAllByUserID(ctx context.Context, userID string) ([]Response, error) {
	args := m.Called(ctx, userID)
	if r, ok := args.Get(0).([]Response); ok {
		return r, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockRepo) FindByID(ctx context.Context, requesterID, conversationID string) (*Response, error) {
	args := m.Called(ctx, requesterID, conversationID)
	if r, ok := args.Get(0).(*Response); ok {
		return r, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockRepo) FindByPair(ctx context.Context, lowID, highID string) (*Conversation, error) {
	args := m.Called(ctx, lowID, highID)
	if c, ok := args.Get(0).(*Conversation); ok {
		return c, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockRepo) Create(ctx context.Context, requesterID, lowID, highID string) (*Response, error) {
	args := m.Called(ctx, requesterID, lowID, highID)
	if r, ok := args.Get(0).(*Response); ok {
		return r, args.Error(1)
	}
	return nil, args.Error(1)
}

// ── mock block checker ────────────────────────────────────────────────────────

type mockBlockChecker struct{ mock.Mock }

func (m *mockBlockChecker) IsBlockedEitherWay(ctx context.Context, a, b string) (bool, error) {
	args := m.Called(ctx, a, b)
	return args.Bool(0), args.Error(1)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newTestService(repo RepositoryInterface, bc BlockChecker) *Service {
	return NewService(repo, bc)
}

func stubResponse(convID string) *Response {
	return &Response{
		ConversationID: convID,
		Name:           "Bob Tables",
		Username:       "bob",
	}
}

func stubConversation(id, lowID, highID string) *Conversation {
	return &Conversation{
		ID:         id,
		UserLowID:  lowID,
		UserHighID: highID,
	}
}

// ── GetConversationList ───────────────────────────────────────────────────────

func TestService_GetConversationList(t *testing.T) {
	ctx := context.Background()

	t.Run("returns list from repository", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)

		expected := []Response{*stubResponse("conv-1"), *stubResponse("conv-2")}
		repo.On("FindAllByUserID", mock.Anything, "user-123").Return(expected, nil)

		svc := newTestService(repo, bc)
		res, err := svc.GetConversationList(ctx, "user-123")

		require.NoError(t, err)
		assert.Len(t, res.Data, 2)
		repo.AssertExpectations(t)
	})

	t.Run("returns empty list when no conversations", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		repo.On("FindAllByUserID", mock.Anything, "user-123").Return([]Response{}, nil)

		svc := newTestService(repo, bc)
		res, err := svc.GetConversationList(ctx, "user-123")

		require.NoError(t, err)
		assert.Empty(t, res.Data)
	})

	t.Run("propagates repository error", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		repo.On("FindAllByUserID", mock.Anything, "user-123").Return(nil, errors.New("db error"))

		svc := newTestService(repo, bc)
		_, err := svc.GetConversationList(ctx, "user-123")

		assert.Error(t, err)
	})
}

// ── GetConversation ───────────────────────────────────────────────────────────

func TestService_GetConversation(t *testing.T) {
	ctx := context.Background()

	t.Run("returns conversation for participant", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)

		expected := stubResponse("conv-1")
		repo.On("FindByID", mock.Anything, "user-123", "conv-1").Return(expected, nil)

		svc := newTestService(repo, bc)
		res, err := svc.GetConversation(ctx, "user-123", "conv-1")

		require.NoError(t, err)
		assert.Equal(t, "conv-1", res.ConversationID)
		repo.AssertExpectations(t)
	})

	t.Run("returns ErrNotFound for non-participant", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		repo.On("FindByID", mock.Anything, "outsider", "conv-1").Return(nil, apperr.ErrNotFound)

		svc := newTestService(repo, bc)
		_, err := svc.GetConversation(ctx, "outsider", "conv-1")

		assert.ErrorIs(t, err, apperr.ErrNotFound)
	})

	t.Run("propagates repository error", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		dbErr := errors.New("db error")
		repo.On("FindByID", mock.Anything, "user-123", "conv-1").Return(nil, dbErr)

		svc := newTestService(repo, bc)
		_, err := svc.GetConversation(ctx, "user-123", "conv-1")

		assert.ErrorIs(t, err, dbErr)
	})
}

// ── CreateConversation ────────────────────────────────────────────────────────

func TestService_CreateConversation(t *testing.T) {
	ctx := context.Background()

	// these IDs need canonical ordering to be predictable in assertions
	// use fixed values, so we know which is low/high
	const (
		userA = "aaaaaaaa-0000-0000-0000-000000000001"
		userB = "bbbbbbbb-0000-0000-0000-000000000002"
		// userA < userB lexicographically, so low=userA, high=userB
	)

	t.Run("self-conversation returns ErrBadRequest", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)

		svc := newTestService(repo, bc)
		_, _, err := svc.CreateConversation(ctx, userA, userA)

		assert.ErrorIs(t, err, apperr.ErrBadRequest)
		repo.AssertNotCalled(t, "FindByPair")
	})

	t.Run("returns existing conversation with created=false", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)

		existing := stubConversation("conv-existing", userA, userB)
		existingResp := stubResponse("conv-existing")

		repo.On("FindByPair", mock.Anything, userA, userB).Return(existing, nil)
		repo.On("FindByID", mock.Anything, userA, "conv-existing").Return(existingResp, nil)

		svc := newTestService(repo, bc)
		res, created, err := svc.CreateConversation(ctx, userA, userB)

		require.NoError(t, err)
		assert.False(t, created)
		assert.Equal(t, "conv-existing", res.ConversationID)
		bc.AssertNotCalled(t, "IsBlockedEitherWay")
		repo.AssertNotCalled(t, "Create")
	})

	t.Run("creates new conversation when none exists and not blocked", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)

		newResp := stubResponse("conv-new")

		repo.On("FindByPair", mock.Anything, userA, userB).Return(nil, nil)
		bc.On("IsBlockedEitherWay", mock.Anything, userA, userB).Return(false, nil)
		repo.On("Create", mock.Anything, userA, userA, userB).Return(newResp, nil)

		svc := newTestService(repo, bc)
		res, created, err := svc.CreateConversation(ctx, userA, userB)

		require.NoError(t, err)
		assert.True(t, created)
		assert.Equal(t, "conv-new", res.ConversationID)
		repo.AssertExpectations(t)
		bc.AssertExpectations(t)
	})

	t.Run("returns ErrNotFound when blocked", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)

		repo.On("FindByPair", mock.Anything, userA, userB).Return(nil, nil)
		bc.On("IsBlockedEitherWay", mock.Anything, userA, userB).Return(true, nil)

		svc := newTestService(repo, bc)
		_, _, err := svc.CreateConversation(ctx, userA, userB)

		assert.ErrorIs(t, err, apperr.ErrNotFound)
		repo.AssertNotCalled(t, "Create")
	})

	t.Run("propagates FindByPair error", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		dbErr := errors.New("db error")

		repo.On("FindByPair", mock.Anything, userA, userB).Return(nil, dbErr)

		svc := newTestService(repo, bc)
		_, _, err := svc.CreateConversation(ctx, userA, userB)

		assert.ErrorIs(t, err, dbErr)
		bc.AssertNotCalled(t, "IsBlockedEitherWay")
	})

	t.Run("propagates block checker error", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		dbErr := errors.New("db error")

		repo.On("FindByPair", mock.Anything, userA, userB).Return(nil, nil)
		bc.On("IsBlockedEitherWay", mock.Anything, userA, userB).Return(false, dbErr)

		svc := newTestService(repo, bc)
		_, _, err := svc.CreateConversation(ctx, userA, userB)

		assert.ErrorIs(t, err, dbErr)
		repo.AssertNotCalled(t, "Create")
	})

	t.Run("propagates Create error", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		dbErr := errors.New("db error")

		repo.On("FindByPair", mock.Anything, userA, userB).Return(nil, nil)
		bc.On("IsBlockedEitherWay", mock.Anything, userA, userB).Return(false, nil)
		repo.On("Create", mock.Anything, userA, userA, userB).Return(nil, dbErr)

		svc := newTestService(repo, bc)
		_, _, err := svc.CreateConversation(ctx, userA, userB)

		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("canonical order enforced — B initiates but low/high same as A initiating", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)

		newResp := stubResponse("conv-new")

		// B initiates — low/high should still be (userA, userB)
		repo.On("FindByPair", mock.Anything, userA, userB).Return(nil, nil)
		bc.On("IsBlockedEitherWay", mock.Anything, userA, userB).Return(false, nil)
		repo.On("Create", mock.Anything, userB, userA, userB).Return(newResp, nil)

		svc := newTestService(repo, bc)
		_, created, err := svc.CreateConversation(ctx, userB, userA)

		require.NoError(t, err)
		assert.True(t, created)
		repo.AssertExpectations(t)
	})
}
