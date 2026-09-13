package main

import (
	"context"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	ytapi "google.golang.org/api/youtube/v3"

	"ytreverse/internal/config"
	"ytreverse/internal/httpapi"
	"ytreverse/internal/quota"
	"ytreverse/internal/store"
	"ytreverse/internal/worker"
	"ytreverse/internal/youtube"
	"ytreverse/migrations"
)

func main() {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatal(err)
	}
	if err := goose.Up(stdlib.OpenDBFromPool(db), "."); err != nil {
		log.Fatal(err)
	}

	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatal(err)
	}
	rdb := redis.NewClient(redisOpts)

	oauth := &oauth2.Config{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		RedirectURL:  cfg.OAuthRedirectURL,
		Scopes:       []string{ytapi.YoutubeScope},
		Endpoint:     google.Endpoint,
	}
	q := quota.New(rdb, cfg.QuotaDailyLimit)
	yt, err := youtube.New(ctx, cfg.YTAPIKey, oauth, q, rdb)
	if err != nil {
		log.Fatal(err)
	}
	st := store.New(db)
	if err := st.ResetRunningJobs(ctx); err != nil {
		log.Fatal(err)
	}

	go worker.New(st, yt).Run(ctx)

	srv := httpapi.New(st, yt, q, oauth, cfg.AdminUser, cfg.AdminPassword, cfg.StaticDir)
	log.Fatal(http.ListenAndServe(cfg.Addr, srv.Handler()))
}
