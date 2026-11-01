package config

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv, Port, DatabaseDSN, JWTSecret, BusinessName, BusinessAddress string
	WhatsAppEnabled                                                     bool
	WhatsAppAPIURL, WhatsAppAccessToken, WhatsAppPhoneNumberID          string
	InitialAdminName, InitialAdminEmail, InitialAdminPassword           string
	AccessTTL, RefreshTTL                                               time.Duration
}

func Load() (Config, error) {
	_ = godotenv.Load()
	port := env("APP_PORT", env("PORT", "8080"))
	accessTTL, err := time.ParseDuration(env("JWT_ACCESS_EXPIRY", "1h"))
	if err != nil {
		return Config{}, err
	}
	refreshTTL, err := time.ParseDuration(env("JWT_REFRESH_EXPIRY", "168h"))
	if err != nil {
		return Config{}, err
	}
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=UTC",
			env("DATABASE_HOST", "localhost"), env("DATABASE_USER", "postgres"),
			env("DATABASE_PASSWORD", "postgres"), env("DATABASE_NAME", "cardamom"),
			env("DATABASE_PORT", "5432"), env("DATABASE_SSLMODE", "disable"))
	}

	return Config{
		AppEnv:                env("APP_ENV", "development"),
		Port:                  port,
		DatabaseDSN:           dsn,
		JWTSecret:             secret,
		BusinessName:          env("BUSINESS_NAME", "Cardamom Drying Centre"),
		BusinessAddress:       env("BUSINESS_ADDRESS", "Munnar, Kerala"),
		InitialAdminName:      env("INITIAL_ADMIN_NAME", "Aroma Admin"),
		InitialAdminEmail:     env("INITIAL_ADMIN_EMAIL", ""),
		InitialAdminPassword:  env("INITIAL_ADMIN_PASSWORD", ""),
		WhatsAppEnabled:       env("WHATSAPP_ENABLED", "false") == "true",
		WhatsAppAPIURL:        env("WHATSAPP_API_URL", ""),
		WhatsAppAccessToken:   env("WHATSAPP_ACCESS_TOKEN", ""),
		WhatsAppPhoneNumberID: env("WHATSAPP_PHONE_NUMBER_ID", ""),
		AccessTTL:             accessTTL,
		RefreshTTL:            refreshTTL,
	}, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
