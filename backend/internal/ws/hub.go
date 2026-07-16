package ws

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/BleKuntay/FlipChat/backend/pkg/logger"
	"go.uber.org/zap"
)

// BlockChecker is used by the hub to filter events across blocked pairs.
type BlockChecker interface {
	IsBlockedEitherWay(ctx context.Context, a, b string) (bool, error)
}

// Hub maintains the registry of active WebSocket clients and
// fans out events to the correct recipients.
//
// All exported methods are safe to call from multiple goroutines.
type Hub struct {
	mu           sync.RWMutex
	clients      map[string]*Client // userID → client
	blockChecker BlockChecker
}

func NewHub(blockChecker BlockChecker) *Hub {
	return &Hub{
		clients:      make(map[string]*Client),
		blockChecker: blockChecker,
	}
}

// Register adds a client to the hub. If the user already has an active
// connection, the old connection is closed before registering the new one.
func (h *Hub) Register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if old, ok := h.clients[client.userID]; ok {
		old.close()
	}

	h.clients[client.userID] = client
	logger.Info("ws: client registered", zap.String("user_id", client.userID))
}

// Unregister removes a client from the hub.
// No-op if the client is no longer the current connection for that userID
// (e.g. already replaced by a newer connection).
func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if current, ok := h.clients[client.userID]; ok && current == client {
		delete(h.clients, client.userID)
		logger.Info("ws: client unregistered", zap.String("user_id", client.userID))
	}
}

// SendToUser delivers an event to a specific user if they are connected.
// Silently drops the event if the user is offline.
// payload can be any JSON-serializable value.
func (h *Hub) SendToUser(userID string, eventType string, payload any) {
	h.mu.RLock()
	client, ok := h.clients[userID]
	h.mu.RUnlock()

	if !ok {
		return
	}

	envelope := map[string]any{
		"type":    eventType,
		"payload": payload,
	}

	b, err := json.Marshal(envelope)
	if err != nil {
		logger.Error("ws: failed to marshal event", zap.Error(err))
		return
	}

	client.send(b)
}

// FanOutToConversation delivers an event to both participants of a conversation,
// filtering out delivery if either party has blocked the other.
//
// senderID is excluded from receiving the event (server echo suppression).
func (h *Hub) FanOutToConversation(ctx context.Context, senderID, recipientID string, eventType string, payload any) {
	blocked, err := h.blockChecker.IsBlockedEitherWay(ctx, senderID, recipientID)
	if err != nil {
		logger.Error("ws: block check failed during fan out",
			zap.String("sender", senderID),
			zap.String("recipient", recipientID),
			zap.Error(err),
		)
		return
	}
	if blocked {
		return
	}

	h.SendToUser(recipientID, eventType, payload)
}

// IsOnline reports whether a user has an active WebSocket connection.
func (h *Hub) IsOnline(userID string) bool {
	h.mu.RLock()
	_, ok := h.clients[userID]
	h.mu.RUnlock()
	return ok
}
