package block

import (
	"context"
	"github.com/BleKuntay/FlipChat/backend/pkg/httputil"
	"github.com/gofiber/fiber/v3"
)

type ServiceInterface interface {
	BlockUser(ctx context.Context, request Request) (*Response, error)
	UnblockUser(ctx context.Context, request Request) error
	GetBlockList(ctx context.Context, blockerID string, query ListQuery) (*ListResponse, error)
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(service ServiceInterface) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router fiber.Router) {
	router.Get("/", h.GetBlockList)
	router.Post("/:id", h.BlockUser)
	router.Delete("/:id", h.UnblockUser)
}

func (h *Handler) BlockUser(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	params := new(URIParams)
	if err := c.Bind().URI(params); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	request := Request{
		BlockerID: userID,
		BlockedID: params.ID,
	}

	response, err := h.service.BlockUser(ctx, request)
	if err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(response)
}

func (h *Handler) UnblockUser(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	params := new(URIParams)
	if err := c.Bind().URI(params); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	request := Request{
		BlockerID: userID,
		BlockedID: params.ID,
	}

	if err := h.service.UnblockUser(ctx, request); err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) GetBlockList(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	query := new(ListQuery)
	if err := c.Bind().Query(query); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	response, err := h.service.GetBlockList(ctx, userID, *query)
	if err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.JSON(response)
}
