package conversation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	. "github.com/BleKuntay/FlipChat/backend/internal/conversation"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
)

// ── mock service ──────────────────────────────────────────────────────────────

type mockService struct{ mock.Mock }

func (m *mockService) GetConversationList(ctx context.Context, requesterID string) (*ListResponse, error) {
	args := m.Called(ctx, requesterID)
	if r, ok := args.Get(0).(*ListResponse); ok {
		return r, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockService) GetConversation(ctx context.Context, requesterID, conversationID string) (*Response, error) {
	args := m.Called(ctx, requesterID, conversationID)
	if r, ok := args.Get(0).(*Response); ok {
		return r, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockService) CreateConversation(ctx context.Context, requesterID, targetUserID string) (*Response, bool, error) {
	args := m.Called(ctx, requesterID, targetUserID)
	if r, ok := args.Get(0).(*Response); ok {
		return r, args.Bool(1), args.Error(2)
	}
	return nil, false, args.Error(2)
}

// ── test app factory ──────────────────────────────────────────────────────────

func newTestApp(svc ServiceInterface, userID string) *fiber.App {
	app := fiber.New()

	app.Use(func(c fiber.Ctx) error {
		fiber.Locals[string](c, "user_id", userID)
		return c.Next()
	})

	h := NewHandler(svc)
	v1 := app.Group("/v1/conversations")
	h.RegisterRoute(v1)

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

// ── GET / ─────────────────────────────────────────────────────────────────────

func TestHandler_GetConversationList(t *testing.T) {
	t.Run("200 returns conversation list", func(t *testing.T) {
		svc := new(mockService)
		expected := &ListResponse{
			Data: []Response{
				{ConversationID: "conv-1", Name: "Bob", Username: "bob"},
				{ConversationID: "conv-2", Name: "Charlie", Username: "charlie"},
			},
		}
		svc.On("GetConversationList", mock.Anything, "user-123").Return(expected, nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodGet, "/v1/conversations/", nil)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body ListResponse
		decodeJSON(t, resp, &body)
		assert.Len(t, body.Data, 2)
		svc.AssertExpectations(t)
	})

	t.Run("200 returns empty list when no conversations", func(t *testing.T) {
		svc := new(mockService)
		svc.On("GetConversationList", mock.Anything, "user-123").Return(&ListResponse{Data: []Response{}}, nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodGet, "/v1/conversations/", nil)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body ListResponse
		decodeJSON(t, resp, &body)
		assert.Empty(t, body.Data)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := new(mockService)
		svc.On("GetConversationList", mock.Anything, "user-123").Return(nil, errors.New("db error"))

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodGet, "/v1/conversations/", nil)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("userID comes from auth context", func(t *testing.T) {
		svc := new(mockService)
		svc.On("GetConversationList", mock.Anything, "context-user").Return(&ListResponse{Data: []Response{}}, nil)

		app := newTestApp(svc, "context-user")
		resp := doRequest(t, app, http.MethodGet, "/v1/conversations/", nil)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		svc.AssertCalled(t, "GetConversationList", mock.Anything, "context-user")
	})
}

// ── GET /:id ──────────────────────────────────────────────────────────────────

func TestHandler_GetConversation(t *testing.T) {
	t.Run("200 returns conversation", func(t *testing.T) {
		svc := new(mockService)
		expected := &Response{ConversationID: "conv-1", Name: "Bob", Username: "bob"}
		svc.On("GetConversation", mock.Anything, "user-123", "conv-1").Return(expected, nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodGet, "/v1/conversations/conv-1", nil)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body Response
		decodeJSON(t, resp, &body)
		assert.Equal(t, "conv-1", body.ConversationID)
		assert.Equal(t, "Bob", body.Name)
		svc.AssertExpectations(t)
	})

	t.Run("404 when not participant or not found", func(t *testing.T) {
		svc := new(mockService)
		svc.On("GetConversation", mock.Anything, "outsider", "conv-1").Return(nil, apperr.ErrNotFound)

		app := newTestApp(svc, "outsider")
		resp := doRequest(t, app, http.MethodGet, "/v1/conversations/conv-1", nil)

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := new(mockService)
		svc.On("GetConversation", mock.Anything, "user-123", "conv-1").Return(nil, errors.New("db error"))

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodGet, "/v1/conversations/conv-1", nil)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}

// ── POST / ────────────────────────────────────────────────────────────────────

func TestHandler_CreateConversation(t *testing.T) {
	t.Run("201 when conversation is newly created", func(t *testing.T) {
		svc := new(mockService)
		expected := &Response{ConversationID: "conv-new", Name: "Bob", Username: "bob"}
		svc.On("CreateConversation", mock.Anything, "user-123", "target-456").Return(expected, true, nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodPost, "/v1/conversations/", map[string]string{
			"target_user_id": "target-456",
		})

		assert.Equal(t, http.StatusCreated, resp.StatusCode)
		var body Response
		decodeJSON(t, resp, &body)
		assert.Equal(t, "conv-new", body.ConversationID)
		svc.AssertExpectations(t)
	})

	t.Run("200 when conversation already exists", func(t *testing.T) {
		svc := new(mockService)
		expected := &Response{ConversationID: "conv-existing", Name: "Bob", Username: "bob"}
		svc.On("CreateConversation", mock.Anything, "user-123", "target-456").Return(expected, false, nil)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodPost, "/v1/conversations/", map[string]string{
			"target_user_id": "target-456",
		})

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var body Response
		decodeJSON(t, resp, &body)
		assert.Equal(t, "conv-existing", body.ConversationID)
	})

	t.Run("400 if body is invalid JSON", func(t *testing.T) {
		svc := new(mockService)
		app := newTestApp(svc, "user-123")

		req := httptest.NewRequest(http.MethodPost, "/v1/conversations/", bytes.NewBufferString("not-json"))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := app.Test(req) //nolint:bodyclose // closed via t.Cleanup below
		t.Cleanup(func() { resp.Body.Close() })

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		svc.AssertNotCalled(t, "CreateConversation")
	})

	t.Run("400 if self-conversation", func(t *testing.T) {
		svc := new(mockService)
		svc.On("CreateConversation", mock.Anything, "user-123", "user-123").Return(nil, false, apperr.ErrBadRequest)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodPost, "/v1/conversations/", map[string]string{
			"target_user_id": "user-123",
		})

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("404 if target has blocked requester", func(t *testing.T) {
		svc := new(mockService)
		svc.On("CreateConversation", mock.Anything, "user-123", "blocker-id").Return(nil, false, apperr.ErrNotFound)

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodPost, "/v1/conversations/", map[string]string{
			"target_user_id": "blocker-id",
		})

		// indistinguishable from not found — by design
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := new(mockService)
		svc.On("CreateConversation", mock.Anything, "user-123", "target-456").Return(nil, false, errors.New("db error"))

		app := newTestApp(svc, "user-123")
		resp := doRequest(t, app, http.MethodPost, "/v1/conversations/", map[string]string{
			"target_user_id": "target-456",
		})

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("requesterID comes from auth context not body", func(t *testing.T) {
		svc := new(mockService)
		expected := &Response{ConversationID: "conv-new", Name: "Bob", Username: "bob"}
		svc.On("CreateConversation", mock.Anything, "context-user", "target-456").Return(expected, true, nil)

		app := newTestApp(svc, "context-user")
		resp := doRequest(t, app, http.MethodPost, "/v1/conversations/", map[string]string{
			"target_user_id": "target-456",
		})

		assert.Equal(t, http.StatusCreated, resp.StatusCode)
		svc.AssertCalled(t, "CreateConversation", mock.Anything, "context-user", "target-456")
	})
}
