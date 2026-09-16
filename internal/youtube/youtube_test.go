package youtube

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/option"
	yt "google.golang.org/api/youtube/v3"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestAPIKeyTransportUsesHeaderWithoutChangingOriginalRequest(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "https://example.com/resource?part=snippet", nil)
	if err != nil {
		t.Fatal(err)
	}
	stopErr := errors.New("stop")
	base := roundTripFunc(func(got *http.Request) (*http.Response, error) {
		if value := got.Header.Get("X-Goog-Api-Key"); value != "secret-key" {
			t.Fatalf("X-Goog-Api-Key = %q, want secret-key", value)
		}
		if value := got.URL.Query().Get("key"); value != "" {
			t.Fatalf("URL key = %q, want empty", value)
		}
		return nil, stopErr
	})
	transport := &apiKeyTransport{apiKey: "secret-key", base: base}

	_, err = transport.RoundTrip(request)
	if !errors.Is(err, stopErr) {
		t.Fatalf("RoundTrip() error = %v, want %v", err, stopErr)
	}
	if value := request.Header.Get("X-Goog-Api-Key"); value != "" {
		t.Fatalf("original request header = %q, want empty", value)
	}
}

func TestAPIErrDoesNotExposeTransportURL(t *testing.T) {
	err := apiErr(errors.New("GET https://example.com?key=secret-key failed"))
	if got := err.Error(); got != "YouTube API transport request failed" {
		t.Fatalf("apiErr() = %q", got)
	}
}

func TestFetchPlaylistItemsCountsShortAndFailingPageCalls(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("pageToken") == "next" {
			http.Error(w, "upstream failure", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"items":[{"snippet":{"title":"one"},"contentDetails":{"videoId":"video-1","videoPublishedAt":"2024-01-01T00:00:00Z"}}],"nextPageToken":"next"}`))
		if err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	service, err := yt.NewService(
		context.Background(),
		option.WithEndpoint(server.URL+"/"),
		option.WithHTTPClient(server.Client()),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{svc: service}
	reservations := 0

	_, calls, err := client.FetchPlaylistItems(context.Background(), "PL123", 3, func() (func(), error) {
		reservations++
		return func() {}, nil
	})
	if err == nil {
		t.Fatal("FetchPlaylistItems() returned nil error")
	}
	if calls != 2 || requests != 2 || reservations != 2 {
		t.Fatalf("calls = %d, requests = %d, reservations = %d; want all 2", calls, requests, reservations)
	}
}

func TestFetchPlaylistItemsStopsBeforeUnreservedPage(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"items":[],"nextPageToken":"next"}`))
		if err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	service, err := yt.NewService(
		context.Background(),
		option.WithEndpoint(server.URL+"/"),
		option.WithHTTPClient(server.Client()),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{svc: service}

	_, calls, err := client.FetchPlaylistItems(context.Background(), "PL123", 2, func() (func(), error) {
		return func() {}, nil
	})
	if !errors.Is(err, ErrPageLimitReached) {
		t.Fatalf("FetchPlaylistItems() error = %v, want %v", err, ErrPageLimitReached)
	}
	if calls != 2 || requests != 2 {
		t.Fatalf("calls = %d, requests = %d; want both 2", calls, requests)
	}
}
