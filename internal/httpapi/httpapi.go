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
	"strings"
	"time"

	"ytreverse/internal/ratelimit"
	"ytreverse/internal/youtube"
	"ytreverse/internal/ytinput"
)

const watchChunk = 50

const maxRequestBodyBytes = 8 << 10

// RequestTimeout bounds the upstream work behind one playlist request, which
// is up to metaCost + maxPlaylistPages sequential YouTube calls. cmd/server
// derives the
// connection write deadline and the shutdown budget from it, so a request that
// runs to this limit is still allowed to finish.
const RequestTimeout = 2 * time.Minute

const (
	quotaExhaustedMessage   = "daily API quota exhausted, try again after quota resets at midnight Pacific Time"
	playlistTooLargeMessage = "playlist too large to process right now"
)

// errBudgetExhausted travels back out of the YouTube page walk when the daily
// budget cannot cover the next page request.
var errBudgetExhausted = errors.New("daily budget exhausted")

// Quota policy: YouTube's default daily API quota is 10,000 units. We only
// budget 8,000/day for this endpoint, keeping headroom for other usage of
// the same key (console testing, future features). A request costs
// 1 unit (playlist metadata) + 1 unit per page of items fetched; a single
// playlist is capped at maxPlaylistPages so no one request can eat the
// whole day's budget. The per-IP limiter (5 burst, 1/min refill) stops a
// single caller from looping requests faster than a person would.
const (
	dailyQuotaBudget = 8000
	metaCost         = 1  // one playlists.list call
	maxPlaylistPages = 40 // up to 2000 items
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
	mux.HandleFunc("GET /healthz", func(http.ResponseWriter, *http.Request) {})
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

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	var req struct {
		Input string `json:"input"`
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// Decode stops at the end of the first JSON value, so a body that carries
	// megabytes of anything after a valid prefix is never read far enough to
	// trip MaxBytesReader.
	if err := dec.Decode(new(struct{})); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	id, err := ytinput.Parse(req.Input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), RequestTimeout)
	defer cancel()

	// Reserved before the call, not after: this call's own result is what says
	// how big the playlist is, so by the time that is known the unit is already
	// spent. It is never given back — a metadata call that failed may still
	// have been billed, and over-charging by one is the safe direction.
	if !s.budget.Reserve(metaCost) {
		writeError(w, http.StatusTooManyRequests, quotaExhaustedMessage)
		return
	}

	meta, err := s.yt.FetchPlaylistMeta(ctx, id)
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

	// The item count is a snapshot from the metadata call, so this only refuses
	// playlists already known to be too large; it is not what keeps the walk
	// below within budget.
	if int((meta.ItemCount+youtube.PageSize-1)/youtube.PageSize) > maxPlaylistPages {
		writeError(w, http.StatusRequestEntityTooLarge, playlistTooLargeMessage)
		return
	}

	// Each page reserves its own unit immediately before the request that
	// spends it, so a short page or a playlist that grew since the metadata
	// call cannot bill past what was claimed, and a walk that crosses midnight
	// charges each request to the day it was actually made in.
	items, err := s.yt.FetchPlaylistItems(ctx, id, maxPlaylistPages, func() error {
		if !s.budget.Reserve(1) {
			return errBudgetExhausted
		}
		return nil
	})
	switch {
	case errors.Is(err, errBudgetExhausted):
		writeError(w, http.StatusTooManyRequests, quotaExhaustedMessage)
		return
	case errors.Is(err, youtube.ErrTooManyPages):
		writeError(w, http.StatusRequestEntityTooLarge, playlistTooLargeMessage)
		return
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

// clientIP prefers X-Real-IP, which the nginx ingress overwrites with the
// address it accepted the connection from. X-Forwarded-For is appended to
// rather than replaced, so its first entry is whatever the caller put there
// and a client could pick its own rate-limit bucket. Falls back to the raw
// connection address for local/direct use.
func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
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
