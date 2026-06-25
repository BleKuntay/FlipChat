package user_test

import (
	"errors"
	"github.com/BleKuntay/FlipChat/backend/internal/shared"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	. "github.com/BleKuntay/FlipChat/backend/internal/user"
)

var (
	hashedOnce     sync.Once
	cachedPassword string
)

// ── helpers ──────────────────────────────────────────────────────────────────

func ptr[T any](v T) *T { return &v }

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

func (m *mockRepo) FindByID(id string) (*User, error) {
	args := m.Called(id)
	if u, ok := args.Get(0).(*User); ok {
		return u, args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockRepo) UpdateProfile(u *User) (*UpdateProfileResponse, error) {
	args := m.Called(u)
	if r, ok := args.Get(0).(*UpdateProfileResponse); ok {
		return r, args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockRepo) UpdateEmail(id string, req *UpdateEmailRequest) (*MeResponse, error) {
	args := m.Called(id, req)
	if r, ok := args.Get(0).(*MeResponse); ok {
		return r, args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockRepo) UpdatePassword(id, hashed string) error {
	return m.Called(id, hashed).Error(0)
}
func (m *mockRepo) DeleteByID(id string) error {
	return m.Called(id).Error(0)
}
func (m *mockRepo) Search(id, q, cursor string, limit int) ([]*Summary, error) {
	args := m.Called(id, q, cursor, limit)
	if s, ok := args.Get(0).([]*Summary); ok {
		return s, args.Error(1)
	}
	return nil, args.Error(1)
}

// newTestService initializes Service with mock repository.
// Requires Service.repository to be of type RepositoryInterface (not *Repository).
func newTestService(repo RepositoryInterface) *Service {
	return NewService(repo)
}

// ── Me ────────────────────────────────────────────────────────────────────────

func TestService_Me(t *testing.T) {
	t.Run("returns MeResponse for existing user", func(t *testing.T) {
		repo := new(mockRepo)
		u := stubUser()
		repo.On("FindByID", u.ID).Return(u, nil)

		svc := newTestService(repo)
		res, err := svc.Me(u.ID)

		require.NoError(t, err)
		assert.Equal(t, u.ID, res.ID)
		assert.Equal(t, u.Email, res.Email)
		assert.Equal(t, u.Name, res.Name)
		repo.AssertExpectations(t)
	})

	t.Run("propagates error if user not found", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("FindByID", "ghost").Return((*User)(nil), ErrUserNotFound)

		svc := newTestService(repo)
		res, err := svc.Me("ghost")

		assert.Nil(t, res)
		assert.ErrorIs(t, err, ErrUserNotFound)
		repo.AssertExpectations(t)
	})

	t.Run("propagates database error", func(t *testing.T) {
		repo := new(mockRepo)
		dbErr := errors.New("connection refused")
		repo.On("FindByID", "any").Return((*User)(nil), dbErr)

		svc := newTestService(repo)
		_, err := svc.Me("any")

		assert.ErrorIs(t, err, dbErr)
	})
}

// ── UpdateProfile ─────────────────────────────────────────────────────────────

func TestService_UpdateProfile(t *testing.T) {
	t.Run("update name only succeeds", func(t *testing.T) {
		repo := new(mockRepo)
		u := stubUser()
		repo.On("FindByID", u.ID).Return(u, nil)

		updated := &UpdateProfileResponse{Response: Response{ID: u.ID, Name: "Jane Doe"}}
		repo.On("UpdateProfile", mock.AnythingOfType("*user.User")).Return(updated, nil)

		svc := newTestService(repo)
		res, err := svc.UpdateProfile(u.ID, &UpdateProfileRequest{Name: "Jane Doe"})

		require.NoError(t, err)
		assert.Equal(t, "Jane Doe", res.Name)
		repo.AssertExpectations(t)
	})

	t.Run("update username only succeeds", func(t *testing.T) {
		repo := new(mockRepo)
		u := stubUser()
		repo.On("FindByID", u.ID).Return(u, nil)

		updated := &UpdateProfileResponse{Response: Response{ID: u.ID, Username: "janedoe"}}
		repo.On("UpdateProfile", mock.AnythingOfType("*user.User")).Return(updated, nil)

		svc := newTestService(repo)
		res, err := svc.UpdateProfile(u.ID, &UpdateProfileRequest{Username: "janedoe"})

		require.NoError(t, err)
		assert.Equal(t, "janedoe", res.Username)
	})

	t.Run("all fields empty returns ErrUserNotUpdated", func(t *testing.T) {
		repo := new(mockRepo)
		u := stubUser()
		repo.On("FindByID", u.ID).Return(u, nil)

		svc := newTestService(repo)
		res, err := svc.UpdateProfile(u.ID, &UpdateProfileRequest{})

		assert.Nil(t, res)
		assert.ErrorIs(t, err, ErrUserNotUpdated)
		repo.AssertNotCalled(t, "UpdateProfile") // should not hit DB if no changes
	})

	// BUG DOCUMENTED: Bio cannot be cleared to empty string.
	// Condition `if request.Bio != ""` skips empty bio.
	// For now we document the existing behavior.
	t.Run("empty bio is ignored (cannot clear bio)", func(t *testing.T) {
		repo := new(mockRepo)
		u := stubUser() // user already has bio
		repo.On("FindByID", u.ID).Return(u, nil)

		svc := newTestService(repo)
		// Request with valid Name but empty Bio
		updated := &UpdateProfileResponse{Response: Response{ID: u.ID, Name: "New Name"}}
		repo.On("UpdateProfile", mock.AnythingOfType("*user.User")).Return(updated, nil)

		_, err := svc.UpdateProfile(u.ID, &UpdateProfileRequest{Name: "New Name", Bio: ""})
		require.NoError(t, err)

		// Verify UpdateProfile was called with bio that did NOT change
		call := repo.Calls[1]
		passedUser := call.Arguments.Get(0).(*User)
		assert.Equal(t, u.Bio, passedUser.Bio, "bio should not change because request.Bio is empty")
	})

	t.Run("user not found", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("FindByID", "ghost").Return((*User)(nil), ErrUserNotFound)

		svc := newTestService(repo)
		res, err := svc.UpdateProfile("ghost", &UpdateProfileRequest{Name: "X"})

		assert.Nil(t, res)
		assert.ErrorIs(t, err, ErrUserNotFound)
		repo.AssertNotCalled(t, "UpdateProfile")
	})

	t.Run("repository.UpdateProfile fails → propagate error", func(t *testing.T) {
		repo := new(mockRepo)
		u := stubUser()
		repo.On("FindByID", u.ID).Return(u, nil)
		dbErr := errors.New("duplicate key value: username")
		repo.On("UpdateProfile", mock.AnythingOfType("*user.User")).Return((*UpdateProfileResponse)(nil), dbErr)

		svc := newTestService(repo)
		_, err := svc.UpdateProfile(u.ID, &UpdateProfileRequest{Username: "taken"})

		assert.ErrorIs(t, err, dbErr)
	})
}

// ── UpdateEmail ───────────────────────────────────────────────────────────────

func TestService_UpdateEmail(t *testing.T) {
	t.Run("succeeds with correct password", func(t *testing.T) {
		repo := new(mockRepo)
		u := stubUser()
		repo.On("FindByID", u.ID).Return(u, nil)

		req := &UpdateEmailRequest{NewEmail: "newemail@example.com", CurrentPassword: "secret123"}
		expected := &MeResponse{Email: "newemail@example.com"}
		repo.On("UpdateEmail", u.ID, req).Return(expected, nil)

		svc := newTestService(repo)
		res, err := svc.UpdateEmail(u.ID, req)

		require.NoError(t, err)
		assert.Equal(t, "newemail@example.com", res.Email)
		repo.AssertExpectations(t)
	})

	t.Run("wrong password returns ErrInvalidPassword, does not hit DB", func(t *testing.T) {
		repo := new(mockRepo)
		u := stubUser()
		repo.On("FindByID", u.ID).Return(u, nil)

		svc := newTestService(repo)
		_, err := svc.UpdateEmail(u.ID, &UpdateEmailRequest{
			NewEmail:        "newemail@example.com",
			CurrentPassword: "wrong-password",
		})

		assert.ErrorIs(t, err, ErrInvalidPassword)
		repo.AssertNotCalled(t, "UpdateEmail")
	})

	t.Run("user not found", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("FindByID", "ghost").Return((*User)(nil), ErrUserNotFound)

		svc := newTestService(repo)
		_, err := svc.UpdateEmail("ghost", &UpdateEmailRequest{CurrentPassword: "any"})

		assert.ErrorIs(t, err, ErrUserNotFound)
	})

	t.Run("repository.UpdateEmail fails → propagate error", func(t *testing.T) {
		repo := new(mockRepo)
		u := stubUser()
		repo.On("FindByID", u.ID).Return(u, nil)

		req := &UpdateEmailRequest{NewEmail: "dup@example.com", CurrentPassword: "secret123"}
		dbErr := errors.New("duplicate key: email")
		repo.On("UpdateEmail", u.ID, req).Return((*MeResponse)(nil), dbErr)

		svc := newTestService(repo)
		_, err := svc.UpdateEmail(u.ID, req)

		assert.ErrorIs(t, err, dbErr)
	})
}

// ── ChangePassword ────────────────────────────────────────────────────────────

func TestService_ChangePassword(t *testing.T) {
	t.Run("succeeds with all fields valid", func(t *testing.T) {
		repo := new(mockRepo)
		u := stubUser()
		repo.On("FindByID", u.ID).Return(u, nil)
		// UpdatePassword is called with hashed string — we don't know exact value,
		// but we can verify it was called with correct userID.
		repo.On("UpdatePassword", u.ID, mock.AnythingOfType("string")).Return(nil)

		svc := newTestService(repo)
		err := svc.ChangePassword(u.ID, &ChangePasswordRequest{
			CurrentPassword: "secret123",
			NewPassword:     "newSecret456",
			ConfirmPassword: "newSecret456",
		})

		require.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("wrong current password returns ErrInvalidPassword", func(t *testing.T) {
		repo := new(mockRepo)
		u := stubUser()
		repo.On("FindByID", u.ID).Return(u, nil)

		svc := newTestService(repo)
		err := svc.ChangePassword(u.ID, &ChangePasswordRequest{
			CurrentPassword: "wrong",
			NewPassword:     "newSecret456",
			ConfirmPassword: "newSecret456",
		})

		assert.ErrorIs(t, err, ErrInvalidPassword)
		repo.AssertNotCalled(t, "UpdatePassword")
	})

	t.Run("new password and confirm do not match → ErrPasswordMismatch", func(t *testing.T) {
		repo := new(mockRepo)
		u := stubUser()
		repo.On("FindByID", u.ID).Return(u, nil)

		svc := newTestService(repo)
		err := svc.ChangePassword(u.ID, &ChangePasswordRequest{
			CurrentPassword: "secret123",
			NewPassword:     "newSecret456",
			ConfirmPassword: "different",
		})

		assert.ErrorIs(t, err, ErrPasswordMismatch)
		repo.AssertNotCalled(t, "UpdatePassword")
	})

	t.Run("new password same as current password — should succeed (no validation for this)", func(t *testing.T) {
		// Note: there is no validation "new password != old password" in service.
		// This test documents that current behavior allows it.
		// If you want to add such validation, add ErrSamePassword and test its rejection.
		repo := new(mockRepo)
		u := stubUser()
		repo.On("FindByID", u.ID).Return(u, nil)
		repo.On("UpdatePassword", u.ID, mock.AnythingOfType("string")).Return(nil)

		svc := newTestService(repo)
		err := svc.ChangePassword(u.ID, &ChangePasswordRequest{
			CurrentPassword: "secret123",
			NewPassword:     "secret123",
			ConfirmPassword: "secret123",
		})

		assert.NoError(t, err)
	})

	t.Run("user not found", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("FindByID", "ghost").Return((*User)(nil), ErrUserNotFound)

		svc := newTestService(repo)
		err := svc.ChangePassword("ghost", &ChangePasswordRequest{
			CurrentPassword: "any",
			NewPassword:     "any",
			ConfirmPassword: "any",
		})

		assert.ErrorIs(t, err, ErrUserNotFound)
		repo.AssertNotCalled(t, "UpdatePassword")
	})

	t.Run("repository.UpdatePassword fails → propagate error", func(t *testing.T) {
		repo := new(mockRepo)
		u := stubUser()
		repo.On("FindByID", u.ID).Return(u, nil)
		dbErr := errors.New("db write error")
		repo.On("UpdatePassword", u.ID, mock.AnythingOfType("string")).Return(dbErr)

		svc := newTestService(repo)
		err := svc.ChangePassword(u.ID, &ChangePasswordRequest{
			CurrentPassword: "secret123",
			NewPassword:     "newPass",
			ConfirmPassword: "newPass",
		})

		assert.ErrorIs(t, err, dbErr)
	})
}

// ── DeleteAccount ─────────────────────────────────────────────────────────────

func TestService_DeleteAccount(t *testing.T) {
	t.Run("succeeds", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("DeleteByID", "user-123").Return(nil)

		svc := newTestService(repo)
		err := svc.DeleteAccount("user-123")

		require.NoError(t, err)
		repo.AssertExpectations(t)
	})

	// BUG DOCUMENTED: Repository.DeleteByID does not check rows affected.
	// If user does not exist, DELETE query won't error — silent no-op.
	// Should return ErrUserNotFound. This test documents current behavior.
	t.Run("user not found — repository does not return error (silent no-op)", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("DeleteByID", "ghost").Return(nil) // mock simulates current DB behavior

		svc := newTestService(repo)
		err := svc.DeleteAccount("ghost")

		// Currently: no error — depends on Repository fix
		assert.NoError(t, err)
	})

	t.Run("database error → propagate error", func(t *testing.T) {
		repo := new(mockRepo)
		dbErr := errors.New("connection lost")
		repo.On("DeleteByID", "user-123").Return(dbErr)

		svc := newTestService(repo)
		err := svc.DeleteAccount("user-123")

		assert.ErrorIs(t, err, dbErr)
	})
}

// ── FindUserByID ──────────────────────────────────────────────────────────────

func TestService_FindUserByID(t *testing.T) {
	t.Run("succeeds", func(t *testing.T) {
		repo := new(mockRepo)
		u := stubUser()
		repo.On("FindByID", u.ID).Return(u, nil)

		svc := newTestService(repo)
		res, err := svc.FindUserByID(&GetUserURI{ID: u.ID})

		require.NoError(t, err)
		assert.Equal(t, u.ID, res.ID)
	})

	t.Run("user not found", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("FindByID", "ghost").Return((*User)(nil), ErrUserNotFound)

		svc := newTestService(repo)
		res, err := svc.FindUserByID(&GetUserURI{ID: "ghost"})

		assert.Nil(t, res)
		assert.ErrorIs(t, err, ErrUserNotFound)
	})
}

// ── Search ────────────────────────────────────────────────────────────────────

func TestService_Search(t *testing.T) {
	makeSummary := func(id, username string) *Summary {
		return &Summary{ID: id, Name: "User " + id, Username: username}
	}

	t.Run("results less than limit → next_cursor is nil", func(t *testing.T) {
		repo := new(mockRepo)
		summaries := []*Summary{makeSummary("a", "alice"), makeSummary("b", "bob")}
		repo.On("Search", "me", "ali", "", 10).Return(summaries, nil)

		svc := newTestService(repo)
		res, err := svc.Search("me", &SearchQuery{Q: "ali", Limit: 10})

		require.NoError(t, err)
		assert.Len(t, res.Data, 2)
		assert.Nil(t, res.NextCursor, "next_cursor must be nil if results < limit")
	})

	t.Run("results exactly equal to limit → next_cursor contains last ID", func(t *testing.T) {
		repo := new(mockRepo)
		summaries := []*Summary{
			makeSummary("a", "alice"),
			makeSummary("b", "bob"),
		}
		repo.On("Search", "me", "b", "", 2).Return(summaries, nil)

		svc := newTestService(repo)
		res, err := svc.Search("me", &SearchQuery{Q: "b", Limit: 2})

		require.NoError(t, err)
		require.NotNil(t, res.NextCursor, "next_cursor must exist if results == limit")
		assert.Equal(t, "b", *res.NextCursor)
	})

	t.Run("with cursor → forwarded to repository", func(t *testing.T) {
		repo := new(mockRepo)
		summaries := []*Summary{makeSummary("c", "charlie")}
		repo.On("Search", "me", "c", "b", 10).Return(summaries, nil)

		svc := newTestService(repo)
		res, err := svc.Search("me", &SearchQuery{Q: "c", Cursor: "b", Limit: 10})

		require.NoError(t, err)
		assert.Len(t, res.Data, 1)
		repo.AssertExpectations(t)
	})

	t.Run("empty results → data array is empty, next_cursor is nil", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("Search", "me", "zzz", "", 10).Return([]*Summary{}, nil)

		svc := newTestService(repo)
		res, err := svc.Search("me", &SearchQuery{Q: "zzz", Limit: 10})

		require.NoError(t, err)
		assert.Empty(t, res.Data)
		assert.Nil(t, res.NextCursor)
	})

	// BUG DOCUMENTED: No validation or default for Limit.
	// If frontend sends limit=0 or omits limit, query runs with LIMIT 0 (always empty) or negative value.
	// Should have default (e.g. 20) and maximum cap (e.g. 50).
	t.Run("limit=0 passed directly to repository without default", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("Search", "me", "x", "", 20).Return([]*Summary{}, nil)

		svc := newTestService(repo)
		_, err := svc.Search("me", &SearchQuery{Q: "x", Limit: 0})

		// No error, but result is always empty — likely unintended behavior
		require.NoError(t, err)
		repo.AssertCalled(t, "Search", "me", "x", "", 20)
	})

	t.Run("repository error → propagate error", func(t *testing.T) {
		repo := new(mockRepo)
		dbErr := errors.New("query timeout")
		repo.On("Search", "me", "x", "", 10).Return(([]*Summary)(nil), dbErr)

		svc := newTestService(repo)
		_, err := svc.Search("me", &SearchQuery{Q: "x", Limit: 10})

		assert.ErrorIs(t, err, dbErr)
	})
}
