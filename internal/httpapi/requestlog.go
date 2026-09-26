package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const maxLoggedPath = 256

type requestLogKey struct{}

type requestLog struct {
	playlistID string
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) ReadFrom(r io.Reader) (int64, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return io.Copy(w.ResponseWriter, r)
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &requestLog{}
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), requestLogKey{}, rec)))

		status := sw.status
		if status == 0 {
			status = http.StatusOK
		}
		path := r.URL.Path
		if len(path) > maxLoggedPath {
			path = path[:maxLoggedPath]
		}
		attrs := []any{
			"method", r.Method,
			"path", path,
			"status", status,
			"duration_ms", float64(time.Since(start).Microseconds()) / 1000,
			"ip", clientIP(r),
		}
		if rec.playlistID != "" {
			attrs = append(attrs, "playlist_id", rec.playlistID)
		}
		slog.Info("request", attrs...)
	})
}

func setPlaylistID(r *http.Request, id string) {
	if rec, ok := r.Context().Value(requestLogKey{}).(*requestLog); ok {
		rec.playlistID = id
	}
}
