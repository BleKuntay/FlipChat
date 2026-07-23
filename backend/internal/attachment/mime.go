package attachment

import (
	"errors"
	"github.com/BleKuntay/FlipChat/backend/pkg/apperr"
	"io"
)

const (
	MaxFileSize = 5 * 1024 * 1024
	// MinFileSize is the number of bytes required for magic-byte detection.
	// WEBP needs 12 bytes (the largest header we check).
	MinFileSize = 12
)

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

// ReadHeader reads the first 12 bytes needed for magic-byte MIME detection.
// io.EOF and io.ErrUnexpectedEOF are treated as normal (file smaller than
// the header buffer); all other errors are propagated.
func ReadHeader(r io.Reader) ([]byte, error) {
	buf := make([]byte, 12)

	n, err := io.ReadFull(r, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}

	return buf[:n], nil
}
