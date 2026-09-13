package config

import "os"

type Config struct {
	YTAPIKey  string
	StaticDir string
	Addr      string
}

func Load() Config {
	return Config{
		YTAPIKey:  os.Getenv("YT_API_KEY"),
		StaticDir: getenv("STATIC_DIR", "../frontend/dist"),
		Addr:      getenv("ADDR", ":8080"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
