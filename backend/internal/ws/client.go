package ws

import (
	"context"
	"encoding/json"
	"time"

	"github.com/BleKuntay/FlipChat/backend/pkg/logger"
	"github.com/fasthttp/websocket"
	"go.uber.org/zap"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 40 * time.Second
	pingPeriod     = 30 * time.Second
	maxMessageSize = 4096
)

type PresenceSetter interface {
	SetOnline(ctx context.Context, userID string) error
	SetOffline(ctx context.Context, userID string) error
}

type PartnerStore interface {
	GetConversationsPartner(ctx context.Context, userID string) ([]string, error)
}

type LastSeenUpdater interface {
	UpdateLastSeen(ctx context.Context, userID string, t time.Time) error
}

// Client represents a single active WebSocket connection.
// Each client owns two goroutines: readPump and writePump.
type Client struct {
	userID         string
	conn           *websocket.Conn
	hub            *Hub
	presence       PresenceSetter
	partnerStore   PartnerStore
	lastSeenUpdate LastSeenUpdater
	sendCh         chan []byte
	done           chan struct{}
}

func NewClient(userID string, conn *websocket.Conn, hub *Hub, presence PresenceSetter, partnerStore PartnerStore, lastSeenUpdater LastSeenUpdater) *Client {
	return &Client{
		userID:         userID,
		conn:           conn,
		hub:            hub,
		presence:       presence,
		partnerStore:   partnerStore,
		lastSeenUpdate: lastSeenUpdater,
		sendCh:         make(chan []byte, 256),
		done:           make(chan struct{}),
	}
}

// Run starts the read and write pumps and blocks until the connection closes.
func (c *Client) Run(ctx context.Context) {
	defer c.close()

	if err := c.presence.SetOnline(ctx, c.userID); err != nil {
		logger.Error("ws: failed to set online", zap.String("user_id", c.userID), zap.Error(err))
	}

	c.hub.Register(c)
	c.fanOutPresence(ctx, EventPresenceOnline)

	go c.writePump()
	c.readPump(ctx)

	// Only clear presence if this client was still the active connection.
	// If the user reconnected (e.g. browser refresh), Register already replaced
	// this client, and we must not overwrite the new connection's presence.
	if c.hub.Unregister(c) {
		c.fanOutPresence(ctx, EventPresenceOffline)

		if err := c.presence.SetOffline(ctx, c.userID); err != nil {
			logger.Error("ws: failed to set offline", zap.String("user_id", c.userID), zap.Error(err))
		}
	}

	if err := c.lastSeenUpdate.UpdateLastSeen(ctx, c.userID, time.Now()); err != nil {
		logger.Error("ws: failed to update lastSeen", zap.String("user_id", c.userID), zap.Error(err))
	}
}

// send delivers a message to the client's write channel.
// Drops the message if the channel is full to avoid blocking the hub.
func (c *Client) send(msg []byte) {
	select {
	case c.sendCh <- msg:
	default:
		logger.Warn("ws: send buffer full, dropping message", zap.String("user_id", c.userID))
	}
}

// close signals the client to shut down.
func (c *Client) close() {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
}

func (c *Client) readPump(ctx context.Context) {
	defer c.conn.Close()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseNormalClosure,
				websocket.CloseNoStatusReceived,
			) {
				logger.Warn("ws: unexpected close", zap.String("user_id", c.userID), zap.Error(err))
			}
			return
		}

		c.handleInbound(ctx, msg)
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.sendCh:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				logger.Warn("ws: write error", zap.String("user_id", c.userID), zap.Error(err))
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		case <-c.done:
			_ = c.conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return
		}
	}
}

func (c *Client) handleInbound(ctx context.Context, raw []byte) {
	var event Event
	if err := json.Unmarshal(raw, &event); err != nil {
		logger.Warn("ws: malformed inbound event",
			zap.String("user_id", c.userID),
			zap.Error(err),
		)
		return
	}

	switch event.Type {
	case EventHeartbeat:
		if err := c.presence.SetOnline(ctx, c.userID); err != nil {
			logger.Error("ws: heartbeat presence update failed",
				zap.String("user_id", c.userID),
				zap.Error(err),
			)
		}

	default:
		logger.Warn("ws: unknown event type",
			zap.String("user_id", c.userID),
			zap.String("type", event.Type),
		)
	}
}

func (c *Client) fanOutPresence(ctx context.Context, eventType string) {
	partners, err := c.partnerStore.GetConversationsPartner(ctx, c.userID)
	if err != nil {
		logger.Error("ws: failed to get partners", zap.String("user_id", c.userID), zap.Error(err))
		return
	}

	for _, partnerID := range partners {
		if partnerID == c.userID {
			continue
		}
		c.hub.FanOutToConversation(ctx, c.userID, partnerID, eventType, PresencePayload{UserID: c.userID})
	}
}
