package message

import (
	"context"
	"strings"
	"time"

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

type Option func(*Service)

func WithIDGen(fn func() string) Option {
	return func(s *Service) { s.idGen = fn }
}

func WithClock(fn func() time.Time) Option {
	return func(s *Service) { s.clock = fn }
}

type Service struct {
	repository        RepositoryInterface
	conversationStore ConversationStore
	blockChecker      BlockChecker
	publisher         EventPublisher
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

	if strings.TrimSpace(request.Content) == "" {
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

	content := request.Content
	msg := &Message{
		ID:             s.idGen(),
		ConversationID: conversationID,
		SenderID:       userID,
		Content:        &content,
		ReplyToID:      request.ReplyToID,
		CreatedAt:      s.clock(),
	}

	if err := s.repository.Create(ctx, msg); err != nil {
		return nil, err
	}

	response := toResponse(msg)
	s.publisher.FanOutToConversation(ctx, userID, other, "message.new", response)

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

	response := toResponse(msg)
	s.publisher.FanOutToConversation(ctx, userID, other, "message.deleted", response)

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

	return response, nil
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
