package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ytreverse/internal/ratelimit"
	"ytreverse/internal/youtube"
	"ytreverse/internal/ytinput"
)

const watchChunk = 50

// Quota policy: YouTube's default daily API quota is 10,000 units. We only
// budget 8,000/day for this endpoint, keeping headroom for other usage of
// the same key (console testing, future features). A request costs
// 1 unit (playlist metadata) + 1 unit per PageSize items fetched; a single
// playlist is capped at maxPlaylistPages so no one request can eat the
// whole day's budget. The per-IP limiter (5 burst, 1/min refill) stops a
// single caller from looping requests faster than a person would.
const (
	dailyQuotaBudget = 8000
	maxPlaylistPages = 40
	ipBurst          = 5
	ipRefill         = time.Minute
	maxTrackedIPs    = 10000
	maxRequestBody   = 8 << 10
	maxConcurrent    = 8
)

// RequestTimeout bounds all YouTube work performed for one HTTP request.
const RequestTimeout = 2 * time.Minute

type youtubeClient interface {
	FetchPlaylistMeta(context.Context, string) (*youtube.Playlist, error)
	FetchPlaylistItems(context.Context, string, int, func() (func(), error)) ([]youtube.Item, int, error)
}

type Server struct {
	yt                youtubeClient
	staticDir         string
	trustProxyHeaders bool
	ipLimiter         *ratelimit.IPLimiter
	budget            *ratelimit.DailyBudget
	slots             chan struct{}
}

func New(yt youtubeClient, staticDir string, trustProxyHeaders bool) *Server {
	return &Server{
		yt:                yt,
		staticDir:         staticDir,
		trustProxyHeaders: trustProxyHeaders,
		ipLimiter:         ratelimit.NewIPLimiter(ipBurst, ipRefill, maxTrackedIPs),
		budget:            ratelimit.NewDailyBudget(dailyQuotaBudget),
		slots:             make(chan struct{}, maxConcurrent),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/playlists", s.createPlaylist)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok\n")); err != nil {
			log.Printf("write healthz response: %v", err)
		}
	})
	mux.Handle("/", s.spa())
	return securityHeaders(mux)
}

type watchLink struct {
	Label        string `json:"label"`
	URL          string `json:"url"`
	ThumbnailURL string `json:"thumbnailUrl"`
	Index        int    `json:"index"`
	Total        int    `json:"total"`
}

type playlistView struct {
	SourceID      string      `json:"sourceId"`
	SourceTitle   string      `json:"sourceTitle"`
	SourceChannel string      `json:"sourceChannel"`
	ReverseTitle  string      `json:"reverseTitle"`
	TotalCount    int         `json:"totalCount"`
	IncludedCount int         `json:"includedCount"`
	ExcludedCount int         `json:"excludedCount"`
	WatchLinks    []watchLink `json:"watchLinks"`
}

func (s *Server) createPlaylist(w http.ResponseWriter, r *http.Request) {
	if !s.ipLimiter.Allow(clientIP(r, s.trustProxyHeaders)) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "too many requests, please slow down and try again shortly")
		return
	}

	var req struct {
		Input string `json:"input"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	id, err := ytinput.Parse(req.Input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusServiceUnavailable, "service busy, please try again shortly")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), RequestTimeout)
	defer cancel()

	metaReservation, ok := s.budget.ReserveForUse(1)
	if !ok {
		s.quotaExhausted(w)
		return
	}
	defer metaReservation.Release()
	meta, err := s.yt.FetchPlaylistMeta(ctx, id)
	metaReservation.Commit(1)
	switch {
	case errors.Is(err, youtube.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, youtube.ErrQuotaExceeded):
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	case err != nil:
		upstreamError(w, err)
		return
	}

	pages := max(int64(1), (meta.ItemCount+youtube.PageSize-1)/youtube.PageSize)
	if pages > maxPlaylistPages {
		writeError(w, http.StatusRequestEntityTooLarge, "playlist too large to process right now")
		return
	}
	itemReservation, ok := s.budget.Reserve(int(pages))
	if !ok {
		s.quotaExhausted(w)
		return
	}
	defer itemReservation.Release()

	items, calls, err := s.yt.FetchPlaylistItems(ctx, id, maxPlaylistPages, func() (func(), error) {
		if itemReservation.Use(1) {
			return func() {}, nil
		}
		extraReservation, ok := s.budget.ReserveForUse(1)
		if !ok {
			return nil, youtube.ErrPageNotReserved
		}
		return func() { extraReservation.Commit(1) }, nil
	})
	itemReservation.Commit(min(calls, int(pages)))
	switch {
	case errors.Is(err, youtube.ErrQuotaExceeded):
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	case errors.Is(err, youtube.ErrPageLimitReached):
		writeError(w, http.StatusRequestEntityTooLarge, "playlist too large to process right now")
		return
	case errors.Is(err, youtube.ErrPageNotReserved):
		s.quotaExhausted(w)
		return
	case err != nil:
		upstreamError(w, err)
		return
	}

	reverseTitle := youtube.ReverseTitle(meta.Title)
	ids := youtube.Reversed(items)
	writeJSON(w, http.StatusOK, playlistView{
		SourceID:      meta.ID,
		SourceTitle:   meta.Title,
		SourceChannel: meta.ChannelTitle,
		ReverseTitle:  reverseTitle,
		TotalCount:    len(items),
		IncludedCount: len(ids),
		ExcludedCount: len(items) - len(ids),
		WatchLinks:    watchLinks(reverseTitle, ids),
	})
}

// clientIP only accepts the ingress-controlled single-value header when the
// deployment explicitly opts into trusting its reverse proxy.
func clientIP(r *http.Request, trustProxyHeaders bool) string {
	if trustProxyHeaders {
		if forwarded := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); forwarded != nil {
			return forwarded.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func watchLinks(reverseTitle string, ids []string) []watchLink {
	links := []watchLink{}
	total := (len(ids) + watchChunk - 1) / watchChunk
	for start := 0; start < len(ids); start += watchChunk {
		end := min(start+watchChunk, len(ids))
		links = append(links, watchLink{
			Label:        fmt.Sprintf("%s %d–%d", reverseTitle, start+1, end),
			URL:          "https://www.youtube.com/watch_videos?video_ids=" + strings.Join(ids[start:end], ","),
			ThumbnailURL: thumbnailURL(ids[start]),
			Index:        start/watchChunk + 1,
			Total:        total,
		})
	}
	return links
}

// thumbnailURL points directly at YouTube's static thumbnail CDN, so the
// browser loads it straight from YouTube without our server fetching or
// proxying any image bytes.
func thumbnailURL(videoID string) string {
	return "https://i.ytimg.com/vi/" + videoID + "/hqdefault.jpg"
}

func (s *Server) spa() http.Handler {
	files := http.FileServer(http.Dir(s.staticDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(filepath.Join(s.staticDir, filepath.Clean(r.URL.Path))); err != nil {
			http.ServeFile(w, r, filepath.Join(s.staticDir, "index.html"))
			return
		}
		files.ServeHTTP(w, r)
	})
}

func serverError(w http.ResponseWriter, err error) {
	log.Print(err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func upstreamError(w http.ResponseWriter, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		writeError(w, http.StatusGatewayTimeout, "upstream request timed out")
		return
	}
	serverError(w, err)
}

func (s *Server) quotaExhausted(w http.ResponseWriter) {
	w.Header().Set("Retry-After", strconv.Itoa(s.budget.RetryAfterSeconds()))
	writeError(w, http.StatusTooManyRequests, "daily API quota exhausted, try again after quota resets at midnight Pacific Time")
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write JSON response: %v", err)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' https://i.ytimg.com; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'; object-src 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}
