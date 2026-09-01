package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestCompressParsesBusinessEnvelopeBeforeStatus(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", req.Method)
		}
		if req.Header.Get("Idempotency-Key") != "tenant-a:asset-7" {
			t.Fatal("missing idempotency key")
		}
		if req.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", req.Header.Get("Content-Type"))
		}
		var body map[string]any
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		image, ok := body["image"].(map[string]any)
		if !ok || image["base64"] != base64.StdEncoding.EncodeToString([]byte("image")) {
			t.Fatalf("image = %#v, want base64 image ref", body["image"])
		}
		if body["idempotency_key"] != "tenant-a:asset-7" {
			t.Fatalf("idempotency_key = %q", body["idempotency_key"])
		}
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"ok":false,"data":null,"error":{"message":"request rejected"},"metadata":{}}`)),
		}, nil
	})}
	compressor := NewCompressor("test-key", client)
	compressor.sleep = func(context.Context, time.Duration) error { return nil }

	_, err := compressor.Compress(context.Background(), []byte("image"), "asset.jpg", "tenant-a:asset-7")
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Message != "request rejected" || apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("Compress() error = %#v", err)
	}
}
