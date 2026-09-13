package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"ytreverse/internal/ordercheck"
	"ytreverse/internal/quota"
	"ytreverse/internal/store"
	"ytreverse/internal/youtube"
	"ytreverse/internal/ytinput"
)

const watchChunk = 50

type Server struct {
	store     *store.Store
	yt        *youtube.Client
	quota     *quota.Counter
	oauth     *oauth2.Config
	adminUser string
	adminPass string
	staticDir string
}

func New(st *store.Store, yt *youtube.Client, q *quota.Counter, oauth *oauth2.Config,
	adminUser, adminPass, staticDir string) *Server {
	return &Server{store: st, yt: yt, quota: q, oauth: oauth, adminUser: adminUser, adminPass: adminPass, staticDir: staticDir}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/playlists", s.createPlaylist)
	mux.HandleFunc("GET /api/playlists/{id}", s.getPlaylist)
	mux.HandleFunc("POST /api/playlists/{id}/refresh", s.refresh("user_refresh"))

	mux.Handle("GET /api/admin/playlists", s.admin(s.adminList))
	mux.Handle("GET /api/admin/playlists/{id}", s.admin(s.adminDetail))
	mux.Handle("POST /api/admin/playlists/{id}/refresh", s.admin(s.refresh("admin")))
	mux.Handle("GET /api/admin/jobs", s.admin(s.adminJobs))
	mux.Handle("GET /api/admin/quota", s.admin(s.adminQuota))
	mux.Handle("GET /api/admin/channel", s.admin(s.adminChannel))
	mux.Handle("GET /api/admin/oauth/start", s.admin(s.oauthStart))
	mux.Handle("GET /api/admin/oauth/callback", s.admin(s.oauthCallback))

	mux.Handle("/", s.spa())
	return mux
}

type watchLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type playlistView struct {
	store.ReversePlaylist
	YTPlaylistURL string      `json:"ytPlaylistUrl,omitempty"`
	ResumeAt      *time.Time  `json:"resumeAt,omitempty"`
	Check         store.Check `json:"check"`
	WatchLinks    []watchLink `json:"watchLinks"`
}

func (s *Server) createPlaylist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
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

	rp, err := s.store.ReverseBySource(ctx, id)
	if err == nil {
		if err := s.store.EnqueueJob(ctx, rp.ID, "sync", "user_submit"); err != nil {
			serverError(w, err)
			return
		}
		s.writeView(w, r, rp.ID)
		return
	}
	if !errors.Is(err, store.ErrNotFound) {
		serverError(w, err)
		return
	}

	p, err := s.yt.FetchPlaylistCached(ctx, id)
	switch {
	case errors.Is(err, youtube.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, quota.ErrExhausted):
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	case err != nil:
		serverError(w, err)
		return
	}
	if err := s.store.SaveSource(ctx, p, ordercheck.Check(p.Items)); err != nil {
		serverError(w, err)
		return
	}
	rpID, err := s.store.CreateReverse(ctx, id)
	if err != nil {
		serverError(w, err)
		return
	}
	if err := s.store.EnqueueJob(ctx, rpID, "create", "user_submit"); err != nil {
		serverError(w, err)
		return
	}
	s.writeView(w, r, rpID)
}

func (s *Server) getPlaylist(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.writeView(w, r, id)
}

func (s *Server) refresh(trigger string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		if _, err := s.store.GetReverse(r.Context(), id); err != nil {
			storeError(w, err)
			return
		}
		if err := s.store.EnqueueJob(r.Context(), id, "sync", trigger); err != nil {
			serverError(w, err)
			return
		}
		s.writeView(w, r, id)
	}
}

func (s *Server) writeView(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	rp, err := s.store.GetReverse(ctx, id)
	if err != nil {
		storeError(w, err)
		return
	}
	items, err := s.store.SourceItems(ctx, rp.SourceID)
	if err != nil {
		serverError(w, err)
		return
	}
	check, err := s.store.LatestCheck(ctx, rp.SourceID)
	if err != nil {
		serverError(w, err)
		return
	}
	job, err := s.store.ActiveJob(ctx, rp.ID)
	if err != nil {
		serverError(w, err)
		return
	}
	v := playlistView{
		ReversePlaylist: rp,
		YTPlaylistURL:   ytPlaylistURL(rp.YTPlaylistID),
		Check:           check,
		WatchLinks:      watchLinks(youtube.Reversed(items)),
	}
	if job != nil && job.Status == "waiting_quota" {
		v.ResumeAt = &job.RunAfter
	}
	writeJSON(w, http.StatusOK, v)
}

func watchLinks(ids []string) []watchLink {
	links := []watchLink{}
	for start := 0; start < len(ids); start += watchChunk {
		end := min(start+watchChunk, len(ids))
		links = append(links, watchLink{
			Label: fmt.Sprintf("%d–%d", start+1, end),
			URL:   "https://www.youtube.com/watch_videos?video_ids=" + strings.Join(ids[start:end], ","),
		})
	}
	return links
}

func ytPlaylistURL(id *string) string {
	if id == nil {
		return ""
	}
	return "https://www.youtube.com/playlist?list=" + *id
}

func (s *Server) adminList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	list, err := s.store.ListReverse(r.Context(), q.Get("verdict"), q.Get("status"))
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type adminItem struct {
	Position    int        `json:"position"`
	VideoID     string     `json:"videoId"`
	Title       string     `json:"title"`
	PublishedAt *time.Time `json:"publishedAt"`
	Episode     string     `json:"episode"`
	Anomaly     bool       `json:"anomaly"`
}

func (s *Server) adminDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	rp, err := s.store.GetReverse(ctx, id)
	if err != nil {
		storeError(w, err)
		return
	}
	items, err := s.store.SourceItems(ctx, rp.SourceID)
	if err != nil {
		serverError(w, err)
		return
	}
	checks, err := s.store.Checks(ctx, rp.SourceID)
	if err != nil {
		serverError(w, err)
		return
	}

	anomalies := map[int]bool{}
	if len(checks) > 0 {
		for _, pos := range checks[0].Anomalies {
			anomalies[pos] = true
		}
	}
	pattern, eps := ordercheck.Episodes(items)
	out := make([]adminItem, len(items))
	for i, it := range items {
		out[i] = adminItem{
			Position:    i,
			VideoID:     it.VideoID,
			Title:       it.Title,
			PublishedAt: it.PublishedAt,
			Episode:     eps[i],
			Anomaly:     anomalies[i],
		}
	}
	writeJSON(w, http.StatusOK, struct {
		Playlist      store.ReversePlaylist `json:"playlist"`
		YTPlaylistURL string                `json:"ytPlaylistUrl"`
		TitlePattern  string                `json:"titlePattern"`
		Items         []adminItem           `json:"items"`
		Checks        []store.Check         `json:"checks"`
	}{rp, ytPlaylistURL(rp.YTPlaylistID), pattern, out, checks})
}

func (s *Server) adminJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.store.ListJobs(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, jobs)
}

func (s *Server) adminQuota(w http.ResponseWriter, r *http.Request) {
	used, err := s.quota.Used(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"used":    used,
		"limit":   s.quota.Limit(),
		"resetAt": quota.NextReset(time.Now()),
	})
}

func (s *Server) adminChannel(w http.ResponseWriter, r *http.Request) {
	_, err := s.store.RefreshToken(r.Context())
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"connected": err == nil})
}

func (s *Server) oauthStart(w http.ResponseWriter, r *http.Request) {
	state := rand.Text()
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/api/admin/oauth",
		MaxAge:   600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, s.oauth.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce), http.StatusFound)
}

func (s *Server) oauthCallback(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("oauth_state")
	if err != nil || c.Value != r.URL.Query().Get("state") {
		writeError(w, http.StatusBadRequest, "invalid oauth state")
		return
	}
	tok, err := s.oauth.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		serverError(w, err)
		return
	}
	if err := s.store.SaveRefreshToken(r.Context(), tok.RefreshToken); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusFound)
}

func (s *Server) admin(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok ||
			subtle.ConstantTimeCompare([]byte(user), []byte(s.adminUser)) != 1 ||
			subtle.ConstantTimeCompare([]byte(pass), []byte(s.adminPass)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="admin"`)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		h(w, r)
	})
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

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}

func storeError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	serverError(w, err)
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
