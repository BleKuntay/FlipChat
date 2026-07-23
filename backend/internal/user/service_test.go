package user_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/BleKuntay/FlipChat/backend/internal/shared"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	. "github.com/BleKuntay/FlipChat/backend/internal/user"
)

var (
	hashedOnce     sync.Once
	cachedPassword string
)

// ── helpers ───────────────────────────────────────────────────────────────────

func stubUser() *User {
	bio := "Hello world!"
	avatar := "https://cdn.example.com/avatar.png"
	seen := time.Now().Add(-5 * time.Minute)
	return &User{
		ID:         "user-123",
		Name:       "John Doe",
		Username:   "johndoe",
		Bio:        &bio,
		Email:      "john@example.com",
		Password:   mustHash("secret123"),
		Language:   "en",
		AvatarURL:  &avatar,
		LastSeenAt: &seen,
		CreatedAt:  time.Now().Add(-24 * time.Hour),
	}
}

func mustHash(plain string) string {
	if plain == "secret123" {
		hashedOnce.Do(func() {
			h, err := shared.HashPassword(plain)
			if err != nil {
				panic(err)
			}
			cachedPassword = h
		})
		return cachedPassword
	}
	h, err := shared.HashPassword(plain)
	if err != nil {
		panic(err)
	}
	return h
}

// ── mock repository ───────────────────────────────────────────────────────────

type mockRepo struct{ mock.Mock }

func (m *mockRepo) FindByID(ctx context.Context, id string) (*User, error) {
	args := m.Called(ctx, id)
	if u, ok := args.Get(0).(*User); ok {
		return u, args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockRepo) UpdateProfile(ctx context.Context, u *User) (*UpdateProfileResponse, error) {
	args := m.Called(ctx, u)
	if r, ok := args.Get(0).(*UpdateProfileResponse); ok {
		return r, args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockRepo) UpdateEmail(ctx context.Context, id string, req *UpdateEmailRequest) (*MeResponse, error) {
	args := m.Called(ctx, id, req)
	if r, ok := args.Get(0).(*MeResponse); ok {
		return r, args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockRepo) UpdatePassword(ctx context.Context, id, hashed string) error {
	return m.Called(ctx, id, hashed).Error(0)
}
func (m *mockRepo) DeleteByID(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}
func (m *mockRepo) Search(ctx context.Context, id, q, cursor string, limit int) ([]*Summary, error) {
	args := m.Called(ctx, id, q, cursor, limit)
	if s, ok := args.Get(0).([]*Summary); ok {
		return s, args.Error(1)
	}
	return nil, args.Error(1)
}

// ── mock block checker ────────────────────────────────────────────────────────

type mockBlockChecker struct{ mock.Mock }

func (m *mockBlockChecker) IsBlockedEitherWay(ctx context.Context, a, b string) (bool, error) {
	args := m.Called(ctx, a, b)
	return args.Bool(0), args.Error(1)
}

type mockPresenceChecker struct{ mock.Mock }

func (mpc *mockPresenceChecker) IsOnline(ctx context.Context, userID string) (bool, error) {
	args := mpc.Called(ctx, userID)
	return args.Bool(0), args.Error(1)
}

func newTestService(repo RepositoryInterface, bc BlockChecker, pc PresenceChecker) *Service {
	return NewService(repo, bc, pc)
}

// ── Me ────────────────────────────────────────────────────────────────────────

func TestService_Me(t *testing.T) {
	ctx := context.Background()

	t.Run("returns MeResponse for existing user", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)
		u := stubUser()
		repo.On("FindByID", mock.Anything, u.ID).Return(u, nil)

		svc := newTestService(repo, bc, pc)
		res, err := svc.Me(ctx, u.ID)

		require.NoError(t, err)
		assert.Equal(t, u.ID, res.ID)
		assert.Equal(t, u.Email, res.Email)
		assert.Equal(t, u.Name, res.Name)
		repo.AssertExpectations(t)
	})

	t.Run("propagates error if user not found", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		repo.On("FindByID", mock.Anything, "ghost").Return((*User)(nil), apperr.ErrNotFound)

		svc := newTestService(repo, bc, pc)
		res, err := svc.Me(ctx, "ghost")

		assert.Nil(t, res)
		assert.ErrorIs(t, err, apperr.ErrNotFound)
		repo.AssertExpectations(t)
	})

	t.Run("propagates database error", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		dbErr := errors.New("connection refused")
		repo.On("FindByID", mock.Anything, "any").Return((*User)(nil), dbErr)

		svc := newTestService(repo, bc, pc)
		_, err := svc.Me(ctx, "any")

		assert.ErrorIs(t, err, dbErr)
	})
}

// ── UpdateProfile ─────────────────────────────────────────────────────────────

func TestService_UpdateProfile(t *testing.T) {
	ctx := context.Background()

	t.Run("update name only succeeds", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		u := stubUser()
		repo.On("FindByID", mock.Anything, u.ID).Return(u, nil)

		updated := &UpdateProfileResponse{Response: Response{ID: u.ID, Name: "Jane Doe"}}
		repo.On("UpdateProfile", mock.Anything, mock.AnythingOfType("*user.User")).Return(updated, nil)

		svc := newTestService(repo, bc, pc)
		res, err := svc.UpdateProfile(ctx, u.ID, &UpdateProfileRequest{Name: "Jane Doe"})

		require.NoError(t, err)
		assert.Equal(t, "Jane Doe", res.Name)
		repo.AssertExpectations(t)
	})

	t.Run("update username only succeeds", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		u := stubUser()
		repo.On("FindByID", mock.Anything, u.ID).Return(u, nil)

		updated := &UpdateProfileResponse{Response: Response{ID: u.ID, Username: "janedoe"}}
		repo.On("UpdateProfile", mock.Anything, mock.AnythingOfType("*user.User")).Return(updated, nil)

		svc := newTestService(repo, bc, pc)
		res, err := svc.UpdateProfile(ctx, u.ID, &UpdateProfileRequest{Username: "janedoe"})

		require.NoError(t, err)
		assert.Equal(t, "janedoe", res.Username)
	})

	t.Run("all fields empty returns ErrUserNotUpdated", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		u := stubUser()
		repo.On("FindByID", mock.Anything, u.ID).Return(u, nil)

		svc := newTestService(repo, bc, pc)
		res, err := svc.UpdateProfile(ctx, u.ID, &UpdateProfileRequest{})

		assert.Nil(t, res)
		assert.ErrorIs(t, err, ErrUserNotUpdated)
		repo.AssertNotCalled(t, "UpdateProfile")
	})

	t.Run("empty bio is ignored (cannot clear bio)", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		u := stubUser()
		repo.On("FindByID", mock.Anything, u.ID).Return(u, nil)

		updated := &UpdateProfileResponse{Response: Response{ID: u.ID, Name: "New Name"}}
		repo.On("UpdateProfile", mock.Anything, mock.AnythingOfType("*user.User")).Return(updated, nil)

		svc := newTestService(repo, bc, pc)
		_, err := svc.UpdateProfile(ctx, u.ID, &UpdateProfileRequest{Name: "New Name", Bio: ""})
		require.NoError(t, err)

		call := repo.Calls[1]
		passedUser := call.Arguments.Get(1).(*User)
		assert.Equal(t, u.Bio, passedUser.Bio, "bio should not change because request.Bio is empty")
	})

	t.Run("user not found", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		repo.On("FindByID", mock.Anything, "ghost").Return((*User)(nil), apperr.ErrNotFound)

		svc := newTestService(repo, bc, pc)
		res, err := svc.UpdateProfile(ctx, "ghost", &UpdateProfileRequest{Name: "X"})

		assert.Nil(t, res)
		assert.ErrorIs(t, err, apperr.ErrNotFound)
		repo.AssertNotCalled(t, "UpdateProfile")
	})

	t.Run("repository.UpdateProfile fails → propagate error", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		u := stubUser()
		repo.On("FindByID", mock.Anything, u.ID).Return(u, nil)
		dbErr := errors.New("duplicate key value: username")
		repo.On("UpdateProfile", mock.Anything, mock.AnythingOfType("*user.User")).Return((*UpdateProfileResponse)(nil), dbErr)

		svc := newTestService(repo, bc, pc)
		_, err := svc.UpdateProfile(ctx, u.ID, &UpdateProfileRequest{Username: "taken"})

		assert.ErrorIs(t, err, dbErr)
	})
}

// ── UpdateEmail ───────────────────────────────────────────────────────────────

func TestService_UpdateEmail(t *testing.T) {
	ctx := context.Background()

	t.Run("succeeds with correct password", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		u := stubUser()
		repo.On("FindByID", mock.Anything, u.ID).Return(u, nil)

		req := &UpdateEmailRequest{NewEmail: "newemail@example.com", CurrentPassword: "secret123"}
		expected := &MeResponse{Email: "newemail@example.com"}
		repo.On("UpdateEmail", mock.Anything, u.ID, req).Return(expected, nil)

		svc := newTestService(repo, bc, pc)
		res, err := svc.UpdateEmail(ctx, u.ID, req)

		require.NoError(t, err)
		assert.Equal(t, "newemail@example.com", res.Email)
		repo.AssertExpectations(t)
	})

	t.Run("wrong password returns ErrInvalidPassword", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		u := stubUser()
		repo.On("FindByID", mock.Anything, u.ID).Return(u, nil)

		svc := newTestService(repo, bc, pc)
		_, err := svc.UpdateEmail(ctx, u.ID, &UpdateEmailRequest{
			NewEmail:        "x@example.com",
			CurrentPassword: "wrong",
		})

		assert.ErrorIs(t, err, ErrInvalidPassword)
		repo.AssertNotCalled(t, "UpdateEmail")
	})

	t.Run("user not found", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		repo.On("FindByID", mock.Anything, "ghost").Return((*User)(nil), apperr.ErrNotFound)

		svc := newTestService(repo, bc, pc)
		_, err := svc.UpdateEmail(ctx, "ghost", &UpdateEmailRequest{
			NewEmail:        "x@example.com",
			CurrentPassword: "secret123",
		})

		assert.ErrorIs(t, err, apperr.ErrNotFound)
	})
}

// ── ChangePassword ────────────────────────────────────────────────────────────

func TestService_ChangePassword(t *testing.T) {
	ctx := context.Background()

	t.Run("succeeds with all fields valid", func(t *testing.T) {
		repo := new(mockRepo)
		pc := new(mockPresenceChecker)
		bc := new(mockBlockChecker)

		u := stubUser()
		repo.On("FindByID", mock.Anything, u.ID).Return(u, nil)
		repo.On("UpdatePassword", mock.Anything, u.ID, mock.AnythingOfType("string")).Return(nil)

		svc := newTestService(repo, bc, pc)
		err := svc.ChangePassword(ctx, u.ID, &ChangePasswordRequest{
			CurrentPassword: "secret123",
			NewPassword:     "newSecret456",
			ConfirmPassword: "newSecret456",
		})

		require.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("wrong current password returns ErrInvalidPassword", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		u := stubUser()
		repo.On("FindByID", mock.Anything, u.ID).Return(u, nil)

		svc := newTestService(repo, bc, pc)
		err := svc.ChangePassword(ctx, u.ID, &ChangePasswordRequest{
			CurrentPassword: "wrong",
			NewPassword:     "newSecret456",
			ConfirmPassword: "newSecret456",
		})

		assert.ErrorIs(t, err, ErrInvalidPassword)
		repo.AssertNotCalled(t, "UpdatePassword")
	})

	t.Run("new password and confirm do not match → ErrPasswordMismatch", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		svc := newTestService(repo, bc, pc)
		err := svc.ChangePassword(ctx, "user-123", &ChangePasswordRequest{
			CurrentPassword: "secret123",
			NewPassword:     "newSecret456",
			ConfirmPassword: "different",
		})

		// ErrPasswordMismatch is checked before FindByID, so repo is never called
		assert.ErrorIs(t, err, ErrPasswordMismatch)
		repo.AssertNotCalled(t, "FindByID")
		repo.AssertNotCalled(t, "UpdatePassword")
	})

	t.Run("new password same as current — allowed (no validation for this)", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		u := stubUser()
		repo.On("FindByID", mock.Anything, u.ID).Return(u, nil)
		repo.On("UpdatePassword", mock.Anything, u.ID, mock.AnythingOfType("string")).Return(nil)

		svc := newTestService(repo, bc, pc)
		err := svc.ChangePassword(ctx, u.ID, &ChangePasswordRequest{
			CurrentPassword: "secret123",
			NewPassword:     "secret123",
			ConfirmPassword: "secret123",
		})

		assert.NoError(t, err)
	})

	t.Run("user not found", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		repo.On("FindByID", mock.Anything, "ghost").Return((*User)(nil), apperr.ErrNotFound)

		svc := newTestService(repo, bc, pc)
		err := svc.ChangePassword(ctx, "ghost", &ChangePasswordRequest{
			CurrentPassword: "any",
			NewPassword:     "any",
			ConfirmPassword: "any",
		})

		assert.ErrorIs(t, err, apperr.ErrNotFound)
		repo.AssertNotCalled(t, "UpdatePassword")
	})

	t.Run("repository.UpdatePassword fails → propagate error", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		u := stubUser()
		repo.On("FindByID", mock.Anything, u.ID).Return(u, nil)
		dbErr := errors.New("db write error")
		repo.On("UpdatePassword", mock.Anything, u.ID, mock.AnythingOfType("string")).Return(dbErr)

		svc := newTestService(repo, bc, pc)
		err := svc.ChangePassword(ctx, u.ID, &ChangePasswordRequest{
			CurrentPassword: "secret123",
			NewPassword:     "newPass",
			ConfirmPassword: "newPass",
		})

		assert.ErrorIs(t, err, dbErr)
	})
}

// ── DeleteAccount ─────────────────────────────────────────────────────────────

func TestService_DeleteAccount(t *testing.T) {
	ctx := context.Background()

	t.Run("succeeds", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		repo.On("DeleteByID", mock.Anything, "user-123").Return(nil)

		svc := newTestService(repo, bc, pc)
		err := svc.DeleteAccount(ctx, "user-123")

		require.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("user not found — repository returns ErrNotFound", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		repo.On("DeleteByID", mock.Anything, "ghost").Return(apperr.ErrNotFound)

		svc := newTestService(repo, bc, pc)
		err := svc.DeleteAccount(ctx, "ghost")

		assert.ErrorIs(t, err, apperr.ErrNotFound)
	})

	t.Run("database error → propagate error", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		dbErr := errors.New("connection lost")
		repo.On("DeleteByID", mock.Anything, "user-123").Return(dbErr)

		svc := newTestService(repo, bc, pc)
		err := svc.DeleteAccount(ctx, "user-123")

		assert.ErrorIs(t, err, dbErr)
	})
}

// ── FindUserByID ──────────────────────────────────────────────────────────────

func TestService_FindUserByID(t *testing.T) {
	ctx := context.Background()

	t.Run("succeeds when no block exists — returns Response with is_online populated", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		u := stubUser()
		repo.On("FindByID", mock.Anything, u.ID).Return(u, nil)
		bc.On("IsBlockedEitherWay", mock.Anything, "caller", u.ID).Return(false, nil)
		pc.On("IsOnline", mock.Anything, u.ID).Return(true, nil)

		svc := newTestService(repo, bc, pc)
		res, err := svc.FindUserByID(ctx, "caller", &GetUserURI{ID: u.ID})

		require.NoError(t, err)
		assert.Equal(t, u.ID, res.ID)
		assert.True(t, res.IsOnline)
		repo.AssertExpectations(t)
		bc.AssertExpectations(t)
		pc.AssertExpectations(t)
	})

	t.Run("presence checker error → is_online defaults to false, no error propagated", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		u := stubUser()
		repo.On("FindByID", mock.Anything, u.ID).Return(u, nil)
		bc.On("IsBlockedEitherWay", mock.Anything, "caller", u.ID).Return(false, nil)
		pc.On("IsOnline", mock.Anything, u.ID).Return(false, errors.New("redis down"))

		svc := newTestService(repo, bc, pc)
		res, err := svc.FindUserByID(ctx, "caller", &GetUserURI{ID: u.ID})

		require.NoError(t, err)
		assert.Equal(t, u.ID, res.ID)
		assert.False(t, res.IsOnline, "is_online defaults to false when presence check fails")
	})

	t.Run("returns ErrNotFound if target has blocked requester", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		u := stubUser()
		repo.On("FindByID", mock.Anything, u.ID).Return(u, nil)
		bc.On("IsBlockedEitherWay", mock.Anything, "caller", u.ID).Return(true, nil)

		svc := newTestService(repo, bc, pc)
		res, err := svc.FindUserByID(ctx, "caller", &GetUserURI{ID: u.ID})

		assert.Nil(t, res)
		assert.ErrorIs(t, err, apperr.ErrNotFound)
		pc.AssertNotCalled(t, "IsOnline")
	})

	t.Run("user not found", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		repo.On("FindByID", mock.Anything, "ghost").Return((*User)(nil), apperr.ErrNotFound)

		svc := newTestService(repo, bc, pc)
		res, err := svc.FindUserByID(ctx, "caller", &GetUserURI{ID: "ghost"})

		assert.Nil(t, res)
		assert.ErrorIs(t, err, apperr.ErrNotFound)
		bc.AssertNotCalled(t, "IsBlockedEitherWay")
		pc.AssertNotCalled(t, "IsOnline")
	})

	t.Run("block checker error → propagate error", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		u := stubUser()
		repo.On("FindByID", mock.Anything, u.ID).Return(u, nil)
		dbErr := errors.New("db error")
		bc.On("IsBlockedEitherWay", mock.Anything, "caller", u.ID).Return(false, dbErr)

		svc := newTestService(repo, bc, pc)
		_, err := svc.FindUserByID(ctx, "caller", &GetUserURI{ID: u.ID})

		assert.ErrorIs(t, err, dbErr)
		pc.AssertNotCalled(t, "IsOnline")
	})
}

// ── Search ────────────────────────────────────────────────────────────────────

func TestService_Search(t *testing.T) {
	ctx := context.Background()

	makeSummary := func(id, username string) *Summary {
		return &Summary{ID: id, Name: "User " + id, Username: username}
	}

	t.Run("results less than limit → next_cursor is nil", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		summaries := []*Summary{makeSummary("a", "alice"), makeSummary("b", "bob")}
		repo.On("Search", mock.Anything, "me", "ali", "", 11).Return(summaries, nil)
		pc.On("IsOnline", mock.Anything, "a").Return(false, nil)
		pc.On("IsOnline", mock.Anything, "b").Return(true, nil)

		svc := newTestService(repo, bc, pc)
		res, err := svc.Search(ctx, "me", &SearchQuery{Q: "ali", Limit: 10})

		require.NoError(t, err)
		assert.Len(t, res.Data, 2)
		assert.Nil(t, res.NextCursor)
		pc.AssertExpectations(t)
	})

	t.Run("results exceed limit → trim and set next_cursor", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		summaries := []*Summary{
			makeSummary("a", "alice"),
			makeSummary("b", "bob"),
			makeSummary("c", "charlie"), // extra record signalling has_more
		}
		repo.On("Search", mock.Anything, "me", "b", "", 3).Return(summaries, nil)
		pc.On("IsOnline", mock.Anything, "a").Return(false, nil)
		pc.On("IsOnline", mock.Anything, "b").Return(false, nil)

		svc := newTestService(repo, bc, pc)
		res, err := svc.Search(ctx, "me", &SearchQuery{Q: "b", Limit: 2})

		require.NoError(t, err)
		assert.Len(t, res.Data, 2, "trimmed to limit")
		require.NotNil(t, res.NextCursor)
		assert.Equal(t, "b", *res.NextCursor, "cursor is last item after trim")
		pc.AssertExpectations(t)
	})

	t.Run("with cursor → forwarded to repository", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		summaries := []*Summary{makeSummary("c", "charlie")}
		repo.On("Search", mock.Anything, "me", "c", "b", 11).Return(summaries, nil)
		pc.On("IsOnline", mock.Anything, "c").Return(false, nil)

		svc := newTestService(repo, bc, pc)
		res, err := svc.Search(ctx, "me", &SearchQuery{Q: "c", Cursor: "b", Limit: 10})

		require.NoError(t, err)
		assert.Len(t, res.Data, 1)
		repo.AssertExpectations(t)
	})

	t.Run("empty results → data array is empty, next_cursor is nil", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		repo.On("Search", mock.Anything, "me", "zzz", "", 11).Return([]*Summary{}, nil)

		svc := newTestService(repo, bc, pc)
		res, err := svc.Search(ctx, "me", &SearchQuery{Q: "zzz", Limit: 10})

		require.NoError(t, err)
		assert.Empty(t, res.Data)
		assert.Nil(t, res.NextCursor)
		pc.AssertNotCalled(t, "IsOnline")
	})

	t.Run("limit=0 defaults to 20", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		repo.On("Search", mock.Anything, "me", "x", "", 21).Return([]*Summary{}, nil)

		svc := newTestService(repo, bc, pc)
		_, err := svc.Search(ctx, "me", &SearchQuery{Q: "x", Limit: 0})

		require.NoError(t, err)
		repo.AssertCalled(t, "Search", mock.Anything, "me", "x", "", 21)
	})

	t.Run("limit above max is capped to 50", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		repo.On("Search", mock.Anything, "me", "x", "", 51).Return([]*Summary{}, nil)

		svc := newTestService(repo, bc, pc)
		_, err := svc.Search(ctx, "me", &SearchQuery{Q: "x", Limit: 999})

		require.NoError(t, err)
		repo.AssertCalled(t, "Search", mock.Anything, "me", "x", "", 51)
	})

	t.Run("repository error → propagate error", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		dbErr := errors.New("query timeout")
		repo.On("Search", mock.Anything, "me", "x", "", 11).Return(([]*Summary)(nil), dbErr)

		svc := newTestService(repo, bc, pc)
		_, err := svc.Search(ctx, "me", &SearchQuery{Q: "x", Limit: 10})

		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("presence check error per user is silently ignored", func(t *testing.T) {
		repo := new(mockRepo)
		bc := new(mockBlockChecker)
		pc := new(mockPresenceChecker)

		summaries := []*Summary{makeSummary("a", "alice")}
		repo.On("Search", mock.Anything, "me", "a", "", 11).Return(summaries, nil)
		pc.On("IsOnline", mock.Anything, "a").Return(false, errors.New("redis timeout"))

		svc := newTestService(repo, bc, pc)
		res, err := svc.Search(ctx, "me", &SearchQuery{Q: "a", Limit: 10})

		require.NoError(t, err)
		assert.Len(t, res.Data, 1)
		assert.False(t, res.Data[0].IsOnline)
	})
}
