package auth_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	. "github.com/BleKuntay/FlipChat/backend/internal/auth"
	"github.com/BleKuntay/FlipChat/backend/internal/config"
	"github.com/BleKuntay/FlipChat/backend/internal/shared"
	pkgjwt "github.com/BleKuntay/FlipChat/backend/pkg/jwt"
)

// ── test setup ────────────────────────────────────────────────────────────────

func init() {
	config.App = &config.Config{
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
	}
	_ = pkgjwt.Init("test-secret-key")
}

// ── helpers ───────────────────────────────────────────────────────────────────

func stubUser() *User {
	return &User{
		ID:       "user-123",
		Name:     "John Doe",
		Username: "johndoe",
		Email:    "john@example.com",
		Password: mustHash("Secret123"),
		Language: "en",
	}
}

func mustHash(plain string) string {
	h, err := shared.HashPassword(plain)
	if err != nil {
		panic(err)
	}
	return h
}

func stubRefreshToken(userID string) *RefreshToken {
	rt, err := pkgjwt.GenerateRefreshToken(userID, "johndoe", "john@example.com", config.App.RefreshTokenExpiry)
	if err != nil {
		panic(err)
	}
	return &RefreshToken{
		ID:        "rt-123",
		UserID:    userID,
		Token:     rt,
		ExpiresAt: time.Now().Add(config.App.RefreshTokenExpiry),
		CreatedAt: time.Now(),
	}
}

// ── mock repository ───────────────────────────────────────────────────────────

type mockRepo struct{ mock.Mock }

func (m *mockRepo) CreateUser(u *User) (*User, error) {
	args := m.Called(u)
	if r, ok := args.Get(0).(*User); ok {
		return r, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockRepo) FindUserByEmail(email string) (*User, error) {
	args := m.Called(email)
	if u, ok := args.Get(0).(*User); ok {
		return u, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockRepo) ExistsByEmail(email string) (bool, error) {
	args := m.Called(email)
	return args.Bool(0), args.Error(1)
}

func (m *mockRepo) ExistsByUsername(username string) (bool, error) {
	args := m.Called(username)
	return args.Bool(0), args.Error(1)
}

func (m *mockRepo) SaveRefreshToken(token RefreshToken) error {
	return m.Called(token).Error(0)
}

func (m *mockRepo) DeleteTokenByToken(token string) error {
	return m.Called(token).Error(0)
}

func (m *mockRepo) DeleteTokenByUserID(userID string) error {
	return m.Called(userID).Error(0)
}

func (m *mockRepo) FindTokenByToken(token string) (*RefreshToken, error) {
	args := m.Called(token)
	if rt, ok := args.Get(0).(*RefreshToken); ok {
		return rt, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockRepo) RotateRefreshToken(oldToken, newToken string, expiresAt time.Time) error {
	return m.Called(oldToken, newToken, expiresAt).Error(0)
}

func newTestService(repo RepositoryInterface) *Service {
	return NewService(repo)
}

// ── Register ──────────────────────────────────────────────────────────────────

func TestService_Register(t *testing.T) {
	validReq := &RegisterRequest{
		Name:     "John Doe",
		Username: "johndoe",
		Email:    "john@example.com",
		Password: "Secret123",
	}

	t.Run("succeeds and returns response and refresh token", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("ExistsByEmail", validReq.Email).Return(false, nil)
		repo.On("ExistsByUsername", validReq.Username).Return(false, nil)
		repo.On("CreateUser", mock.AnythingOfType("*auth.User")).Return(stubUser(), nil)
		repo.On("SaveRefreshToken", mock.AnythingOfType("auth.RefreshToken")).Return(nil)

		svc := newTestService(repo)
		res, rt, err := svc.Register(validReq)

		require.NoError(t, err)
		require.NotNil(t, res)
		assert.NotEmpty(t, res.AccessToken)
		assert.NotEmpty(t, rt)
		assert.Equal(t, "john@example.com", res.User.Email)
		repo.AssertExpectations(t)
	})

	t.Run("password too short returns ErrPasswordTooShort", func(t *testing.T) {
		repo := new(mockRepo)
		svc := newTestService(repo)

		req := *validReq
		req.Password = "Ab1"
		res, rt, err := svc.Register(&req)

		assert.Nil(t, res)
		assert.Empty(t, rt)
		assert.ErrorIs(t, err, shared.ErrPasswordTooShort)
		repo.AssertNotCalled(t, "ExistsByEmail")
	})

	t.Run("password too long returns ErrPasswordTooLong", func(t *testing.T) {
		repo := new(mockRepo)
		svc := newTestService(repo)

		req := *validReq
		req.Password = "Secret12345678901234567890123456X" // 33 chars
		res, rt, err := svc.Register(&req)

		assert.Nil(t, res)
		assert.Empty(t, rt)
		assert.ErrorIs(t, err, shared.ErrPasswordTooLong)
		repo.AssertNotCalled(t, "ExistsByEmail")
	})

	t.Run("weak password returns ErrPasswordWeak", func(t *testing.T) {
		repo := new(mockRepo)
		svc := newTestService(repo)

		req := *validReq
		req.Password = "alllowercase"
		res, rt, err := svc.Register(&req)

		assert.Nil(t, res)
		assert.Empty(t, rt)
		assert.ErrorIs(t, err, shared.ErrPasswordWeak)
		repo.AssertNotCalled(t, "ExistsByEmail")
	})

	t.Run("email already in use returns ErrEmailAlreadyInUse", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("ExistsByEmail", validReq.Email).Return(true, nil)

		svc := newTestService(repo)
		res, rt, err := svc.Register(validReq)

		assert.Nil(t, res)
		assert.Empty(t, rt)
		assert.ErrorIs(t, err, ErrEmailAlreadyInUse)
		repo.AssertNotCalled(t, "ExistsByUsername")
	})

	t.Run("username already taken returns ErrUsernameAlreadyTaken", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("ExistsByEmail", validReq.Email).Return(false, nil)
		repo.On("ExistsByUsername", validReq.Username).Return(true, nil)

		svc := newTestService(repo)
		res, rt, err := svc.Register(validReq)

		assert.Nil(t, res)
		assert.Empty(t, rt)
		assert.ErrorIs(t, err, ErrUsernameAlreadyTaken)
		repo.AssertNotCalled(t, "CreateUser")
	})

	t.Run("language defaults to en if not provided", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("ExistsByEmail", validReq.Email).Return(false, nil)
		repo.On("ExistsByUsername", validReq.Username).Return(false, nil)
		repo.On("CreateUser", mock.MatchedBy(func(u *User) bool {
			return u.Language == "en"
		})).Return(stubUser(), nil)
		repo.On("SaveRefreshToken", mock.AnythingOfType("auth.RefreshToken")).Return(nil)

		svc := newTestService(repo)
		_, _, err := svc.Register(validReq)

		require.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("language set if provided", func(t *testing.T) {
		repo := new(mockRepo)
		lang := "id"
		req := *validReq
		req.Language = &lang

		repo.On("ExistsByEmail", req.Email).Return(false, nil)
		repo.On("ExistsByUsername", req.Username).Return(false, nil)
		repo.On("CreateUser", mock.MatchedBy(func(u *User) bool {
			return u.Language == "id"
		})).Return(stubUser(), nil)
		repo.On("SaveRefreshToken", mock.AnythingOfType("auth.RefreshToken")).Return(nil)

		svc := newTestService(repo)
		_, _, err := svc.Register(&req)

		require.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("repository.CreateUser fails → propagate error", func(t *testing.T) {
		repo := new(mockRepo)
		dbErr := errors.New("db write error")
		repo.On("ExistsByEmail", validReq.Email).Return(false, nil)
		repo.On("ExistsByUsername", validReq.Username).Return(false, nil)
		repo.On("CreateUser", mock.AnythingOfType("*auth.User")).Return((*User)(nil), dbErr)

		svc := newTestService(repo)
		res, rt, err := svc.Register(validReq)

		assert.Nil(t, res)
		assert.Empty(t, rt)
		assert.ErrorIs(t, err, dbErr)
		repo.AssertNotCalled(t, "SaveRefreshToken")
	})

	t.Run("repository.SaveRefreshToken fails → propagate error", func(t *testing.T) {
		repo := new(mockRepo)
		dbErr := errors.New("db write error")
		repo.On("ExistsByEmail", validReq.Email).Return(false, nil)
		repo.On("ExistsByUsername", validReq.Username).Return(false, nil)
		repo.On("CreateUser", mock.AnythingOfType("*auth.User")).Return(stubUser(), nil)
		repo.On("SaveRefreshToken", mock.AnythingOfType("auth.RefreshToken")).Return(dbErr)

		svc := newTestService(repo)
		res, rt, err := svc.Register(validReq)

		assert.Nil(t, res)
		assert.Empty(t, rt)
		assert.ErrorIs(t, err, dbErr)
	})
}

// ── Login ─────────────────────────────────────────────────────────────────────

func TestService_Login(t *testing.T) {
	validReq := &LoginRequest{
		Email:    "john@example.com",
		Password: "Secret123",
	}

	t.Run("succeeds and returns response and refresh token", func(t *testing.T) {
		repo := new(mockRepo)
		u := stubUser()
		repo.On("FindUserByEmail", validReq.Email).Return(u, nil)
		repo.On("SaveRefreshToken", mock.AnythingOfType("auth.RefreshToken")).Return(nil)

		svc := newTestService(repo)
		res, rt, err := svc.Login(validReq)

		require.NoError(t, err)
		require.NotNil(t, res)
		assert.NotEmpty(t, res.AccessToken)
		assert.NotEmpty(t, rt)
		assert.Equal(t, u.Email, res.User.Email)
		repo.AssertExpectations(t)
	})

	t.Run("email not found returns ErrInvalidCredentials", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("FindUserByEmail", validReq.Email).Return((*User)(nil), nil)

		svc := newTestService(repo)
		res, rt, err := svc.Login(validReq)

		assert.Nil(t, res)
		assert.Empty(t, rt)
		assert.ErrorIs(t, err, ErrInvalidCredentials)
		repo.AssertNotCalled(t, "SaveRefreshToken")
	})

	t.Run("wrong password returns ErrInvalidCredentials", func(t *testing.T) {
		repo := new(mockRepo)
		u := stubUser()
		repo.On("FindUserByEmail", validReq.Email).Return(u, nil)

		svc := newTestService(repo)
		res, rt, err := svc.Login(&LoginRequest{
			Email:    validReq.Email,
			Password: "WrongPass1",
		})

		assert.Nil(t, res)
		assert.Empty(t, rt)
		assert.ErrorIs(t, err, ErrInvalidCredentials)
		repo.AssertNotCalled(t, "SaveRefreshToken")
	})

	t.Run("db error returns ErrInvalidCredentials (timing attack mitigation)", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("FindUserByEmail", validReq.Email).Return((*User)(nil), errors.New("connection refused"))

		svc := newTestService(repo)
		_, _, err := svc.Login(validReq)

		assert.ErrorIs(t, err, ErrInvalidCredentials)
	})

	t.Run("repository.SaveRefreshToken fails → propagate error", func(t *testing.T) {
		repo := new(mockRepo)
		u := stubUser()
		dbErr := errors.New("db write error")
		repo.On("FindUserByEmail", validReq.Email).Return(u, nil)
		repo.On("SaveRefreshToken", mock.AnythingOfType("auth.RefreshToken")).Return(dbErr)

		svc := newTestService(repo)
		res, rt, err := svc.Login(validReq)

		assert.Nil(t, res)
		assert.Empty(t, rt)
		assert.ErrorIs(t, err, dbErr)
	})
}

// ── Logout ────────────────────────────────────────────────────────────────────

func TestService_Logout(t *testing.T) {
	t.Run("succeeds", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("DeleteTokenByToken", "some-token").Return(nil)

		svc := newTestService(repo)
		err := svc.Logout("some-token")

		require.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("repository error → propagate error", func(t *testing.T) {
		repo := new(mockRepo)
		dbErr := errors.New("db error")
		repo.On("DeleteTokenByToken", "some-token").Return(dbErr)

		svc := newTestService(repo)
		err := svc.Logout("some-token")

		assert.ErrorIs(t, err, dbErr)
	})
}

// ── LogoutAll ─────────────────────────────────────────────────────────────────

func TestService_LogoutAll(t *testing.T) {
	t.Run("succeeds", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("DeleteTokenByUserID", "user-123").Return(nil)

		svc := newTestService(repo)
		err := svc.LogoutAll("user-123")

		require.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("repository error → propagate error", func(t *testing.T) {
		repo := new(mockRepo)
		dbErr := errors.New("db error")
		repo.On("DeleteTokenByUserID", "user-123").Return(dbErr)

		svc := newTestService(repo)
		err := svc.LogoutAll("user-123")

		assert.ErrorIs(t, err, dbErr)
	})
}

// ── Refresh ───────────────────────────────────────────────────────────────────

func TestService_Refresh(t *testing.T) {
	t.Run("succeeds and returns new access and refresh token", func(t *testing.T) {
		repo := new(mockRepo)
		rt := stubRefreshToken("user-123")
		repo.On("FindTokenByToken", rt.Token).Return(rt, nil)
		repo.On("RotateRefreshToken", rt.Token, mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).Return(nil)

		svc := newTestService(repo)
		at, newRt, err := svc.Refresh(rt.Token)

		require.NoError(t, err)
		assert.NotEmpty(t, at)
		assert.NotEmpty(t, newRt)
		// Verifikasi RotateRefreshToken dipanggil dengan token lama dan token baru yang di-return
		repo.AssertCalled(t, "RotateRefreshToken", rt.Token, newRt, mock.AnythingOfType("time.Time"))
		repo.AssertExpectations(t)
	})

	t.Run("token not found returns ErrInvalidToken", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("FindTokenByToken", "ghost-token").Return((*RefreshToken)(nil), nil)

		svc := newTestService(repo)
		at, rt, err := svc.Refresh("ghost-token")

		assert.Empty(t, at)
		assert.Empty(t, rt)
		assert.ErrorIs(t, err, pkgjwt.ErrInvalidToken)
	})

	t.Run("expired token returns ErrRefreshTokenExpired", func(t *testing.T) {
		repo := new(mockRepo)
		expiredRt := stubRefreshToken("user-123")
		expiredRt.ExpiresAt = time.Now().Add(-1 * time.Hour)
		repo.On("FindTokenByToken", expiredRt.Token).Return(expiredRt, nil)

		svc := newTestService(repo)
		at, rt, err := svc.Refresh(expiredRt.Token)

		assert.Empty(t, at)
		assert.Empty(t, rt)
		assert.ErrorIs(t, err, pkgjwt.ErrRefreshTokenExpired)
		repo.AssertNotCalled(t, "RotateRefreshToken")
	})

	t.Run("repository.FindTokenByToken fails → propagate error", func(t *testing.T) {
		repo := new(mockRepo)
		dbErr := errors.New("db error")
		repo.On("FindTokenByToken", "some-token").Return((*RefreshToken)(nil), dbErr)

		svc := newTestService(repo)
		at, rt, err := svc.Refresh("some-token")

		assert.Empty(t, at)
		assert.Empty(t, rt)
		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("repository.RotateRefreshToken fails → propagate error", func(t *testing.T) {
		repo := new(mockRepo)
		dbErr := errors.New("db write error")
		rt := stubRefreshToken("user-123")
		repo.On("FindTokenByToken", rt.Token).Return(rt, nil)
		repo.On("RotateRefreshToken", rt.Token, mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).Return(dbErr)

		svc := newTestService(repo)
		at, newRt, err := svc.Refresh(rt.Token)

		assert.Empty(t, at)
		assert.Empty(t, newRt)
		assert.ErrorIs(t, err, dbErr)
	})
}
