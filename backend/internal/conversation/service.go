package conversation

import (
	"context"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
)

type RepositoryInterface interface {
	FindAllByUserID(ctx context.Context, userID string) ([]Response, error)
	FindByID(ctx context.Context, requesterID, conversationID string) (*Response, error)
	FindByPair(ctx context.Context, lowID, highID string) (*Conversation, error)
	Create(ctx context.Context, requesterID, lowID, highID string) (*Response, error)
}

type BlockChecker interface {
	IsBlockedEitherWay(ctx context.Context, a, b string) (bool, error)
}

type Service struct {
	repository   RepositoryInterface
	blockChecker BlockChecker
}

func NewService(repository RepositoryInterface, blockChecker BlockChecker) *Service {
	return &Service{repository: repository, blockChecker: blockChecker}
}

func (s *Service) GetConversationList(ctx context.Context, requesterID string) (*ListResponse, error) {
	list, err := s.repository.FindAllByUserID(ctx, requesterID)
	if err != nil {
		return nil, err
	}

	return &ListResponse{Data: list}, nil
}

func (s *Service) GetConversation(ctx context.Context, requesterID, conversationID string) (*Response, error) {
	return s.repository.FindByID(ctx, requesterID, conversationID)
}

func (s *Service) CreateConversation(ctx context.Context, requesterID, targetUserID string) (*Response, bool, error) {
	if requesterID == targetUserID {
		return nil, false, apperr.ErrBadRequest
	}

	low, high := canonicalize(requesterID, targetUserID)

	pair, err := s.repository.FindByPair(ctx, low, high)
	if err != nil {
		return nil, false, err
	}
	if pair != nil {
		response, err := s.repository.FindByID(ctx, requesterID, pair.ID)
		if err != nil {
			return nil, false, err
		}

		return response, false, nil
	}

	blocked, err := s.blockChecker.IsBlockedEitherWay(ctx, low, high)
	if err != nil {
		return nil, false, err
	}
	if blocked {
		return nil, false, apperr.ErrNotFound
	}

	response, err := s.repository.Create(ctx, requesterID, low, high)
	if err != nil {
		return nil, false, err
	}

	return response, true, nil
}

func canonicalize(a, b string) (string, string) {
	if a < b {
		return a, b
	}

	return b, a
}
