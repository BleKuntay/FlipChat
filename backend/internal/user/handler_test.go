package user_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	. "github.com/BleKuntay/FlipChat/backend/internal/user"
)

// ── mock service ──────────────────────────────────────────────────────────────

type mockService struct{ mock.Mock }

func (m *mockService) Me(ctx context.Context, userID string) (*MeResponse, error) {
	args := m.Called(ctx, userID)
	if r, ok := args.Get(0).(*MeResponse); ok {
		return r, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockService) UpdateProfile(ctx context.Context, userID string, req *UpdateProfileRequest) (*UpdateProfileResponse, error) {
	args := m.Called(ctx, userID, req)
	if r, ok := args.Get(0).(*UpdateProfileResponse); ok {
		return r, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockService) UpdateEmail(ctx context.Context, userID string, req *UpdateEmailRequest) (*UpdateEmailResponse, error) {
	args := m.Called(ctx, userID, req)
	if r, ok := args.Get(0).(*UpdateEmailResponse); ok {
		return r, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockService) ChangePassword(ctx context.Context, userID string, req *ChangePasswordRequest) error {
	return m.Called(ctx, userID, req).Error(0)
}

func (m *mockService) DeleteAccount(ctx context.Context, userID string) error {
	return m.Called(ctx, userID).Error(0)
}

func (m *mockService) FindUserByID(ctx context.Context, requesterID string, param *GetUserURI) (*Response, error) {
	args := m.Called(ctx, requesterID, param)
	if r, ok := args.Get(0).(*Response); ok {
		return r, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockService) Search(ctx context.Context, userID string, query *SearchQuery) (*SearchResponse, error) {
	args := m.Called(ctx, userID, query)
	if r, ok := args.Get(0).(*SearchResponse); ok {
		return r, args.Error(1)
	}
	return nil, args.Error(1)
}

// ── test app factory ──────────────────────────────────────────────────────────

func newTestApp(svc ServiceInterface, userID string) *fiber.App {
	app := fiber.New()

	app.Use(func(c fiber.Ctx) error {
		fiber.Locals[string](c, "user_id", userID)
		return c.Next()
	})

	h := NewHandler(svc)
	v1 := app.Group("/v1/users")
	h.RegisterRoutes(v1)

	return app
}

func doRequest(t *testing.T, app *fiber.App, method, path string, body any) *http.Response {
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

func decodeJSON(t *testing.T, resp *http.Response, dst any) {
	t.Helper()
	require.NoError(t, json.NewDecoder(resp.Body).Decode(dst))
}

// ── GET /me ───────────────────────────────────────────────────────────────────

func TestHandler_GetProfile(t *testing.T) {
	t.Run("200 with user data", func(t *testing.T) {
		svc := new(mockService)
		expected := &MeResponse{Email: "john@example.com"}
		expected.ID = "user-123"
		expected.Name = "John Doe"
		svc.On("Me", mock.Anything, "user-123").Return(expected, nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodGet, "/v1/users/me", nil)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body MeResponse
		decodeJSON(t, resp, &body)
		assert.Equal(t, "john@example.com", body.Email)
	})

	t.Run("404 if ErrNotFound", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Me", mock.Anything, "ghost").Return((*MeResponse)(nil), apperr.ErrNotFound)

		app := newTestApp(svc, "ghost")
		resp := doRequest(t, app, http.MethodGet, "/v1/users/me", nil)

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("500 if database error", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Me", mock.Anything, "user-123").Return((*MeResponse)(nil), errors.New("db down"))

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodGet, "/v1/users/me", nil)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}

// ── PATCH /me ─────────────────────────────────────────────────────────────────

func TestHandler_UpdateProfile(t *testing.T) {
	t.Run("200 update succeeds", func(t *testing.T) {
		svc := new(mockService)
		req := &UpdateProfileRequest{Name: "Jane Doe"}
		res := &UpdateProfileResponse{Response: Response{ID: "user-123", Name: "Jane Doe"}}
		svc.On("UpdateProfile", mock.Anything, "user-123", mock.AnythingOfType("*user.UpdateProfileRequest")).Return(res, nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodPatch, "/v1/users/me", req)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("400 if body is not valid JSON", func(t *testing.T) {
		svc := new(mockService)
		app := newTestApp(svc, "user-123")

		req := httptest.NewRequest(http.MethodPatch, "/v1/users/me", bytes.NewBufferString("not-json"))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := app.Test(req) //nolint:bodyclose // closed via t.Cleanup below
		t.Cleanup(func() { resp.Body.Close() })

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		svc.AssertNotCalled(t, "UpdateProfile")
	})

	t.Run("400 if ErrUserNotUpdated", func(t *testing.T) {
		svc := new(mockService)
		svc.On("UpdateProfile", mock.Anything, "user-123", mock.Anything).Return((*UpdateProfileResponse)(nil), ErrUserNotUpdated)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodPatch, "/v1/users/me", map[string]string{})

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("500 if database error", func(t *testing.T) {
		svc := new(mockService)
		svc.On("UpdateProfile", mock.Anything, "user-123", mock.Anything).Return((*UpdateProfileResponse)(nil), errors.New("db error"))

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodPatch, "/v1/users/me", map[string]string{"name": "Jane"})

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}

// ── PATCH /me/email ───────────────────────────────────────────────────────────

func TestHandler_UpdateEmail(t *testing.T) {
	t.Run("200 email update succeeds", func(t *testing.T) {
		svc := new(mockService)
		req := &UpdateEmailRequest{NewEmail: "newemail@example.com", CurrentPassword: "secret"}
		res := &UpdateEmailResponse{Email: "newemail@example.com"}
		svc.On("UpdateEmail", mock.Anything, "user-123", mock.AnythingOfType("*user.UpdateEmailRequest")).Return(res, nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodPatch, "/v1/users/me/email", req)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("401 if ErrInvalidPassword", func(t *testing.T) {
		svc := new(mockService)
		svc.On("UpdateEmail", mock.Anything, "user-123", mock.Anything).Return((*UpdateEmailResponse)(nil), ErrInvalidPassword)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodPatch, "/v1/users/me/email", map[string]string{
			"new_email":        "newemail@example.com",
			"current_password": "wrong",
		})

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("500 if database error", func(t *testing.T) {
		svc := new(mockService)
		svc.On("UpdateEmail", mock.Anything, "user-123", mock.Anything).Return(
			(*UpdateEmailResponse)(nil),
			errors.New("db timeout"),
		)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodPatch, "/v1/users/me/email", map[string]string{
			"new_email":        "x@example.com",
			"current_password": "secret",
		})

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}

// ── PATCH /me/password ────────────────────────────────────────────────────────

func TestHandler_ChangePassword(t *testing.T) {
	validReq := &ChangePasswordRequest{
		CurrentPassword: "oldpass",
		NewPassword:     "newpass",
		ConfirmPassword: "newpass",
	}

	t.Run("200 password changed", func(t *testing.T) {
		svc := new(mockService)
		svc.On("ChangePassword", mock.Anything, "user-123", mock.AnythingOfType("*user.ChangePasswordRequest")).Return(nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodPatch, "/v1/users/me/password", validReq)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body map[string]string
		decodeJSON(t, resp, &body)
		assert.Equal(t, "password changed successfully", body["message"])
	})

	t.Run("401 if ErrInvalidPassword", func(t *testing.T) {
		svc := new(mockService)
		svc.On("ChangePassword", mock.Anything, "user-123", mock.Anything).Return(ErrInvalidPassword)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodPatch, "/v1/users/me/password", validReq)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("400 if ErrPasswordMismatch", func(t *testing.T) {
		svc := new(mockService)
		svc.On("ChangePassword", mock.Anything, "user-123", mock.Anything).Return(ErrPasswordMismatch)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodPatch, "/v1/users/me/password", validReq)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("404 if ErrNotFound", func(t *testing.T) {
		svc := new(mockService)
		svc.On("ChangePassword", mock.Anything, "user-123", mock.Anything).Return(apperr.ErrNotFound)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodPatch, "/v1/users/me/password", validReq)

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("500 for database error", func(t *testing.T) {
		svc := new(mockService)
		svc.On("ChangePassword", mock.Anything, "user-123", mock.Anything).Return(errors.New("db error"))

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodPatch, "/v1/users/me/password", validReq)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("400 if body is not valid JSON", func(t *testing.T) {
		svc := new(mockService)
		app := newTestApp(svc, "user-123")

		req := httptest.NewRequest(http.MethodPatch, "/v1/users/me/password", bytes.NewBufferString("garbage"))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := app.Test(req) //nolint:bodyclose // closed via t.Cleanup below
		t.Cleanup(func() { resp.Body.Close() })

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		svc.AssertNotCalled(t, "ChangePassword")
	})
}

// ── DELETE /me ────────────────────────────────────────────────────────────────

func TestHandler_DeleteAccount(t *testing.T) {
	t.Run("204 succeeds", func(t *testing.T) {
		svc := new(mockService)
		svc.On("DeleteAccount", mock.Anything, "user-123").Return(nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodDelete, "/v1/users/me", nil)

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("500 if database error", func(t *testing.T) {
		svc := new(mockService)
		svc.On("DeleteAccount", mock.Anything, "user-123").Return(errors.New("db error"))

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodDelete, "/v1/users/me", nil)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("userID comes from context (auth middleware), not body", func(t *testing.T) {
		svc := new(mockService)
		svc.On("DeleteAccount", mock.Anything, "context-user").Return(nil)

		app := newTestApp(svc, "context-user")
		resp := doRequest(t, app, http.MethodDelete, "/v1/users/me", map[string]string{
			"user_id": "attacker-trying-to-override",
		})

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
		svc.AssertCalled(t, "DeleteAccount", mock.Anything, "context-user")
	})
}

// ── GET /search ───────────────────────────────────────────────────────────────

func TestHandler_Search(t *testing.T) {
	t.Run("200 with search results", func(t *testing.T) {
		svc := new(mockService)
		expected := &SearchResponse{
			Data:       []*Summary{{ID: "x", Username: "xena"}},
			NextCursor: nil,
		}
		svc.On("Search", mock.Anything, "user-123", mock.AnythingOfType("*user.SearchQuery")).Return(expected, nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodGet, "/v1/users/search?q=xena&limit=10", nil)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body SearchResponse
		decodeJSON(t, resp, &body)
		assert.Len(t, body.Data, 1)
		assert.Nil(t, body.NextCursor)
	})

	t.Run("200 with next_cursor if there is next page", func(t *testing.T) {
		svc := new(mockService)
		cursor := "last-id"
		expected := &SearchResponse{
			Data:       []*Summary{{ID: "a"}, {ID: "b"}},
			NextCursor: &cursor,
		}
		svc.On("Search", mock.Anything, "user-123", mock.AnythingOfType("*user.SearchQuery")).Return(expected, nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodGet, "/v1/users/search?q=b&limit=2", nil)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body SearchResponse
		decodeJSON(t, resp, &body)
		require.NotNil(t, body.NextCursor)
		assert.Equal(t, "last-id", *body.NextCursor)
	})

	t.Run("200 empty results (not 404)", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Search", mock.Anything, "user-123", mock.Anything).Return(&SearchResponse{Data: []*Summary{}}, nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodGet, "/v1/users/search?q=nobody", nil)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("500 if service error", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Search", mock.Anything, "user-123", mock.Anything).Return((*SearchResponse)(nil), errors.New("db timeout"))

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodGet, "/v1/users/search?q=x", nil)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("userID forwarded to service to exclude self from results", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Search", mock.Anything, "user-123", mock.MatchedBy(func(q *SearchQuery) bool {
			return q.Q == "john"
		})).Return(&SearchResponse{Data: []*Summary{}}, nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodGet, "/v1/users/search?q=john&limit=10", nil)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		svc.AssertExpectations(t)
	})
}

// ── GET /:id ──────────────────────────────────────────────────────────────────

func TestHandler_FindByID(t *testing.T) {
	t.Run("200 user found", func(t *testing.T) {
		svc := new(mockService)
		u := stubUser()
		// Handler memanggil service.FindUserByID yang return *Response, bukan *User.
		// Kita buat Response yang sesuai dengan data stubUser.
		expected := &Response{
			ID:       u.ID,
			Name:     u.Name,
			Username: u.Username,
		}
		svc.On("FindUserByID", mock.Anything, "caller-id", &GetUserURI{ID: u.ID}).Return(expected, nil)

		app := newTestApp(svc, "caller-id")
		resp := doRequest(t, app, http.MethodGet, "/v1/users/"+u.ID, nil)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body Response
		decodeJSON(t, resp, &body)
		assert.Equal(t, u.ID, body.ID)
	})

	t.Run("404 user not found", func(t *testing.T) {
		svc := new(mockService)
		svc.On("FindUserByID", mock.Anything, "caller-id", &GetUserURI{ID: "ghost"}).Return((*Response)(nil), apperr.ErrNotFound)

		app := newTestApp(svc, "caller-id")
		resp := doRequest(t, app, http.MethodGet, "/v1/users/ghost", nil)

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		var body map[string]string
		decodeJSON(t, resp, &body)
		assert.Equal(t, "not found", body["error"])
	})

	t.Run("404 if target has blocked requester", func(t *testing.T) {
		svc := new(mockService)
		svc.On("FindUserByID", mock.Anything, "caller-id", mock.Anything).Return((*Response)(nil), apperr.ErrNotFound)

		app := newTestApp(svc, "caller-id")
		resp := doRequest(t, app, http.MethodGet, "/v1/users/blocker-id", nil)

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("500 internal error returns generic message", func(t *testing.T) {
		svc := new(mockService)
		svc.On("FindUserByID", mock.Anything, mock.Anything, mock.Anything).Return(
			(*Response)(nil),
			errors.New("pq: could not connect to server"),
		)

		app := newTestApp(svc, "caller-id")
		resp := doRequest(t, app, http.MethodGet, "/v1/users/some-id", nil)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		var body map[string]string
		decodeJSON(t, resp, &body)
		assert.Equal(t, "internal server error", body["error"])
	})

	t.Run("password field does not appear in response", func(t *testing.T) {
		svc := new(mockService)
		expected := &Response{
			ID:       "user-123",
			Name:     "John Doe",
			Username: "johndoe",
		}
		svc.On("FindUserByID", mock.Anything, mock.Anything, mock.Anything).Return(expected, nil)

		app := newTestApp(svc, "caller-id")
		resp := doRequest(t, app, http.MethodGet, "/v1/users/user-123", nil)

		var rawBody map[string]any
		decodeJSON(t, resp, &rawBody)
		_, hasPassword := rawBody["password"]
		assert.False(t, hasPassword, "'password' field must not appear in response JSON")
	})
}
