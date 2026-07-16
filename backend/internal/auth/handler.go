package auth

import (
	"context"
	"time"

	"github.com/BleKuntay/FlipChat/backend/internal/config"
	"github.com/BleKuntay/FlipChat/backend/internal/shared"
	"github.com/BleKuntay/FlipChat/backend/pkg/jwt"
	"github.com/gofiber/fiber/v3"
	"github.com/pkg/errors"
)

type ServiceInterface interface {
	Register(ctx context.Context, request *RegisterRequest) (*Response, string, error)
	Login(ctx context.Context, request *LoginRequest) (*Response, string, error)
	Logout(ctx context.Context, refreshToken string) error
	LogoutAll(ctx context.Context, userID string) error
	Refresh(ctx context.Context, refreshToken string) (access, refresh string, err error)
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(service ServiceInterface) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router fiber.Router) {
	router.Post("/register", h.Register)
	router.Post("/login", h.Login)
	router.Post("/logout", h.Logout)
	router.Post("/logout-all", h.LogoutAll)
	router.Post("/refresh", h.Refresh)
}

func (h *Handler) Register(c fiber.Ctx) error {
	ctx := c.Context()

	request := new(RegisterRequest)
	if err := c.Bind().Body(request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	response, refreshToken, err := h.service.Register(ctx, request)
	if err != nil {
		switch {
		case errors.Is(err, ErrEmailAlreadyInUse):
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
		case errors.Is(err, ErrUsernameAlreadyTaken):
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
		case errors.Is(err, shared.ErrPasswordWeak):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
		}
	}

	setCookie(c, refreshToken)

	return c.Status(fiber.StatusCreated).JSON(response)
}

func (h *Handler) Login(c fiber.Ctx) error {
	ctx := c.Context()

	request := new(LoginRequest)
	if err := c.Bind().Body(request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	response, refreshToken, err := h.service.Login(ctx, request)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCredentials):
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
		}
	}

	setCookie(c, refreshToken)

	return c.JSON(response)
}

func (h *Handler) Logout(c fiber.Ctx) error {
	ctx := c.Context()

	if refreshToken := c.Cookies("refresh_token"); refreshToken != "" {
		_ = h.service.Logout(ctx, refreshToken)
	}

	clearCookie(c)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "logged out successfully"})
}

func (h *Handler) LogoutAll(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	if err := h.service.LogoutAll(ctx, userID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	clearCookie(c)

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) Refresh(c fiber.Ctx) error {
	ctx := c.Context()

	refreshToken := c.Cookies("refresh_token")
	if refreshToken == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "refresh token required"})
	}

	accessToken, newRefreshToken, err := h.service.Refresh(ctx, refreshToken)
	if err != nil {
		switch {
		case errors.Is(err, jwt.ErrInvalidToken):
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
		case errors.Is(err, jwt.ErrRefreshTokenExpired):
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
		}
	}

	setCookie(c, newRefreshToken)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"access_token": accessToken})
}

func clearCookie(c fiber.Ctx) {
	c.Cookie(&fiber.Cookie{
		Name:     "refresh_token",
		Value:    "",
		HTTPOnly: true,
		Path:     "/v1/auth/refresh",
		Expires:  time.Unix(0, 0),
	})
}

func setCookie(c fiber.Ctx, refresh string) {
	isProd := config.App.AppEnv == "production"

	c.Cookie(&fiber.Cookie{
		Name:     "refresh_token",
		Value:    refresh,
		Expires:  time.Now().Add(config.App.RefreshTokenExpiry),
		Secure:   isProd,
		HTTPOnly: true,
		SameSite: "Strict",
		Path:     "/v1/auth/refresh",
	})
}
