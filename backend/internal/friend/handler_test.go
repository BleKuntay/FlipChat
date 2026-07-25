package friend

import (
	"context"
	"database/sql"
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

type mockHandlerService struct {
	findAllRequestsFn      func(ctx context.Context, userID string, query RequestListQuery) (*RequestListResponse, error)
	findAllFn              func(ctx context.Context, userID string, query ListQuery) (*ListResponse, error)
	findOneFn              func(ctx context.Context, userID, targetID string) (*StatusResponse, error)
	addFriendFn            func(ctx context.Context, userID, targetID string) (*PendingResponse, error)
	unfriendFn             func(ctx context.Context, userID, targetID string) error
	cancelFriendRequestFn  func(ctx context.Context, userID, targetID string) error
	acceptFriendRequestFn  func(ctx context.Context, userID, targetID string) (*Response, error)
	declineFriendRequestFn func(ctx context.Context, userID, targetID string) error
}

func (m *mockHandlerService) FindAllRequests(ctx context.Context, userID string, query RequestListQuery) (*RequestListResponse, error) {
	if m.findAllRequestsFn != nil {
		return m.findAllRequestsFn(ctx, userID, query)
	}
	return &RequestListResponse{Requests: []PendingResponse{}}, nil
}

func (m *mockHandlerService) FindAll(ctx context.Context, userID string, query ListQuery) (*ListResponse, error) {
	if m.findAllFn != nil {
		return m.findAllFn(ctx, userID, query)
	}
	return &ListResponse{Friends: []Response{}}, nil
}

func (m *mockHandlerService) FindOne(ctx context.Context, userID, targetID string) (*StatusResponse, error) {
	if m.findOneFn != nil {
		return m.findOneFn(ctx, userID, targetID)
	}
	return &StatusResponse{Status: StatusNone}, nil
}

func (m *mockHandlerService) AddFriend(ctx context.Context, userID, targetID string) (*PendingResponse, error) {
	if m.addFriendFn != nil {
		return m.addFriendFn(ctx, userID, targetID)
	}
	return &PendingResponse{UserID: targetID, Direction: "sent", CreatedAt: time.Now(), Status: StatusPending}, nil
}

func (m *mockHandlerService) Unfriend(ctx context.Context, userID, targetID string) error {
	if m.unfriendFn != nil {
		return m.unfriendFn(ctx, userID, targetID)
	}
	return nil
}

func (m *mockHandlerService) CancelFriendRequest(ctx context.Context, userID, targetID string) error {
	if m.cancelFriendRequestFn != nil {
		return m.cancelFriendRequestFn(ctx, userID, targetID)
	}
	return nil
}

func (m *mockHandlerService) AcceptFriendRequest(ctx context.Context, userID, targetID string) (*Response, error) {
	if m.acceptFriendRequestFn != nil {
		return m.acceptFriendRequestFn(ctx, userID, targetID)
	}
	return &Response{UserID: targetID, FriendSince: time.Now()}, nil
}

func (m *mockHandlerService) DeclineFriendRequest(ctx context.Context, userID, targetID string) error {
	if m.declineFriendRequestFn != nil {
		return m.declineFriendRequestFn(ctx, userID, targetID)
	}
	return nil
}

// ------------------------------------------------------------------ //
// Setup                                                                 //
// ------------------------------------------------------------------ //

const handlerUserA = "user-aaa"

func newTestApp(svc ServiceInterface) *fiber.App {
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user_id", handlerUserA)
		return c.Next()
	})
	h := NewHandler(svc)
	h.RegisterRoutes(app.Group("/friends"))
	return app
}

func doRequest(t *testing.T, app *fiber.App, method, url string) *http.Response {
	req := httptest.NewRequest(method, url, nil)
	resp, _ := app.Test(req) //nolint:bodyclose // closed via t.Cleanup below
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// ------------------------------------------------------------------ //
// Tests                                                              //
// ------------------------------------------------------------------ //

func TestHandler_FindAll(t *testing.T) {
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
			name: "service error returns 500",
			setupSvc: func(m *mockHandlerService) {
				m.findAllFn = func(_ context.Context, _ string, _ ListQuery) (*ListResponse, error) {
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
			app := newTestApp(svc)

			resp := doRequest(t, app, http.MethodGet, "/friends/") //nolint:bodyclose // body closed via t.Cleanup in helper

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}

func TestHandler_ListRequests(t *testing.T) {
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
			name: "service error returns 500",
			setupSvc: func(m *mockHandlerService) {
				m.findAllRequestsFn = func(_ context.Context, _ string, _ RequestListQuery) (*RequestListResponse, error) {
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
			app := newTestApp(svc)

			resp := doRequest(t, app, http.MethodGet, "/friends/requests") //nolint:bodyclose // body closed via t.Cleanup in helper

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}

func TestHandler_AddFriend(t *testing.T) {
	url := fmt.Sprintf("/friends/%s", "user-bbb")

	tests := []struct {
		name       string
		setupSvc   func(*mockHandlerService)
		wantStatus int
	}{
		{
			name:       "happy path returns 201",
			wantStatus: http.StatusCreated,
		},
		{
			name: "ErrNotFound returns 404",
			setupSvc: func(m *mockHandlerService) {
				m.addFriendFn = func(_ context.Context, _, _ string) (*PendingResponse, error) {
					return nil, apperr.ErrNotFound
				}
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "ErrConflict returns 409",
			setupSvc: func(m *mockHandlerService) {
				m.addFriendFn = func(_ context.Context, _, _ string) (*PendingResponse, error) {
					return nil, apperr.ErrConflict
				}
			},
			wantStatus: http.StatusConflict,
		},
		{
			name: "ErrForbidden returns 403",
			setupSvc: func(m *mockHandlerService) {
				m.addFriendFn = func(_ context.Context, _, _ string) (*PendingResponse, error) {
					return nil, apperr.ErrForbidden
				}
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "ErrBadRequest returns 400",
			setupSvc: func(m *mockHandlerService) {
				m.addFriendFn = func(_ context.Context, _, _ string) (*PendingResponse, error) {
					return nil, apperr.ErrBadRequest
				}
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "unknown error returns 500",
			setupSvc: func(m *mockHandlerService) {
				m.addFriendFn = func(_ context.Context, _, _ string) (*PendingResponse, error) {
					return nil, sql.ErrNoRows
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
			app := newTestApp(svc)

			resp := doRequest(t, app, http.MethodPost, url) //nolint:bodyclose // body closed via t.Cleanup in helper

			assert.Equal(t, tt.wantStatus, resp.StatusCode)

			if resp.StatusCode >= 400 {
				var body map[string]any
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
				assert.Contains(t, body, "error")
			}
		})
	}
}

func TestHandler_Unfriend(t *testing.T) {
	url := fmt.Sprintf("/friends/%s", "user-bbb")

	tests := []struct {
		name       string
		setupSvc   func(*mockHandlerService)
		wantStatus int
	}{
		{
			name:       "happy path returns 204",
			wantStatus: http.StatusNoContent,
		},
		{
			name: "ErrNotFound returns 404",
			setupSvc: func(m *mockHandlerService) {
				m.unfriendFn = func(_ context.Context, _, _ string) error {
					return apperr.ErrNotFound
				}
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mockHandlerService{}
			if tt.setupSvc != nil {
				tt.setupSvc(svc)
			}
			app := newTestApp(svc)

			resp := doRequest(t, app, http.MethodDelete, url) //nolint:bodyclose // body closed via t.Cleanup in helper

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}

func TestHandler_AcceptFriendRequest(t *testing.T) {
	url := fmt.Sprintf("/friends/requests/%s/accept", "user-bbb")

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
				m.acceptFriendRequestFn = func(_ context.Context, _, _ string) (*Response, error) {
					return nil, apperr.ErrNotFound
				}
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "ErrForbidden returns 403",
			setupSvc: func(m *mockHandlerService) {
				m.acceptFriendRequestFn = func(_ context.Context, _, _ string) (*Response, error) {
					return nil, apperr.ErrForbidden
				}
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "ErrConflict returns 409",
			setupSvc: func(m *mockHandlerService) {
				m.acceptFriendRequestFn = func(_ context.Context, _, _ string) (*Response, error) {
					return nil, apperr.ErrConflict
				}
			},
			wantStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mockHandlerService{}
			if tt.setupSvc != nil {
				tt.setupSvc(svc)
			}
			app := newTestApp(svc)

			resp := doRequest(t, app, http.MethodPut, url) //nolint:bodyclose // body closed via t.Cleanup in helper

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}
