package user

import (
	"github.com/BleKuntay/FlipChat/backend/internal/shared"
)

type RepositoryInterface interface {
	FindByID(userID string) (*User, error)
	UpdateProfile(u *User) (*UpdateProfileResponse, error)
	UpdateEmail(userID string, req *UpdateEmailRequest) (*MeResponse, error)
	UpdatePassword(userID, hashedPassword string) error
	DeleteByID(userID string) error
	Search(userID, q, cursor string, limit int) ([]*Summary, error)
}

type Service struct {
	repository RepositoryInterface
}

func NewService(repository RepositoryInterface) *Service {
	return &Service{repository: repository}
}

func (s *Service) Me(userID string) (*MeResponse, error) {
	user, err := s.repository.FindByID(userID)
	if err != nil {
		return nil, err
	}

	me := &MeResponse{
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
	}

	return me, nil
}

func (s *Service) UpdateProfile(userID string, request *UpdateProfileRequest) (*UpdateProfileResponse, error) {
	user, err := s.repository.FindByID(userID)
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

	updatedUser, err := s.repository.UpdateProfile(user)
	if err != nil {
		return nil, err
	}

	return updatedUser, nil
}

func (s *Service) UpdateEmail(userID string, request *UpdateEmailRequest) (*UpdateEmailResponse, error) {
	user, err := s.repository.FindByID(userID)
	if err != nil {
		return nil, err
	}

	if !shared.VerifyPassword(request.CurrentPassword, user.Password) {
		return nil, ErrInvalidPassword
	}

	me, err := s.repository.UpdateEmail(userID, request)
	if err != nil {
		return nil, err
	}

	return me, nil
}

func (s *Service) ChangePassword(userID string, request *ChangePasswordRequest) error {
	if request.NewPassword != request.ConfirmPassword {
		return ErrPasswordMismatch
	}

	user, err := s.repository.FindByID(userID)
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

	return s.repository.UpdatePassword(user.ID, hashedPassword)
}

func (s *Service) DeleteAccount(userID string) error {
	return s.repository.DeleteByID(userID)
}

func (s *Service) FindUserByID(param *GetUserURI) (*User, error) {
	user, err := s.repository.FindByID(param.ID)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (s *Service) Search(userID string, query *SearchQuery) (*SearchResponse, error) {
	const defaultLimit = 20
	const maxLimit = 50

	if query.Limit <= 0 {
		query.Limit = defaultLimit
	}
	if query.Limit > maxLimit {
		query.Limit = maxLimit
	}

	summaries, err := s.repository.Search(userID, query.Q, query.Cursor, query.Limit)
	if err != nil {
		return nil, err
	}

	var nextCursor *string
	if len(summaries) == query.Limit {
		last := summaries[len(summaries)-1]
		nextCursor = &last.ID
	}

	return &SearchResponse{
		Data:       summaries,
		NextCursor: nextCursor,
	}, nil
}
