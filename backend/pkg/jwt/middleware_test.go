// pkg/jwt/middleware_test.go
package jwt_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BleKuntay/FlipChat/backend/pkg/jwt"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupMiddlewareApp(t *testing.T) *fiber.App {
	t.Helper()

	err := jwt.Init("test-secret-key")
	if err != nil && !errors.Is(err, jwt.ErrAlreadyInitialized) {
		require.NoError(t, err)
	}

	app := fiber.New()
	app.Get("/protected", jwt.Protected(), func(c fiber.Ctx) error {
		userID := fiber.Locals[string](c, "user_id")
		return c.JSON(fiber.Map{"user_id": userID})
	})

	return app
}

func TestProtected_MissingHeader_Returns401(t *testing.T) {
	app := setupMiddlewareApp(t)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	resp, _ := app.Test(req)

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestProtected_InvalidBearerFormat_Returns401(t *testing.T) {
	app := setupMiddlewareApp(t)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Token abc123") // bukan Bearer
	resp, _ := app.Test(req)

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestProtected_InvalidToken_Returns401(t *testing.T) {
	app := setupMiddlewareApp(t)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer this.is.not.valid")
	resp, _ := app.Test(req)

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestProtected_ValidToken_InjectsUserID(t *testing.T) {
	app := setupMiddlewareApp(t)

	token, err := jwt.GenerateAccessToken("user-123", "johndoe", "john@example.com", 15*time.Minute)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ := app.Test(req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "user-123", body["user_id"])
}

func TestProtected_RefreshTokenRejected_Returns401(t *testing.T) {
	app := setupMiddlewareApp(t)

	// refresh token tidak boleh dipakai sebagai access token
	token, err := jwt.GenerateRefreshToken("user-123", "johndoe", "john@example.com", 7*24*time.Hour)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ := app.Test(req)

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
