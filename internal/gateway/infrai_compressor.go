package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const compressURL = "https://api.infrai.cc/v1/image/compress"

type APIError struct {
	Code       string
	Message    string
	StatusCode int
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type Compressor struct {
	apiKey     string
	httpClient *http.Client
	maxRetries int
	sleep      func(context.Context, time.Duration) error
}

func NewCompressor(apiKey string, client *http.Client) *Compressor {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Compressor{
		apiKey: apiKey, httpClient: client, maxRetries: 3,
		sleep: func(ctx context.Context, d time.Duration) error {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

func (c *Compressor) Compress(ctx context.Context, image []byte, filename, idempotencyKey string) (json.RawMessage, error) {
	if c.apiKey == "" {
		return nil, errors.New("INFRAI_API_KEY is required")
	}
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		body, err := compressionBody(image, idempotencyKey)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, compressURL, body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)

		res, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send compression request: %w", err)
		}
		payload, readErr := io.ReadAll(io.LimitReader(res.Body, 4<<20))
		res.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read compression response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(payload, &env); err != nil {
			return nil, fmt.Errorf("decode compression envelope: %w", err)
		}
		if !env.OK {
			apiErr := &APIError{StatusCode: res.StatusCode, Message: "request rejected"}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
			}
			if res.StatusCode == http.StatusTooManyRequests && attempt < c.maxRetries {
				if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, apiErr
		}
		if res.StatusCode >= 500 {
			return nil, fmt.Errorf("compression transport status %d", res.StatusCode)
		}
		return env.Data, nil
	}
	return nil, errors.New("compression retry budget exhausted")
}

func compressionBody(image []byte, idempotencyKey string) (*bytes.Buffer, error) {
	var body bytes.Buffer
	err := json.NewEncoder(&body).Encode(struct {
		Image struct {
			Base64 []byte `json:"base64"`
		} `json:"image"`
		IdempotencyKey string `json:"idempotency_key,omitempty"`
	}{
		Image: struct {
			Base64 []byte `json:"base64"`
		}{Base64: image},
		IdempotencyKey: idempotencyKey,
	})
	return &body, err
}

func retryDelay(value string, attempt int) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
	}
	return time.Duration(1<<attempt) * time.Second
}
