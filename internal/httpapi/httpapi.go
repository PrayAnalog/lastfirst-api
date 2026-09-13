package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
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
// playlist is capped at maxPlaylistCost so no one request can eat the
// whole day's budget. The per-IP limiter (5 burst, 1/min refill) stops a
// single caller from looping requests faster than a person would.
const (
	dailyQuotaBudget = 8000
	maxPlaylistCost  = 41 // 1 + 40 pages = up to 2000 items
	ipBurst          = 5
	ipRefill         = time.Minute
)

type Server struct {
	yt        *youtube.Client
	staticDir string
	ipLimiter *ratelimit.IPLimiter
	budget    *ratelimit.DailyBudget
}

func New(yt *youtube.Client, staticDir string) *Server {
	return &Server{
		yt:        yt,
		staticDir: staticDir,
		ipLimiter: ratelimit.NewIPLimiter(ipBurst, ipRefill),
		budget:    ratelimit.NewDailyBudget(dailyQuotaBudget),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/playlists", s.createPlaylist)
	mux.Handle("/", s.spa())
	return mux
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
	if !s.ipLimiter.Allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "too many requests, please slow down and try again shortly")
		return
	}

	var req struct {
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	id, err := ytinput.Parse(req.Input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	meta, err := s.yt.FetchPlaylistMeta(r.Context(), id)
	switch {
	case errors.Is(err, youtube.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, youtube.ErrQuotaExceeded):
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	case err != nil:
		serverError(w, err)
		return
	}

	pages := (meta.ItemCount + youtube.PageSize - 1) / youtube.PageSize
	cost := int(1 + pages)
	if cost > maxPlaylistCost {
		writeError(w, http.StatusRequestEntityTooLarge, "playlist too large to process right now")
		return
	}
	if !s.budget.Reserve(cost) {
		writeError(w, http.StatusTooManyRequests, "daily API quota exhausted, try again after quota resets at midnight Pacific Time")
		return
	}

	items, err := s.yt.FetchPlaylistItems(r.Context(), id)
	switch {
	case errors.Is(err, youtube.ErrQuotaExceeded):
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	case err != nil:
		serverError(w, err)
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

// clientIP prefers X-Forwarded-For's first entry, since the service sits
// behind a reverse proxy/ingress in deployment; it falls back to the raw
// connection address for local/direct use.
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.TrimSpace(strings.Split(fwd, ",")[0])
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

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
