package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	. "github.com/BleKuntay/FlipChat/backend/internal/auth"
	"github.com/BleKuntay/FlipChat/backend/internal/shared"
	pkgjwt "github.com/BleKuntay/FlipChat/backend/pkg/jwt"
)

// ── mock service ──────────────────────────────────────────────────────────────

type mockService struct{ mock.Mock }

func (m *mockService) Register(ctx context.Context, req *RegisterRequest) (*Response, string, error) {
	args := m.Called(ctx, req)
	if r, ok := args.Get(0).(*Response); ok {
		return r, args.String(1), args.Error(2)
	}
	return nil, "", args.Error(2)
}

func (m *mockService) Login(ctx context.Context, req *LoginRequest) (*Response, string, error) {
	args := m.Called(ctx, req)
	if r, ok := args.Get(0).(*Response); ok {
		return r, args.String(1), args.Error(2)
	}
	return nil, "", args.Error(2)
}

func (m *mockService) Logout(ctx context.Context, refreshToken string) error {
	return m.Called(ctx, refreshToken).Error(0)
}

func (m *mockService) LogoutAll(ctx context.Context, userID string) error {
	return m.Called(ctx, userID).Error(0)
}

func (m *mockService) Refresh(ctx context.Context, refreshToken string) (string, string, error) {
	args := m.Called(ctx, refreshToken)
	return args.String(0), args.String(1), args.Error(2)
}

// ── test app factory ──────────────────────────────────────────────────────────

func newTestApp(svc ServiceInterface, userID string) *fiber.App {
	app := fiber.New()

	app.Use(func(c fiber.Ctx) error {
		fiber.Locals[string](c, "user_id", userID)
		return c.Next()
	})

	h := NewHandler(svc)
	v1 := app.Group("/v1/auth")
	h.RegisterRoutes(v1)

	return app
}

func doRequest(t *testing.T, app *fiber.App, method, path string, body any) *http.Response {
	t.Helper()
	var bodyBytes []byte
	if body != nil {
		bodyBytes, _ = json.Marshal(body)
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req) //nolint:bodyclose // closed via t.Cleanup below
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func doRequestWithCookie(t *testing.T, app *fiber.App, method, path string, body any, cookieName, cookieValue string) *http.Response {
	t.Helper()
	var bodyBytes []byte
	if body != nil {
		bodyBytes, _ = json.Marshal(body)
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: cookieName, Value: cookieValue})

	resp, _ := app.Test(req) //nolint:bodyclose // closed via t.Cleanup below
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, dst any) {
	t.Helper()
	require.NoError(t, json.NewDecoder(resp.Body).Decode(dst))
}

func stubAuthResponse() *Response {
	return &Response{
		AccessToken: "access-token-value",
		User: UserResponse{
			ID:       "user-123",
			Name:     "John Doe",
			Username: "johndoe",
			Email:    "john@example.com",
			Language: "en",
		},
	}
}

// ── POST /register ────────────────────────────────────────────────────────────

func TestHandler_Register(t *testing.T) {
	validReq := map[string]any{
		"name":     "John Doe",
		"username": "johndoe",
		"email":    "john@example.com",
		"password": "Secret123",
	}

	t.Run("201 succeeds and sets refresh token cookie", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Register", mock.Anything, mock.AnythingOfType("*auth.RegisterRequest")).
			Return(stubAuthResponse(), "refresh-token-value", nil)

		app := newTestApp(svc, "")
		resp := doRequest(t, app, http.MethodPost, "/v1/auth/register", validReq) //nolint:bodyclose // body closed via t.Cleanup in helper

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var body Response
		decodeJSON(t, resp, &body)
		assert.NotEmpty(t, body.AccessToken)
		assert.Equal(t, "john@example.com", body.User.Email)

		var hasRefreshCookie bool
		for _, c := range resp.Cookies() {
			if c.Name == "refresh_token" {
				hasRefreshCookie = true
				assert.True(t, c.HttpOnly, "refresh_token cookie harus HttpOnly")
				break
			}
		}
		assert.True(t, hasRefreshCookie, "refresh_token cookie harus di-set")
		svc.AssertExpectations(t)
	})

	t.Run("400 if body is invalid JSON", func(t *testing.T) {
		svc := new(mockService)
		app := newTestApp(svc, "")

		req := httptest.NewRequest(http.MethodPost, "/v1/auth/register", bytes.NewBufferString("not-json"))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := app.Test(req) //nolint:bodyclose // closed via t.Cleanup below
		t.Cleanup(func() { resp.Body.Close() })

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		svc.AssertNotCalled(t, "Register")
	})

	t.Run("400 if password weak", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Register", mock.Anything, mock.AnythingOfType("*auth.RegisterRequest")).
			Return((*Response)(nil), "", shared.ErrPasswordWeak)

		app := newTestApp(svc, "")
		resp := doRequest(t, app, http.MethodPost, "/v1/auth/register", validReq) //nolint:bodyclose // body closed via t.Cleanup in helper

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("409 if email already in use", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Register", mock.Anything, mock.AnythingOfType("*auth.RegisterRequest")).
			Return((*Response)(nil), "", ErrEmailAlreadyInUse)

		app := newTestApp(svc, "")
		resp := doRequest(t, app, http.MethodPost, "/v1/auth/register", validReq) //nolint:bodyclose // body closed via t.Cleanup in helper

		assert.Equal(t, http.StatusConflict, resp.StatusCode)
		var body map[string]string
		decodeJSON(t, resp, &body)
		assert.Equal(t, "email already in use", body["error"])
	})

	t.Run("409 if username already taken", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Register", mock.Anything, mock.AnythingOfType("*auth.RegisterRequest")).
			Return((*Response)(nil), "", ErrUsernameAlreadyTaken)

		app := newTestApp(svc, "")
		resp := doRequest(t, app, http.MethodPost, "/v1/auth/register", validReq) //nolint:bodyclose // body closed via t.Cleanup in helper

		assert.Equal(t, http.StatusConflict, resp.StatusCode)
		var body map[string]string
		decodeJSON(t, resp, &body)
		assert.Equal(t, "username already taken", body["error"])
	})

	t.Run("500 for unexpected error — returns generic message", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Register", mock.Anything, mock.AnythingOfType("*auth.RegisterRequest")).
			Return((*Response)(nil), "", errors.New("unexpected db error"))

		app := newTestApp(svc, "")
		resp := doRequest(t, app, http.MethodPost, "/v1/auth/register", validReq) //nolint:bodyclose // body closed via t.Cleanup in helper

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		var body map[string]string
		decodeJSON(t, resp, &body)
		assert.Equal(t, "internal server error", body["error"])
	})

	t.Run("password field does not appear in response", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Register", mock.Anything, mock.AnythingOfType("*auth.RegisterRequest")).
			Return(stubAuthResponse(), "refresh-token-value", nil)

		app := newTestApp(svc, "")
		resp := doRequest(t, app, http.MethodPost, "/v1/auth/register", validReq) //nolint:bodyclose // body closed via t.Cleanup in helper

		var rawBody map[string]any
		decodeJSON(t, resp, &rawBody)
		_, hasPassword := rawBody["password"]
		assert.False(t, hasPassword, "password tidak boleh muncul di response")
	})
}

// ── POST /login ───────────────────────────────────────────────────────────────

func TestHandler_Login(t *testing.T) {
	validReq := map[string]any{
		"email":    "john@example.com",
		"password": "Secret123",
	}

	t.Run("200 succeeds and sets refresh token cookie", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Login", mock.Anything, mock.AnythingOfType("*auth.LoginRequest")).
			Return(stubAuthResponse(), "refresh-token-value", nil)

		app := newTestApp(svc, "")
		resp := doRequest(t, app, http.MethodPost, "/v1/auth/login", validReq) //nolint:bodyclose // body closed via t.Cleanup in helper

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var body Response
		decodeJSON(t, resp, &body)
		assert.NotEmpty(t, body.AccessToken)

		var hasRefreshCookie bool
		for _, c := range resp.Cookies() {
			if c.Name == "refresh_token" {
				hasRefreshCookie = true
				assert.True(t, c.HttpOnly)
				break
			}
		}
		assert.True(t, hasRefreshCookie)
		svc.AssertExpectations(t)
	})

	t.Run("400 if body is invalid JSON", func(t *testing.T) {
		svc := new(mockService)
		app := newTestApp(svc, "")

		req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewBufferString("not-json"))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := app.Test(req) //nolint:bodyclose // closed via t.Cleanup below
		t.Cleanup(func() { resp.Body.Close() })

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		svc.AssertNotCalled(t, "Login")
	})

	t.Run("401 if invalid credentials", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Login", mock.Anything, mock.AnythingOfType("*auth.LoginRequest")).
			Return((*Response)(nil), "", ErrInvalidCredentials)

		app := newTestApp(svc, "")
		resp := doRequest(t, app, http.MethodPost, "/v1/auth/login", validReq) //nolint:bodyclose // body closed via t.Cleanup in helper

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		var body map[string]string
		decodeJSON(t, resp, &body)
		assert.Equal(t, "invalid credentials", body["error"])
	})

	t.Run("500 for unexpected error — returns generic message", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Login", mock.Anything, mock.AnythingOfType("*auth.LoginRequest")).
			Return((*Response)(nil), "", errors.New("db down"))

		app := newTestApp(svc, "")
		resp := doRequest(t, app, http.MethodPost, "/v1/auth/login", validReq) //nolint:bodyclose // body closed via t.Cleanup in helper

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		var body map[string]string
		decodeJSON(t, resp, &body)
		assert.Equal(t, "internal server error", body["error"])
	})
}

// ── POST /logout ──────────────────────────────────────────────────────────────

func TestHandler_Logout(t *testing.T) {
	t.Run("200 succeeds and clears cookie", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Logout", mock.Anything, "valid-refresh-token").Return(nil)

		app := newTestApp(svc, "user-123")
		resp := doRequestWithCookie(t, app, http.MethodPost, "/v1/auth/logout", nil, "refresh_token", "valid-refresh-token") //nolint:bodyclose // body closed via t.Cleanup in helper

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body map[string]string
		decodeJSON(t, resp, &body)
		assert.Equal(t, "logged out successfully", body["message"])

		for _, c := range resp.Cookies() {
			if c.Name == "refresh_token" {
				assert.True(t, c.Expires.Before(time.Now()), "refresh_token cookie harus expired")
				break
			}
		}
		svc.AssertExpectations(t)
	})

	t.Run("200 even if no refresh token cookie — graceful", func(t *testing.T) {
		svc := new(mockService)
		app := newTestApp(svc, "user-123")

		resp := doRequest(t, app, http.MethodPost, "/v1/auth/logout", nil) //nolint:bodyclose // body closed via t.Cleanup in helper

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		svc.AssertNotCalled(t, "Logout")
	})

	t.Run("200 even if service.Logout fails — cookie tetap di-clear", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Logout", mock.Anything, "some-token").Return(errors.New("db error"))

		app := newTestApp(svc, "user-123")
		resp := doRequestWithCookie(t, app, http.MethodPost, "/v1/auth/logout", nil, "refresh_token", "some-token") //nolint:bodyclose // body closed via t.Cleanup in helper

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

// ── POST /logout-all ──────────────────────────────────────────────────────────

func TestHandler_LogoutAll(t *testing.T) {
	t.Run("204 succeeds", func(t *testing.T) {
		svc := new(mockService)
		svc.On("LogoutAll", mock.Anything, "user-123").Return(nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodPost, "/v1/auth/logout-all", nil) //nolint:bodyclose // body closed via t.Cleanup in helper

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
		svc.AssertExpectations(t)
	})

	t.Run("500 if service fails", func(t *testing.T) {
		svc := new(mockService)
		svc.On("LogoutAll", mock.Anything, "user-123").Return(errors.New("db error"))

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodPost, "/v1/auth/logout-all", nil) //nolint:bodyclose // body closed via t.Cleanup in helper

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("userID comes from context (auth middleware), not body", func(t *testing.T) {
		svc := new(mockService)
		svc.On("LogoutAll", mock.Anything, "context-user").Return(nil)

		app := newTestApp(svc, "context-user")
		resp := doRequest(t, app, http.MethodPost, "/v1/auth/logout-all", map[string]string{ //nolint:bodyclose // body closed via t.Cleanup in helper
			"user_id": "attacker-trying-to-override",
		})

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
		svc.AssertCalled(t, "LogoutAll", mock.Anything, "context-user")
	})
}

// ── POST /refresh ─────────────────────────────────────────────────────────────

func TestHandler_Refresh(t *testing.T) {
	t.Run("200 succeeds — returns new access token and rotates cookie", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Refresh", mock.Anything, "valid-refresh-token").
			Return("new-access-token", "new-refresh-token", nil)

		app := newTestApp(svc, "")
		resp := doRequestWithCookie(t, app, http.MethodPost, "/v1/auth/refresh", nil, "refresh_token", "valid-refresh-token") //nolint:bodyclose // body closed via t.Cleanup in helper

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body map[string]string
		decodeJSON(t, resp, &body)
		assert.Equal(t, "new-access-token", body["access_token"])

		var hasRefreshCookie bool
		for _, c := range resp.Cookies() {
			if c.Name == "refresh_token" {
				hasRefreshCookie = true
				assert.True(t, c.HttpOnly)
				break
			}
		}
		assert.True(t, hasRefreshCookie, "refresh_token cookie harus di-rotate")
		svc.AssertExpectations(t)
	})

	t.Run("401 if no refresh token cookie", func(t *testing.T) {
		svc := new(mockService)
		app := newTestApp(svc, "")

		resp := doRequest(t, app, http.MethodPost, "/v1/auth/refresh", nil) //nolint:bodyclose // body closed via t.Cleanup in helper

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		svc.AssertNotCalled(t, "Refresh")
	})

	t.Run("401 if token invalid", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Refresh", mock.Anything, "bad-token").
			Return("", "", pkgjwt.ErrInvalidToken)

		app := newTestApp(svc, "")
		resp := doRequestWithCookie(t, app, http.MethodPost, "/v1/auth/refresh", nil, "refresh_token", "bad-token") //nolint:bodyclose // body closed via t.Cleanup in helper

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("401 if token expired", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Refresh", mock.Anything, "expired-token").
			Return("", "", pkgjwt.ErrRefreshTokenExpired)

		app := newTestApp(svc, "")
		resp := doRequestWithCookie(t, app, http.MethodPost, "/v1/auth/refresh", nil, "refresh_token", "expired-token") //nolint:bodyclose // body closed via t.Cleanup in helper

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("500 for unexpected error — returns generic message", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Refresh", mock.Anything, "some-token").
			Return("", "", errors.New("db down"))

		app := newTestApp(svc, "")
		resp := doRequestWithCookie(t, app, http.MethodPost, "/v1/auth/refresh", nil, "refresh_token", "some-token") //nolint:bodyclose // body closed via t.Cleanup in helper

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		var body map[string]string
		decodeJSON(t, resp, &body)
		assert.Equal(t, "internal server error", body["error"])
	})
}
