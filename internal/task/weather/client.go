// Package weather implements the OpenWeather task and client.
package weather

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Client is a minimal OpenWeather One Call 3.0 client.
// It only knows how to call API and decode JSON.
// Business rules (dedup, urgent codes, messaging) live in the Task.
type Client struct {
	apiKey string
	http   *http.Client

	maxAttempts int
	baseBackoff time.Duration
	maxBackoff  time.Duration
}

// NewOpenWeatherClient constructs an OpenWeather One Call 3.0 API client.
func NewOpenWeatherClient(apiKey string) *Client {
	return &Client{
		apiKey:      apiKey,
		http:        &http.Client{Timeout: 12 * time.Second},
		maxAttempts: 4,
		baseBackoff: 600 * time.Millisecond,
		maxBackoff:  10 * time.Second,
	}
}

// Alert is an OpenWeather weather alert.
type Alert struct {
	SenderName  string   `json:"sender_name"`
	Event       string   `json:"event"`
	Start       int64    `json:"start"`
	End         int64    `json:"end"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type weatherItem struct {
	ID          int    `json:"id"`
	Main        string `json:"main"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

type oneCallResponse struct {
	Alerts  []Alert `json:"alerts"`
	Current struct {
		Weather []weatherItem `json:"weather"`
	} `json:"current"`
}

type apiErrorResponse struct {
	Cod        any      `json:"cod"`
	Message    string   `json:"message"`
	Parameters []string `json:"parameters"`
}

func (e apiErrorResponse) codeInt() int {
	switch v := e.Cod.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}

// OneCall is a minimal decoded payload for task layer.
type OneCall struct {
	Alerts    []Alert
	WeatherID []int
}

// OneCall executes OpenWeather One Call 3.0 request and returns decoded response and raw details.
func (c *Client) OneCall(ctx context.Context, lat, lon float64) (OneCall, int, http.Header, []byte, error) {
	url := fmt.Sprintf(
		"https://api.openweathermap.org/data/3.0/onecall?lat=%f&lon=%f&lang=ru&units=metric&appid=%s",
		lat, lon, c.apiKey,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return OneCall{}, 0, nil, nil, fmt.Errorf("create request: %w", err)
	}

	body, status, hdr, err := c.doWithRetry(ctx, req)
	if err != nil {
		return OneCall{}, 0, nil, nil, err
	}

	// Non-200: still return raw body to task for logging.
	if status != http.StatusOK {
		return OneCall{}, status, hdr, body, nil
	}

	var resp oneCallResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return OneCall{}, status, hdr, body, fmt.Errorf("decode response: %w", err)
	}

	out := OneCall{Alerts: resp.Alerts}
	for _, w := range resp.Current.Weather {
		out.WeatherID = append(out.WeatherID, w.ID)
	}
	return out, status, hdr, body, nil
}

func (c *Client) doWithRetry(ctx context.Context, req *http.Request) ([]byte, int, http.Header, error) {
	attempts := c.maxAttempts
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for i := 1; i <= attempts; i++ {
		if ctx.Err() != nil {
			return nil, 0, nil, ctx.Err()
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			if i == attempts {
				break
			}
			c.sleep(ctx, i, 0)
			continue
		}

		hdr := resp.Header
		status := resp.StatusCode
		b, rerr := readAllLimit(resp.Body, 2<<20)
		_ = resp.Body.Close()
		if rerr != nil {
			lastErr = rerr
			if i == attempts {
				break
			}
			c.sleep(ctx, i, 0)
			continue
		}

		// Decide retry.
		switch {
		case status == http.StatusOK:
			return b, status, hdr, nil
		case status == http.StatusBadRequest || status == http.StatusUnauthorized || status == http.StatusNotFound:
			// do not retry
			return b, status, hdr, nil
		case status == http.StatusTooManyRequests:
			if i == attempts {
				return b, status, hdr, nil
			}
			c.sleep(ctx, i, retryAfter(hdr))
			continue
		case status >= 500 && status <= 599:
			if i == attempts {
				return b, status, hdr, nil
			}
			c.sleep(ctx, i, 0)
			continue
		default:
			return b, status, hdr, nil
		}
	}

	if lastErr != nil {
		return nil, 0, nil, lastErr
	}
	return nil, 0, nil, errors.New("request failed")
}

func (c *Client) sleep(ctx context.Context, attempt int, forced time.Duration) {
	d := forced
	if d <= 0 {
		d = c.baseBackoff * time.Duration(1<<(attempt-1))
		if d > c.maxBackoff {
			d = c.maxBackoff
		}
		d += time.Duration(rand.Intn(250)) * time.Millisecond
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func retryAfter(h http.Header) time.Duration {
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	if tm, err := http.ParseTime(v); err == nil {
		d := time.Until(tm)
		if d < 0 {
			return 0
		}
		return d
	}
	return 0
}

func readAllLimit(r io.Reader, limit int64) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := io.CopyN(&buf, r, limit+1); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if int64(buf.Len()) > limit {
		return buf.Bytes()[:limit], fmt.Errorf("response too large")
	}
	return buf.Bytes(), nil
}

// DecodeAPIError attempts to parse an OpenWeather error response payload.
func DecodeAPIError(body []byte) (apiErrorResponse, bool) {
	var e apiErrorResponse
	if json.Unmarshal(body, &e) == nil && strings.TrimSpace(e.Message) != "" {
		return e, true
	}
	return apiErrorResponse{}, false
}
