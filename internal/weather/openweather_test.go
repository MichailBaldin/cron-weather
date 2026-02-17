package weather

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestOpenWeatherFetcher_FetchAlerts(t *testing.T) {
	mockResponse := `{
		"alerts": [
			{
				"sender_name": "TestSender",
				"event": "TestEvent",
				"start": 1771313634,
				"end": 1771394400,
				"description": "Test description",
				"tags": ["tag1", "tag2"]
			}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Проверяем, что в запросе есть параметр appid с нужным значением
		if r.URL.Query().Get("appid") != "testkey" {
			t.Errorf("missing or wrong appid, got %q", r.URL.Query().Get("appid"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	fetcher := NewOpenWeatherFetcher("testkey", time.Second)
	// Подменяем транспорт, чтобы запросы шли на наш сервер
	fetcher.httpClient.Transport = &mockTransport{target: server.URL}

	ctx := context.Background()
	messages, err := fetcher.FetchAlerts(ctx, 55.6073, 38.1684)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	// Вычисляем ожидаемую строку динамически, используя локальное время,
	// как это делает код FetchAlerts.
	startTime := time.Unix(1771313634, 0).Format("02.01.2006 15:04")
	endTime := time.Unix(1771394400, 0).Format("02.01.2006 15:04")
	expected := fmt.Sprintf("[TestSender] TestEvent: Test description (с %s до %s). Теги: [tag1 tag2]",
		startTime, endTime)

	if messages[0] != expected {
		t.Errorf("wrong message:\n got: %q\nwant: %q", messages[0], expected)
	}
}

// mockTransport перенаправляет запросы на заданный URL.
type mockTransport struct {
	target string
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Создаём новый URL на основе target
	newURL, err := url.Parse(m.target)
	if err != nil {
		return nil, err
	}
	// Копируем исходный запрос
	newReq := req.Clone(req.Context())
	newReq.URL.Scheme = newURL.Scheme
	newReq.URL.Host = newURL.Host
	newReq.URL.Path = newURL.Path
	// Сохраняем query параметры из исходного запроса
	newReq.URL.RawQuery = req.URL.RawQuery
	return http.DefaultTransport.RoundTrip(newReq)
}
