package config

import (
	"fmt"
	"github.com/joho/godotenv"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppPort string
	AppEnv  string

	AllowedOrigins []string

	PostgresHost        string
	PostgresPort        string
	PostgresUser        string
	PostgresPassword    string
	PostgresDatabase    string
	PostgresSSLMode     string
	PostgresMaxOpenConn int
	PostgresMaxIdleConn int

	JWTSecret          string
	AccessTokenExpiry  time.Duration
	RefreshTokenExpiry time.Duration
}

var App *Config

func LoadConfig() {
	_ = godotenv.Load()

	accessExpiry, err := time.ParseDuration(getEnv("ACCESS_TOKEN_EXPIRY", "15m"))
	if err != nil {
		accessExpiry = 15 * time.Minute
	}

	refreshExpiry, err := time.ParseDuration(getEnv("REFRESH_TOKEN_EXPIRY", "168h"))
	if err != nil {
		refreshExpiry = 7 * 24 * time.Hour
	}

	App = &Config{
		AppPort:             getEnv("APP_PORT", "8080"),
		AppEnv:              getEnv("APP_ENV", "development"),
		AllowedOrigins:      getAllowedOrigins(),
		PostgresHost:        getEnv("POSTGRES_HOST", "localhost"),
		PostgresPort:        getEnv("POSTGRES_PORT", "5432"),
		PostgresUser:        getEnv("POSTGRES_USER", "flip_chat_user"),
		PostgresPassword:    getEnv("POSTGRES_PASSWORD", "flip_chat_password"),
		PostgresDatabase:    getEnv("POSTGRES_DATABASE", "flip_chat_db"),
		PostgresSSLMode:     getEnv("POSTGRES_SSLMODE", "disable"),
		PostgresMaxOpenConn: getEnvInt("POSTGRES_MAX_OPEN_CONNECTIONS", 25),
		PostgresMaxIdleConn: getEnvInt("POSTGRES_MAX_IDLE_CONNECTION", 5),
		JWTSecret:           getEnv("JWT_SECRET", ""),
		AccessTokenExpiry:   accessExpiry,
		RefreshTokenExpiry:  refreshExpiry,
	}
}

func (c *Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.PostgresHost, c.PostgresPort, c.PostgresUser, c.PostgresPassword, c.PostgresDatabase, c.PostgresSSLMode,
	)
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}

	value, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}

	return value
}

func getAllowedOrigins() []string {
	value := getEnv("ALLOWED_ORIGINS", "")

	return strings.Split(value, ",")
}
