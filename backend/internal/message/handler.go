package message

import (
	"context"
	"github.com/BleKuntay/FlipChat/backend/pkg/httputil"
	"github.com/gofiber/fiber/v3"
)

type ServiceInterface interface {
	ListMessages(ctx context.Context, userID, conversationID string, query ListQuery) (*ListResponse, error)
	SendMessage(ctx context.Context, userID, conversationID string, request SendRequest) (*Response, error)
	EditMessage(ctx context.Context, userID, conversationID, messageID string, request EditRequest) (*Response, error)
	DeleteMessage(ctx context.Context, userID, conversationID, messageID string) (*Response, error)
	MarkAsRead(ctx context.Context, userID, conversationID, messageID string) (*Response, error)
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(service ServiceInterface) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router fiber.Router) {
	router.Post("/:id/messages", h.SendMessage)
	router.Get("/:id/messages", h.ListMessages)
	router.Post("/:id/messages/:msg_id/read", h.MarkAsRead)
	router.Patch("/:id/messages/:msg_id", h.EditMessage)
	router.Delete("/:id/messages/:msg_id", h.DeleteMessage)
}

func (h *Handler) ListMessages(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	uri := new(URIParams)
	if err := c.Bind().URI(uri); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	query := new(ListQuery)
	if err := c.Bind().Query(query); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	response, err := h.service.ListMessages(ctx, userID, uri.ConversationID, *query)
	if err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.JSON(response)
}

func (h *Handler) SendMessage(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	uri := new(URIParams)
	if err := c.Bind().URI(uri); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	request := new(SendRequest)
	if err := c.Bind().Body(request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	response, err := h.service.SendMessage(ctx, userID, uri.ConversationID, *request)
	if err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(response)
}

func (h *Handler) EditMessage(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	uri := new(URIParams)
	if err := c.Bind().URI(uri); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	request := new(EditRequest)
	if err := c.Bind().Body(request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	response, err := h.service.EditMessage(ctx, userID, uri.ConversationID, uri.MessageID, *request)
	if err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.JSON(response)
}

func (h *Handler) DeleteMessage(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	uri := new(URIParams)
	if err := c.Bind().URI(uri); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	response, err := h.service.DeleteMessage(ctx, userID, uri.ConversationID, uri.MessageID)
	if err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.JSON(response)
}

func (h *Handler) MarkAsRead(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	uri := new(URIParams)
	if err := c.Bind().URI(uri); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	response, err := h.service.MarkAsRead(ctx, userID, uri.ConversationID, uri.MessageID)
	if err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.JSON(response)
}
