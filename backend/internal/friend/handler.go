package friend

import (
	"context"

	"github.com/BleKuntay/FlipChat/backend/pkg/httputil"
	"github.com/gofiber/fiber/v3"
)

type ServiceInterface interface {
	FindAllRequests(ctx context.Context, userID string, query RequestListQuery) (*RequestListResponse, error)
	FindAll(ctx context.Context, userID string, query ListQuery) (*ListResponse, error)
	FindOne(ctx context.Context, userID, targetID string) (*StatusResponse, error)
	AddFriend(ctx context.Context, userID, targetID string) (*PendingResponse, error)
	Unfriend(ctx context.Context, userID, targetID string) error
	CancelFriendRequest(ctx context.Context, userID, targetID string) error
	AcceptFriendRequest(ctx context.Context, userID, targetID string) (*Response, error)
	DeclineFriendRequest(ctx context.Context, userID, targetID string) error
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(service ServiceInterface) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router fiber.Router) {
	router.Get("/requests", h.ListRequests)
	router.Get("/", h.FindAll)
	router.Get("/:id", h.FindOne)
	router.Post("/:id", h.AddFriend)
	router.Delete("/:id", h.Unfriend)
	router.Delete("/requests/:id", h.CancelFriendRequest)
	router.Put("/requests/:id/accept", h.AcceptFriendRequest)
	router.Put("/requests/:id/decline", h.DeclineFriendRequest)
}

func (h *Handler) ListRequests(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	query := new(RequestListQuery)
	if err := c.Bind().Query(query); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	response, err := h.service.FindAllRequests(ctx, userID, *query)
	if err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.JSON(response)
}

func (h *Handler) FindAll(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	query := new(ListQuery)
	if err := c.Bind().Query(query); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	response, err := h.service.FindAll(ctx, userID, *query)
	if err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.JSON(response)
}

func (h *Handler) FindOne(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	uri := new(URIParams)
	if err := c.Bind().URI(uri); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	response, err := h.service.FindOne(ctx, userID, uri.ID)
	if err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.JSON(response)
}

func (h *Handler) AddFriend(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	uri := new(URIParams)
	if err := c.Bind().URI(uri); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	response, err := h.service.AddFriend(ctx, userID, uri.ID)
	if err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(response)
}

func (h *Handler) Unfriend(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	uri := new(URIParams)
	if err := c.Bind().URI(uri); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.service.Unfriend(ctx, userID, uri.ID); err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) CancelFriendRequest(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	uri := new(URIParams)
	if err := c.Bind().URI(uri); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.service.CancelFriendRequest(ctx, userID, uri.ID); err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) AcceptFriendRequest(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	uri := new(URIParams)
	if err := c.Bind().URI(uri); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	response, err := h.service.AcceptFriendRequest(ctx, userID, uri.ID)
	if err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.JSON(response)
}

func (h *Handler) DeclineFriendRequest(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	uri := new(URIParams)
	if err := c.Bind().URI(uri); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.service.DeclineFriendRequest(ctx, userID, uri.ID); err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}
