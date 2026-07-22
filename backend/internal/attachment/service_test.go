package attachment_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/BleKuntay/FlipChat/backend/internal/attachment"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ── mocks ─────────────────────────────────────────────────────────────────────

type mockObjectStore struct {
	m *mock.Mock
}

func newMockObjectStore() *mockObjectStore {
	return &mockObjectStore{m: new(mock.Mock)}
}

func (m *mockObjectStore) PutObject(ctx context.Context, objectKey string, reader io.Reader, size int64, mimeType string) error {
	return m.m.Called(ctx, objectKey, reader, size, mimeType).Error(0)
}

func (m *mockObjectStore) GetObject(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	args := m.m.Called(ctx, objectKey)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (m *mockObjectStore) DeleteObject(ctx context.Context, objectKey string) error {
	return m.m.Called(ctx, objectKey).Error(0)
}

func (m *mockObjectStore) On(methodName string, arguments ...interface{}) *mock.Call {
	return m.m.On(methodName, arguments...)
}

func (m *mockObjectStore) AssertExpectations(t mock.TestingT) bool {
	return m.m.AssertExpectations(t)
}

func (m *mockObjectStore) AssertNotCalled(t mock.TestingT, methodName string, arguments ...interface{}) bool {
	return m.m.AssertNotCalled(t, methodName, arguments...)
}

type mockMessageStore struct {
	m *mock.Mock
}

func newMockMessageStore() *mockMessageStore {
	return &mockMessageStore{m: new(mock.Mock)}
}

func (m *mockMessageStore) FindByAttachmentID(ctx context.Context, attachmentID string) (conversationID string, metadata map[string]any, deletedAt *time.Time, err error) {
	args := m.m.Called(ctx, attachmentID)
	metadata = nil
	if args.Get(1) != nil {
		metadata = args.Get(1).(map[string]any)
	}
	deletedAt = nil
	if args.Get(2) != nil {
		deletedAt = args.Get(2).(*time.Time)
	}
	return args.String(0), metadata, deletedAt, args.Error(3)
}

func (m *mockMessageStore) On(methodName string, arguments ...interface{}) *mock.Call {
	return m.m.On(methodName, arguments...)
}

func (m *mockMessageStore) AssertExpectations(t mock.TestingT) bool {
	return m.m.AssertExpectations(t)
}

type mockConversationStore struct {
	m *mock.Mock
}

func newMockConversationStore() *mockConversationStore {
	return &mockConversationStore{m: new(mock.Mock)}
}

func (m *mockConversationStore) GetParticipants(ctx context.Context, conversationID string) (string, string, error) {
	args := m.m.Called(ctx, conversationID)
	return args.String(0), args.String(1), args.Error(2)
}

func (m *mockConversationStore) On(methodName string, arguments ...interface{}) *mock.Call {
	return m.m.On(methodName, arguments...)
}

func (m *mockConversationStore) AssertExpectations(t mock.TestingT) bool {
	return m.m.AssertExpectations(t)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newTestService(objects *mockObjectStore, messages *mockMessageStore, conversations *mockConversationStore) *attachment.Service {
	return attachment.NewService(objects, messages, conversations)
}

// jpegMagic returns a valid JPEG magic bytes sequence
func jpegMagic() []byte {
	return []byte("\xff\xd8\xff")
}

// pngMagic returns a valid PNG magic bytes sequence
func pngMagic() []byte {
	return []byte("\x89PNG\r\n\x1a\n")
}

// gif87Magic returns a valid GIF87a magic bytes sequence
func gif87Magic() []byte {
	return []byte("GIF87a")
}

// gif89Magic returns a valid GIF89a magic bytes sequence
func gif89Magic() []byte {
	return []byte("GIF89a")
}

// webpMagic returns a valid WebP magic bytes sequence
func webpMagic() []byte {
	return []byte("RIFF\x00\x00\x00\x00WEBP")
}

// pdfMagic returns a PDF magic bytes sequence (unsupported)
func pdfMagic() []byte {
	return []byte("%PDF")
}

// exeMagic returns an executable magic bytes sequence (unsupported)
func exeMagic() []byte {
	return []byte("MZ")
}

// zipMagic returns a ZIP magic bytes sequence (unsupported)
func zipMagic() []byte {
	return []byte("PK")
}

// ── Upload tests ──────────────────────────────────────────────────────────────

func TestService_Upload_JPEG_Valid(t *testing.T) {
	ctx := context.Background()
	objects := newMockObjectStore()

	magic := jpegMagic()
	extra := bytes.Repeat([]byte("x"), 100)
	reader := io.NopCloser(bytes.NewReader(append(magic, extra...)))

	objects.On("PutObject", ctx, mock.MatchedBy(func(key string) bool {
		return len(key) > 0 && key[:12] == "attachments/"
	}), mock.Anything, int64(103), "image/jpeg").Return(nil)

	svc := newTestService(objects, nil, nil)
	resp, err := svc.Upload(ctx, "user-123", "photo.jpg", int64(103), reader)

	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "image/jpeg", resp.MIMEType)
	assert.Equal(t, "user-123", resp.UploaderID)
	assert.True(t, len(resp.AttachmentID) > 0)
	assert.Equal(t, "attachments/"+resp.AttachmentID, resp.ObjectKey)
	objects.AssertExpectations(t)
}

func TestService_Upload_PNG_Valid(t *testing.T) {
	ctx := context.Background()
	objects := newMockObjectStore()

	magic := pngMagic()
	extra := bytes.Repeat([]byte("x"), 100)
	reader := io.NopCloser(bytes.NewReader(append(magic, extra...)))

	objects.On("PutObject", ctx, mock.MatchedBy(func(key string) bool {
		return len(key) > 0 && key[:12] == "attachments/"
	}), mock.Anything, int64(108), "image/png").Return(nil)

	svc := newTestService(objects, nil, nil)
	resp, err := svc.Upload(ctx, "user-456", "image.png", int64(108), reader)

	require.NoError(t, err)
	assert.Equal(t, "image/png", resp.MIMEType)
	objects.AssertExpectations(t)
}

func TestService_Upload_GIF87a_Valid(t *testing.T) {
	ctx := context.Background()
	objects := newMockObjectStore()

	magic := gif87Magic()
	extra := bytes.Repeat([]byte("x"), 50)
	reader := io.NopCloser(bytes.NewReader(append(magic, extra...)))

	objects.On("PutObject", ctx, mock.Anything, mock.Anything, int64(56), "image/gif").Return(nil)

	svc := newTestService(objects, nil, nil)
	resp, err := svc.Upload(ctx, "user-789", "anim.gif", int64(56), reader)

	require.NoError(t, err)
	assert.Equal(t, "image/gif", resp.MIMEType)
	objects.AssertExpectations(t)
}

func TestService_Upload_GIF89a_Valid(t *testing.T) {
	ctx := context.Background()
	objects := newMockObjectStore()

	magic := gif89Magic()
	extra := bytes.Repeat([]byte("x"), 50)
	reader := io.NopCloser(bytes.NewReader(append(magic, extra...)))

	objects.On("PutObject", ctx, mock.Anything, mock.Anything, int64(56), "image/gif").Return(nil)

	svc := newTestService(objects, nil, nil)
	resp, err := svc.Upload(ctx, "user-012", "modern.gif", int64(56), reader)

	require.NoError(t, err)
	assert.Equal(t, "image/gif", resp.MIMEType)
	objects.AssertExpectations(t)
}

func TestService_Upload_WebP_Valid(t *testing.T) {
	ctx := context.Background()
	objects := newMockObjectStore()

	magic := webpMagic()
	extra := bytes.Repeat([]byte("x"), 50)
	reader := io.NopCloser(bytes.NewReader(append(magic, extra...)))

	objects.On("PutObject", ctx, mock.Anything, mock.Anything, int64(62), "image/webp").Return(nil)

	svc := newTestService(objects, nil, nil)
	resp, err := svc.Upload(ctx, "user-345", "modern.webp", int64(62), reader)

	require.NoError(t, err)
	assert.Equal(t, "image/webp", resp.MIMEType)
	objects.AssertExpectations(t)
}

func TestService_Upload_FilenameStored(t *testing.T) {
	ctx := context.Background()
	objects := newMockObjectStore()

	magic := jpegMagic()
	extra := bytes.Repeat([]byte("x"), 100)
	reader := io.NopCloser(bytes.NewReader(append(magic, extra...)))

	objects.On("PutObject", ctx, mock.Anything, mock.Anything, int64(103), "image/jpeg").Return(nil)

	svc := newTestService(objects, nil, nil)
	resp, err := svc.Upload(ctx, "user-123", "vacation.jpg", int64(103), reader)

	require.NoError(t, err)
	assert.Equal(t, "vacation.jpg", resp.Filename)
}

func TestService_Upload_SizeMax_Boundary_5MB(t *testing.T) {
	ctx := context.Background()
	objects := newMockObjectStore()

	maxSize := int64(5 * 1024 * 1024)
	magic := jpegMagic()
	extra := bytes.Repeat([]byte("x"), int(maxSize-int64(len(magic))))
	reader := io.NopCloser(bytes.NewReader(append(magic, extra...)))

	objects.On("PutObject", ctx, mock.Anything, mock.Anything, maxSize, "image/jpeg").Return(nil)

	svc := newTestService(objects, nil, nil)
	resp, err := svc.Upload(ctx, "user-123", "large.jpg", maxSize, reader)

	require.NoError(t, err)
	assert.NotNil(t, resp)
	objects.AssertExpectations(t)
}

func TestService_Upload_SizeExceeds_5MBPlus1(t *testing.T) {
	ctx := context.Background()
	objects := newMockObjectStore()

	oversizeSize := int64(5*1024*1024 + 1)
	magic := jpegMagic()
	reader := io.NopCloser(bytes.NewReader(magic))

	// PutObject should NOT be called
	svc := newTestService(objects, nil, nil)
	resp, err := svc.Upload(ctx, "user-123", "huge.jpg", oversizeSize, reader)

	assert.ErrorIs(t, err, apperr.ErrFileTooLarge)
	assert.Nil(t, resp)
	objects.AssertNotCalled(t, "PutObject")
}

func TestService_Upload_SizeMin_Boundary_12B(t *testing.T) {
	ctx := context.Background()
	objects := newMockObjectStore()

	content := []byte{0xFF, 0xD8, 0xFF, 'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x'} // 12 bytes
	reader := io.NopCloser(bytes.NewReader(content))

	objects.On("PutObject", ctx, mock.Anything, mock.Anything, int64(12), "image/jpeg").Return(nil)

	svc := newTestService(objects, nil, nil)
	resp, err := svc.Upload(ctx, "user-123", "tiny.jpg", int64(12), reader)

	require.NoError(t, err)
	assert.NotNil(t, resp)
	objects.AssertExpectations(t)
}

func TestService_Upload_SizeBelowMin_11B_Rejected(t *testing.T) {
	ctx := context.Background()
	objects := newMockObjectStore()

	content := []byte{0xFF, 0xD8, 0xFF, 'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x'} // 11 bytes
	reader := io.NopCloser(bytes.NewReader(content))

	svc := newTestService(objects, nil, nil)
	resp, err := svc.Upload(ctx, "user-123", "tiny.jpg", int64(11), reader)

	assert.ErrorIs(t, err, apperr.ErrBadRequest)
	assert.Nil(t, resp)
	objects.AssertNotCalled(t, "PutObject")
}

func TestService_Upload_Size0_Rejected(t *testing.T) {
	ctx := context.Background()
	objects := newMockObjectStore()

	reader := io.NopCloser(bytes.NewReader([]byte("")))

	svc := newTestService(objects, nil, nil)
	resp, err := svc.Upload(ctx, "user-123", "empty.jpg", 0, reader)

	assert.ErrorIs(t, err, apperr.ErrFileTooLarge)
	assert.Nil(t, resp)
	objects.AssertNotCalled(t, "PutObject")
}

func TestService_Upload_SizeNegative_Rejected(t *testing.T) {
	ctx := context.Background()
	objects := newMockObjectStore()

	reader := io.NopCloser(bytes.NewReader([]byte("")))

	svc := newTestService(objects, nil, nil)
	resp, err := svc.Upload(ctx, "user-123", "negative.jpg", -1, reader)

	assert.ErrorIs(t, err, apperr.ErrFileTooLarge)
	assert.Nil(t, resp)
	objects.AssertNotCalled(t, "PutObject")
}

func TestService_Upload_PDF_MagicBytes_Rejected(t *testing.T) {
	ctx := context.Background()
	objects := newMockObjectStore()

	magic := pdfMagic()
	extra := bytes.Repeat([]byte("x"), 100)
	reader := io.NopCloser(bytes.NewReader(append(magic, extra...)))

	svc := newTestService(objects, nil, nil)
	resp, err := svc.Upload(ctx, "user-123", "doc.pdf", int64(103), reader)

	assert.ErrorIs(t, err, apperr.ErrUnsupportedMIME)
	assert.Nil(t, resp)
	objects.AssertNotCalled(t, "PutObject")
}

func TestService_Upload_Executable_MagicBytes_Rejected(t *testing.T) {
	ctx := context.Background()
	objects := newMockObjectStore()

	magic := exeMagic()
	extra := bytes.Repeat([]byte("x"), 100)
	reader := io.NopCloser(bytes.NewReader(append(magic, extra...)))

	svc := newTestService(objects, nil, nil)
	resp, err := svc.Upload(ctx, "user-123", "malware.exe", int64(102), reader)

	assert.ErrorIs(t, err, apperr.ErrUnsupportedMIME)
	assert.Nil(t, resp)
	objects.AssertNotCalled(t, "PutObject")
}

func TestService_Upload_ZIP_MagicBytes_Rejected(t *testing.T) {
	ctx := context.Background()
	objects := newMockObjectStore()

	magic := zipMagic()
	extra := bytes.Repeat([]byte("x"), 100)
	reader := io.NopCloser(bytes.NewReader(append(magic, extra...)))

	svc := newTestService(objects, nil, nil)
	resp, err := svc.Upload(ctx, "user-123", "archive.zip", int64(102), reader)

	assert.ErrorIs(t, err, apperr.ErrUnsupportedMIME)
	assert.Nil(t, resp)
	objects.AssertNotCalled(t, "PutObject")
}

func TestService_Upload_FileTooSmall_LessThan4Bytes(t *testing.T) {
	ctx := context.Background()
	objects := newMockObjectStore()

	// ReadHeader expects at least 4 bytes for magic detection
	reader := io.NopCloser(bytes.NewReader([]byte("ab")))

	svc := newTestService(objects, nil, nil)
	resp, err := svc.Upload(ctx, "user-123", "tiny.jpg", int64(2), reader)

	assert.ErrorIs(t, err, apperr.ErrBadRequest)
	assert.Nil(t, resp)
	objects.AssertNotCalled(t, "PutObject")
}

func TestService_Upload_12BytesBadImage(t *testing.T) {
	ctx := context.Background()
	objects := newMockObjectStore()

	// Exactly 12 bytes but invalid magic
	badMagic := bytes.Repeat([]byte("x"), 12)
	reader := io.NopCloser(bytes.NewReader(badMagic))

	svc := newTestService(objects, nil, nil)
	resp, err := svc.Upload(ctx, "user-123", "invalid.jpg", int64(12), reader)

	assert.ErrorIs(t, err, apperr.ErrUnsupportedMIME)
	assert.Nil(t, resp)
	objects.AssertNotCalled(t, "PutObject")
}

func TestService_Upload_ValidImageWrongExtension(t *testing.T) {
	ctx := context.Background()
	objects := newMockObjectStore()

	// PNG magic bytes but .txt filename
	magic := pngMagic()
	extra := bytes.Repeat([]byte("x"), 100)
	reader := io.NopCloser(bytes.NewReader(append(magic, extra...)))

	objects.On("PutObject", ctx, mock.Anything, mock.Anything, int64(108), "image/png").Return(nil)

	svc := newTestService(objects, nil, nil)
	resp, err := svc.Upload(ctx, "user-123", "image.txt", int64(108), reader)

	// Should succeed because validation is based on magic bytes, not extension
	require.NoError(t, err)
	assert.Equal(t, "image/png", resp.MIMEType)
	objects.AssertExpectations(t)
}

func TestService_Upload_PutObject_Error_Propagated(t *testing.T) {
	ctx := context.Background()
	objects := newMockObjectStore()

	magic := jpegMagic()
	extra := bytes.Repeat([]byte("x"), 100)
	reader := io.NopCloser(bytes.NewReader(append(magic, extra...)))

	testErr := errors.New("storage unavailable")
	objects.On("PutObject", ctx, mock.Anything, mock.Anything, int64(103), "image/jpeg").Return(testErr)

	svc := newTestService(objects, nil, nil)
	resp, err := svc.Upload(ctx, "user-123", "photo.jpg", int64(103), reader)

	assert.ErrorIs(t, err, testErr)
	assert.Nil(t, resp)
	objects.AssertExpectations(t)
}

// ── Download tests ────────────────────────────────────────────────────────────

func TestService_Download_HappyPath_LowParticipant(t *testing.T) {
	ctx := context.Background()
	messages := newMockMessageStore()
	conversations := newMockConversationStore()
	objects := newMockObjectStore()

	const (
		requesterID    = "user-low"
		otherID        = "user-high"
		attachmentID   = "att-123"
		conversationID = "conv-456"
		objectKey      = "attachments/att-123"
	)

	metadata := map[string]any{
		"attachment_id": attachmentID,
		"object_key":    objectKey,
		"filename":      "photo.jpg",
		"mime_type":     "image/jpeg",
		"size":          int64(1024),
		"uploader_id":   "user-high",
	}

	messages.On("FindByAttachmentID", ctx, attachmentID).Return(conversationID, metadata, nil, nil)
	conversations.On("GetParticipants", ctx, conversationID).Return(requesterID, otherID, nil)

	mockReader := io.NopCloser(bytes.NewReader([]byte("fake image data")))
	objects.On("GetObject", ctx, objectKey).Return(mockReader, nil)

	svc := newTestService(objects, messages, conversations)
	reader, resp, err := svc.Download(ctx, requesterID, attachmentID)

	require.NoError(t, err)
	require.NotNil(t, reader)
	require.NotNil(t, resp)
	assert.Equal(t, attachmentID, resp.AttachmentID)
	assert.Equal(t, objectKey, resp.ObjectKey)
	assert.Equal(t, "photo.jpg", resp.Filename)
	assert.Equal(t, "image/jpeg", resp.MIMEType)
	assert.Equal(t, int64(1024), resp.Size)
	assert.Equal(t, "user-high", resp.UploaderID)
	reader.Close()
	messages.AssertExpectations(t)
	conversations.AssertExpectations(t)
	objects.AssertExpectations(t)
}

func TestService_Download_HappyPath_HighParticipant(t *testing.T) {
	ctx := context.Background()
	messages := newMockMessageStore()
	conversations := newMockConversationStore()
	objects := newMockObjectStore()

	const (
		requesterID    = "user-high"
		otherID        = "user-low"
		attachmentID   = "att-789"
		conversationID = "conv-789"
		objectKey      = "attachments/att-789"
	)

	metadata := map[string]any{
		"attachment_id": attachmentID,
		"object_key":    objectKey,
		"filename":      "image.png",
		"mime_type":     "image/png",
		"size":          int64(2048),
		"uploader_id":   otherID,
	}

	messages.On("FindByAttachmentID", ctx, attachmentID).Return(conversationID, metadata, nil, nil)
	conversations.On("GetParticipants", ctx, conversationID).Return(otherID, requesterID, nil)

	mockReader := io.NopCloser(bytes.NewReader([]byte("fake image data")))
	objects.On("GetObject", ctx, objectKey).Return(mockReader, nil)

	svc := newTestService(objects, messages, conversations)
	reader, resp, err := svc.Download(ctx, requesterID, attachmentID)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "image.png", resp.Filename)
	reader.Close()
	messages.AssertExpectations(t)
	conversations.AssertExpectations(t)
	objects.AssertExpectations(t)
}

func TestService_Download_MetadataParsingAllFields(t *testing.T) {
	ctx := context.Background()
	messages := newMockMessageStore()
	conversations := newMockConversationStore()
	objects := newMockObjectStore()

	const (
		requesterID    = "user-low"
		otherID        = "user-high"
		attachmentID   = "att-full"
		conversationID = "conv-full"
		objectKey      = "attachments/att-full"
	)

	metadata := map[string]any{
		"attachment_id": attachmentID,
		"object_key":    objectKey,
		"filename":      "document.webp",
		"mime_type":     "image/webp",
		"size":          float64(5120), // JSON float64 conversion
		"uploader_id":   "user-other",
	}

	messages.On("FindByAttachmentID", ctx, attachmentID).Return(conversationID, metadata, nil, nil)
	conversations.On("GetParticipants", ctx, conversationID).Return(requesterID, otherID, nil)

	mockReader := io.NopCloser(bytes.NewReader([]byte("")))
	objects.On("GetObject", ctx, objectKey).Return(mockReader, nil)

	svc := newTestService(objects, messages, conversations)
	_, resp, err := svc.Download(ctx, requesterID, attachmentID)

	require.NoError(t, err)
	assert.Equal(t, attachmentID, resp.AttachmentID)
	assert.Equal(t, objectKey, resp.ObjectKey)
	assert.Equal(t, "document.webp", resp.Filename)
	assert.Equal(t, "image/webp", resp.MIMEType)
	assert.Equal(t, int64(5120), resp.Size) // float64 converted to int64
	assert.Equal(t, "user-other", resp.UploaderID)
}

func TestService_Download_AttachmentNotFound(t *testing.T) {
	ctx := context.Background()
	messages := newMockMessageStore()

	const attachmentID = "att-notfound"

	messages.On("FindByAttachmentID", ctx, attachmentID).Return("", nil, nil, nil)

	svc := newTestService(nil, messages, nil)
	reader, resp, err := svc.Download(ctx, "user-123", attachmentID)

	assert.ErrorIs(t, err, apperr.ErrNotFound)
	assert.Nil(t, reader)
	assert.Nil(t, resp)
	messages.AssertExpectations(t)
}

func TestService_Download_MetadataNil(t *testing.T) {
	ctx := context.Background()
	messages := newMockMessageStore()

	const (
		attachmentID   = "att-123"
		conversationID = "conv-456"
	)

	messages.On("FindByAttachmentID", ctx, attachmentID).Return(conversationID, nil, nil, nil)

	svc := newTestService(nil, messages, nil)
	reader, resp, err := svc.Download(ctx, "user-123", attachmentID)

	assert.ErrorIs(t, err, apperr.ErrNotFound)
	assert.Nil(t, reader)
	assert.Nil(t, resp)
	messages.AssertExpectations(t)
}

func TestService_Download_ObjectKeyEmpty(t *testing.T) {
	ctx := context.Background()
	messages := newMockMessageStore()

	const (
		attachmentID   = "att-123"
		conversationID = "conv-456"
	)

	// Metadata exists but object_key is empty
	metadata := map[string]any{
		"attachment_id": attachmentID,
		"object_key":    "", // empty
		"filename":      "photo.jpg",
		"mime_type":     "image/jpeg",
		"size":          int64(1024),
		"uploader_id":   "user-123",
	}

	messages.On("FindByAttachmentID", ctx, attachmentID).Return(conversationID, metadata, nil, nil)

	svc := newTestService(nil, messages, nil)
	reader, resp, err := svc.Download(ctx, "user-123", attachmentID)

	assert.ErrorIs(t, err, apperr.ErrNotFound)
	assert.Nil(t, reader)
	assert.Nil(t, resp)
	messages.AssertExpectations(t)
}

func TestService_Download_MessageDeleted(t *testing.T) {
	ctx := context.Background()
	messages := newMockMessageStore()

	const (
		attachmentID   = "att-123"
		conversationID = "conv-456"
	)

	metadata := map[string]any{
		"attachment_id": attachmentID,
		"object_key":    "attachments/att-123",
		"filename":      "photo.jpg",
		"mime_type":     "image/jpeg",
		"size":          int64(1024),
		"uploader_id":   "user-123",
	}

	deletedAt := time.Now()
	messages.On("FindByAttachmentID", ctx, attachmentID).Return(conversationID, metadata, &deletedAt, nil)

	svc := newTestService(nil, messages, nil)
	reader, resp, err := svc.Download(ctx, "user-123", attachmentID)

	assert.ErrorIs(t, err, apperr.ErrNotFound)
	assert.Nil(t, reader)
	assert.Nil(t, resp)
	messages.AssertExpectations(t)
}

func TestService_Download_RequesterNotParticipant(t *testing.T) {
	ctx := context.Background()
	messages := newMockMessageStore()
	conversations := newMockConversationStore()

	const (
		requesterID    = "user-stranger"
		otherID1       = "user-low"
		otherID2       = "user-high"
		attachmentID   = "att-123"
		conversationID = "conv-456"
	)

	metadata := map[string]any{
		"attachment_id": attachmentID,
		"object_key":    "attachments/att-123",
		"filename":      "photo.jpg",
		"mime_type":     "image/jpeg",
		"size":          int64(1024),
		"uploader_id":   otherID1,
	}

	messages.On("FindByAttachmentID", ctx, attachmentID).Return(conversationID, metadata, nil, nil)
	conversations.On("GetParticipants", ctx, conversationID).Return(otherID1, otherID2, nil)

	svc := newTestService(nil, messages, conversations)
	reader, resp, err := svc.Download(ctx, requesterID, attachmentID)

	assert.ErrorIs(t, err, apperr.ErrNotFound)
	assert.Nil(t, reader)
	assert.Nil(t, resp)
	messages.AssertExpectations(t)
	conversations.AssertExpectations(t)
}

func TestService_Download_FindByAttachmentID_Error(t *testing.T) {
	ctx := context.Background()
	messages := newMockMessageStore()

	const attachmentID = "att-123"

	testErr := errors.New("database error")
	messages.On("FindByAttachmentID", ctx, attachmentID).Return("", nil, nil, testErr)

	svc := newTestService(nil, messages, nil)
	reader, resp, err := svc.Download(ctx, "user-123", attachmentID)

	assert.ErrorIs(t, err, testErr)
	assert.Nil(t, reader)
	assert.Nil(t, resp)
	messages.AssertExpectations(t)
}

func TestService_Download_GetParticipants_Error(t *testing.T) {
	ctx := context.Background()
	messages := newMockMessageStore()
	conversations := newMockConversationStore()

	const (
		attachmentID   = "att-123"
		conversationID = "conv-456"
	)

	metadata := map[string]any{
		"attachment_id": attachmentID,
		"object_key":    "attachments/att-123",
		"filename":      "photo.jpg",
		"mime_type":     "image/jpeg",
		"size":          int64(1024),
		"uploader_id":   "user-123",
	}

	messages.On("FindByAttachmentID", ctx, attachmentID).Return(conversationID, metadata, nil, nil)

	testErr := errors.New("conversation not found")
	conversations.On("GetParticipants", ctx, conversationID).Return("", "", testErr)

	svc := newTestService(nil, messages, conversations)
	reader, resp, err := svc.Download(ctx, "user-123", attachmentID)

	assert.ErrorIs(t, err, testErr)
	assert.Nil(t, reader)
	assert.Nil(t, resp)
	messages.AssertExpectations(t)
	conversations.AssertExpectations(t)
}

func TestService_Download_GetObject_Error(t *testing.T) {
	ctx := context.Background()
	messages := newMockMessageStore()
	conversations := newMockConversationStore()
	objects := newMockObjectStore()

	const (
		requesterID    = "user-low"
		otherID        = "user-high"
		attachmentID   = "att-123"
		conversationID = "conv-456"
		objectKey      = "attachments/att-123"
	)

	metadata := map[string]any{
		"attachment_id": attachmentID,
		"object_key":    objectKey,
		"filename":      "photo.jpg",
		"mime_type":     "image/jpeg",
		"size":          int64(1024),
		"uploader_id":   otherID,
	}

	messages.On("FindByAttachmentID", ctx, attachmentID).Return(conversationID, metadata, nil, nil)
	conversations.On("GetParticipants", ctx, conversationID).Return(requesterID, otherID, nil)

	testErr := errors.New("object not found in storage")
	objects.On("GetObject", ctx, objectKey).Return(nil, testErr)

	svc := newTestService(objects, messages, conversations)
	reader, resp, err := svc.Download(ctx, requesterID, attachmentID)

	assert.ErrorIs(t, err, testErr)
	assert.Nil(t, reader)
	assert.Nil(t, resp)
	messages.AssertExpectations(t)
	conversations.AssertExpectations(t)
	objects.AssertExpectations(t)
}
