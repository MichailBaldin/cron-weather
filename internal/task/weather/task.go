package weather

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"cron-weather/internal/storage"
	"cron-weather/internal/task"
)

// Task calls OpenWeather One Call API and produces messages.
// It is intentionally small and depends only on:
//   - storage.Repo (for quota + dedup)
//   - Client (for API call)
//
// This makes replacing API/task logic easy.
type Task struct {
	log        *slog.Logger
	repo       storage.Repo
	client     *Client
	dailyLimit int

	urgentCodes map[int]struct{}
}

// NewTask constructs a weather task runner.
func NewTask(log *slog.Logger, repo storage.Repo, client *Client, dailyLimit int) *Task {
	if log == nil {
		log = slog.Default()
	}
	t := &Task{
		log:         log,
		repo:        repo,
		client:      client,
		dailyLimit:  dailyLimit,
		urgentCodes: map[int]struct{}{},
	}
	for _, c := range []int{202, 212, 221, 232, 314, 504, 511, 522, 531, 602, 622, 761, 762, 771, 781} {
		t.urgentCodes[c] = struct{}{}
	}
	return t
}

// Run executes one weather check iteration and returns user-facing messages.
func (t *Task) Run(ctx context.Context, in task.Input) (task.Result, error) {
	// Ensure coordinates exist.
	if in.Subscription.Lat == 0 && in.Subscription.Lon == 0 {
		return task.Result{}, fmt.Errorf("subscription has no location; set it via /set_location <lat> <lon>")
	}

	if t.repo != nil {
		ok, used, err := t.repo.ReserveDailyUsage(ctx, in.Subscription.ID, time.Now(), t.dailyLimit)
		if err != nil {
			return task.Result{}, err
		}
		if !ok {
			return task.Result{}, fmt.Errorf("daily limit exceeded (%d/%d)", used, t.dailyLimit)
		}
	}

	oc, status, hdr, raw, err := t.client.OneCall(ctx, in.Subscription.Lat, in.Subscription.Lon)
	if err != nil {
		return task.Result{}, err
	}

	if status != 200 {
		if apiErr, ok := DecodeAPIError(raw); ok {
			return task.Result{}, fmt.Errorf("openweather error: http=%d cod=%d message=%q parameters=%v", status, apiErr.codeInt(), apiErr.Message, apiErr.Parameters)
		}
		preview := string(raw)
		if len(preview) > 300 {
			preview = preview[:300] + "..."
		}
		return task.Result{}, fmt.Errorf("openweather error: http=%d body=%q", status, preview)
	}

	// Alerts -> messages with dedup.
	var msgs []string
	for _, a := range oc.Alerts {
		fp := alertFingerprint(a)
		send := true
		if t.repo != nil {
			inserted, derr := t.repo.MarkAlertSent(ctx, in.Subscription.ID, fp)
			if derr != nil {
				return task.Result{}, derr
			}
			send = inserted
		}
		if !send {
			continue
		}

		msg := fmt.Sprintf("[%s] %s: %s (с %s до %s). Теги: %v",
			a.SenderName,
			a.Event,
			strings.TrimSpace(a.Description),
			time.Unix(a.Start, 0).Format("02.01.2006 15:04"),
			time.Unix(a.End, 0).Format("02.01.2006 15:04"),
			a.Tags,
		)
		msgs = append(msgs, msg)
	}

	// Urgent weather codes.
	urgentIDs := make([]int, 0, 2)
	for _, id := range oc.WeatherID {
		if _, ok := t.urgentCodes[id]; ok {
			urgentIDs = append(urgentIDs, id)
		}
	}
	if len(urgentIDs) > 0 {
		msgs = append(msgs, "позвони срочно родителям")
		t.log.Warn("openweather urgent weather code",
			slog.Any("ids", urgentIDs),
			slog.String("x_request_id", hdr.Get("X-Request-Id")),
			slog.String("subscription_id", in.Subscription.ID),
		)
	}

	payload := ""
	if len(urgentIDs) > 0 {
		b, _ := json.Marshal(map[string]any{"urgent_weather_ids": urgentIDs})
		payload = string(b)
	}

	return task.Result{Messages: msgs, Payload: payload}, nil
}

func alertFingerprint(a Alert) string {
	// Stable fingerprint: sha256(json(sender,event,start,end,description,tags))
	b, _ := json.Marshal(struct {
		SenderName  string   `json:"sender_name"`
		Event       string   `json:"event"`
		Start       int64    `json:"start"`
		End         int64    `json:"end"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}{
		SenderName:  a.SenderName,
		Event:       a.Event,
		Start:       a.Start,
		End:         a.End,
		Description: strings.TrimSpace(a.Description),
		Tags:        a.Tags,
	})

	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
