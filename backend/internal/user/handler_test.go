package user_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	. "github.com/BleKuntay/FlipChat/backend/internal/user"
)

// ── mock service ──────────────────────────────────────────────────────────────

type mockService struct{ mock.Mock }

func (m *mockService) Me(userID string) (*MeResponse, error) {
	args := m.Called(userID)
	if r, ok := args.Get(0).(*MeResponse); ok {
		return r, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockService) UpdateProfile(userID string, req *UpdateProfileRequest) (*UpdateProfileResponse, error) {
	args := m.Called(userID, req)
	if r, ok := args.Get(0).(*UpdateProfileResponse); ok {
		return r, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockService) UpdateEmail(userID string, req *UpdateEmailRequest) (*UpdateEmailResponse, error) {
	args := m.Called(userID, req)
	if r, ok := args.Get(0).(*UpdateEmailResponse); ok {
		return r, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockService) ChangePassword(userID string, req *ChangePasswordRequest) error {
	return m.Called(userID, req).Error(0)
}

func (m *mockService) DeleteAccount(userID string) error {
	return m.Called(userID).Error(0)
}

func (m *mockService) FindUserByID(param *GetUserURI) (*User, error) {
	args := m.Called(param)
	if u, ok := args.Get(0).(*User); ok {
		return u, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockService) Search(userID string, query *SearchQuery) (*SearchResponse, error) {
	args := m.Called(userID, query)
	if r, ok := args.Get(0).(*SearchResponse); ok {
		return r, args.Error(1)
	}
	return nil, args.Error(1)
}

// ── test app factory ──────────────────────────────────────────────────────────

// newTestApp creates Fiber app with Handler attached and minimal middleware
// that injects user_id into context (simulating auth middleware).
func newTestApp(svc ServiceInterface, userID string) *fiber.App {
	app := fiber.New(fiber.Config{
		// Disable logging for clean test output
	})

	// Simulate auth middleware
	app.Use(func(c fiber.Ctx) error {
		fiber.Locals[string](c, "user_id", userID)
		return c.Next()
	})

	h := NewHandler(svc)
	v1 := app.Group("/v1/users")
	h.RegisterRoutes(v1)

	return app
}

func doRequest(app *fiber.App, method, path string, body any) *http.Response {
	var bodyBytes []byte
	if body != nil {
		bodyBytes, _ = json.Marshal(body)
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)
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
		svc.On("Me", "user-123").Return(expected, nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(app, http.MethodGet, "/v1/users/me", nil)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body MeResponse
		decodeJSON(t, resp, &body)
		assert.Equal(t, "john@example.com", body.Email)
	})

	// BUG DOCUMENTED: Handler returns 404 for all errors from service,
	// including database errors that should be 500.
	t.Run("404 if ErrUserNotFound", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Me", "ghost").Return((*MeResponse)(nil), ErrUserNotFound)

		app := newTestApp(svc, "ghost")
		resp := doRequest(app, http.MethodGet, "/v1/users/me", nil)

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	// BUG DOCUMENTED: Database error also results in 404, not 500.
	// This hides infrastructure failures from monitoring.
	t.Run("404 (should be 500) if database error", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Me", "user-123").Return((*MeResponse)(nil), errors.New("db down"))

		app := newTestApp(svc, "user-123")
		resp := doRequest(app, http.MethodGet, "/v1/users/me", nil)

		// Document bug: should be 500
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, "BUG: should be 500 for database error")
	})
}

// ── PATCH /me ─────────────────────────────────────────────────────────────────

func TestHandler_UpdateProfile(t *testing.T) {
	t.Run("200 update succeeds", func(t *testing.T) {
		svc := new(mockService)
		req := &UpdateProfileRequest{Name: "Jane Doe"}
		res := &UpdateProfileResponse{Response: Response{ID: "user-123", Name: "Jane Doe"}}
		svc.On("UpdateProfile", "user-123", mock.AnythingOfType("*user.UpdateProfileRequest")).Return(res, nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(app, http.MethodPatch, "/v1/users/me", req)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("400 if body is not valid JSON", func(t *testing.T) {
		svc := new(mockService)
		app := newTestApp(svc, "user-123")

		req := httptest.NewRequest(http.MethodPatch, "/v1/users/me", bytes.NewBufferString("not-json"))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := app.Test(req)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		svc.AssertNotCalled(t, "UpdateProfile")
	})

	// BUG DOCUMENTED: Handler returns 404 for all service errors,
	// but ErrUserNotUpdated (empty body) should be 400,
	// and database error should be 500.
	t.Run("404 (should be 400) if ErrUserNotUpdated", func(t *testing.T) {
		svc := new(mockService)
		svc.On("UpdateProfile", "user-123", mock.Anything).Return((*UpdateProfileResponse)(nil), ErrUserNotUpdated)

		app := newTestApp(svc, "user-123")
		resp := doRequest(app, http.MethodPatch, "/v1/users/me", map[string]string{})

		assert.Equal(t, http.StatusNotFound, resp.StatusCode, "BUG: ErrUserNotUpdated should be 400")
	})
}

// ── PATCH /me/email ───────────────────────────────────────────────────────────

func TestHandler_UpdateEmail(t *testing.T) {
	t.Run("200 email update succeeds", func(t *testing.T) {
		svc := new(mockService)
		req := &UpdateEmailRequest{NewEmail: "newemail@example.com", CurrentPassword: "secret"}
		res := &UpdateEmailResponse{Email: "newemail@example.com"}
		svc.On("UpdateEmail", "user-123", mock.AnythingOfType("*user.UpdateEmailRequest")).Return(res, nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(app, http.MethodPatch, "/v1/users/me/email", req)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	// BUG DOCUMENTED: Handler does not distinguish ErrInvalidPassword from other errors.
	// ErrInvalidPassword should be 401/400, not 500.
	t.Run("500 (should be 401) if ErrInvalidPassword", func(t *testing.T) {
		svc := new(mockService)
		svc.On("UpdateEmail", "user-123", mock.Anything).Return((*UpdateEmailResponse)(nil), ErrInvalidPassword)

		app := newTestApp(svc, "user-123")
		resp := doRequest(app, http.MethodPatch, "/v1/users/me/email", map[string]string{
			"new_email":        "newemail@example.com",
			"current_password": "wrong",
		})

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode, "BUG: should be 401 for wrong password")
	})

	// BUG DOCUMENTED: Handler also leaks internal error messages to client via err.Error()
	t.Run("500 with error message from service (information leak)", func(t *testing.T) {
		svc := new(mockService)
		svc.On("UpdateEmail", "user-123", mock.Anything).Return(
			(*UpdateEmailResponse)(nil),
			errors.New("pq: duplicate key value violates unique constraint \"users_email_key\""),
		)

		app := newTestApp(svc, "user-123")
		resp := doRequest(app, http.MethodPatch, "/v1/users/me/email", map[string]string{
			"new_email":        "dup@example.com",
			"current_password": "secret",
		})

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

		var body map[string]string
		decodeJSON(t, resp, &body)
		// This documents that DB message leaks to client — must be fixed
		assert.Contains(t, body["error"], "duplicate key", "BUG: internal error message leaks to client")
	})

	t.Run("400 if body is not valid", func(t *testing.T) {
		svc := new(mockService)
		app := newTestApp(svc, "user-123")

		req := httptest.NewRequest(http.MethodPatch, "/v1/users/me/email", bytes.NewBufferString("{invalid}"))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := app.Test(req)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// ── PATCH /me/password ────────────────────────────────────────────────────────

func TestHandler_ChangePassword(t *testing.T) {
	validReq := &ChangePasswordRequest{
		CurrentPassword: "secret123",
		NewPassword:     "newSecret456",
		ConfirmPassword: "newSecret456",
	}

	t.Run("200 succeeds", func(t *testing.T) {
		svc := new(mockService)
		svc.On("ChangePassword", "user-123", mock.AnythingOfType("*user.ChangePasswordRequest")).Return(nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(app, http.MethodPatch, "/v1/users/me/password", validReq)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body map[string]string
		decodeJSON(t, resp, &body)
		assert.Equal(t, "password changed successfully", body["message"])
	})

	t.Run("401 if current password is wrong", func(t *testing.T) {
		svc := new(mockService)
		svc.On("ChangePassword", "user-123", mock.Anything).Return(ErrInvalidPassword)

		app := newTestApp(svc, "user-123")
		resp := doRequest(app, http.MethodPatch, "/v1/users/me/password", validReq)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		var body map[string]string
		decodeJSON(t, resp, &body)
		assert.Equal(t, "invalid current password", body["error"])
	})

	t.Run("400 if new password does not match", func(t *testing.T) {
		svc := new(mockService)
		svc.On("ChangePassword", "user-123", mock.Anything).Return(ErrPasswordMismatch)

		app := newTestApp(svc, "user-123")
		resp := doRequest(app, http.MethodPatch, "/v1/users/me/password", validReq)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		var body map[string]string
		decodeJSON(t, resp, &body)
		assert.Equal(t, "new password does not match", body["error"])
	})

	t.Run("404 if user not found", func(t *testing.T) {
		svc := new(mockService)
		svc.On("ChangePassword", "user-123", mock.Anything).Return(ErrUserNotFound)

		app := newTestApp(svc, "user-123")
		resp := doRequest(app, http.MethodPatch, "/v1/users/me/password", validReq)

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("500 for database error", func(t *testing.T) {
		svc := new(mockService)
		svc.On("ChangePassword", "user-123", mock.Anything).Return(errors.New("db error"))

		app := newTestApp(svc, "user-123")
		resp := doRequest(app, http.MethodPatch, "/v1/users/me/password", validReq)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		var body map[string]string
		decodeJSON(t, resp, &body)
		// Handler correctly returns generic message for 500
		assert.Equal(t, "failed to change password", body["error"])
	})

	t.Run("400 if body is not valid JSON", func(t *testing.T) {
		svc := new(mockService)
		app := newTestApp(svc, "user-123")

		req := httptest.NewRequest(http.MethodPatch, "/v1/users/me/password", bytes.NewBufferString("garbage"))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := app.Test(req)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		svc.AssertNotCalled(t, "ChangePassword")
	})
}

// ── DELETE /me ────────────────────────────────────────────────────────────────

func TestHandler_DeleteAccount(t *testing.T) {
	t.Run("204 succeeds", func(t *testing.T) {
		svc := new(mockService)
		svc.On("DeleteAccount", "user-123").Return(nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(app, http.MethodDelete, "/v1/users/me", nil)

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("500 if database error", func(t *testing.T) {
		svc := new(mockService)
		svc.On("DeleteAccount", "user-123").Return(errors.New("db error"))

		app := newTestApp(svc, "user-123")
		resp := doRequest(app, http.MethodDelete, "/v1/users/me", nil)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	// Verify that userID comes from auth context, not body
	t.Run("userID comes from context (auth middleware), not body", func(t *testing.T) {
		svc := new(mockService)
		svc.On("DeleteAccount", "context-user").Return(nil)

		app := newTestApp(svc, "context-user") // user_id from middleware
		resp := doRequest(app, http.MethodDelete, "/v1/users/me", map[string]string{
			"user_id": "attacker-trying-to-override", // body should be ignored
		})

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
		svc.AssertCalled(t, "DeleteAccount", "context-user")
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
		svc.On("Search", "user-123", mock.AnythingOfType("*user.SearchQuery")).Return(expected, nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(app, http.MethodGet, "/v1/users/search?q=xena&limit=10", nil)

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
		svc.On("Search", "user-123", mock.AnythingOfType("*user.SearchQuery")).Return(expected, nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(app, http.MethodGet, "/v1/users/search?q=b&limit=2", nil)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body SearchResponse
		decodeJSON(t, resp, &body)
		require.NotNil(t, body.NextCursor)
		assert.Equal(t, "last-id", *body.NextCursor)
	})

	t.Run("200 empty results (not 404)", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Search", "user-123", mock.Anything).Return(&SearchResponse{Data: []*Summary{}}, nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(app, http.MethodGet, "/v1/users/search?q=nobody", nil)

		// Important: empty search must be 200 with empty array, not 404
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("500 if service error", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Search", "user-123", mock.Anything).Return((*SearchResponse)(nil), errors.New("db timeout"))

		app := newTestApp(svc, "user-123")
		resp := doRequest(app, http.MethodGet, "/v1/users/search?q=x", nil)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	// Verify that user cannot appear in their own search results
	t.Run("userID forwarded to service to exclude self from results", func(t *testing.T) {
		svc := new(mockService)
		svc.On("Search", "user-123", mock.MatchedBy(func(q *SearchQuery) bool {
			return q.Q == "john"
		})).Return(&SearchResponse{Data: []*Summary{}}, nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(app, http.MethodGet, "/v1/users/search?q=john&limit=10", nil)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		svc.AssertExpectations(t) // Verify userID "user-123" was forwarded
	})
}

// ── GET /:id ──────────────────────────────────────────────────────────────────

func TestHandler_FindByID(t *testing.T) {
	t.Run("200 user found", func(t *testing.T) {
		svc := new(mockService)
		u := stubUser()
		svc.On("FindUserByID", &GetUserURI{ID: u.ID}).Return(u, nil)

		app := newTestApp(svc, "caller-id")
		resp := doRequest(app, http.MethodGet, "/v1/users/"+u.ID, nil)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body User
		decodeJSON(t, resp, &body)
		assert.Equal(t, u.ID, body.ID)
	})

	t.Run("404 user not found", func(t *testing.T) {
		svc := new(mockService)
		svc.On("FindUserByID", &GetUserURI{ID: "ghost"}).Return((*User)(nil), ErrUserNotFound)

		app := newTestApp(svc, "caller-id")
		resp := doRequest(app, http.MethodGet, "/v1/users/ghost", nil)

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		var body map[string]string
		decodeJSON(t, resp, &body)
		assert.Equal(t, "user not found", body["error"])
	})

	// BUG DOCUMENTED: For internal errors, handler returns raw err.Error() to client.
	// This can leak database details (table names, constraint names, etc.)
	t.Run("500 internal error — response contains raw error string (information leak)", func(t *testing.T) {
		svc := new(mockService)
		svc.On("FindUserByID", mock.Anything).Return(
			(*User)(nil),
			errors.New("pq: could not connect to server"),
		)

		app := newTestApp(svc, "caller-id")
		resp := doRequest(app, http.MethodGet, "/v1/users/some-id", nil)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		var body map[string]string
		decodeJSON(t, resp, &body)
		// Document bug: internal message leaks
		assert.Equal(t, "pq: could not connect to server", body["error"],
			"BUG: internal error message leaks to client, should be generic")
	})

	// Security: ensure caller cannot access all user data without restrictions.
	// For MVP there is no authorization check (any logged-in user can view any other user).
	// Document this so Phase 2 can add friendship/block checks.
	t.Run("AUTHORIZATION: any logged-in user can view other users (will need block check in Phase 2)", func(t *testing.T) {
		svc := new(mockService)
		targetUser := stubUser()
		svc.On("FindUserByID", &GetUserURI{ID: targetUser.ID}).Return(targetUser, nil)

		// caller is different from target
		app := newTestApp(svc, "different-user-id")
		resp := doRequest(app, http.MethodGet, "/v1/users/"+targetUser.ID, nil)

		// Currently: allowed — this is expected for MVP
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	// Security: password field must never appear in response JSON
	t.Run("password field does not appear in response", func(t *testing.T) {
		svc := new(mockService)
		u := stubUser()
		u.Password = "hashed-super-secret"
		svc.On("FindUserByID", mock.Anything).Return(u, nil)

		app := newTestApp(svc, "caller-id")
		resp := doRequest(app, http.MethodGet, "/v1/users/"+u.ID, nil)

		var rawBody map[string]any
		decodeJSON(t, resp, &rawBody)
		_, hasPassword := rawBody["password"]
		assert.False(t, hasPassword, "'password' field must not appear in response JSON")
	})
}
