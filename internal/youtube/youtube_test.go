package youtube

import (
	"errors"
	"net/http"
	"testing"
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
	base := roundTripFunc(func(got *http.Request) (*http.Response, error) {
		if value := got.Header.Get("X-Goog-Api-Key"); value != "secret-key" {
			t.Fatalf("X-Goog-Api-Key = %q, want secret-key", value)
		}
		if value := got.URL.Query().Get("key"); value != "" {
			t.Fatalf("URL key = %q, want empty", value)
		}
		return nil, errors.New("stop")
	})
	transport := &apiKeyTransport{apiKey: "secret-key", base: base}

	_, _ = transport.RoundTrip(request)
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
