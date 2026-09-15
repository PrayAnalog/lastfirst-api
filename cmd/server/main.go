package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"ytreverse/internal/config"
	"ytreverse/internal/httpapi"
	"ytreverse/internal/youtube"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()

	yt, err := youtube.New(ctx, cfg.YTAPIKey)
	if err != nil {
		log.Fatal(err)
	}

	srv := httpapi.New(yt, cfg.StaticDir)
	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		// Generous: fetching a large playlist can make up to ~40
		// sequential YouTube API calls before the response is written.
		WriteTimeout: 2 * time.Minute,
		IdleTimeout:  90 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}
