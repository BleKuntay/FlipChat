package attachment

import (
	"context"
	"fmt"
	"github.com/BleKuntay/FlipChat/backend/pkg/httputil"
	"github.com/gofiber/fiber/v3"
	"io"
	"strconv"
)

type ServiceInterface interface {
	Upload(ctx context.Context, uploaderID, filename string, size int64, reader io.Reader) (*UploadResponse, error)
	Download(ctx context.Context, requesterID, attachmentID string) (io.ReadCloser, *Metadata, error)
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(service ServiceInterface) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router fiber.Router) {
	router.Post("/upload", h.Upload)
	router.Get("/:id", h.Download)
}

func (h *Handler) Upload(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")

	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file is required"})
	}

	file, err := fileHeader.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to open file"})
	}
	defer file.Close()

	response, err := h.service.Upload(ctx, userID, fileHeader.Filename, fileHeader.Size, file)
	if err != nil {
		return httputil.ErrorStatus(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(response)
}

func (h *Handler) Download(c fiber.Ctx) error {
	ctx := c.Context()
	userID := fiber.Locals[string](c, "user_id")
	attachmentID := c.Params("id")

	reader, metadata, err := h.service.Download(ctx, userID, attachmentID)
	if err != nil {
		return httputil.ErrorStatus(c, err)
	}
	defer reader.Close()

	contentDisposition := fmt.Sprintf(`inline; filename="%s"`, metadata.Filename)
	contentLength := strconv.FormatInt(metadata.Size, 10)

	c.Set(fiber.HeaderContentType, metadata.MIMEType)
	c.Set(fiber.HeaderContentDisposition, contentDisposition)
	c.Set(fiber.HeaderContentLength, contentLength)

	_, err = io.Copy(c.Response().BodyWriter(), reader)
	return err
}
