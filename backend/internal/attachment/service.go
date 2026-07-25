package attachment

import (
	"bytes"
	"context"
	"errors"
	"io"
	"time"

	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/BleKuntay/FlipChat/backend/pkg/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ObjectStore abstracts MinIO operations.
type ObjectStore interface {
	PutObject(ctx context.Context, objectKey string, reader io.Reader, size int64, mimeType string) error
	GetObject(ctx context.Context, objectKey string) (io.ReadCloser, error)
	DeleteObject(ctx context.Context, objectKey string) error
}

// MessageStore fetches message metadata for attachment authorization.
type MessageStore interface {
	FindByAttachmentID(ctx context.Context, attachmentID string) (conversationID string, metadata map[string]any, deletedAt *time.Time, err error)
}

// ConversationStore checks conversation participants.
type ConversationStore interface {
	GetParticipants(ctx context.Context, conversationID string) (userLowID, userHighID string, err error)
}

type Service struct {
	objects       ObjectStore
	uploads       UploadStore
	messages      MessageStore
	conversations ConversationStore
}

func NewService(objects ObjectStore, uploads UploadStore, messages MessageStore, conversations ConversationStore) *Service {
	return &Service{
		objects:       objects,
		uploads:       uploads,
		messages:      messages,
		conversations: conversations,
	}
}

func (s *Service) Upload(ctx context.Context, uploaderID, filename string, size int64, reader io.Reader) (*UploadResponse, error) {
	if size <= 0 || size > MaxFileSize {
		logger.Warn("attachment: rejected, file too large",
			zap.String("uploader_id", uploaderID),
			zap.Int64("size", size),
		)

		return nil, apperr.ErrFileTooLarge
	}
	if size < MinFileSize {
		return nil, apperr.ErrBadRequest
	}

	header, err := ReadHeader(reader)
	if err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, apperr.ErrBadRequest
		}

		return nil, err
	}

	mimeType, err := DetectMIME(header)
	if err != nil {
		logger.Warn("attachment: rejected, unsupported mime type",
			zap.String("uploader_id", uploaderID),
			zap.String("filename", filename),
		)

		return nil, apperr.ErrUnsupportedMIME
	}

	fullReader := io.MultiReader(bytes.NewReader(header), reader)

	attachmentID := uuid.New().String()
	objectKey := "attachments/" + attachmentID

	if err := s.objects.PutObject(ctx, objectKey, fullReader, size, mimeType); err != nil {
		return nil, err
	}

	record := UploadRecord{
		UploaderID: uploaderID,
		ObjectKey:  objectKey,
		MIMEType:   mimeType,
		Filename:   filename,
		Size:       size,
	}

	if err := s.uploads.SaveUploadRecord(ctx, attachmentID, record); err != nil {
		// Object is already in MinIO. Log and continue — the orphan will be
		// cleaned up by TTL expiry. Do not fail the upload over a Redis write.
		logger.Error("attachment: failed to save upload record",
			zap.String("attachment_id", attachmentID),
			zap.Error(err),
		)
	}

	logger.Info("attachment: uploaded",
		zap.String("attachment_id", attachmentID),
		zap.String("uploader_id", uploaderID),
		zap.String("mime_type", mimeType),
		zap.Int64("size", size),
	)

	return &UploadResponse{
		AttachmentID: attachmentID,
		Filename:     filename,
		MIMEType:     mimeType,
		Size:         size,
	}, nil
}

func (s *Service) Download(ctx context.Context, requesterID, attachmentID string) (io.ReadCloser, *Metadata, error) {
	conversationID, metadata, deletedAt, err := s.messages.FindByAttachmentID(ctx, attachmentID)
	if err != nil {
		return nil, nil, err
	}
	if conversationID == "" || metadata == nil {
		return nil, nil, apperr.ErrNotFound
	}
	if deletedAt != nil {
		return nil, nil, apperr.ErrNotFound
	}

	objectKey := getString(metadata, "object_key")
	if objectKey == "" {
		return nil, nil, apperr.ErrNotFound
	}

	low, high, err := s.conversations.GetParticipants(ctx, conversationID)
	if err != nil {
		return nil, nil, err
	}
	if requesterID != low && requesterID != high {
		logger.Warn("attachment: unauthorized download attempt",
			zap.String("requester_id", requesterID),
			zap.String("attachment_id", attachmentID),
		)

		return nil, nil, apperr.ErrNotFound
	}

	reader, err := s.objects.GetObject(ctx, objectKey)
	if err != nil {
		return nil, nil, err
	}

	attachmentMetadata := &Metadata{
		AttachmentID: getString(metadata, "attachment_id"),
		ObjectKey:    objectKey,
		Filename:     getString(metadata, "filename"),
		MIMEType:     getString(metadata, "mime_type"),
		Size:         getInt64(metadata, "size"),
	}

	return reader, attachmentMetadata, nil
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}

	return ""
}

func getInt64(m map[string]any, key string) int64 {
	switch v := m[key].(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	}

	return 0
}
