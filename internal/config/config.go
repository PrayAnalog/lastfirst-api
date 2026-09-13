package config

import (
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL        string
	RedisURL           string
	YTAPIKey           string
	GoogleClientID     string
	GoogleClientSecret string
	OAuthRedirectURL   string
	AdminUser          string
	AdminPassword      string
	QuotaDailyLimit    int64
	StaticDir          string
	Addr               string
}

func Load() (Config, error) {
	limit, err := strconv.ParseInt(getenv("QUOTA_DAILY_LIMIT", "10000"), 10, 64)
	if err != nil {
		return Config{}, err
	}
	return Config{
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		RedisURL:           os.Getenv("REDIS_URL"),
		YTAPIKey:           os.Getenv("YT_API_KEY"),
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		OAuthRedirectURL:   os.Getenv("OAUTH_REDIRECT_URL"),
		AdminUser:          os.Getenv("ADMIN_USER"),
		AdminPassword:      os.Getenv("ADMIN_PASSWORD"),
		QuotaDailyLimit:    limit,
		StaticDir:          getenv("STATIC_DIR", "../frontend/dist"),
		Addr:               getenv("ADDR", ":8080"),
	}, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
