package config

import (
	"os"
	"strconv"
)

type Config struct {
	YTAPIKey          string
	StaticDir         string
	Addr              string
	TrustProxyHeaders bool
}

func Load() Config {
	return Config{
		YTAPIKey:          os.Getenv("YT_API_KEY"),
		StaticDir:         getenv("STATIC_DIR", "../frontend/dist"),
		Addr:              getenv("ADDR", ":8080"),
		TrustProxyHeaders: getenvBool("TRUST_PROXY_HEADERS", false),
	}
}

func getenvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
