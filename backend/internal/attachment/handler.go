package attachment

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/BleKuntay/FlipChat/backend/pkg/httputil"
	"github.com/gofiber/fiber/v3"
)

// allowedMIMETypes is the allowlist for Content-Type on download.
// Prevents a client-supplied mime_type from being reflected as-is.
var allowedMIMETypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

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

	mimeType := metadata.MIMEType
	if !allowedMIMETypes[mimeType] {
		mimeType = "application/octet-stream"
	}

	c.Set(fiber.HeaderContentType, mimeType)
	c.Set(fiber.HeaderContentDisposition, safeContentDisposition(metadata.Filename))
	c.Set(fiber.HeaderContentLength, strconv.FormatInt(metadata.Size, 10))
	c.Set("X-Content-Type-Options", "nosniff")

	_, err = io.Copy(c.Response().BodyWriter(), reader)
	return err
}

// safeContentDisposition builds a Content-Disposition header value that
// is safe against header injection and quote-breaking. Filename is
// truncated to 200 runes, control characters are stripped, and
// double-quotes are escaped.
func safeContentDisposition(filename string) string {
	// Strip control characters and limit length.
	var b strings.Builder
	count := 0
	for _, r := range filename {
		if count >= 200 {
			break
		}
		if r >= 0x20 && utf8.ValidRune(r) {
			b.WriteRune(r)
			count++
		}
	}

	safe := strings.ReplaceAll(b.String(), `"`, `\"`)
	return fmt.Sprintf(`inline; filename="%s"`, safe)
}
