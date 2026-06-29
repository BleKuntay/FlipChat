package friend

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	userA = "00000000-0000-0000-0000-000000000001"
	userB = "00000000-0000-0000-0000-000000000002"
)

// ------------------------------------------------------------------ //
// Mock                                                                  //
// ------------------------------------------------------------------ //

type mockRepo struct {
	findAllRequestsFn func(ctx context.Context, userID string, query RequestListQuery) ([]Record, error)
	findAllFn         func(ctx context.Context, userID string, query ListQuery) ([]Response, error)
	existsByUserIDFn  func(ctx context.Context, userID string) (bool, error)
	findByPairFn      func(ctx context.Context, lowID, highID string) (*Friend, error)
	upsertFriendFn    func(ctx context.Context, lowID, highID, requesterID string) (*Record, error)
	deleteByPairFn    func(ctx context.Context, lowID, highID string) error
}

func (m *mockRepo) FindAllRequests(ctx context.Context, userID string, query RequestListQuery) ([]Record, error) {
	if m.findAllRequestsFn != nil {
		return m.findAllRequestsFn(ctx, userID, query)
	}
	return nil, nil
}

func (m *mockRepo) FindAll(ctx context.Context, userID string, query ListQuery) ([]Response, error) {
	if m.findAllFn != nil {
		return m.findAllFn(ctx, userID, query)
	}
	return nil, nil
}

func (m *mockRepo) ExistsByUserID(ctx context.Context, userID string) (bool, error) {
	if m.existsByUserIDFn != nil {
		return m.existsByUserIDFn(ctx, userID)
	}
	return true, nil
}

func (m *mockRepo) FindByPair(ctx context.Context, lowID, highID string) (*Friend, error) {
	if m.findByPairFn != nil {
		return m.findByPairFn(ctx, lowID, highID)
	}
	return nil, nil
}

func (m *mockRepo) UpsertFriend(ctx context.Context, lowID, highID, requesterID string) (*Record, error) {
	if m.upsertFriendFn != nil {
		return m.upsertFriendFn(ctx, lowID, highID, requesterID)
	}
	return &Record{UserID: userB, Username: "bob", FullName: "Bob", CreatedAt: time.Now(), Status: StatusPending}, nil
}

func (m *mockRepo) DeleteByPair(ctx context.Context, lowID, highID string) error {
	if m.deleteByPairFn != nil {
		return m.deleteByPairFn(ctx, lowID, highID)
	}
	return nil
}

func newMockRepo() *mockRepo {
	return &mockRepo{}
}

// ------------------------------------------------------------------ //
// Mock BlockChecker                                                     //
// ------------------------------------------------------------------ //

type mockBlockChecker struct {
	isBlockedEitherWayFn func(ctx context.Context, a, b string) (bool, error)
}

func (m *mockBlockChecker) IsBlockedEitherWay(ctx context.Context, a, b string) (bool, error) {
	if m.isBlockedEitherWayFn != nil {
		return m.isBlockedEitherWayFn(ctx, a, b)
	}
	return false, nil
}

func newMockBlockChecker() *mockBlockChecker {
	return &mockBlockChecker{}
}

// ------------------------------------------------------------------ //
// Tests                                                                 //
// ------------------------------------------------------------------ //

func TestService_FindOne(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		setupRepo  func(*mockRepo)
		wantStatus Status
	}{
		{
			name:       "no relation returns none",
			wantStatus: StatusNone,
		},
		{
			name: "friendship accepted returns accepted",
			setupRepo: func(m *mockRepo) {
				m.findByPairFn = func(_ context.Context, _, _ string) (*Friend, error) {
					return &Friend{RequesterID: userA, Status: StatusAccepted}, nil
				}
			},
			wantStatus: StatusAccepted,
		},
		{
			name: "pending and userA is requester returns pending_sent",
			setupRepo: func(m *mockRepo) {
				m.findByPairFn = func(_ context.Context, _, _ string) (*Friend, error) {
					return &Friend{RequesterID: userA, Status: StatusPending}, nil
				}
			},
			wantStatus: StatusPendingSent,
		},
		{
			name: "pending and userB is requester returns pending_received",
			setupRepo: func(m *mockRepo) {
				m.findByPairFn = func(_ context.Context, _, _ string) (*Friend, error) {
					return &Friend{RequesterID: userB, Status: StatusPending}, nil
				}
			},
			wantStatus: StatusPendingReceived,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			if tt.setupRepo != nil {
				tt.setupRepo(repo)
			}
			svc := NewService(repo, newMockBlockChecker())

			got, err := svc.FindOne(ctx, userA, userB)

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantStatus, got.Status)
		})
	}
}

func TestService_AddFriend(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		userID        string
		targetID      string
		setupRepo     func(*mockRepo)
		setupBlocker  func(*mockBlockChecker)
		wantErr       error
		wantDirection string
	}{
		{
			name:     "self-add returns ErrBadRequest",
			userID:   userA,
			targetID: userA,
			wantErr:  apperr.ErrBadRequest,
		},
		{
			name:     "target not found returns ErrNotFound",
			userID:   userA,
			targetID: userB,
			setupRepo: func(m *mockRepo) {
				m.existsByUserIDFn = func(_ context.Context, _ string) (bool, error) {
					return false, nil
				}
			},
			wantErr: apperr.ErrNotFound,
		},
		{
			name:     "blocked returns ErrForbidden",
			userID:   userA,
			targetID: userB,
			setupBlocker: func(m *mockBlockChecker) {
				m.isBlockedEitherWayFn = func(_ context.Context, _, _ string) (bool, error) {
					return true, nil
				}
			},
			wantErr: apperr.ErrForbidden,
		},
		{
			name:     "already friends returns ErrConflict",
			userID:   userA,
			targetID: userB,
			setupRepo: func(m *mockRepo) {
				m.findByPairFn = func(_ context.Context, _, _ string) (*Friend, error) {
					return &Friend{RequesterID: userA, Status: StatusAccepted}, nil
				}
				m.upsertFriendFn = func(_ context.Context, _, _, _ string) (*Record, error) {
					return nil, sql.ErrNoRows
				}
			},
			wantErr: apperr.ErrConflict,
		},
		{
			name:     "already sent request returns ErrConflict",
			userID:   userA,
			targetID: userB,
			setupRepo: func(m *mockRepo) {
				m.findByPairFn = func(_ context.Context, _, _ string) (*Friend, error) {
					return &Friend{RequesterID: userA, Status: StatusPending}, nil
				}
				m.upsertFriendFn = func(_ context.Context, _, _, _ string) (*Record, error) {
					return nil, sql.ErrNoRows
				}
			},
			wantErr: apperr.ErrConflict,
		},
		{
			name:     "upsert returns conflict (sql.ErrNoRows)",
			userID:   userA,
			targetID: userB,
			setupRepo: func(m *mockRepo) {
				m.upsertFriendFn = func(_ context.Context, _, _, _ string) (*Record, error) {
					return nil, sql.ErrNoRows
				}
			},
			wantErr: apperr.ErrConflict,
		},
		{
			name:          "happy path returns direction sent",
			userID:        userA,
			targetID:      userB,
			wantDirection: "sent",
		},
		{
			name:          "mutual request auto-accepts",
			userID:        userA,
			targetID:      userB,
			wantDirection: "accepted",
			setupRepo: func(m *mockRepo) {
				m.upsertFriendFn = func(_ context.Context, _, _, _ string) (*Record, error) {
					return &Record{
						UserID:      userB,
						Username:    "bob",
						FullName:    "Bob",
						CreatedAt:   time.Now(),
						RequesterID: userA,
						Status:      StatusAccepted,
					}, nil
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			if tt.setupRepo != nil {
				tt.setupRepo(repo)
			}
			blocker := newMockBlockChecker()
			if tt.setupBlocker != nil {
				tt.setupBlocker(blocker)
			}
			svc := NewService(repo, blocker)

			got, err := svc.AddFriend(ctx, tt.userID, tt.targetID)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantDirection, got.Direction)
		})
	}
}

func TestService_Unfriend(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		userID    string
		targetID  string
		setupRepo func(*mockRepo)
		wantErr   error
	}{
		{
			name:     "self returns ErrBadRequest",
			userID:   userA,
			targetID: userA,
			wantErr:  apperr.ErrBadRequest,
		},
		{
			name:     "pair not found returns ErrNotFound",
			userID:   userA,
			targetID: userB,
			wantErr:  apperr.ErrNotFound,
		},
		{
			name:     "status still pending returns ErrNotFound",
			userID:   userA,
			targetID: userB,
			setupRepo: func(m *mockRepo) {
				m.findByPairFn = func(_ context.Context, _, _ string) (*Friend, error) {
					return &Friend{RequesterID: userA, Status: StatusPending}, nil
				}
			},
			wantErr: apperr.ErrNotFound,
		},
		{
			name:     "happy path returns nil",
			userID:   userA,
			targetID: userB,
			setupRepo: func(m *mockRepo) {
				m.findByPairFn = func(_ context.Context, _, _ string) (*Friend, error) {
					return &Friend{RequesterID: userA, Status: StatusAccepted}, nil
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			if tt.setupRepo != nil {
				tt.setupRepo(repo)
			}
			svc := NewService(repo, newMockBlockChecker())

			err := svc.Unfriend(ctx, tt.userID, tt.targetID)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestService_CancelFriendRequest(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		userID    string
		targetID  string
		setupRepo func(*mockRepo)
		wantErr   error
	}{
		{
			name:     "self returns ErrBadRequest",
			userID:   userA,
			targetID: userA,
			wantErr:  apperr.ErrBadRequest,
		},
		{
			name:     "pair not found returns ErrNotFound",
			userID:   userA,
			targetID: userB,
			wantErr:  apperr.ErrNotFound,
		},
		{
			name:     "already accepted returns ErrConflict",
			userID:   userA,
			targetID: userB,
			setupRepo: func(m *mockRepo) {
				m.findByPairFn = func(_ context.Context, _, _ string) (*Friend, error) {
					return &Friend{RequesterID: userA, Status: StatusAccepted}, nil
				}
			},
			wantErr: apperr.ErrConflict,
		},
		{
			name:     "not requester returns ErrForbidden",
			userID:   userA,
			targetID: userB,
			setupRepo: func(m *mockRepo) {
				m.findByPairFn = func(_ context.Context, _, _ string) (*Friend, error) {
					return &Friend{RequesterID: userB, Status: StatusPending}, nil
				}
			},
			wantErr: apperr.ErrForbidden,
		},
		{
			name:     "happy path returns nil",
			userID:   userA,
			targetID: userB,
			setupRepo: func(m *mockRepo) {
				m.findByPairFn = func(_ context.Context, _, _ string) (*Friend, error) {
					return &Friend{RequesterID: userA, Status: StatusPending}, nil
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			if tt.setupRepo != nil {
				tt.setupRepo(repo)
			}
			svc := NewService(repo, newMockBlockChecker())

			err := svc.CancelFriendRequest(ctx, tt.userID, tt.targetID)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestService_AcceptFriendRequest(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		userID    string
		targetID  string
		setupRepo func(*mockRepo)
		wantErr   error
	}{
		{
			name:     "self returns ErrBadRequest",
			userID:   userA,
			targetID: userA,
			wantErr:  apperr.ErrBadRequest,
		},
		{
			name:     "pair not found returns ErrNotFound",
			userID:   userB,
			targetID: userA,
			wantErr:  apperr.ErrNotFound,
		},
		{
			name:     "already accepted returns ErrConflict",
			userID:   userB,
			targetID: userA,
			setupRepo: func(m *mockRepo) {
				m.findByPairFn = func(_ context.Context, _, _ string) (*Friend, error) {
					return &Friend{RequesterID: userA, Status: StatusAccepted}, nil
				}
			},
			wantErr: apperr.ErrConflict,
		},
		{
			name:     "requester tries to accept own request returns ErrForbidden",
			userID:   userA,
			targetID: userB,
			setupRepo: func(m *mockRepo) {
				m.findByPairFn = func(_ context.Context, _, _ string) (*Friend, error) {
					return &Friend{RequesterID: userA, Status: StatusPending}, nil
				}
			},
			wantErr: apperr.ErrForbidden,
		},
		{
			name:     "happy path returns Response",
			userID:   userB,
			targetID: userA,
			setupRepo: func(m *mockRepo) {
				m.findByPairFn = func(_ context.Context, _, _ string) (*Friend, error) {
					return &Friend{RequesterID: userA, Status: StatusPending}, nil
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			if tt.setupRepo != nil {
				tt.setupRepo(repo)
			}
			svc := NewService(repo, newMockBlockChecker())

			got, err := svc.AcceptFriendRequest(ctx, tt.userID, tt.targetID)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
		})
	}
}

func TestService_DeclineFriendRequest(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		userID    string
		targetID  string
		setupRepo func(*mockRepo)
		wantErr   error
	}{
		{
			name:     "self returns ErrBadRequest",
			userID:   userA,
			targetID: userA,
			wantErr:  apperr.ErrBadRequest,
		},
		{
			name:     "pair not found returns ErrNotFound",
			userID:   userB,
			targetID: userA,
			wantErr:  apperr.ErrNotFound,
		},
		{
			name:     "status not pending returns ErrNotFound",
			userID:   userB,
			targetID: userA,
			setupRepo: func(m *mockRepo) {
				m.findByPairFn = func(_ context.Context, _, _ string) (*Friend, error) {
					return &Friend{RequesterID: userA, Status: StatusAccepted}, nil
				}
			},
			wantErr: apperr.ErrNotFound,
		},
		{
			name:     "requester tries to decline own request returns ErrForbidden",
			userID:   userA,
			targetID: userB,
			setupRepo: func(m *mockRepo) {
				m.findByPairFn = func(_ context.Context, _, _ string) (*Friend, error) {
					return &Friend{RequesterID: userA, Status: StatusPending}, nil
				}
			},
			wantErr: apperr.ErrForbidden,
		},
		{
			name:     "happy path returns nil",
			userID:   userB,
			targetID: userA,
			setupRepo: func(m *mockRepo) {
				m.findByPairFn = func(_ context.Context, _, _ string) (*Friend, error) {
					return &Friend{RequesterID: userA, Status: StatusPending}, nil
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			if tt.setupRepo != nil {
				tt.setupRepo(repo)
			}
			svc := NewService(repo, newMockBlockChecker())

			err := svc.DeclineFriendRequest(ctx, tt.userID, tt.targetID)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
