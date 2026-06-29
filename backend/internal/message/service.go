package message

import (
	"context"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/google/uuid"
	"strings"
	"time"
)

const (
	DefaultLimit = 50
	MaxLimit     = 100
)

type RepositoryInterface interface {
	Create(ctx context.Context, message *Message) error
	GetByID(ctx context.Context, messageID string) (*Message, error)
	ListByConversation(ctx context.Context, conversationID, cursor string, limit int) ([]*Message, error)
}

type ConversationStore interface {
	GetParticipants(ctx context.Context, conversationID string) (userLowID, userHighID string, err error)
}

type BlockChecker interface {
	IsBlockedEitherWay(ctx context.Context, a, b string) (bool, error)
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
	idGen             func() string
	clock             func() time.Time
}

func NewService(repository RepositoryInterface, store ConversationStore, checker BlockChecker, opts ...Option) *Service {
	service := &Service{
		repository:        repository,
		conversationStore: store,
		blockChecker:      checker,
		idGen:             func() string { v, _ := uuid.NewV7(); return v.String() },
		clock:             time.Now,
	}

	for _, o := range opts {
		o(service)
	}

	return service
}

func (s *Service) ListMessages(ctx context.Context, userID, conversationID string, query ListQuery) (*ListResponse, error) {
	low, high, err := s.conversationStore.GetParticipants(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if userID != low && userID != high {
		return nil, apperr.ErrNotFound
	}

	limit := query.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

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
	for _, message := range messages {
		data = append(data, toResponse(message))
	}

	response := &ListResponse{
		Data:       data,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}

	return response, nil
}

func (s *Service) SendMessage(ctx context.Context, userID, conversationID string, request SendRequest) (*Response, error) {
	low, high, err := s.conversationStore.GetParticipants(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if userID != low && userID != high {
		return nil, apperr.ErrNotFound
	}

	other := low
	if userID == low {
		other = high
	}

	blocked, err := s.blockChecker.IsBlockedEitherWay(ctx, userID, other)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, apperr.ErrForbidden
	}

	content := request.Content
	if strings.TrimSpace(request.Content) == "" {
		return nil, apperr.ErrBadRequest
	}

	if request.ReplyToID != nil {
		replyMessage, err := s.repository.GetByID(ctx, *request.ReplyToID)
		if err != nil {
			return nil, err
		}
		if replyMessage.ConversationID != conversationID {
			return nil, apperr.ErrForbidden
		}
	}

	message := &Message{
		ID:             s.idGen(),
		ConversationID: conversationID,
		SenderID:       userID,
		Content:        &content,
		ReplyToID:      request.ReplyToID,
		CreatedAt:      s.clock(),
	}

	if err := s.repository.Create(ctx, message); err != nil {
		return nil, err
	}

	return toResponse(message), nil
}

func toResponse(m *Message) *Response {
	return &Response{
		ID:             m.ID,
		ConversationID: m.ConversationID,
		SenderID:       m.SenderID,
		Content:        m.Content,
		ReplyToID:      m.ReplyToID,
		Metadata:       m.Metadata,
		IsEdited:       m.IsEdited,
		CreatedAt:      m.CreatedAt,
	}
}
