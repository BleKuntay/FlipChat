package ws

import (
	"context"

	"github.com/BleKuntay/FlipChat/backend/pkg/jwt"
	"github.com/BleKuntay/FlipChat/backend/pkg/logger"
	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

var upgrader = websocket.FastHTTPUpgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// CheckOrigin delegated to Fiber's CORS middleware.
	CheckOrigin: func(ctx *fasthttp.RequestCtx) bool { return true },
}

// Handler handles WebSocket upgrade requests.
type Handler struct {
	hub            *Hub
	presence       PresenceSetter
	partnerStore   PartnerStore
	lastSeenUpdate LastSeenUpdater
}

func NewHandler(hub *Hub, presence PresenceSetter, partnerStore PartnerStore, lastSeenUpdater LastSeenUpdater) *Handler {
	return &Handler{
		hub:            hub,
		presence:       presence,
		partnerStore:   partnerStore,
		lastSeenUpdate: lastSeenUpdater,
	}
}

func (h *Handler) RegisterRoute(router fiber.Router) {
	router.Get("/ws", h.HandleWS)
}

// HandleWS upgrades an HTTP request to a WebSocket connection.
// Auth via ?token=<access_token> — browsers cannot set Authorization
// header during WS handshake.
func (h *Handler) HandleWS(c fiber.Ctx) error {
	token := c.Query("token")
	if token == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "missing token",
		})
	}

	claims, err := jwt.VerifyAccessToken(token)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "invalid or expired token",
		})
	}

	userID := claims.UserID

	err = upgrader.Upgrade(c.RequestCtx(), func(conn *websocket.Conn) {
		client := NewClient(userID, conn, h.hub, h.presence, h.partnerStore, h.lastSeenUpdate)
		client.Run(context.Background())
	})
	if err != nil {
		logger.Error("ws: upgrade failed", zap.String("user_id", userID), zap.Error(err))
		return err
	}

	return nil
}
