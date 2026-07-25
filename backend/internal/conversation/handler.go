package conversation

import (
	"context"
	"github.com/BleKuntay/FlipChat/backend/pkg/httputil"
	"github.com/BleKuntay/FlipChat/backend/pkg/logger"
	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

type ServiceInterface interface {
	GetConversationList(ctx context.Context, requesterID string) (*ListResponse, error)
	GetConversation(ctx context.Context, requesterID, conversationID string) (*Response, error)
	CreateConversation(ctx context.Context, requesterID, targetUserID string) (*Response, bool, error)
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(service ServiceInterface) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoute(router fiber.Router) {
	router.Get("/", h.GetConversationList)
	router.Get("/:id", h.GetConversation)
	router.Post("/", h.CreateConversation)
}

func (h *Handler) GetConversationList(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	response, err := h.service.GetConversationList(ctx, userID)
	if err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.JSON(response)
}

func (h *Handler) GetConversation(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	uri := new(URIParams)
	if err := c.Bind().URI(uri); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	response, err := h.service.GetConversation(ctx, userID, uri.ID)
	if err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.JSON(response)
}

func (h *Handler) CreateConversation(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	request := new(CreateRequest)
	if err := c.Bind().Body(request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	response, created, err := h.service.CreateConversation(ctx, userID, request.TargetUserID)
	if err != nil {
		return httputil.ErrorStatus(c, err)
	}
	if created {
		logger.Info("conversation: created",
			zap.String("conversation_id", response.ConversationID),
			zap.String("initiator_id", userID),
			zap.String("partner_id", request.TargetUserID),
		)

		return c.Status(fiber.StatusCreated).JSON(response)
	}

	return c.JSON(response)
}
