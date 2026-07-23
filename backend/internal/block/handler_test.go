package block_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BleKuntay/FlipChat/backend/internal/block"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ------------------------------------------------------------------ //
// Mock                                                                 //
// ------------------------------------------------------------------ //

type mockService struct {
	mock.Mock
}

func (m *mockService) BlockUser(ctx context.Context, request block.Request) (*block.Response, error) {
	args := m.Called(ctx, request)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*block.Response), args.Error(1)
}

func (m *mockService) UnblockUser(ctx context.Context, request block.Request) error {
	return m.Called(ctx, request).Error(0)
}

func (m *mockService) GetBlockList(ctx context.Context, blockerID string, query block.ListQuery) (*block.ListResponse, error) {
	args := m.Called(ctx, blockerID, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*block.ListResponse), args.Error(1)
}

// ------------------------------------------------------------------ //
// Helper                                                               //
// ------------------------------------------------------------------ //

func newApp(svc *mockService, userID string) *fiber.App {
	app := fiber.New()

	app.Use(func(c fiber.Ctx) error {
		c.Locals("user_id", userID)
		return c.Next()
	})

	h := block.NewHandler(svc)
	h.RegisterRoutes(app)

	return app
}

func doRequest(t *testing.T, app *fiber.App, method, url string) *http.Response {
	req := httptest.NewRequest(method, url, nil)
	resp, _ := app.Test(req) //nolint:bodyclose // closed via t.Cleanup below
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, dest any) {
	t.Helper()
	require.NoError(t, json.NewDecoder(resp.Body).Decode(dest))
}

// ------------------------------------------------------------------ //
// POST /:id/block                                                      //
// ------------------------------------------------------------------ //

func TestHandler_BlockUser(t *testing.T) {
	const callerID = "user-aaa"
	const targetID = "user-bbb"

	t.Run("201 - block successfully", func(t *testing.T) {
		svc := new(mockService)
		app := newApp(svc, callerID)

		req := block.Request{BlockerID: callerID, BlockedID: targetID}
		want := &block.Response{BlockerID: callerID, BlockedID: targetID}
		svc.On("BlockUser", mock.Anything, req).Return(want, nil)

		resp := doRequest(t, app, http.MethodPost, "/"+targetID+"/block")
		assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

		var body block.Response
		decodeJSON(t, resp, &body)
		assert.Equal(t, callerID, body.BlockerID)
		assert.Equal(t, targetID, body.BlockedID)
		svc.AssertExpectations(t)
	})

	t.Run("400 - service returns ErrBadRequest (self-block)", func(t *testing.T) {
		svc := new(mockService)
		app := newApp(svc, callerID)

		req := block.Request{BlockerID: callerID, BlockedID: targetID}
		svc.On("BlockUser", mock.Anything, req).Return(nil, apperr.ErrBadRequest)

		resp := doRequest(t, app, http.MethodPost, "/"+targetID+"/block")
		assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
	})

	t.Run("500 - service returns unexpected error", func(t *testing.T) {
		svc := new(mockService)
		app := newApp(svc, callerID)

		req := block.Request{BlockerID: callerID, BlockedID: targetID}
		svc.On("BlockUser", mock.Anything, req).Return(nil, errors.New("unexpected"))

		resp := doRequest(t, app, http.MethodPost, "/"+targetID+"/block")
		assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)
	})
}

// ------------------------------------------------------------------ //
// DELETE /:id/block                                                    //
// ------------------------------------------------------------------ //

func TestHandler_UnblockUser(t *testing.T) {
	const callerID = "user-aaa"
	const targetID = "user-bbb"

	t.Run("204 - unblock successfully", func(t *testing.T) {
		svc := new(mockService)
		app := newApp(svc, callerID)

		req := block.Request{BlockerID: callerID, BlockedID: targetID}
		svc.On("UnblockUser", mock.Anything, req).Return(nil)

		resp := doRequest(t, app, http.MethodDelete, "/"+targetID+"/block")
		assert.Equal(t, fiber.StatusNoContent, resp.StatusCode)
		svc.AssertExpectations(t)
	})

	t.Run("400 - service returns ErrBadRequest", func(t *testing.T) {
		svc := new(mockService)
		app := newApp(svc, callerID)

		req := block.Request{BlockerID: callerID, BlockedID: targetID}
		svc.On("UnblockUser", mock.Anything, req).Return(apperr.ErrBadRequest)

		resp := doRequest(t, app, http.MethodDelete, "/"+targetID+"/block")
		assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
	})

	t.Run("500 - service returns unexpected error", func(t *testing.T) {
		svc := new(mockService)
		app := newApp(svc, callerID)

		req := block.Request{BlockerID: callerID, BlockedID: targetID}
		svc.On("UnblockUser", mock.Anything, req).Return(errors.New("unexpected"))

		resp := doRequest(t, app, http.MethodDelete, "/"+targetID+"/block")
		assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)
	})
}

// ------------------------------------------------------------------ //
// GET /me/blocks                                                       //
// ------------------------------------------------------------------ //

func TestHandler_GetBlockList(t *testing.T) {
	const callerID = "user-aaa"

	t.Run("200 - return block list with next cursor", func(t *testing.T) {
		svc := new(mockService)
		app := newApp(svc, callerID)

		query := block.ListQuery{Limit: 2}
		want := &block.ListResponse{
			Data: []block.BlockedSummary{
				{UserID: "user-bbb", Name: "Beta", Username: "beta"},
				{UserID: "user-ccc", Name: "Gamma", Username: "gamma"},
			},
			NextCursor: "user-ccc",
		}
		svc.On("GetBlockList", mock.Anything, callerID, query).Return(want, nil)

		resp := doRequest(t, app, http.MethodGet, "/me/blocks?limit=2")
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)

		var body block.ListResponse
		decodeJSON(t, resp, &body)
		assert.Len(t, body.Data, 2)
		assert.Equal(t, "user-ccc", body.NextCursor)
		svc.AssertExpectations(t)
	})

	t.Run("200 - last page, next cursor empty", func(t *testing.T) {
		svc := new(mockService)
		app := newApp(svc, callerID)

		query := block.ListQuery{Limit: 10}
		want := &block.ListResponse{
			Data:       []block.BlockedSummary{{UserID: "user-bbb"}},
			NextCursor: "",
		}
		svc.On("GetBlockList", mock.Anything, callerID, query).Return(want, nil)

		resp := doRequest(t, app, http.MethodGet, "/me/blocks?limit=10")
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)

		var body block.ListResponse
		decodeJSON(t, resp, &body)
		assert.Empty(t, body.NextCursor)
	})

	t.Run("200 - query param not set, service applies default limit", func(t *testing.T) {
		svc := new(mockService)
		app := newApp(svc, callerID)

		query := block.ListQuery{Limit: 0}
		want := &block.ListResponse{Data: []block.BlockedSummary{}}
		svc.On("GetBlockList", mock.Anything, callerID, query).Return(want, nil)

		resp := doRequest(t, app, http.MethodGet, "/me/blocks")
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)
		svc.AssertExpectations(t)
	})

	t.Run("500 - service returns unexpected error", func(t *testing.T) {
		svc := new(mockService)
		app := newApp(svc, callerID)

		query := block.ListQuery{Limit: 0}
		svc.On("GetBlockList", mock.Anything, callerID, query).Return(nil, errors.New("unexpected"))

		resp := doRequest(t, app, http.MethodGet, "/me/blocks")
		assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)
	})
}
