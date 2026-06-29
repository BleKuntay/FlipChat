package shared_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BleKuntay/FlipChat/backend/internal/shared"
)

// ── HashPassword ──────────────────────────────────────────────────────────────

func TestHashPassword(t *testing.T) {
	t.Run("returns encoded string, not plaintext", func(t *testing.T) {
		hash, err := shared.HashPassword("secret123")

		require.NoError(t, err)
		assert.NotEqual(t, "secret123", hash)
	})

	t.Run("output has salt.hash format (two parts separated by dot)", func(t *testing.T) {
		hash, err := shared.HashPassword("secret123")

		require.NoError(t, err)
		parts := strings.Split(hash, ".")
		assert.Len(t, parts, 2, "encoded must be in 'salt.hash' format")
		assert.NotEmpty(t, parts[0], "salt must not be empty")
		assert.NotEmpty(t, parts[1], "hash must not be empty")
	})

	t.Run("same password produces different hashes (random salt)", func(t *testing.T) {
		hash1, err1 := shared.HashPassword("secret123")
		hash2, err2 := shared.HashPassword("secret123")

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2, "two hashes of same password must differ due to random salt")
	})

	t.Run("empty password can be hashed without error", func(t *testing.T) {
		// We don't enforce non-empty password here — that's the handler/validator's job.
		// This test just ensures the function doesn't panic or error on empty input.
		hash, err := shared.HashPassword("")

		require.NoError(t, err)
		assert.NotEmpty(t, hash)
	})
}

// ── VerifyPassword ────────────────────────────────────────────────────────────

func TestVerifyPassword(t *testing.T) {
	t.Run("correct password returns true", func(t *testing.T) {
		hash, err := shared.HashPassword("secret123")
		require.NoError(t, err)

		assert.True(t, shared.VerifyPassword("secret123", hash))
	})

	t.Run("wrong password returns false", func(t *testing.T) {
		hash, err := shared.HashPassword("secret123")
		require.NoError(t, err)

		assert.False(t, shared.VerifyPassword("wrong-password", hash))
	})

	t.Run("empty string against valid hash returns false", func(t *testing.T) {
		hash, err := shared.HashPassword("secret123")
		require.NoError(t, err)

		assert.False(t, shared.VerifyPassword("", hash))
	})

	t.Run("tampered hash returns false", func(t *testing.T) {
		hash, err := shared.HashPassword("secret123")
		require.NoError(t, err)

		tampered := hash[:len(hash)-4] + "XXXX"
		assert.False(t, shared.VerifyPassword("secret123", tampered))
	})

	// ── malformed encoded string ──────────────────────────────────────────────

	t.Run("encoded with no dot separator returns false", func(t *testing.T) {
		assert.False(t, shared.VerifyPassword("secret123", "nodothere"))
	})

	t.Run("encoded with more than one dot returns false", func(t *testing.T) {
		assert.False(t, shared.VerifyPassword("secret123", "a.b.c"))
	})

	t.Run("empty encoded string returns false", func(t *testing.T) {
		assert.False(t, shared.VerifyPassword("secret123", ""))
	})

	t.Run("invalid base64 in salt part returns false", func(t *testing.T) {
		assert.False(t, shared.VerifyPassword("secret123", "!!!invalid_base64!!!.validhashpart"))
	})

	t.Run("invalid base64 in hash part returns false", func(t *testing.T) {
		// Valid base64 salt, invalid base64 hash
		assert.False(t, shared.VerifyPassword("secret123", "dmFsaWRzYWx0.!!!invalid!!!"))
	})
}

// ── HashPassword + VerifyPassword roundtrip ───────────────────────────────────

func TestPasswordRoundtrip(t *testing.T) {
	passwords := []string{
		"simple",
		"With Spaces",
		"w!th$p3c!@lCh@r$",
		"very-long-password-that-exceeds-typical-length-expectations-1234567890",
		"unicode-こんにちは",
	}

	for _, pw := range passwords {
		pw := pw // capture for parallel sub-test
		t.Run("roundtrip: "+pw, func(t *testing.T) {
			t.Parallel()

			hash, err := shared.HashPassword(pw)
			require.NoError(t, err)

			assert.True(t, shared.VerifyPassword(pw, hash), "password should verify against its own hash")
			assert.False(t, shared.VerifyPassword(pw+"x", hash), "modified password should not verify")
		})
	}
}
