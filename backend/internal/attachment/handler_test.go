package attachment_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BleKuntay/FlipChat/backend/internal/attachment"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ── mock ──────────────────────────────────────────────────────────────────────

type mockService struct{ mock.Mock }

func (m *mockService) Upload(ctx context.Context, uploaderID, filename string, size int64, reader io.Reader) (*attachment.UploadResponse, error) {
	args := m.Called(ctx, uploaderID, filename, size, reader)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*attachment.UploadResponse), args.Error(1)
}

func (m *mockService) Download(ctx context.Context, requesterID, attachmentID string) (io.ReadCloser, *attachment.Metadata, error) {
	args := m.Called(ctx, requesterID, attachmentID)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	return args.Get(0).(io.ReadCloser), args.Get(1).(*attachment.Metadata), args.Error(2)
}

// ── helpers ───────────────────────────────────────────────────────────────────

const handlerUserID = "user-aaa"

func newTestApp(svc attachment.ServiceInterface) *fiber.App {
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user_id", handlerUserID)
		return c.Next()
	})
	h := attachment.NewHandler(svc)
	h.RegisterRoutes(app.Group("/attachments"))
	return app
}

// newMultipartRequest builds a multipart/form-data POST request with a file field.
func newMultipartRequest(fieldName, filename string, content []byte) (*http.Request, error) {
	body := new(bytes.Buffer)
	w := multipart.NewWriter(body)
	fw, err := w.CreateFormFile(fieldName, filename)
	if err != nil {
		return nil, err
	}
	if _, err = fw.Write(content); err != nil {
		return nil, err
	}
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/attachments/upload", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req, nil
}

// ── Upload tests ──────────────────────────────────────────────────────────────

func TestHandler_Upload_ValidFile_Returns201(t *testing.T) {
	svc := new(mockService)

	fileContent := bytes.Repeat([]byte("x"), 100)
	uploadResp := &attachment.UploadResponse{
		AttachmentID: "att-abc",
		ObjectKey:    "attachments/att-abc",
		Filename:     "photo.jpg",
		MIMEType:     "image/jpeg",
		Size:         int64(len(fileContent)),
		UploaderID:   handlerUserID,
	}

	svc.On("Upload", mock.Anything, handlerUserID, "photo.jpg", int64(len(fileContent)), mock.Anything).
		Return(uploadResp, nil)

	req, err := newMultipartRequest("file", "photo.jpg", fileContent)
	require.NoError(t, err)

	resp, err := newTestApp(svc).Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var body attachment.UploadResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "att-abc", body.AttachmentID)
	assert.Equal(t, "image/jpeg", body.MIMEType)
	svc.AssertExpectations(t)
}

func TestHandler_Upload_NoFormFile_Returns400(t *testing.T) {
	svc := new(mockService)

	req := httptest.NewRequest(http.MethodPost, "/attachments/upload", nil)

	resp, err := newTestApp(svc).Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body, "error")
	svc.AssertNotCalled(t, "Upload")
}

func TestHandler_Upload_WrongFieldName_Returns400(t *testing.T) {
	svc := new(mockService)

	req, err := newMultipartRequest("image", "photo.jpg", []byte("content"))
	require.NoError(t, err)

	resp, err := newTestApp(svc).Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	svc.AssertNotCalled(t, "Upload")
}

func TestHandler_Upload_ServiceErrBadRequest_Returns400(t *testing.T) {
	svc := new(mockService)

	svc.On("Upload", mock.Anything, handlerUserID, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, apperr.ErrBadRequest)

	req, err := newMultipartRequest("file", "bad.jpg", []byte("not an image"))
	require.NoError(t, err)

	resp, err := newTestApp(svc).Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body, "error")
	svc.AssertExpectations(t)
}

func TestHandler_Upload_ServiceInternalError_Returns500(t *testing.T) {
	svc := new(mockService)

	svc.On("Upload", mock.Anything, handlerUserID, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("storage unavailable"))

	req, err := newMultipartRequest("file", "photo.jpg", []byte("content"))
	require.NoError(t, err)

	resp, err := newTestApp(svc).Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	svc.AssertExpectations(t)
}

func TestHandler_Upload_UserIDFromAuthContext(t *testing.T) {
	svc := new(mockService)

	fileContent := []byte("content")
	uploadResp := &attachment.UploadResponse{
		AttachmentID: "att-123",
		ObjectKey:    "attachments/att-123",
		Filename:     "photo.jpg",
		MIMEType:     "image/jpeg",
		Size:         int64(len(fileContent)),
		UploaderID:   handlerUserID,
	}

	svc.On("Upload", mock.Anything, handlerUserID, mock.Anything, mock.Anything, mock.Anything).
		Return(uploadResp, nil)

	req, err := newMultipartRequest("file", "photo.jpg", fileContent)
	require.NoError(t, err)

	resp, err := newTestApp(svc).Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	svc.AssertExpectations(t)
}

// ── Download tests ────────────────────────────────────────────────────────────

func TestHandler_Download_Success_Returns200(t *testing.T) {
	svc := new(mockService)

	const attachmentID = "att-abc"
	imageData := []byte("fake image data")

	metadata := &attachment.Metadata{
		AttachmentID: attachmentID,
		ObjectKey:    "attachments/att-abc",
		Filename:     "photo.jpg",
		MIMEType:     "image/jpeg",
		Size:         int64(len(imageData)),
		UploaderID:   "user-other",
	}

	svc.On("Download", mock.Anything, handlerUserID, attachmentID).
		Return(io.NopCloser(bytes.NewReader(imageData)), metadata, nil)

	req := httptest.NewRequest(http.MethodGet, "/attachments/"+attachmentID, nil)
	resp, err := newTestApp(svc).Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	svc.AssertExpectations(t)
}

func TestHandler_Download_ContentTypeHeader(t *testing.T) {
	svc := new(mockService)

	const attachmentID = "att-abc"
	metadata := &attachment.Metadata{
		AttachmentID: attachmentID,
		ObjectKey:    "attachments/att-abc",
		Filename:     "image.webp",
		MIMEType:     "image/webp",
		Size:         100,
		UploaderID:   "user-other",
	}

	svc.On("Download", mock.Anything, handlerUserID, attachmentID).
		Return(io.NopCloser(bytes.NewReader([]byte("data"))), metadata, nil)

	req := httptest.NewRequest(http.MethodGet, "/attachments/"+attachmentID, nil)
	resp, err := newTestApp(svc).Test(req)
	require.NoError(t, err)

	assert.Equal(t, "image/webp", resp.Header.Get("Content-Type"))
	svc.AssertExpectations(t)
}

func TestHandler_Download_ContentDispositionHeader(t *testing.T) {
	svc := new(mockService)

	const (
		attachmentID = "att-abc"
		filename     = "vacation_photo.jpg"
	)
	metadata := &attachment.Metadata{
		AttachmentID: attachmentID,
		ObjectKey:    "attachments/att-abc",
		Filename:     filename,
		MIMEType:     "image/jpeg",
		Size:         100,
		UploaderID:   "user-other",
	}

	svc.On("Download", mock.Anything, handlerUserID, attachmentID).
		Return(io.NopCloser(bytes.NewReader([]byte("data"))), metadata, nil)

	req := httptest.NewRequest(http.MethodGet, "/attachments/"+attachmentID, nil)
	resp, err := newTestApp(svc).Test(req)
	require.NoError(t, err)

	assert.Equal(t, `inline; filename="vacation_photo.jpg"`, resp.Header.Get("Content-Disposition"))
	svc.AssertExpectations(t)
}

func TestHandler_Download_ContentLengthHeader(t *testing.T) {
	svc := new(mockService)

	const attachmentID = "att-abc"
	imageData := bytes.Repeat([]byte("x"), 12345)
	metadata := &attachment.Metadata{
		AttachmentID: attachmentID,
		ObjectKey:    "attachments/att-abc",
		Filename:     "image.png",
		MIMEType:     "image/png",
		Size:         int64(len(imageData)),
		UploaderID:   "user-other",
	}

	svc.On("Download", mock.Anything, handlerUserID, attachmentID).
		Return(io.NopCloser(bytes.NewReader(imageData)), metadata, nil)

	req := httptest.NewRequest(http.MethodGet, "/attachments/"+attachmentID, nil)
	resp, err := newTestApp(svc).Test(req)
	require.NoError(t, err)

	assert.Equal(t, "12345", resp.Header.Get("Content-Length"))
	svc.AssertExpectations(t)
}

func TestHandler_Download_AttachmentIDFromPathParam(t *testing.T) {
	svc := new(mockService)

	const attachmentID = "att-from-path"
	metadata := &attachment.Metadata{
		AttachmentID: attachmentID,
		ObjectKey:    "attachments/" + attachmentID,
		Filename:     "image.jpg",
		MIMEType:     "image/jpeg",
		Size:         100,
		UploaderID:   "user-other",
	}

	svc.On("Download", mock.Anything, handlerUserID, attachmentID).
		Return(io.NopCloser(bytes.NewReader([]byte("data"))), metadata, nil)

	req := httptest.NewRequest(http.MethodGet, "/attachments/"+attachmentID, nil)
	resp, err := newTestApp(svc).Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	svc.AssertExpectations(t)
}

func TestHandler_Download_ServiceErrNotFound_Returns404(t *testing.T) {
	svc := new(mockService)

	const attachmentID = "att-notfound"

	svc.On("Download", mock.Anything, handlerUserID, attachmentID).
		Return(nil, nil, apperr.ErrNotFound)

	req := httptest.NewRequest(http.MethodGet, "/attachments/"+attachmentID, nil)
	resp, err := newTestApp(svc).Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body, "error")
	svc.AssertExpectations(t)
}

func TestHandler_Download_ServiceErrForbidden_Returns403(t *testing.T) {
	svc := new(mockService)

	const attachmentID = "att-forbidden"

	svc.On("Download", mock.Anything, handlerUserID, attachmentID).
		Return(nil, nil, apperr.ErrForbidden)

	req := httptest.NewRequest(http.MethodGet, "/attachments/"+attachmentID, nil)
	resp, err := newTestApp(svc).Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	svc.AssertExpectations(t)
}

func TestHandler_Download_ServiceInternalError_Returns500(t *testing.T) {
	svc := new(mockService)

	const attachmentID = "att-abc"

	svc.On("Download", mock.Anything, handlerUserID, attachmentID).
		Return(nil, nil, errors.New("storage error"))

	req := httptest.NewRequest(http.MethodGet, "/attachments/"+attachmentID, nil)
	resp, err := newTestApp(svc).Test(req)
	require.NoError(t, err)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	svc.AssertExpectations(t)
}
