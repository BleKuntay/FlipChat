package user

import (
	"context"
	"github.com/BleKuntay/FlipChat/backend/internal/shared"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/BleKuntay/FlipChat/backend/pkg/logger"
	"go.uber.org/zap"
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

type PresenceChecker interface {
	IsOnline(ctx context.Context, userID string) (bool, error)
	AreOnline(ctx context.Context, userIDs []string) (map[string]bool, error)
}

// SessionRevoker revokes all active sessions for a user.
// Implemented by auth.Repository; injected via interface to avoid
// a circular import between user and auth packages.
type SessionRevoker interface {
	DeleteTokenByUserID(ctx context.Context, userID string) error
}

type Service struct {
	repository      RepositoryInterface
	blockChecker    BlockChecker
	presenceChecker PresenceChecker
	sessionRevoker  SessionRevoker
}

func NewService(repository RepositoryInterface, blockChecker BlockChecker, presenceChecker PresenceChecker, sessionRevoker SessionRevoker) *Service {
	return &Service{
		repository:      repository,
		blockChecker:    blockChecker,
		presenceChecker: presenceChecker,
		sessionRevoker:  sessionRevoker,
	}
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

	if request.Bio != nil {
		if *request.Bio == "" {
			user.Bio = nil
		} else {
			user.Bio = request.Bio
		}
		updated = true
	}

	if request.Language != "" {
		user.Language = request.Language
		updated = true
	}

	if !updated {
		return nil, ErrUserNotUpdated
	}

	response, err := s.repository.UpdateProfile(ctx, user)
	if err != nil {
		return nil, err
	}

	logger.Info("user: profile updated", zap.String("user_id", userID))

	return response, nil
}

func (s *Service) UpdateEmail(ctx context.Context, userID string, request *UpdateEmailRequest) (*UpdateEmailResponse, error) {
	user, err := s.repository.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if !shared.VerifyPassword(request.CurrentPassword, user.Password) {
		return nil, ErrInvalidPassword
	}

	logger.Info("user: email updated", zap.String("user_id", userID))

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

	if err := s.repository.UpdatePassword(ctx, user.ID, hashedPassword); err != nil {
		return err
	}

	if s.sessionRevoker != nil {
		if err := s.sessionRevoker.DeleteTokenByUserID(ctx, userID); err != nil {
			logger.Error("user: failed to revoke sessions after password change",
				zap.String("user_id", userID),
				zap.Error(err),
			)
		}
	}

	logger.Info("user: password changed, all sessions revoked", zap.String("user_id", userID))

	return nil
}

func (s *Service) DeleteAccount(ctx context.Context, userID string) error {
	// Revoke sessions first so the user cannot refresh their way back in
	// after the account row is deleted.
	if s.sessionRevoker != nil {
		if err := s.sessionRevoker.DeleteTokenByUserID(ctx, userID); err != nil {
			logger.Error("user: failed to revoke sessions before account deletion",
				zap.String("user_id", userID),
				zap.Error(err),
			)
		}
	}

	if err := s.repository.DeleteByID(ctx, userID); err != nil {
		return err
	}

	logger.Info("user: account deleted", zap.String("user_id", userID))

	return nil
}

func (s *Service) FindUserByID(ctx context.Context, requesterID string, param *GetUserURI) (*Response, error) {
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

	isOnline, err := s.presenceChecker.IsOnline(ctx, param.ID)
	if err != nil {
		logger.Warn("user: failed to check presence, defaulting to offline",
			zap.String("user_id", param.ID),
			zap.Error(err),
		)

		isOnline = false
	}

	response := &Response{
		ID:         user.ID,
		Name:       user.Name,
		Username:   user.Username,
		Bio:        user.Bio,
		Language:   user.Language,
		AvatarURL:  user.AvatarURL,
		IsOnline:   isOnline,
		LastSeenAt: user.LastSeenAt,
		CreatedAt:  user.CreatedAt,
	}

	return response, nil
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

	if len(summaries) > 0 {
		onlineMap, err := s.presenceChecker.AreOnline(ctx, summaryIDs(summaries))
		if err != nil {
			logger.Warn("user: presence check failed, defaulting to offline",
				zap.String("query", query.Q),
				zap.Error(err),
			)
		} else {
			for i := range summaries {
				summaries[i].IsOnline = onlineMap[summaries[i].ID]
			}
		}
	}

	logger.Debug("user: search completed",
		zap.String("query", query.Q),
		zap.Int("result_count", len(summaries)),
	)

	return &SearchResponse{
		Data:       summaries,
		NextCursor: nextCursor,
	}, nil
}

func summaryIDs(summaries []*Summary) []string {
	ids := make([]string, len(summaries))
	for i, s := range summaries {
		ids[i] = s.ID
	}
	return ids
}
