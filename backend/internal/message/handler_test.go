package message

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── mock service ──────────────────────────────────────────────────────────────

type mockHandlerService struct {
	sendMessageFn   func(ctx context.Context, userID, conversationID string, req SendRequest) (*Response, error)
	listMessagesFn  func(ctx context.Context, userID, conversationID string, query ListQuery) (*ListResponse, error)
	editMessageFn   func(ctx context.Context, userID, conversationID, messageID string, req EditRequest) (*Response, error)
	deleteMessageFn func(ctx context.Context, userID, conversationID, messageID string) (*Response, error)
	markAsReadFn    func(ctx context.Context, userID, conversationID, messageID string) (*Response, error)
}

func (m *mockHandlerService) MarkAsRead(ctx context.Context, userID, conversationID, messageID string) (*Response, error) {
	if m.markAsReadFn != nil {
		return m.markAsReadFn(ctx, userID, conversationID, messageID)
	}
	content := "test"
	return &Response{ID: "msg-1", ConversationID: conversationID, SenderID: userID, Content: &content}, nil
}

func (m *mockHandlerService) SendMessage(ctx context.Context, userID, conversationID string, req SendRequest) (*Response, error) {
	if m.sendMessageFn != nil {
		return m.sendMessageFn(ctx, userID, conversationID, req)
	}
	content := "halo!"
	return &Response{ID: "msg-1", ConversationID: conversationID, SenderID: userID, Content: &content}, nil
}

func (m *mockHandlerService) ListMessages(ctx context.Context, userID, conversationID string, query ListQuery) (*ListResponse, error) {
	if m.listMessagesFn != nil {
		return m.listMessagesFn(ctx, userID, conversationID, query)
	}
	return &ListResponse{Data: []*Response{}, HasMore: false}, nil
}

func (m *mockHandlerService) EditMessage(ctx context.Context, userID, conversationID, messageID string, req EditRequest) (*Response, error) {
	if m.editMessageFn != nil {
		return m.editMessageFn(ctx, userID, conversationID, messageID, req)
	}
	content := req.Content
	return &Response{ID: messageID, ConversationID: conversationID, SenderID: userID, Content: &content, IsEdited: true}, nil
}

func (m *mockHandlerService) DeleteMessage(ctx context.Context, userID, conversationID, messageID string) (*Response, error) {
	if m.deleteMessageFn != nil {
		return m.deleteMessageFn(ctx, userID, conversationID, messageID)
	}
	return &Response{ID: messageID, ConversationID: conversationID, SenderID: userID, IsDeleted: true}, nil
}

// ── setup ─────────────────────────────────────────────────────────────────────

const (
	handlerUserID  = "user-aaa"
	handlerConvoID = "conv-111"
	handlerMsgID   = "msg-222"
)

func newTestApp(svc ServiceInterface) *fiber.App {
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user_id", handlerUserID)
		return c.Next()
	})
	h := NewHandler(svc)
	h.RegisterRoutes(app.Group("/conversations"))
	return app
}

func doRequest(t *testing.T, app *fiber.App, method, url string, body []byte) *http.Response {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, url, nil)
	}
	resp, _ := app.Test(req) //nolint:bodyclose // closed via t.Cleanup below
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func strPtr(s string) *string { return &s }

// ── SendMessage ───────────────────────────────────────────────────────────────

func TestHandler_SendMessage(t *testing.T) {
	url := "/conversations/" + handlerConvoID + "/messages"
	validBody, _ := json.Marshal(SendRequest{Content: "halo!"})

	tests := []struct {
		name       string
		body       []byte
		setupSvc   func(*mockHandlerService)
		wantStatus int
	}{
		{
			name:       "happy path returns 201",
			body:       validBody,
			wantStatus: http.StatusCreated,
		},
		{
			name: "ErrNotFound returns 404",
			body: validBody,
			setupSvc: func(m *mockHandlerService) {
				m.sendMessageFn = func(_ context.Context, _, _ string, _ SendRequest) (*Response, error) {
					return nil, apperr.ErrNotFound
				}
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "ErrForbidden returns 403",
			body: validBody,
			setupSvc: func(m *mockHandlerService) {
				m.sendMessageFn = func(_ context.Context, _, _ string, _ SendRequest) (*Response, error) {
					return nil, apperr.ErrForbidden
				}
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "ErrBadRequest returns 400",
			body: validBody,
			setupSvc: func(m *mockHandlerService) {
				m.sendMessageFn = func(_ context.Context, _, _ string, _ SendRequest) (*Response, error) {
					return nil, apperr.ErrBadRequest
				}
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "unknown error returns 500",
			body: validBody,
			setupSvc: func(m *mockHandlerService) {
				m.sendMessageFn = func(_ context.Context, _, _ string, _ SendRequest) (*Response, error) {
					return nil, assert.AnError
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mockHandlerService{}
			if tt.setupSvc != nil {
				tt.setupSvc(svc)
			}
			resp := doRequest(t, newTestApp(svc), http.MethodPost, url, tt.body)
			assert.Equal(t, tt.wantStatus, resp.StatusCode)
			if resp.StatusCode >= 400 {
				var body map[string]any
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
				assert.Contains(t, body, "error")
			}
		})
	}
}

// ── ListMessages ──────────────────────────────────────────────────────────────

func TestHandler_ListMessages(t *testing.T) {
	url := "/conversations/" + handlerConvoID + "/messages"

	tests := []struct {
		name       string
		query      string
		setupSvc   func(*mockHandlerService)
		wantStatus int
	}{
		{
			name:       "happy path returns 200",
			wantStatus: http.StatusOK,
		},
		{
			name:       "with cursor and limit query params returns 200",
			query:      "?cursor=msg-abc&limit=10",
			wantStatus: http.StatusOK,
		},
		{
			name: "ErrNotFound returns 404",
			setupSvc: func(m *mockHandlerService) {
				m.listMessagesFn = func(_ context.Context, _, _ string, _ ListQuery) (*ListResponse, error) {
					return nil, apperr.ErrNotFound
				}
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "unknown error returns 500",
			setupSvc: func(m *mockHandlerService) {
				m.listMessagesFn = func(_ context.Context, _, _ string, _ ListQuery) (*ListResponse, error) {
					return nil, assert.AnError
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mockHandlerService{}
			if tt.setupSvc != nil {
				tt.setupSvc(svc)
			}
			resp := doRequest(t, newTestApp(svc), http.MethodGet, url+tt.query, nil)
			assert.Equal(t, tt.wantStatus, resp.StatusCode)
			if resp.StatusCode >= 400 {
				var body map[string]any
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
				assert.Contains(t, body, "error")
			}
		})
	}
}

// ── EditMessage ───────────────────────────────────────────────────────────────

func TestHandler_EditMessage(t *testing.T) {
	url := "/conversations/" + handlerConvoID + "/messages/" + handlerMsgID
	validBody, _ := json.Marshal(EditRequest{Content: "edited content"})

	tests := []struct {
		name       string
		body       []byte
		setupSvc   func(*mockHandlerService)
		wantStatus int
	}{
		{
			name:       "happy path returns 200",
			body:       validBody,
			wantStatus: http.StatusOK,
		},
		{
			name: "ErrNotFound returns 404",
			body: validBody,
			setupSvc: func(m *mockHandlerService) {
				m.editMessageFn = func(_ context.Context, _, _, _ string, _ EditRequest) (*Response, error) {
					return nil, apperr.ErrNotFound
				}
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "ErrForbidden returns 403",
			body: validBody,
			setupSvc: func(m *mockHandlerService) {
				m.editMessageFn = func(_ context.Context, _, _, _ string, _ EditRequest) (*Response, error) {
					return nil, apperr.ErrForbidden
				}
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "unknown error returns 500",
			body: validBody,
			setupSvc: func(m *mockHandlerService) {
				m.editMessageFn = func(_ context.Context, _, _, _ string, _ EditRequest) (*Response, error) {
					return nil, assert.AnError
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mockHandlerService{}
			if tt.setupSvc != nil {
				tt.setupSvc(svc)
			}
			resp := doRequest(t, newTestApp(svc), http.MethodPatch, url, tt.body)
			assert.Equal(t, tt.wantStatus, resp.StatusCode)
			if resp.StatusCode >= 400 {
				var body map[string]any
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
				assert.Contains(t, body, "error")
			}
		})
	}
}

// ── DeleteMessage ─────────────────────────────────────────────────────────────

func TestHandler_DeleteMessage(t *testing.T) {
	url := "/conversations/" + handlerConvoID + "/messages/" + handlerMsgID

	tests := []struct {
		name       string
		setupSvc   func(*mockHandlerService)
		wantStatus int
	}{
		{
			name:       "happy path returns 200",
			wantStatus: http.StatusOK,
		},
		{
			name: "ErrNotFound returns 404",
			setupSvc: func(m *mockHandlerService) {
				m.deleteMessageFn = func(_ context.Context, _, _, _ string) (*Response, error) {
					return nil, apperr.ErrNotFound
				}
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "ErrForbidden returns 403",
			setupSvc: func(m *mockHandlerService) {
				m.deleteMessageFn = func(_ context.Context, _, _, _ string) (*Response, error) {
					return nil, apperr.ErrForbidden
				}
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "ErrConflict returns 409",
			setupSvc: func(m *mockHandlerService) {
				m.deleteMessageFn = func(_ context.Context, _, _, _ string) (*Response, error) {
					return nil, apperr.ErrConflict
				}
			},
			wantStatus: http.StatusConflict,
		},
		{
			name: "unknown error returns 500",
			setupSvc: func(m *mockHandlerService) {
				m.deleteMessageFn = func(_ context.Context, _, _, _ string) (*Response, error) {
					return nil, assert.AnError
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mockHandlerService{}
			if tt.setupSvc != nil {
				tt.setupSvc(svc)
			}
			resp := doRequest(t, newTestApp(svc), http.MethodDelete, url, nil)
			assert.Equal(t, tt.wantStatus, resp.StatusCode)
			if resp.StatusCode >= 400 {
				var body map[string]any
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
				assert.Contains(t, body, "error")
			}
		})
	}
}

// ── SendMessage with attachment ───────────────────────────────────────────

func TestHandler_SendMessage_WithAttachmentID_Returns201(t *testing.T) {
	url := "/conversations/" + handlerConvoID + "/messages"

	size := int64(1024)
	body, _ := json.Marshal(SendRequest{
		AttachmentID: strPtr("att-abc"),
		Filename:     strPtr("photo.jpg"),
		MIMEType:     strPtr("image/jpeg"),
		Size:         &size,
	})

	svc := &mockHandlerService{}
	svc.sendMessageFn = func(_ context.Context, _, _ string, req SendRequest) (*Response, error) {
		if req.AttachmentID == nil || *req.AttachmentID != "att-abc" {
			return nil, apperr.ErrBadRequest
		}
		return &Response{ID: "msg-1", ConversationID: handlerConvoID, SenderID: handlerUserID}, nil
	}

	resp := doRequest(t, newTestApp(svc), http.MethodPost, url, body)

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestHandler_SendMessage_NoContentNoAttachment_Returns400(t *testing.T) {
	url := "/conversations/" + handlerConvoID + "/messages"

	body, _ := json.Marshal(SendRequest{})

	svc := &mockHandlerService{}
	svc.sendMessageFn = func(_ context.Context, _, _ string, _ SendRequest) (*Response, error) {
		return nil, apperr.ErrBadRequest
	}

	resp := doRequest(t, newTestApp(svc), http.MethodPost, url, body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var respBody map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&respBody))
	assert.Contains(t, respBody, "error")
}
