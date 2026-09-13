package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"ytreverse/internal/youtube"
	"ytreverse/internal/ytinput"
)

const watchChunk = 50

type Server struct {
	yt        *youtube.Client
	staticDir string
}

func New(yt *youtube.Client, staticDir string) *Server {
	return &Server{yt: yt, staticDir: staticDir}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/playlists", s.createPlaylist)
	mux.Handle("/", s.spa())
	return mux
}

type watchLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
	Index int    `json:"index"`
	Total int    `json:"total"`
}

type playlistView struct {
	SourceID      string      `json:"sourceId"`
	SourceTitle   string      `json:"sourceTitle"`
	SourceChannel string      `json:"sourceChannel"`
	ReverseTitle  string      `json:"reverseTitle"`
	WatchLinks    []watchLink `json:"watchLinks"`
}

func (s *Server) createPlaylist(w http.ResponseWriter, r *http.Request) {
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

	p, err := s.yt.FetchPlaylist(r.Context(), id)
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

	writeJSON(w, http.StatusOK, playlistView{
		SourceID:      p.ID,
		SourceTitle:   p.Title,
		SourceChannel: p.ChannelTitle,
		ReverseTitle:  youtube.ReverseTitle(p.Title),
		WatchLinks:    watchLinks(youtube.Reversed(p.Items)),
	})
}

func watchLinks(ids []string) []watchLink {
	links := []watchLink{}
	total := (len(ids) + watchChunk - 1) / watchChunk
	for start := 0; start < len(ids); start += watchChunk {
		end := min(start+watchChunk, len(ids))
		links = append(links, watchLink{
			Label: fmt.Sprintf("%d–%d", start+1, end),
			URL:   "https://www.youtube.com/watch_videos?video_ids=" + strings.Join(ids[start:end], ","),
			Index: start/watchChunk + 1,
			Total: total,
		})
	}
	return links
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
