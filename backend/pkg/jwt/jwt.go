package jwt

import (
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"sync"
	"time"
)

const (
	AccessToken  = "access_token"
	RefreshToken = "refresh_token"
)

var (
	ErrSecretEmpty         = errors.New("jwt secret cannot be empty")
	ErrAlreadyInitialized  = errors.New("jwt already initialized")
	ErrNotInitialized      = errors.New("jwt not initialized, call Init() first")
	ErrInvalidToken        = errors.New("invalid token")
	ErrRefreshTokenExpired = errors.New("refresh token expired")
	ErrUnexpectedMethod    = errors.New("unexpected method")
)

var (
	secretKey []byte
	once      sync.Once
)

type Claims struct {
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	TokenType string `json:"token_type"`
	jwt.RegisteredClaims
}

func Init(secret string) error {
	if secret == "" {
		return ErrSecretEmpty
	}

	var firstCall bool
	once.Do(func() {
		secretKey = []byte(secret)
		firstCall = true
	})

	if !firstCall {
		return ErrAlreadyInitialized
	}

	return nil
}

func GenerateAccessToken(userID, username, email string, expiry time.Duration) (string, error) {
	return generateToken(userID, username, email, AccessToken, expiry)
}

func GenerateRefreshToken(userID, username, email string, expiry time.Duration) (string, error) {
	return generateToken(userID, username, email, RefreshToken, expiry)
}

func VerifyAccessToken(token string) (*Claims, error) {
	return verifyToken(token, AccessToken)
}

func VerifyRefreshToken(token string) (*Claims, error) {
	return verifyToken(token, RefreshToken)
}

func generateToken(userID, username, email, tokenType string, expiry time.Duration) (string, error) {
	if secretKey == nil {
		return "", ErrNotInitialized
	}

	claims := Claims{
		UserID:    userID,
		Username:  username,
		Email:     email,
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secretKey)
}

func keyFunc(token *jwt.Token) (interface{}, error) {
	if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
		return nil, ErrUnexpectedMethod
	}

	return secretKey, nil
}

func verifyToken(tokenStr, expectedType string) (*Claims, error) {
	t, err := jwt.ParseWithClaims(tokenStr, &Claims{}, keyFunc)
	if err != nil {
		return nil, err
	}

	if !t.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := t.Claims.(*Claims)
	if !ok {
		return nil, ErrInvalidToken
	}

	if claims.TokenType != expectedType {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
