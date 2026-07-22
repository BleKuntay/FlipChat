package attachment

import (
	"errors"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"io"
)

const MaxFileSize = 5 * 1024 * 1024

var allowedMIMEs = map[string]string{
	"\xff\xd8\xff":      "image/jpeg",
	"\x89PNG\r\n\x1a\n": "image/png",
	"GIF87a":            "image/gif",
	"GIF89a":            "image/gif",
	"RIFF????WEBP":      "image/webp",
}

func DetectMIME(header []byte) (mime string, err error) {
	if len(header) < 4 {
		return "", apperr.ErrUnsupportedMIME
	}

	h := string(header)

	// JPEG
	if h[:3] == "\xff\xd8\xff" {
		return "image/jpeg", nil
	}

	// PNG
	if len(h) >= 8 && h[:8] == "\x89PNG\r\n\x1a\n" {
		return "image/png", nil
	}

	// GIF87a or GIF89a
	if len(h) >= 6 && (h[:6] == "GIF87a" || h[:6] == "GIF89a") {
		return "image/gif", nil
	}

	// WEBP: RIF????WEBP
	if len(h) >= 12 && h[:4] == "RIFF" && h[8:12] == "WEBP" {
		return "image/webp", nil
	}

	return "", apperr.ErrUnsupportedMIME
}

func ReadHeader(r io.Reader) ([]byte, error) {
	buf := make([]byte, 12)

	n, err := io.ReadFull(r, buf)
	if err != nil && errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}

	return buf[:n], nil
}
