package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Alert struct {
	SenderName  string   `json:"sender_name"`
	Event       string   `json:"event"`
	Start       int64    `json:"start"`
	End         int64    `json:"end"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type oneCallResponse struct {
	Alerts []Alert `json:"alerts"`
}

type OpenWeatherFetcher struct {
	apiKey     string
	httpClient *http.Client
}

func NewOpenWeatherFetcher(apiKey string, timeout time.Duration) *OpenWeatherFetcher {
	return &OpenWeatherFetcher{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (f *OpenWeatherFetcher) FetchAlerts(ctx context.Context, lat, lon float64) ([]string, error) {
	url := fmt.Sprintf(
		"https://api.openweathermap.org/data/3.0/onecall?lat=%f&lon=%f&lang=ru&units=metric&appid=%s",
		lat, lon, f.apiKey,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("request cancelled: %w", ctx.Err())
		default:
			return nil, fmt.Errorf("do request: %w", err)
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned non-200 status: %d", resp.StatusCode)
	}

	var apiResp oneCallResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	messages := make([]string, 0, len(apiResp.Alerts))
	for _, a := range apiResp.Alerts {
		msg := fmt.Sprintf("[%s] %s: %s (с %s до %s). Теги: %v",
			a.SenderName,
			a.Event,
			a.Description,
			time.Unix(a.Start, 0).Format("02.01.2006 15:04"),
			time.Unix(a.End, 0).Format("02.01.2006 15:04"),
			a.Tags,
		)
		messages = append(messages, msg)
	}
	return messages, nil
}

