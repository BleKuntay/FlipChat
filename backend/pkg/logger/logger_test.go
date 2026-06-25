package logger

import (
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"testing"
)

func TestInit_Production(t *testing.T) {
	Init("production")
	assert.NotNil(t, Get())
}

func TestInit_Singleton(t *testing.T) {
	Init("development")
	a := Get()

	Init("development")
	b := Get()

	assert.Same(t, a, b)
}

func TestInfo_LogWritten(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	instance = zap.New(core)

	Info("user created", zap.String("user_id", "123"))

	require.Equal(t, 1, logs.Len())
	entry := logs.All()[0]
	assert.Equal(t, "user created", entry.Message)
	assert.Equal(t, "123", entry.ContextMap()["user_id"])
}

func TestWarn_LogWritten(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	instance = zap.New(core)

	Warn("wrong password", zap.String("user_id", "123"))

	require.Equal(t, 1, logs.Len())
	entry := logs.All()[0]
	assert.Equal(t, "wrong password", entry.Message)
	assert.Equal(t, zap.WarnLevel.String(), entry.Level.String())
	assert.Equal(t, "123", entry.ContextMap()["user_id"])
}

func TestDebug_NotLoggedInProduction(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	instance = zap.New(core)

	Debug("this is debug")

	assert.Equal(t, 0, logs.Len())
}

func TestError_ContainsFields(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	instance = zap.New(core)

	err := errors.New("db connection failed")
	Error("query failed",
		zap.Error(err),
		zap.String("layer", "repository"),
	)

	entry := logs.All()[0]
	fields := entry.ContextMap()
	assert.Equal(t, "repository", fields["layer"])
	assert.Contains(t, fmt.Sprint(fields["error"]), "db connection failed")
}

func TestSync(t *testing.T) {
	Init("development")
	err := Sync()

	_ = err
}
