package auth

import (
	"github.com/BleKuntay/FlipChat/backend/internal/config"
	"github.com/BleKuntay/FlipChat/backend/internal/shared"
	"github.com/BleKuntay/FlipChat/backend/pkg/jwt"
	"time"
)

type RepositoryInterface interface {
	CreateUser(user *User) (*User, error)
	FindUserByEmail(email string) (*User, error)
	ExistsByEmail(email string) (bool, error)
	ExistsByUsername(username string) (bool, error)
	SaveRefreshToken(refreshToken RefreshToken) error
	DeleteTokenByToken(token string) error
	DeleteTokenByUserID(token string) error
	FindTokenByToken(token string) (*RefreshToken, error)
	RotateRefreshToken(oldToken, newToken string, expiresAt time.Time) error
}

type Service struct {
	repository RepositoryInterface
}

func NewService(repository RepositoryInterface) *Service {
	return &Service{repository: repository}
}

func (s *Service) Register(request *RegisterRequest) (response *Response, refresh string, error error) {
	if err := shared.ValidatePassword(request.Password); err != nil {
		return nil, "", err
	}

	exists, err := s.repository.ExistsByEmail(request.Email)
	if err != nil {
		return nil, "", err
	}
	if exists {
		return nil, "", ErrEmailAlreadyInUse
	}

	exists, err = s.repository.ExistsByUsername(request.Username)
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

	user, err := s.repository.CreateUser(u)
	if err != nil {
		return nil, "", err
	}

	at, rt, err := s.generateTokenPair(user.ID, user.Username, user.Email)
	if err != nil {
		return nil, "", err
	}

	if err := s.saveRefreshToken(user.ID, rt); err != nil {
		return nil, "", err
	}

	res := &Response{
		AccessToken: at,
		User:        toUserResponse(user),
	}

	return res, rt, nil
}

func (s *Service) Login(request *LoginRequest) (response *Response, refresh string, error error) {
	user, err := s.repository.FindUserByEmail(request.Email)
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

	if err := s.saveRefreshToken(user.ID, rt); err != nil {
		return nil, "", err
	}

	res := &Response{
		AccessToken: at,
		User:        toUserResponse(user),
	}

	return res, rt, nil
}

func (s *Service) Logout(refreshToken string) error {
	if err := s.repository.DeleteTokenByToken(refreshToken); err != nil {
		return err
	}

	return nil
}

func (s *Service) LogoutAll(userID string) error {
	if err := s.repository.DeleteTokenByUserID(userID); err != nil {
		return err
	}

	return nil
}

func (s *Service) Refresh(refreshToken string) (access, refresh string, error error) {
	token, err := s.repository.FindTokenByToken(refreshToken)
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

	if err := s.repository.RotateRefreshToken(token.Token, rt, now.Add(config.App.RefreshTokenExpiry)); err != nil {
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

func (s *Service) saveRefreshToken(userID, token string) error {
	now := time.Now()
	return s.repository.SaveRefreshToken(RefreshToken{
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
