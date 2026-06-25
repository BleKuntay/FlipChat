package user

import (
	"errors"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"github.com/gofiber/fiber/v3"
)

type ServiceInterface interface {
	Me(userID string) (*MeResponse, error)
	UpdateProfile(userID string, req *UpdateProfileRequest) (*UpdateProfileResponse, error)
	UpdateEmail(userID string, req *UpdateEmailRequest) (*UpdateEmailResponse, error)
	ChangePassword(userID string, req *ChangePasswordRequest) error
	DeleteAccount(userID string) error
	FindUserByID(param *GetUserURI) (*User, error)
	Search(userID string, query *SearchQuery) (*SearchResponse, error)
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
	userID := fiber.Locals[string](c, "user_id")

	me, err := h.service.Me(userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	return c.JSON(me)
}

func (h *Handler) UpdateProfile(c fiber.Ctx) error {
	userID := fiber.Locals[string](c, "user_id")

	req := new(UpdateProfileRequest)
	if err := c.Bind().Body(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	res, err := h.service.UpdateProfile(userID, req)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "failed to update profile"})
	}

	return c.JSON(res)
}

func (h *Handler) UpdateEmail(c fiber.Ctx) error {
	userID := fiber.Locals[string](c, "user_id")

	req := new(UpdateEmailRequest)
	if err := c.Bind().Body(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	res, err := h.service.UpdateEmail(userID, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(res)
}

func (h *Handler) ChangePassword(c fiber.Ctx) error {
	userID := fiber.Locals[string](c, "user_id")

	req := new(ChangePasswordRequest)
	if err := c.Bind().Body(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.service.ChangePassword(userID, req); err != nil {
		if errors.Is(err, ErrInvalidPassword) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid current password"})
		}

		if errors.Is(err, apperr.ErrNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
		}

		if errors.Is(err, ErrPasswordMismatch) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "new password does not match"})
		}

		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to change password"})
	}

	return c.JSON(fiber.Map{"message": "password changed successfully"})
}

func (h *Handler) DeleteAccount(c fiber.Ctx) error {
	userID := fiber.Locals[string](c, "user_id")

	if err := h.service.DeleteAccount(userID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) Search(c fiber.Ctx) error {
	userID := fiber.Locals[string](c, "user_id")

	query := new(SearchQuery)
	if err := c.Bind().Query(query); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	res, err := h.service.Search(userID, query)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(res)
}

func (h *Handler) FindByID(c fiber.Ctx) error {
	param := new(GetUserURI)
	if err := c.Bind().URI(param); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	res, err := h.service.FindUserByID(param)
	if err != nil {
		if errors.Is(err, apperr.ErrNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
		}

		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(res)
}
