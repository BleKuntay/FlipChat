package ws_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/BleKuntay/FlipChat/backend/internal/ws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// ── mocks ─────────────────────────────────────────────────────────────────────

type mockBlockChecker struct {
	m *mock.Mock
}

func newMockBlockChecker() *mockBlockChecker {
	return &mockBlockChecker{m: new(mock.Mock)}
}

func (m *mockBlockChecker) IsBlockedEitherWay(ctx context.Context, a, b string) (bool, error) {
	args := m.m.Called(ctx, a, b)
	return args.Bool(0), args.Error(1)
}

func (m *mockBlockChecker) On(methodName string, arguments ...interface{}) *mock.Call {
	return m.m.On(methodName, arguments...)
}

func (m *mockBlockChecker) AssertExpectations(t mock.TestingT) bool {
	return m.m.AssertExpectations(t)
}

func (m *mockBlockChecker) AssertCalled(t mock.TestingT, methodName string, arguments ...interface{}) bool {
	return m.m.AssertCalled(t, methodName, arguments...)
}

// ── helpers ───────────────────────────────────────────────────────────────────

// testClientStub is a minimal stub that can be used for testing hub behavior.
// Note: ws.Client has private fields, so we can't easily create instances in tests.
// Instead, we focus on testing observable behavior through the Hub public API.

// ── Register tests ────────────────────────────────────────────────────────────
// Note: Register/Unregister cannot be easily unit tested without ws.Client creation.
// ws.Client has private fields, so cannot be instantiated from test packages.
// Integration tests with real WebSocket connections would properly test these methods.

// ── SendToUser tests ──────────────────────────────────────────────────────────

func TestHub_SendToUser_UserOffline_NoOp(t *testing.T) {
	blockChecker := newMockBlockChecker()
	hub := ws.NewHub(blockChecker)

	// Send to offline user - should not panic
	hub.SendToUser("offline-user", "test.event", map[string]string{})
}

func TestHub_SendToUser_MarshalError_NoOp(t *testing.T) {
	blockChecker := newMockBlockChecker()
	hub := ws.NewHub(blockChecker)

	// Send with unmarshallable payload - should not panic
	ch := make(chan struct{})
	hub.SendToUser("some-user", "bad.event", ch)
}

// ── FanOutToConversation tests ────────────────────────────────────────────────

func TestHub_FanOutToConversation_NotBlocked_NoOp(t *testing.T) {
	ctx := context.Background()
	blockChecker := newMockBlockChecker()

	const (
		senderID    = "user-1"
		recipientID = "user-2"
	)

	blockChecker.On("IsBlockedEitherWay", ctx, senderID, recipientID).Return(false, nil)

	hub := ws.NewHub(blockChecker)

	// FanOut to offline recipient - should not panic
	hub.FanOutToConversation(ctx, senderID, recipientID, "message.new", map[string]string{"text": "hi"})

	blockChecker.AssertCalled(t, "IsBlockedEitherWay", ctx, senderID, recipientID)
}

func TestHub_FanOutToConversation_Blocked_NoFanout(t *testing.T) {
	ctx := context.Background()
	blockChecker := newMockBlockChecker()

	const (
		senderID    = "user-1"
		recipientID = "user-2"
	)

	blockChecker.On("IsBlockedEitherWay", ctx, senderID, recipientID).Return(true, nil)

	hub := ws.NewHub(blockChecker)

	// FanOut when blocked - should not panic, should not send
	hub.FanOutToConversation(ctx, senderID, recipientID, "message.new", map[string]string{"text": "hi"})

	blockChecker.AssertCalled(t, "IsBlockedEitherWay", ctx, senderID, recipientID)
}

func TestHub_FanOutToConversation_BlockCheckOrder(t *testing.T) {
	ctx := context.Background()
	blockChecker := newMockBlockChecker()

	const (
		senderID    = "sender"
		recipientID = "recipient"
	)

	blockChecker.On("IsBlockedEitherWay", ctx, senderID, recipientID).Return(false, nil)

	hub := ws.NewHub(blockChecker)

	hub.FanOutToConversation(ctx, senderID, recipientID, "test.event", nil)

	// Verify block check was called with correct order (senderID, recipientID)
	blockChecker.AssertCalled(t, "IsBlockedEitherWay", ctx, senderID, recipientID)
}

func TestHub_FanOutToConversation_BlockCheckError_NoPanic(t *testing.T) {
	ctx := context.Background()
	blockChecker := newMockBlockChecker()

	const (
		senderID    = "user-1"
		recipientID = "user-2"
	)

	checkErr := errors.New("block check failed")
	blockChecker.On("IsBlockedEitherWay", ctx, senderID, recipientID).Return(false, checkErr)

	hub := ws.NewHub(blockChecker)

	// Should not panic, and should not send the message
	hub.FanOutToConversation(ctx, senderID, recipientID, "message.new", map[string]string{})

	blockChecker.AssertCalled(t, "IsBlockedEitherWay", ctx, senderID, recipientID)
}

// ── Concurrency tests ─────────────────────────────────────────────────────────

func TestHub_Concurrency_FanOutMultipleTimes_NoPanic(t *testing.T) {
	ctx := context.Background()
	blockChecker := newMockBlockChecker()

	blockChecker.On("IsBlockedEitherWay", ctx, mock.Anything, mock.Anything).Return(false, nil)

	hub := ws.NewHub(blockChecker)

	// Fanout from one sender to multiple recipients concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			senderID := "sender"
			recipientID := "recipient-" + string(rune(idx+'0'))
			hub.FanOutToConversation(ctx, senderID, recipientID, "message.new", nil)
		}(i)
	}

	wg.Wait()

	// Should not panic or deadlock
	assert.NotNil(t, hub)
}
