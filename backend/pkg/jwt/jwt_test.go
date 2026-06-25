package jwt

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func reset() {
	secretKey = nil
	once = sync.Once{}
}

// ————— Init —————————————————————————
func TestInit_Success(t *testing.T) {
	reset()
	err := Init("valid-secret")
	assert.NoError(t, err)
	assert.NotNil(t, secretKey)
}

func TestInit_EmptySecret(t *testing.T) {
	reset()
	err := Init("")
	assert.ErrorIs(t, err, ErrSecretEmpty)
}

func TestInit_AlreadyInitialized(t *testing.T) {
	reset()
	_ = Init("first-secret")

	err := Init("second-secret")
	assert.ErrorIs(t, err, ErrAlreadyInitialized)
}

// ————— GenerateAccessToken —————————————————————————
func TestGenerateAccessToken_Success(t *testing.T) {
	reset()
	require.NoError(t, Init("test-secret"))

	token, err := GenerateAccessToken("user-123", "johndoe", "john@email.com", time.Hour)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestGenerateAccessToken_NotInitialized(t *testing.T) {
	reset()

	token, err := GenerateAccessToken("user-123", "johndoe", "john@email.com", time.Hour)
	assert.ErrorIs(t, err, ErrNotInitialized)
	assert.Empty(t, token)
}

// ————— GenerateRefreshToken —————————————————————————
func TestGenerateRefreshToken_Success(t *testing.T) {
	reset()
	require.NoError(t, Init("test-secret"))

	token, err := GenerateRefreshToken("user-123", "johndoe", "john@email.com", time.Hour*24)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestGenerateRefreshToken_NotInitialized(t *testing.T) {
	reset()

	token, err := GenerateRefreshToken("user-123", "johndoe", "john@email.com", time.Hour*24)
	assert.ErrorIs(t, err, ErrNotInitialized)
	assert.Empty(t, token)
}

// ————— VerifyAccessToken —————————————————————————
func TestVerifyAccessToken_Success(t *testing.T) {
	reset()
	require.NoError(t, Init("test-secret"))

	token, err := GenerateAccessToken("user-123", "johndoe", "john@email.com", time.Hour)
	require.NoError(t, err)

	claims, err := VerifyAccessToken(token)
	assert.NoError(t, err)
	assert.Equal(t, "user-123", claims.UserID)
	assert.Equal(t, "johndoe", claims.Username)
	assert.Equal(t, "john@email.com", claims.Email)
	assert.Equal(t, AccessToken, claims.TokenType)
}

func TestVerifyAccessToken_Expired(t *testing.T) {
	reset()
	require.NoError(t, Init("test-secret"))

	token, err := GenerateAccessToken("user-123", "johndoe", "john@email.com", -time.Hour)
	require.NoError(t, err)

	claims, err := VerifyAccessToken(token)
	assert.Error(t, err)
	assert.Nil(t, claims)
}

func TestVerifyAccessToken_TamperedToken(t *testing.T) {
	reset()
	require.NoError(t, Init("test-secret"))

	token, err := GenerateAccessToken("user-123", "johndoe", "john@email.com", time.Hour)
	require.NoError(t, err)

	claims, err := VerifyAccessToken(token + "tampered")
	assert.Error(t, err)
	assert.Nil(t, claims)
}

func TestVerifyAccessToken_InvalidString(t *testing.T) {
	reset()
	require.NoError(t, Init("test-secret"))

	claims, err := VerifyAccessToken("ini-bukan-token")
	assert.Error(t, err)
	assert.Nil(t, claims)
}

func TestVerifyAccessToken_EmptyString(t *testing.T) {
	reset()
	require.NoError(t, Init("test-secret"))

	claims, err := VerifyAccessToken("")
	assert.Error(t, err)
	assert.Nil(t, claims)
}

func TestVerifyAccessToken_WrongSecret(t *testing.T) {
	reset()
	require.NoError(t, Init("secret-A"))

	token, err := GenerateAccessToken("user-123", "johndoe", "john@email.com", time.Hour)
	require.NoError(t, err)

	reset()
	require.NoError(t, Init("secret-B"))

	claims, err := VerifyAccessToken(token)
	assert.Error(t, err)
	assert.Nil(t, claims)
}

func TestVerifyAccessToken_UsingRefreshToken(t *testing.T) {
	reset()
	require.NoError(t, Init("test-secret"))

	refreshToken, err := GenerateRefreshToken("user-123", "johndoe", "john@email.com", time.Hour*24)
	require.NoError(t, err)

	claims, err := VerifyAccessToken(refreshToken)
	assert.ErrorIs(t, err, ErrInvalidToken)
	assert.Nil(t, claims)
}

func TestVerifyAccessToken_AlgNoneAttack(t *testing.T) {
	reset()
	require.NoError(t, Init("test-secret"))

	unsignedToken := "eyJhbGciOiJub25lIn0.eyJ1c2VyX2lkIjoiYWRtaW4ifQ."

	claims, err := VerifyAccessToken(unsignedToken)
	assert.Error(t, err)
	assert.Nil(t, claims)
}

// ————— VerifyRefreshToken —————————————————————————
func TestVerifyRefreshToken_Success(t *testing.T) {
	reset()
	require.NoError(t, Init("test-secret"))

	token, err := GenerateRefreshToken("user-123", "johndoe", "john@email.com", time.Hour*24)
	require.NoError(t, err)

	claims, err := VerifyRefreshToken(token)
	assert.NoError(t, err)
	assert.Equal(t, "user-123", claims.UserID)
	assert.Equal(t, "johndoe", claims.Username)
	assert.Equal(t, "john@email.com", claims.Email)
	assert.Equal(t, RefreshToken, claims.TokenType)
}

func TestVerifyRefreshToken_Expired(t *testing.T) {
	reset()
	require.NoError(t, Init("test-secret"))

	token, err := GenerateRefreshToken("user-123", "johndoe", "john@email.com", -time.Hour)
	require.NoError(t, err)

	claims, err := VerifyRefreshToken(token)
	assert.Error(t, err)
	assert.Nil(t, claims)
}

func TestVerifyRefreshToken_UsingAccessToken(t *testing.T) {
	reset()
	require.NoError(t, Init("test-secret"))

	accessToken, err := GenerateAccessToken("user-123", "johndoe", "john@email.com", time.Hour)
	require.NoError(t, err)

	claims, err := VerifyRefreshToken(accessToken)
	assert.ErrorIs(t, err, ErrInvalidToken)
	assert.Nil(t, claims)
}

func TestVerifyRefreshToken_TamperedToken(t *testing.T) {
	reset()
	require.NoError(t, Init("test-secret"))

	token, err := GenerateRefreshToken("user-123", "johndoe", "john@email.com", time.Hour*24)
	require.NoError(t, err)

	claims, err := VerifyRefreshToken(token + "tampered")
	assert.Error(t, err)
	assert.Nil(t, claims)
}
