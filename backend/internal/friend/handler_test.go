package friend

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockService struct {
	findAllRequestsFn      func(ctx context.Context, userID string, query RequestListQuery) (*RequestListResponse, error)
	findAllFn              func(ctx context.Context, userID string, query ListQuery) (*ListResponse, error)
	findOneFn              func(ctx context.Context, userID, targetID string) (*StatusResponse, error)
	addFriendFn            func(ctx context.Context, userID, targetID string) (*PendingResponse, error)
	unfriendFn             func(ctx context.Context, userID, targetID string) error
	cancelFriendRequestFn  func(ctx context.Context, userID, targetID string) error
	acceptFriendRequestFn  func(ctx context.Context, userID, targetID string) (*Response, error)
	declineFriendRequestFn func(ctx context.Context, userID, targetID string) error
}

func (m *mockService) FindAllRequests(ctx context.Context, userID string, query RequestListQuery) (*RequestListResponse, error) {
	if m.findAllRequestsFn != nil {
		return m.findAllRequestsFn(ctx, userID, query)
	}
	return &RequestListResponse{Requests: []PendingResponse{}}, nil
}

func (m *mockService) FindAll(ctx context.Context, userID string, query ListQuery) (*ListResponse, error) {
	if m.findAllFn != nil {
		return m.findAllFn(ctx, userID, query)
	}
	return &ListResponse{Friends: []Response{}}, nil
}

func (m *mockService) FindOne(ctx context.Context, userID, targetID string) (*StatusResponse, error) {
	if m.findOneFn != nil {
		return m.findOneFn(ctx, userID, targetID)
	}
	return &StatusResponse{Status: StatusNone}, nil
}

func (m *mockService) AddFriend(ctx context.Context, userID, targetID string) (*PendingResponse, error) {
	if m.addFriendFn != nil {
		return m.addFriendFn(ctx, userID, targetID)
	}
	return &PendingResponse{UserID: targetID, Direction: "sent", CreatedAt: time.Now()}, nil
}

func (m *mockService) Unfriend(ctx context.Context, userID, targetID string) error {
	if m.unfriendFn != nil {
		return m.unfriendFn(ctx, userID, targetID)
	}
	return nil
}

func (m *mockService) CancelFriendRequest(ctx context.Context, userID, targetID string) error {
	if m.cancelFriendRequestFn != nil {
		return m.cancelFriendRequestFn(ctx, userID, targetID)
	}
	return nil
}

func (m *mockService) AcceptFriendRequest(ctx context.Context, userID, targetID string) (*Response, error) {
	if m.acceptFriendRequestFn != nil {
		return m.acceptFriendRequestFn(ctx, userID, targetID)
	}
	return &Response{UserID: targetID, FriendSince: time.Now()}, nil
}

func (m *mockService) DeclineFriendRequest(ctx context.Context, userID, targetID string) error {
	if m.declineFriendRequestFn != nil {
		return m.declineFriendRequestFn(ctx, userID, targetID)
	}
	return nil
}

func newMockService() *mockService {
	return &mockService{}
}

// ------------------------------------------------------------------ //
// Setup                                                                 //
// ------------------------------------------------------------------ //

func newTestApp(svc ServiceInterface) *fiber.App {
	app := fiber.New()
	// simulasi auth middleware
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user_id", userA)
		return c.Next()
	})
	h := NewHandler(svc)
	h.RegisterRoutes(app.Group("/friends"))
	return app
}

func doRequest(app *fiber.App, method, url string) *http.Response {
	req := httptest.NewRequest(method, url, nil)
	resp, _ := app.Test(req)
	return resp
}

// ------------------------------------------------------------------ //
// Tests                                                                 //
// ------------------------------------------------------------------ //

func TestHandler_FindAll(t *testing.T) {
	tests := []struct {
		name       string
		setupSvc   func(*mockService)
		wantStatus int
	}{
		{
			name:       "happy path returns 200",
			wantStatus: http.StatusOK,
		},
		{
			name: "service error returns 500",
			setupSvc: func(m *mockService) {
				m.findAllFn = func(_ context.Context, _ string, _ ListQuery) (*ListResponse, error) {
					return nil, assert.AnError
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newMockService()
			if tt.setupSvc != nil {
				tt.setupSvc(svc)
			}
			app := newTestApp(svc)

			resp := doRequest(app, http.MethodGet, "/friends/")

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}

func TestHandler_ListRequests(t *testing.T) {
	tests := []struct {
		name       string
		setupSvc   func(*mockService)
		wantStatus int
	}{
		{
			name:       "happy path returns 200",
			wantStatus: http.StatusOK,
		},
		{
			name: "service error returns 500",
			setupSvc: func(m *mockService) {
				m.findAllRequestsFn = func(_ context.Context, _ string, _ RequestListQuery) (*RequestListResponse, error) {
					return nil, assert.AnError
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newMockService()
			if tt.setupSvc != nil {
				tt.setupSvc(svc)
			}
			app := newTestApp(svc)

			resp := doRequest(app, http.MethodGet, "/friends/requests")

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}

func TestHandler_FindOne(t *testing.T) {
	url := fmt.Sprintf("/friends/%s", userB)

	tests := []struct {
		name        string
		setupSvc    func(*mockService)
		wantStatus  int
		wantBodyKey string
	}{
		{
			name:        "happy path returns 200 dengan status field",
			wantStatus:  http.StatusOK,
			wantBodyKey: "status",
		},
		{
			name: "service error returns 500",
			setupSvc: func(m *mockService) {
				m.findOneFn = func(_ context.Context, _, _ string) (*StatusResponse, error) {
					return nil, assert.AnError
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newMockService()
			if tt.setupSvc != nil {
				tt.setupSvc(svc)
			}
			app := newTestApp(svc)

			resp := doRequest(app, http.MethodGet, url)

			assert.Equal(t, tt.wantStatus, resp.StatusCode)

			if tt.wantBodyKey != "" {
				var body map[string]any
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
				assert.Contains(t, body, tt.wantBodyKey)
			}
		})
	}
}

func TestHandler_AddFriend(t *testing.T) {
	url := fmt.Sprintf("/friends/%s", userB)

	tests := []struct {
		name       string
		setupSvc   func(*mockService)
		wantStatus int
	}{
		{
			name:       "happy path returns 201",
			wantStatus: http.StatusCreated,
		},
		{
			name: "ErrNotFound returns 404",
			setupSvc: func(m *mockService) {
				m.addFriendFn = func(_ context.Context, _, _ string) (*PendingResponse, error) {
					return nil, apperr.ErrNotFound
				}
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "ErrConflict returns 409",
			setupSvc: func(m *mockService) {
				m.addFriendFn = func(_ context.Context, _, _ string) (*PendingResponse, error) {
					return nil, apperr.ErrConflict
				}
			},
			wantStatus: http.StatusConflict,
		},
		{
			name: "ErrForbidden returns 403",
			setupSvc: func(m *mockService) {
				m.addFriendFn = func(_ context.Context, _, _ string) (*PendingResponse, error) {
					return nil, apperr.ErrForbidden
				}
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "ErrBadRequest returns 400",
			setupSvc: func(m *mockService) {
				m.addFriendFn = func(_ context.Context, _, _ string) (*PendingResponse, error) {
					return nil, apperr.ErrBadRequest
				}
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "unknown error returns 500 tanpa bocorkan pesan internal",
			setupSvc: func(m *mockService) {
				m.addFriendFn = func(_ context.Context, _, _ string) (*PendingResponse, error) {
					return nil, assert.AnError
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newMockService()
			if tt.setupSvc != nil {
				tt.setupSvc(svc)
			}
			app := newTestApp(svc)

			resp := doRequest(app, http.MethodPost, url)

			assert.Equal(t, tt.wantStatus, resp.StatusCode)

			// pastikan error response selalu punya field "error"
			if resp.StatusCode >= 400 {
				var body map[string]any
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
				assert.Contains(t, body, "error")
			}
		})
	}
}

func TestHandler_Unfriend(t *testing.T) {
	url := fmt.Sprintf("/friends/%s", userB)

	tests := []struct {
		name       string
		setupSvc   func(*mockService)
		wantStatus int
	}{
		{
			name:       "happy path returns 204",
			wantStatus: http.StatusNoContent,
		},
		{
			name: "ErrNotFound returns 404",
			setupSvc: func(m *mockService) {
				m.unfriendFn = func(_ context.Context, _, _ string) error {
					return apperr.ErrNotFound
				}
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newMockService()
			if tt.setupSvc != nil {
				tt.setupSvc(svc)
			}
			app := newTestApp(svc)

			resp := doRequest(app, http.MethodDelete, url)

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}

func TestHandler_CancelFriendRequest(t *testing.T) {
	url := fmt.Sprintf("/friends/requests/%s", userB)

	tests := []struct {
		name       string
		setupSvc   func(*mockService)
		wantStatus int
	}{
		{
			name:       "happy path returns 204",
			wantStatus: http.StatusNoContent,
		},
		{
			name: "ErrForbidden returns 403",
			setupSvc: func(m *mockService) {
				m.cancelFriendRequestFn = func(_ context.Context, _, _ string) error {
					return apperr.ErrForbidden
				}
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "ErrNotFound returns 404",
			setupSvc: func(m *mockService) {
				m.cancelFriendRequestFn = func(_ context.Context, _, _ string) error {
					return apperr.ErrNotFound
				}
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newMockService()
			if tt.setupSvc != nil {
				tt.setupSvc(svc)
			}
			app := newTestApp(svc)

			resp := doRequest(app, http.MethodDelete, url)

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}

func TestHandler_AcceptFriendRequest(t *testing.T) {
	url := fmt.Sprintf("/friends/requests/%s/accept", userB)

	tests := []struct {
		name       string
		setupSvc   func(*mockService)
		wantStatus int
	}{
		{
			name:       "happy path returns 200 dengan Response",
			wantStatus: http.StatusOK,
		},
		{
			name: "ErrNotFound returns 404",
			setupSvc: func(m *mockService) {
				m.acceptFriendRequestFn = func(_ context.Context, _, _ string) (*Response, error) {
					return nil, apperr.ErrNotFound
				}
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "ErrForbidden returns 403",
			setupSvc: func(m *mockService) {
				m.acceptFriendRequestFn = func(_ context.Context, _, _ string) (*Response, error) {
					return nil, apperr.ErrForbidden
				}
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "ErrConflict returns 409",
			setupSvc: func(m *mockService) {
				m.acceptFriendRequestFn = func(_ context.Context, _, _ string) (*Response, error) {
					return nil, apperr.ErrConflict
				}
			},
			wantStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newMockService()
			if tt.setupSvc != nil {
				tt.setupSvc(svc)
			}
			app := newTestApp(svc)

			resp := doRequest(app, http.MethodPut, url)

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}

func TestHandler_DeclineFriendRequest(t *testing.T) {
	url := fmt.Sprintf("/friends/requests/%s/decline", userB)

	tests := []struct {
		name       string
		setupSvc   func(*mockService)
		wantStatus int
	}{
		{
			name:       "happy path returns 204",
			wantStatus: http.StatusNoContent,
		},
		{
			name: "ErrForbidden returns 403",
			setupSvc: func(m *mockService) {
				m.declineFriendRequestFn = func(_ context.Context, _, _ string) error {
					return apperr.ErrForbidden
				}
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "ErrNotFound returns 404",
			setupSvc: func(m *mockService) {
				m.declineFriendRequestFn = func(_ context.Context, _, _ string) error {
					return apperr.ErrNotFound
				}
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newMockService()
			if tt.setupSvc != nil {
				tt.setupSvc(svc)
			}
			app := newTestApp(svc)

			resp := doRequest(app, http.MethodPut, url)

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}

// TestHandler_500BodyTidakBocorkanPesanInternal memastikan default error
// tidak mengekspos detail internal ke client.
func TestHandler_500BodyTidakBocorkanPesanInternal(t *testing.T) {
	svc := newMockService()
	svc.addFriendFn = func(_ context.Context, _, _ string) (*PendingResponse, error) {
		return nil, assert.AnError // pesan: "assert.AnError general error for testing"
	}
	app := newTestApp(svc)

	resp := doRequest(app, http.MethodPost, fmt.Sprintf("/friends/%s", userB))

	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	// harus "internal server error", bukan pesan dari assert.AnError
	assert.Equal(t, "internal server error", body["error"])
}
