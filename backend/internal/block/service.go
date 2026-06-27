package block

import (
	"context"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
)

const defaultLimit = 20

type RepositoryInterface interface {
	BlockUserAtomic(ctx context.Context, request Request, low, high string) (*Response, error)
	UnblockUser(ctx context.Context, request Request) error
	GetBlockList(ctx context.Context, blockerID string, query ListQuery) ([]BlockedSummary, error)
	IsBlockedEitherWay(ctx context.Context, a, b string) (bool, error)
}

type Service struct {
	repository RepositoryInterface
}

func NewService(repository RepositoryInterface) *Service {
	return &Service{repository: repository}
}

func (s *Service) BlockUser(ctx context.Context, request Request) (*Response, error) {
	if request.BlockerID == request.BlockedID {
		return nil, apperr.ErrBadRequest
	}

	low, high := canonical(request.BlockerID, request.BlockedID)

	return s.repository.BlockUserAtomic(ctx, request, low, high)
}

func (s *Service) UnblockUser(ctx context.Context, request Request) error {
	if request.BlockerID == request.BlockedID {
		return apperr.ErrBadRequest
	}

	return s.repository.UnblockUser(ctx, request)
}

func (s *Service) GetBlockList(ctx context.Context, blockerID string, query ListQuery) (*ListResponse, error) {
	if query.Limit <= 0 {
		query.Limit = defaultLimit
	}

	blocks, err := s.repository.GetBlockList(ctx, blockerID, query)
	if err != nil {
		return nil, err
	}

	var nextCursor string
	if len(blocks) > query.Limit {
		blocks = blocks[:query.Limit]
		nextCursor = blocks[len(blocks)-1].UserID
	}

	return &ListResponse{
		Data:       blocks,
		NextCursor: nextCursor,
	}, nil
}

func (s *Service) IsBlockedEitherWay(ctx context.Context, a, b string) (bool, error) {
	return s.repository.IsBlockedEitherWay(ctx, a, b)
}

func canonical(a, b string) (low, high string) {
	if a < b {
		return a, b
	}
	return b, a
}
