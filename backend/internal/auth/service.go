package auth

import (
	"context"
	"time"

	"github.com/BleKuntay/FlipChat/backend/internal/config"
	"github.com/BleKuntay/FlipChat/backend/internal/shared"
	"github.com/BleKuntay/FlipChat/backend/pkg/jwt"
)

type RepositoryInterface interface {
	CreateUser(ctx context.Context, user *User) (*User, error)
	FindUserByEmail(ctx context.Context, email string) (*User, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	ExistsByUsername(ctx context.Context, username string) (bool, error)
	SaveRefreshToken(ctx context.Context, refreshToken RefreshToken) error
	DeleteTokenByToken(ctx context.Context, token string) error
	DeleteTokenByUserID(ctx context.Context, userID string) error
	FindTokenByToken(ctx context.Context, token string) (*RefreshToken, error)
	RotateRefreshToken(ctx context.Context, oldToken, newToken string, expiresAt time.Time) error
}

type Service struct {
	repository RepositoryInterface
}

func NewService(repository RepositoryInterface) *Service {
	return &Service{repository: repository}
}

func (s *Service) Register(ctx context.Context, request *RegisterRequest) (response *Response, refresh string, error error) {
	if err := shared.ValidatePassword(request.Password); err != nil {
		return nil, "", err
	}

	exists, err := s.repository.ExistsByEmail(ctx, request.Email)
	if err != nil {
		return nil, "", err
	}
	if exists {
		return nil, "", ErrEmailAlreadyInUse
	}

	exists, err = s.repository.ExistsByUsername(ctx, request.Username)
	if err != nil {
		return nil, "", err
	}
	if exists {
		return nil, "", ErrUsernameAlreadyTaken
	}

	hashedPassword, err := shared.HashPassword(request.Password)
	if err != nil {
		return nil, "", err
	}

	u := &User{
		Name:     request.Name,
		Username: request.Username,
		Email:    request.Email,
		Password: hashedPassword,
	}

	if request.Language != nil {
		u.Language = *request.Language
	} else {
		u.Language = "en"
	}

	user, err := s.repository.CreateUser(ctx, u)
	if err != nil {
		return nil, "", err
	}

	at, rt, err := s.generateTokenPair(user.ID, user.Username, user.Email)
	if err != nil {
		return nil, "", err
	}

	if err := s.saveRefreshToken(ctx, user.ID, rt); err != nil {
		return nil, "", err
	}

	return &Response{
		AccessToken: at,
		User:        toUserResponse(user),
	}, rt, nil
}

func (s *Service) Login(ctx context.Context, request *LoginRequest) (response *Response, refresh string, error error) {
	user, err := s.repository.FindUserByEmail(ctx, request.Email)
	if err != nil || user == nil {
		shared.DummyVerify(request.Password)
		return nil, "", ErrInvalidCredentials
	}

	if !shared.VerifyPassword(request.Password, user.Password) {
		return nil, "", ErrInvalidCredentials
	}

	at, rt, err := s.generateTokenPair(user.ID, user.Username, user.Email)
	if err != nil {
		return nil, "", err
	}

	if err := s.saveRefreshToken(ctx, user.ID, rt); err != nil {
		return nil, "", err
	}

	return &Response{
		AccessToken: at,
		User:        toUserResponse(user),
	}, rt, nil
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	return s.repository.DeleteTokenByToken(ctx, refreshToken)
}

func (s *Service) LogoutAll(ctx context.Context, userID string) error {
	return s.repository.DeleteTokenByUserID(ctx, userID)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (access, refresh string, error error) {
	token, err := s.repository.FindTokenByToken(ctx, refreshToken)
	if err != nil {
		return "", "", err
	}
	if token == nil {
		return "", "", jwt.ErrInvalidToken
	}

	now := time.Now()

	if token.ExpiresAt.Before(now) {
		return "", "", jwt.ErrRefreshTokenExpired
	}

	claims, err := jwt.VerifyRefreshToken(token.Token)
	if err != nil {
		return "", "", jwt.ErrInvalidToken
	}

	at, err := jwt.GenerateAccessToken(claims.UserID, claims.Username, claims.Email, config.App.AccessTokenExpiry)
	if err != nil {
		return "", "", err
	}

	rt, err := jwt.GenerateRefreshToken(claims.UserID, claims.Username, claims.Email, config.App.RefreshTokenExpiry)
	if err != nil {
		return "", "", err
	}

	if err := s.repository.RotateRefreshToken(ctx, token.Token, rt, now.Add(config.App.RefreshTokenExpiry)); err != nil {
		return "", "", err
	}

	return at, rt, nil
}

func (s *Service) generateTokenPair(userID, username, email string) (at, rt string, err error) {
	at, err = jwt.GenerateAccessToken(userID, username, email, config.App.AccessTokenExpiry)
	if err != nil {
		return "", "", err
	}

	rt, err = jwt.GenerateRefreshToken(userID, username, email, config.App.RefreshTokenExpiry)
	if err != nil {
		return "", "", err
	}

	return at, rt, nil
}

func (s *Service) saveRefreshToken(ctx context.Context, userID, token string) error {
	now := time.Now()
	return s.repository.SaveRefreshToken(ctx, RefreshToken{
		UserID:    userID,
		Token:     token,
		ExpiresAt: now.Add(config.App.RefreshTokenExpiry),
	})
}

func toUserResponse(user *User) UserResponse {
	return UserResponse{
		ID:        user.ID,
		Name:      user.Name,
		Username:  user.Username,
		Bio:       user.Bio,
		Email:     user.Email,
		Language:  user.Language,
		AvatarURL: user.AvatarURL,
		CreatedAt: user.CreatedAt,
	}
}
