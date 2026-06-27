package user

import (
	"context"

	"github.com/BleKuntay/FlipChat/backend/internal/shared"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
)

type RepositoryInterface interface {
	FindByID(ctx context.Context, userID string) (*User, error)
	UpdateProfile(ctx context.Context, u *User) (*UpdateProfileResponse, error)
	UpdateEmail(ctx context.Context, userID string, req *UpdateEmailRequest) (*MeResponse, error)
	UpdatePassword(ctx context.Context, userID, hashedPassword string) error
	DeleteByID(ctx context.Context, userID string) error
	Search(ctx context.Context, userID, q, cursor string, limit int) ([]*Summary, error)
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

func (s *Service) Me(ctx context.Context, userID string) (*MeResponse, error) {
	user, err := s.repository.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &MeResponse{
		Response: Response{
			ID:         user.ID,
			Name:       user.Name,
			Username:   user.Username,
			Bio:        user.Bio,
			Language:   user.Language,
			AvatarURL:  user.AvatarURL,
			LastSeenAt: user.LastSeenAt,
			CreatedAt:  user.CreatedAt,
		},
		Email: user.Email,
	}, nil
}

func (s *Service) UpdateProfile(ctx context.Context, userID string, request *UpdateProfileRequest) (*UpdateProfileResponse, error) {
	user, err := s.repository.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	updated := false

	if request.Name != "" {
		user.Name = request.Name
		updated = true
	}

	if request.Username != "" {
		user.Username = request.Username
		updated = true
	}

	if request.Bio != "" {
		user.Bio = &request.Bio
		updated = true
	}

	if request.Language != "" {
		user.Language = request.Language
		updated = true
	}

	if !updated {
		return nil, ErrUserNotUpdated
	}

	return s.repository.UpdateProfile(ctx, user)
}

func (s *Service) UpdateEmail(ctx context.Context, userID string, request *UpdateEmailRequest) (*UpdateEmailResponse, error) {
	user, err := s.repository.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if !shared.VerifyPassword(request.CurrentPassword, user.Password) {
		return nil, ErrInvalidPassword
	}

	return s.repository.UpdateEmail(ctx, userID, request)
}

func (s *Service) ChangePassword(ctx context.Context, userID string, request *ChangePasswordRequest) error {
	if request.NewPassword != request.ConfirmPassword {
		return ErrPasswordMismatch
	}

	user, err := s.repository.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	if !shared.VerifyPassword(request.CurrentPassword, user.Password) {
		return ErrInvalidPassword
	}

	hashedPassword, err := shared.HashPassword(request.NewPassword)
	if err != nil {
		return err
	}

	return s.repository.UpdatePassword(ctx, user.ID, hashedPassword)
}

func (s *Service) DeleteAccount(ctx context.Context, userID string) error {
	return s.repository.DeleteByID(ctx, userID)
}

func (s *Service) FindUserByID(ctx context.Context, requesterID string, param *GetUserURI) (*User, error) {
	user, err := s.repository.FindByID(ctx, param.ID)
	if err != nil {
		return nil, err
	}

	blocked, err := s.blockChecker.IsBlockedEitherWay(ctx, requesterID, param.ID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, apperr.ErrNotFound
	}

	return user, nil
}

func (s *Service) Search(ctx context.Context, userID string, query *SearchQuery) (*SearchResponse, error) {
	const defaultLimit = 20
	const maxLimit = 50

	if query.Limit <= 0 {
		query.Limit = defaultLimit
	}
	if query.Limit > maxLimit {
		query.Limit = maxLimit
	}

	summaries, err := s.repository.Search(ctx, userID, query.Q, query.Cursor, query.Limit+1)
	if err != nil {
		return nil, err
	}

	var nextCursor *string
	if len(summaries) > query.Limit {
		summaries = summaries[:query.Limit]
		cursor := summaries[len(summaries)-1].ID
		nextCursor = &cursor
	}

	return &SearchResponse{
		Data:       summaries,
		NextCursor: nextCursor,
	}, nil
}
