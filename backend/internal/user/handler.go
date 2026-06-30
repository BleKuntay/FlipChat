package user

import (
	"context"
	"errors"

	"github.com/BleKuntay/FlipChat/backend/pkg/httputil"
	"github.com/gofiber/fiber/v3"
)

type ServiceInterface interface {
	Me(ctx context.Context, userID string) (*MeResponse, error)
	UpdateProfile(ctx context.Context, userID string, req *UpdateProfileRequest) (*UpdateProfileResponse, error)
	UpdateEmail(ctx context.Context, userID string, req *UpdateEmailRequest) (*UpdateEmailResponse, error)
	ChangePassword(ctx context.Context, userID string, req *ChangePasswordRequest) error
	DeleteAccount(ctx context.Context, userID string) error
	FindUserByID(ctx context.Context, requesterID string, param *GetUserURI) (*Response, error)
	Search(ctx context.Context, userID string, query *SearchQuery) (*SearchResponse, error)
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(service ServiceInterface) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router fiber.Router) {
	router.Get("/me", h.GetProfile)
	router.Patch("/me", h.UpdateProfile)
	router.Delete("/me", h.DeleteAccount)
	router.Patch("/me/email", h.UpdateEmail)
	router.Patch("/me/password", h.ChangePassword)
	router.Get("/search", h.Search)
	router.Get("/:id", h.FindByID)
}

func (h *Handler) GetProfile(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	me, err := h.service.Me(ctx, userID)
	if err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.JSON(me)
}

func (h *Handler) UpdateProfile(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	req := new(UpdateProfileRequest)
	if err := c.Bind().Body(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	res, err := h.service.UpdateProfile(ctx, userID, req)
	if err != nil {
		if errors.Is(err, ErrUserNotUpdated) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return httputil.ErrorStatus(c, err)
	}

	return c.JSON(res)
}

func (h *Handler) UpdateEmail(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	req := new(UpdateEmailRequest)
	if err := c.Bind().Body(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	res, err := h.service.UpdateEmail(ctx, userID, req)
	if err != nil {
		if errors.Is(err, ErrInvalidPassword) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
		}
		return httputil.ErrorStatus(c, err)
	}

	return c.JSON(res)
}

func (h *Handler) ChangePassword(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	req := new(ChangePasswordRequest)
	if err := c.Bind().Body(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.service.ChangePassword(ctx, userID, req); err != nil {
		if errors.Is(err, ErrInvalidPassword) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid current password"})
		}
		if errors.Is(err, ErrPasswordMismatch) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "new password does not match"})
		}
		return httputil.ErrorStatus(c, err)
	}

	return c.JSON(fiber.Map{"message": "password changed successfully"})
}

func (h *Handler) DeleteAccount(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	if err := h.service.DeleteAccount(ctx, userID); err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) Search(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	query := new(SearchQuery)
	if err := c.Bind().Query(query); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	res, err := h.service.Search(ctx, userID, query)
	if err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.JSON(res)
}

func (h *Handler) FindByID(c fiber.Ctx) error {
	ctx := c.Context()
	requesterID := fiber.Locals[string](c, "user_id")

	param := new(GetUserURI)
	if err := c.Bind().URI(param); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	res, err := h.service.FindUserByID(ctx, requesterID, param)
	if err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.JSON(res)
}
