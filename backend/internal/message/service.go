package message

import (
	"context"
	"encoding/json"
	"github.com/BleKuntay/FlipChat/backend/pkg/logger"
	"go.uber.org/zap"
	"strings"
	"time"

	"github.com/BleKuntay/FlipChat/backend/internal/attachment"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/google/uuid"
)

const (
	DefaultLimit  = 50
	MaxLimit      = 100
	editWindowDur = 15 * time.Minute
)

type RepositoryInterface interface {
	Create(ctx context.Context, message *Message) error
	GetByID(ctx context.Context, messageID string) (*Message, error)
	ListByConversation(ctx context.Context, conversationID, cursor string, limit int) ([]*Message, error)
	EditMessage(ctx context.Context, message *Message) error
	Delete(ctx context.Context, message *Message) error
	MarkAsRead(ctx context.Context, message *Message) error
}

type ConversationStore interface {
	GetParticipants(ctx context.Context, conversationID string) (userLowID, userHighID string, err error)
}

type BlockChecker interface {
	IsBlockedEitherWay(ctx context.Context, a, b string) (bool, error)
}

type EventPublisher interface {
	FanOutToConversation(ctx context.Context, senderID, recipientID string, eventType string, payload any)
}

type ObjectDeleter interface {
	DeleteObject(ctx context.Context, objectKey string) error
}

// AttachmentStore atomically fetches and deletes a temporary upload record.
// Returns apperr.ErrNotFound if the record does not exist or has expired.
type AttachmentStore interface {
	PopUploadRecord(ctx context.Context, attachmentID string) (*attachment.UploadRecord, error)
}

type Option func(*Service)

func WithIDGen(fn func() string) Option {
	return func(s *Service) { s.idGen = fn }
}

func WithClock(fn func() time.Time) Option {
	return func(s *Service) { s.clock = fn }
}

func WithAttachmentStore(store AttachmentStore) Option {
	return func(s *Service) { s.attachments = store }
}

type Service struct {
	repository        RepositoryInterface
	conversationStore ConversationStore
	blockChecker      BlockChecker
	publisher         EventPublisher
	objects           ObjectDeleter
	attachments       AttachmentStore
	idGen             func() string
	clock             func() time.Time
}

func NewService(repository RepositoryInterface, store ConversationStore, checker BlockChecker, publisher EventPublisher, opts ...Option) *Service {
	svc := &Service{
		repository:        repository,
		conversationStore: store,
		publisher:         publisher,
		blockChecker:      checker,
		idGen:             func() string { v, _ := uuid.NewV7(); return v.String() },
		clock:             time.Now,
	}

	for _, o := range opts {
		o(svc)
	}

	return svc
}

// ── public methods ────────────────────────────────────────────────────────────

func (s *Service) ListMessages(ctx context.Context, userID, conversationID string, query ListQuery) (*ListResponse, error) {
	if err := s.mustBeParticipant(ctx, userID, conversationID); err != nil {
		return nil, err
	}

	limit := clampLimit(query.Limit)

	messages, err := s.repository.ListByConversation(ctx, conversationID, query.Cursor, limit+1)
	if err != nil {
		return nil, err
	}

	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}

	var nextCursor *string
	if hasMore {
		lastID := messages[len(messages)-1].ID
		nextCursor = &lastID
	}

	data := make([]*Response, 0, len(messages))
	for _, m := range messages {
		data = append(data, toResponse(m))
	}

	return &ListResponse{
		Data:       data,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (s *Service) SendMessage(ctx context.Context, userID, conversationID string, request SendRequest) (*Response, error) {
	other, err := s.otherParticipant(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}

	blocked, err := s.blockChecker.IsBlockedEitherWay(ctx, userID, other)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, apperr.ErrNotFound
	}

	hasContent := strings.TrimSpace(request.Content) != ""
	hasAttachment := request.AttachmentID != nil

	if !hasContent && !hasAttachment {
		return nil, apperr.ErrBadRequest
	}

	if request.ReplyToID != nil {
		reply, err := s.repository.GetByID(ctx, *request.ReplyToID)
		if err != nil {
			return nil, err
		}
		if reply.ConversationID != conversationID {
			return nil, apperr.ErrForbidden
		}
	}

	var content *string
	if hasContent {
		c := request.Content
		content = &c
	}

	var rawMetadata *json.RawMessage
	if hasAttachment {
		if s.attachments == nil {
			return nil, apperr.ErrBadRequest
		}

		record, err := s.attachments.PopUploadRecord(ctx, *request.AttachmentID)
		if err != nil {
			return nil, err
		}
		if record.UploaderID != userID {
			logger.Warn("message: attachment ownership mismatch",
				zap.String("sender_id", userID),
				zap.String("uploader_id", record.UploaderID),
				zap.String("attachment_id", *request.AttachmentID),
			)
			return nil, apperr.ErrForbidden
		}

		meta := map[string]any{
			"attachment_id": *request.AttachmentID,
			"object_key":    record.ObjectKey,
			"filename":      record.Filename,
			"mime_type":     record.MIMEType,
			"size":          record.Size,
		}

		b, err := json.Marshal(meta)
		if err != nil {
			return nil, err
		}

		raw := json.RawMessage(b)
		rawMetadata = &raw
	}

	msg := &Message{
		ID:             s.idGen(),
		ConversationID: conversationID,
		SenderID:       userID,
		Content:        content,
		ReplyToID:      request.ReplyToID,
		Metadata:       rawMetadata,
		CreatedAt:      s.clock(),
	}

	if err := s.repository.Create(ctx, msg); err != nil {
		return nil, err
	}

	response := toResponse(msg)
	s.publisher.FanOutToConversation(ctx, userID, other, "message.new", response)

	logger.Info("message: sent",
		zap.String("message_id", response.ID),
		zap.String("conversation_id", response.ConversationID),
		zap.String("sender_id", response.SenderID),
	)

	return response, nil
}

func (s *Service) EditMessage(ctx context.Context, userID, conversationID, messageID string, request EditRequest) (*Response, error) {
	other, err := s.otherParticipant(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}

	msg, err := s.fetchMessageInConversation(ctx, messageID, conversationID)
	if err != nil {
		return nil, err
	}
	if msg.SenderID != userID {
		return nil, apperr.ErrForbidden
	}
	if msg.DeletedAt != nil {
		return nil, apperr.ErrForbidden
	}
	if s.clock().Sub(msg.CreatedAt) > editWindowDur {
		return nil, apperr.ErrForbidden
	}

	now := s.clock()
	msg.Content = &request.Content
	msg.IsEdited = true
	msg.UpdatedAt = &now

	if err := s.repository.EditMessage(ctx, msg); err != nil {
		return nil, err
	}

	response := toResponse(msg)
	s.publisher.FanOutToConversation(ctx, userID, other, "message.edited", response)

	logger.Info("message: edited",
		zap.String("message_id", response.ID),
		zap.String("conversation_id", response.ConversationID),
	)

	return response, nil
}

func (s *Service) DeleteMessage(ctx context.Context, userID, conversationID, messageID string) (*Response, error) {
	other, err := s.otherParticipant(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}

	msg, err := s.fetchMessageInConversation(ctx, messageID, conversationID)
	if err != nil {
		return nil, err
	}
	if msg.SenderID != userID {
		return nil, apperr.ErrForbidden
	}
	if msg.DeletedAt != nil {
		return nil, apperr.ErrConflict
	}

	now := s.clock()
	msg.Content = nil
	msg.DeletedBy = &userID
	msg.DeletedAt = &now

	if err := s.repository.Delete(ctx, msg); err != nil {
		return nil, err
	}

	// Delete the MinIO object after the DB row is committed.
	// A failure here is logged but does not fail the request — a dangling
	// object is recoverable; an inconsistent DB row is not.
	if s.objects != nil && msg.Metadata != nil {
		if err := s.deleteAttachmentObject(ctx, msg.ID, msg.Metadata); err != nil {
			logger.Error("message: failed to delete attachment object, manual cleanup may be needed",
				zap.String("message_id", msg.ID),
				zap.Error(err),
			)
		}
	}

	response := toResponse(msg)
	s.publisher.FanOutToConversation(ctx, userID, other, "message.deleted", response)

	logger.Info("message: deleted",
		zap.String("message_id", response.ID),
		zap.String("conversation_id", response.ConversationID),
	)

	return response, nil
}

func (s *Service) MarkAsRead(ctx context.Context, userID, conversationID, messageID string) (*Response, error) {
	other, err := s.otherParticipant(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}

	message, err := s.fetchMessageInConversation(ctx, messageID, conversationID)
	if err != nil {
		return nil, err
	}
	if message.SenderID == userID {
		return nil, apperr.ErrForbidden
	}
	if message.ReadAt != nil {
		return toResponse(message), nil
	}
	if message.DeletedAt != nil {
		return nil, apperr.ErrForbidden
	}

	now := s.clock()
	message.ReadAt = &now

	if err := s.repository.MarkAsRead(ctx, message); err != nil {
		return nil, err
	}

	response := toResponse(message)
	s.publisher.FanOutToConversation(ctx, userID, other, "message.read", response)

	logger.Info("message: marked as read",
		zap.String("message_id", response.ID),
		zap.String("reader_id", userID),
	)

	return response, nil
}

func WithObjectDeleter(d ObjectDeleter) Option {
	return func(s *Service) { s.objects = d }
}

// ── private helpers ───────────────────────────────────────────────────────────

// mustBeParticipant verifies userID is a participant of conversationID.
// Returns ErrNotFound if the conversation does not exist or user is not a participant.
func (s *Service) mustBeParticipant(ctx context.Context, userID, conversationID string) error {
	low, high, err := s.conversationStore.GetParticipants(ctx, conversationID)
	if err != nil {
		return err
	}
	if userID != low && userID != high {
		return apperr.ErrNotFound
	}
	return nil
}

// otherParticipant returns the participant ID that is not userID.
// Returns ErrNotFound if the conversation does not exist or user is not a participant.
func (s *Service) otherParticipant(ctx context.Context, userID, conversationID string) (string, error) {
	low, high, err := s.conversationStore.GetParticipants(ctx, conversationID)
	if err != nil {
		return "", err
	}
	if userID != low && userID != high {
		return "", apperr.ErrNotFound
	}
	if userID == low {
		return high, nil
	}
	return low, nil
}

// fetchMessageInConversation fetches a message and verifies it belongs to conversationID.
// Returns ErrNotFound if the message does not exist or belongs to a different conversation.
func (s *Service) fetchMessageInConversation(ctx context.Context, messageID, conversationID string) (*Message, error) {
	msg, err := s.repository.GetByID(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if msg.ConversationID != conversationID {
		return nil, apperr.ErrNotFound
	}
	return msg, nil
}

// clampLimit applies default and max bounds to a requested limit.
func clampLimit(limit int) int {
	if limit <= 0 {
		return DefaultLimit
	}
	if limit > MaxLimit {
		return MaxLimit
	}
	return limit
}

// toResponse converts a Message to a Response.
// Content is always nil for deleted messages regardless of DB state.
func toResponse(m *Message) *Response {
	content := m.Content
	if m.DeletedAt != nil {
		content = nil
	}

	return &Response{
		ID:             m.ID,
		ConversationID: m.ConversationID,
		SenderID:       m.SenderID,
		Content:        content,
		ReplyToID:      m.ReplyToID,
		Metadata:       m.Metadata,
		IsEdited:       m.IsEdited,
		IsDeleted:      m.DeletedAt != nil,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
		ReadAt:         m.ReadAt,
	}
}

func (s *Service) deleteAttachmentObject(ctx context.Context, messageID string, raw *json.RawMessage) error {
	var metadata map[string]any
	if err := json.Unmarshal(*raw, &metadata); err != nil {
		logger.Warn("message: failed to unmarshal attachment metadata, skipping object deletion",
			zap.String("message_id", messageID),
			zap.Error(err),
		)

		return nil
	}

	key, ok := metadata["object_key"].(string)
	if !ok || key == "" {
		return nil
	}

	return s.objects.DeleteObject(ctx, key)
}
