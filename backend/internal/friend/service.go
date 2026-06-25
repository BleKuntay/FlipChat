package friend

import (
	"context"

	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
)

const defaultLimit = 20

type RepositoryInterface interface {
	FindAllRequests(ctx context.Context, userID string, query RequestListQuery) ([]Record, error)
	FindAll(ctx context.Context, userID string, query ListQuery) ([]Response, error)
	ExistsByUserID(ctx context.Context, userID string) (bool, error)
	FindByPair(ctx context.Context, lowID, highID string) (*Friend, error)
	AddFriend(ctx context.Context, lowID, highID, requesterID string) (*Record, error)
	AcceptFriend(ctx context.Context, lowID, highID string) (*Record, error)
	DeleteByPair(ctx context.Context, lowID, highID string) error
}

type Service struct {
	repository RepositoryInterface
}

func NewService(repository RepositoryInterface) *Service {
	return &Service{repository: repository}
}

func (s *Service) FindAllRequests(ctx context.Context, userID string, query RequestListQuery) (*RequestListResponse, error) {
	if query.Limit <= 0 {
		query.Limit = defaultLimit
	}

	records, err := s.repository.FindAllRequests(ctx, userID, query)
	if err != nil {
		return nil, err
	}

	var nextCursor *string
	if len(records) > query.Limit {
		nextCursor = &records[len(records)-1].UserID
		records = records[:query.Limit]
	}

	requests := make([]PendingResponse, len(records))
	for i, r := range records {
		direction := "received"
		if r.RequesterID == userID {
			direction = "sent"
		}
		requests[i] = PendingResponse{
			UserID:    r.UserID,
			Username:  r.Username,
			FullName:  r.FullName,
			Direction: direction,
			CreatedAt: r.CreatedAt,
		}
	}

	return &RequestListResponse{
		Requests:   requests,
		NextCursor: nextCursor,
	}, nil
}

func (s *Service) FindAll(ctx context.Context, userID string, query ListQuery) (*ListResponse, error) {
	if query.Limit <= 0 {
		query.Limit = defaultLimit
	}

	friends, err := s.repository.FindAll(ctx, userID, query)
	if err != nil {
		return nil, err
	}

	var nextCursor *string
	if len(friends) > query.Limit {
		nextCursor = &friends[len(friends)-1].UserID
		friends = friends[:query.Limit]
	}

	return &ListResponse{
		Friends:    friends,
		NextCursor: nextCursor,
	}, nil
}

func (s *Service) FindOne(ctx context.Context, userID, targetID string) (*StatusResponse, error) {
	low, high := canonical(userID, targetID)

	f, err := s.repository.FindByPair(ctx, low, high)
	if err != nil {
		return nil, err
	}

	if f == nil {
		return &StatusResponse{Status: StatusNone}, nil
	}

	if f.Status == StatusAccepted {
		return &StatusResponse{Status: StatusAccepted}, nil
	}

	if f.RequesterID == userID {
		return &StatusResponse{Status: StatusPendingSent}, nil
	}

	return &StatusResponse{Status: StatusPendingReceived}, nil
}

func (s *Service) AddFriend(ctx context.Context, userID, targetID string) (*PendingResponse, error) {
	if userID == targetID {
		return nil, apperr.ErrBadRequest
	}

	exists, err := s.repository.ExistsByUserID(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, apperr.ErrNotFound
	}

	// TODO: check block (both ways)

	low, high := canonical(userID, targetID)

	pair, err := s.repository.FindByPair(ctx, low, high)
	if err != nil {
		return nil, err
	}

	if pair != nil {
		if pair.Status == StatusAccepted {
			return nil, apperr.ErrConflict
		}
		if pair.RequesterID == userID {
			return nil, apperr.ErrConflict
		}
		// mutual request → auto-accept
		record, err := s.repository.AcceptFriend(ctx, low, high)
		if err != nil {
			return nil, err
		}
		return toSentPending(record), nil
	}

	record, err := s.repository.AddFriend(ctx, low, high, userID)
	if err != nil {
		return nil, err
	}

	return toSentPending(record), nil
}

func (s *Service) Unfriend(ctx context.Context, userID, targetID string) error {
	if userID == targetID {
		return apperr.ErrBadRequest
	}

	low, high := canonical(userID, targetID)

	pair, err := s.repository.FindByPair(ctx, low, high)
	if err != nil {
		return err
	}
	if pair == nil || pair.Status != StatusAccepted {
		return apperr.ErrNotFound
	}

	return s.repository.DeleteByPair(ctx, low, high)
}

func (s *Service) CancelFriendRequest(ctx context.Context, userID, targetID string) error {
	if userID == targetID {
		return apperr.ErrBadRequest
	}

	low, high := canonical(userID, targetID)

	pair, err := s.repository.FindByPair(ctx, low, high)
	if err != nil {
		return err
	}
	if pair == nil {
		return apperr.ErrNotFound
	}
	if pair.Status == StatusAccepted {
		return apperr.ErrConflict
	}
	if pair.RequesterID != userID {
		return apperr.ErrForbidden
	}

	return s.repository.DeleteByPair(ctx, low, high)
}

func (s *Service) AcceptFriendRequest(ctx context.Context, userID, targetID string) (*Response, error) {
	if userID == targetID {
		return nil, apperr.ErrBadRequest
	}

	low, high := canonical(userID, targetID)

	pair, err := s.repository.FindByPair(ctx, low, high)
	if err != nil {
		return nil, err
	}
	if pair == nil {
		return nil, apperr.ErrNotFound
	}
	if pair.Status == StatusAccepted {
		return nil, apperr.ErrConflict
	}
	if pair.RequesterID == userID {
		return nil, apperr.ErrForbidden
	}

	record, err := s.repository.AcceptFriend(ctx, low, high)
	if err != nil {
		return nil, err
	}

	return &Response{
		UserID:      record.UserID,
		Username:    record.Username,
		FullName:    record.FullName,
		FriendSince: record.CreatedAt,
	}, nil
}

func (s *Service) DeclineFriendRequest(ctx context.Context, userID, targetID string) error {
	if userID == targetID {
		return apperr.ErrBadRequest
	}

	low, high := canonical(userID, targetID)

	pair, err := s.repository.FindByPair(ctx, low, high)
	if err != nil {
		return err
	}
	if pair == nil || pair.Status != StatusPending {
		return apperr.ErrNotFound
	}
	if pair.RequesterID == userID {
		return apperr.ErrForbidden
	}

	return s.repository.DeleteByPair(ctx, low, high)
}

func canonical(a, b string) (low, high string) {
	if a < b {
		return a, b
	}
	return b, a
}

func toSentPending(r *Record) *PendingResponse {
	return &PendingResponse{
		UserID:    r.UserID,
		Username:  r.Username,
		FullName:  r.FullName,
		Direction: "sent",
		CreatedAt: r.CreatedAt,
	}
}
