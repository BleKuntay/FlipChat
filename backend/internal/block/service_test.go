package block_test

import (
	"context"
	"errors"
	"testing"

	"github.com/BleKuntay/FlipChat/backend/internal/block"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// ------------------------------------------------------------------ //
// Mock                                                                 //
// ------------------------------------------------------------------ //

type mockRepository struct {
	mock.Mock
}

func (m *mockRepository) BlockUserAtomic(ctx context.Context, request block.Request, low, high string) (*block.Response, error) {
	args := m.Called(ctx, request, low, high)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*block.Response), args.Error(1)
}

func (m *mockRepository) UnblockUser(ctx context.Context, request block.Request) error {
	return m.Called(ctx, request).Error(0)
}

func (m *mockRepository) GetBlockList(ctx context.Context, blockerID string, query block.ListQuery) ([]block.BlockedSummary, error) {
	args := m.Called(ctx, blockerID, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]block.BlockedSummary), args.Error(1)
}

func (m *mockRepository) IsBlockedEitherWay(ctx context.Context, a, b string) (bool, error) {
	args := m.Called(ctx, a, b)
	return args.Bool(0), args.Error(1)
}

// ------------------------------------------------------------------ //
// Helper                                                               //
// ------------------------------------------------------------------ //

func newService(repo *mockRepository) *block.Service {
	return block.NewService(repo)
}

// ------------------------------------------------------------------ //
// BlockUser                                                            //
// ------------------------------------------------------------------ //

func TestBlockUser(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully block another user", func(t *testing.T) {
		repo := new(mockRepository)
		svc := newService(repo)

		req := block.Request{BlockerID: "aaa", BlockedID: "bbb"}
		want := &block.Response{BlockerID: "aaa", BlockedID: "bbb"}

		repo.On("BlockUserAtomic", ctx, req, "aaa", "bbb").Return(want, nil)

		got, err := svc.BlockUser(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, want, got)
		repo.AssertExpectations(t)
	})

	t.Run("canonical ensures low < high", func(t *testing.T) {
		repo := new(mockRepository)
		svc := newService(repo)

		req := block.Request{BlockerID: "zzz", BlockedID: "aaa"}
		want := &block.Response{BlockerID: "zzz", BlockedID: "aaa"}

		repo.On("BlockUserAtomic", ctx, req, "aaa", "zzz").Return(want, nil)

		got, err := svc.BlockUser(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, want, got)
		repo.AssertExpectations(t)
	})

	t.Run("self-block returns ErrBadRequest", func(t *testing.T) {
		repo := new(mockRepository)
		svc := newService(repo)

		req := block.Request{BlockerID: "aaa", BlockedID: "aaa"}

		got, err := svc.BlockUser(ctx, req)
		assert.Nil(t, got)
		assert.ErrorIs(t, err, apperr.ErrBadRequest)
		repo.AssertNotCalled(t, "BlockUserAtomic")
	})

	t.Run("repository error passed through", func(t *testing.T) {
		repo := new(mockRepository)
		svc := newService(repo)

		req := block.Request{BlockerID: "aaa", BlockedID: "bbb"}
		dbErr := errors.New("db error")

		repo.On("BlockUserAtomic", ctx, req, "aaa", "bbb").Return(nil, dbErr)

		got, err := svc.BlockUser(ctx, req)
		assert.Nil(t, got)
		assert.ErrorIs(t, err, dbErr)
	})
}

// ------------------------------------------------------------------ //
// UnblockUser                                                          //
// ------------------------------------------------------------------ //

func TestUnblockUser(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully unblock", func(t *testing.T) {
		repo := new(mockRepository)
		svc := newService(repo)

		req := block.Request{BlockerID: "aaa", BlockedID: "bbb"}
		repo.On("UnblockUser", ctx, req).Return(nil)

		err := svc.UnblockUser(ctx, req)
		assert.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("self-unblock returns ErrBadRequest", func(t *testing.T) {
		repo := new(mockRepository)
		svc := newService(repo)

		req := block.Request{BlockerID: "aaa", BlockedID: "aaa"}

		err := svc.UnblockUser(ctx, req)
		assert.ErrorIs(t, err, apperr.ErrBadRequest)
		repo.AssertNotCalled(t, "UnblockUser")
	})

	t.Run("repository error passed through", func(t *testing.T) {
		repo := new(mockRepository)
		svc := newService(repo)

		req := block.Request{BlockerID: "aaa", BlockedID: "bbb"}
		dbErr := errors.New("db error")
		repo.On("UnblockUser", ctx, req).Return(dbErr)

		err := svc.UnblockUser(ctx, req)
		assert.ErrorIs(t, err, dbErr)
	})
}

// ------------------------------------------------------------------ //
// GetBlockList                                                         //
// ------------------------------------------------------------------ //

func TestGetBlockList(t *testing.T) {
	ctx := context.Background()
	blockerID := "aaa"

	makeSummaries := func(n int) []block.BlockedSummary {
		out := make([]block.BlockedSummary, n)
		for i := range out {
			out[i] = block.BlockedSummary{UserID: "user" + string(rune('a'+i))}
		}
		return out
	}

	t.Run("default limit applied when not set", func(t *testing.T) {
		repo := new(mockRepository)
		svc := newService(repo)

		query := block.ListQuery{Limit: 0}
		repo.On("GetBlockList", ctx, blockerID, block.ListQuery{Limit: 20}).
			Return(makeSummaries(3), nil)

		resp, err := svc.GetBlockList(ctx, blockerID, query)
		assert.NoError(t, err)
		assert.Len(t, resp.Data, 3)
		assert.Empty(t, resp.NextCursor)
		repo.AssertExpectations(t)
	})

	t.Run("full page - has next cursor", func(t *testing.T) {
		repo := new(mockRepository)
		svc := newService(repo)

		query := block.ListQuery{Limit: 5}
		summaries := makeSummaries(6)
		repo.On("GetBlockList", ctx, blockerID, query).Return(summaries, nil)

		resp, err := svc.GetBlockList(ctx, blockerID, query)
		assert.NoError(t, err)
		assert.Len(t, resp.Data, 5)
		assert.Equal(t, summaries[4].UserID, resp.NextCursor)
	})

	t.Run("last page - no next cursor", func(t *testing.T) {
		repo := new(mockRepository)
		svc := newService(repo)

		query := block.ListQuery{Limit: 5}
		repo.On("GetBlockList", ctx, blockerID, query).Return(makeSummaries(5), nil)

		resp, err := svc.GetBlockList(ctx, blockerID, query)
		assert.NoError(t, err)
		assert.Len(t, resp.Data, 5)
		assert.Empty(t, resp.NextCursor)
	})

	t.Run("empty result", func(t *testing.T) {
		repo := new(mockRepository)
		svc := newService(repo)

		query := block.ListQuery{Limit: 5}
		repo.On("GetBlockList", ctx, blockerID, query).Return([]block.BlockedSummary{}, nil)

		resp, err := svc.GetBlockList(ctx, blockerID, query)
		assert.NoError(t, err)
		assert.Empty(t, resp.Data)
		assert.Empty(t, resp.NextCursor)
	})

	t.Run("repository error passed through", func(t *testing.T) {
		repo := new(mockRepository)
		svc := newService(repo)

		query := block.ListQuery{Limit: 5}
		dbErr := errors.New("db error")
		repo.On("GetBlockList", ctx, blockerID, query).Return(nil, dbErr)

		resp, err := svc.GetBlockList(ctx, blockerID, query)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, dbErr)
	})
}

// ------------------------------------------------------------------ //
// IsBlockedEitherWay                                                   //
// ------------------------------------------------------------------ //

func TestIsBlockedEitherWay(t *testing.T) {
	ctx := context.Background()

	t.Run("blocked - return true", func(t *testing.T) {
		repo := new(mockRepository)
		svc := newService(repo)

		repo.On("IsBlockedEitherWay", ctx, "aaa", "bbb").Return(true, nil)

		ok, err := svc.IsBlockedEitherWay(ctx, "aaa", "bbb")
		assert.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("not blocked - return false", func(t *testing.T) {
		repo := new(mockRepository)
		svc := newService(repo)

		repo.On("IsBlockedEitherWay", ctx, "aaa", "bbb").Return(false, nil)

		ok, err := svc.IsBlockedEitherWay(ctx, "aaa", "bbb")
		assert.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("repository error passed through", func(t *testing.T) {
		repo := new(mockRepository)
		svc := newService(repo)

		dbErr := errors.New("db error")
		repo.On("IsBlockedEitherWay", ctx, "aaa", "bbb").Return(false, dbErr)

		ok, err := svc.IsBlockedEitherWay(ctx, "aaa", "bbb")
		assert.False(t, ok)
		assert.ErrorIs(t, err, dbErr)
	})
}
