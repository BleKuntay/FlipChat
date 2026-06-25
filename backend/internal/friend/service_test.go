package friend

import (
	"context"
	"testing"
	"time"

	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// userA < userB dijamin dengan string comparison
// sehingga canonical(userA, userB) selalu return (userA, userB)
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
	addFriendFn       func(ctx context.Context, lowID, highID, requesterID string) (*Record, error)
	acceptFriendFn    func(ctx context.Context, lowID, highID string) (*Record, error)
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
	return true, nil // default: user ada
}

func (m *mockRepo) FindByPair(ctx context.Context, lowID, highID string) (*Friend, error) {
	if m.findByPairFn != nil {
		return m.findByPairFn(ctx, lowID, highID)
	}
	return nil, nil // default: tidak ada pair
}

func (m *mockRepo) AddFriend(ctx context.Context, lowID, highID, requesterID string) (*Record, error) {
	if m.addFriendFn != nil {
		return m.addFriendFn(ctx, lowID, highID, requesterID)
	}
	return &Record{UserID: userB, Username: "bob", FullName: "Bob", CreatedAt: time.Now()}, nil
}

func (m *mockRepo) AcceptFriend(ctx context.Context, lowID, highID string) (*Record, error) {
	if m.acceptFriendFn != nil {
		return m.acceptFriendFn(ctx, lowID, highID)
	}
	return &Record{UserID: userA, Username: "alice", FullName: "Alice", CreatedAt: time.Now()}, nil
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
			name:       "tidak ada relasi returns none",
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
			name: "pending dan userA adalah requester returns pending_sent",
			setupRepo: func(m *mockRepo) {
				m.findByPairFn = func(_ context.Context, _, _ string) (*Friend, error) {
					return &Friend{RequesterID: userA, Status: StatusPending}, nil
				}
			},
			wantStatus: StatusPendingSent,
		},
		{
			name: "pending dan userB adalah requester returns pending_received",
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
			svc := NewService(repo)

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
		name      string
		userID    string
		targetID  string
		setupRepo func(*mockRepo)
		wantErr   error
		wantDir   string
	}{
		{
			name:     "self-add returns ErrBadRequest",
			userID:   userA,
			targetID: userA,
			wantErr:  apperr.ErrBadRequest,
		},
		{
			name:     "target tidak ditemukan returns ErrNotFound",
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
			name:     "sudah berteman returns ErrConflict",
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
			name:     "sudah kirim request returns ErrConflict",
			userID:   userA,
			targetID: userB,
			setupRepo: func(m *mockRepo) {
				m.findByPairFn = func(_ context.Context, _, _ string) (*Friend, error) {
					// A sudah kirim request sebelumnya
					return &Friend{RequesterID: userA, Status: StatusPending}, nil
				}
			},
			wantErr: apperr.ErrConflict,
		},
		{
			name:     "mutual request auto-accept",
			userID:   userA,
			targetID: userB,
			setupRepo: func(m *mockRepo) {
				m.findByPairFn = func(_ context.Context, _, _ string) (*Friend, error) {
					// B sudah kirim request ke A sebelumnya
					return &Friend{RequesterID: userB, Status: StatusPending}, nil
				}
			},
			wantDir: "sent",
		},
		{
			name:     "happy path returns PendingResponse direction sent",
			userID:   userA,
			targetID: userB,
			wantDir:  "sent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			if tt.setupRepo != nil {
				tt.setupRepo(repo)
			}
			svc := NewService(repo)

			got, err := svc.AddFriend(ctx, tt.userID, tt.targetID)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantDir, got.Direction)
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
			name:     "pair tidak ada returns ErrNotFound",
			userID:   userA,
			targetID: userB,
			wantErr:  apperr.ErrNotFound,
		},
		{
			name:     "status masih pending returns ErrNotFound",
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
			svc := NewService(repo)

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
			name:     "pair tidak ada returns ErrNotFound",
			userID:   userA,
			targetID: userB,
			wantErr:  apperr.ErrNotFound,
		},
		{
			name:     "sudah accepted returns ErrConflict",
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
			name:     "bukan requester returns ErrForbidden",
			userID:   userA,
			targetID: userB,
			setupRepo: func(m *mockRepo) {
				m.findByPairFn = func(_ context.Context, _, _ string) (*Friend, error) {
					// B yang kirim request, bukan A
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
			svc := NewService(repo)

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
			name:     "pair tidak ada returns ErrNotFound",
			userID:   userB,
			targetID: userA,
			wantErr:  apperr.ErrNotFound,
		},
		{
			name:     "sudah accepted returns ErrConflict",
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
			name:     "requester mencoba accept request-nya sendiri returns ErrForbidden",
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
			userID:   userB, // B menerima request dari A
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
			svc := NewService(repo)

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
			name:     "pair tidak ada returns ErrNotFound",
			userID:   userB,
			targetID: userA,
			wantErr:  apperr.ErrNotFound,
		},
		{
			name:     "status bukan pending returns ErrNotFound",
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
			name:     "requester mencoba decline request-nya sendiri returns ErrForbidden",
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
			userID:   userB, // B menolak request dari A
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
			svc := NewService(repo)

			err := svc.DeclineFriendRequest(ctx, tt.userID, tt.targetID)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
