# cron-weather

cron-weather is a lightweight Go service that periodically fetches weather alerts
from the OpenWeather One Call API and delivers them to Telegram chats.

It supports multiple chat subscriptions stored in SQLite and prevents duplicate
alert delivery across restarts.

---

## Project Purpose

The goal of this project is to provide a simple, reliable background service
that monitors weather alerts and sends notifications to configured Telegram chats.

It is designed as:

- A small standalone service
- Docker-friendly
- Persistent between restarts
- Safe against duplicate alert delivery
- Easy to configure via environment variables

---

## Tech Stack

- **Go** 1.25+
- **SQLite** (via mattn/go-sqlite3)
- **Telegram Bot API**
- **OpenWeather One Call API 3.0**
- **Docker / Docker Compose**

---

## Project Status

Current version: **v0.1.0**

Status:  
✔ Core functionality implemented  
✔ Persistent subscriptions  
✔ Persistent alert deduplication  
✔ Graceful shutdown  
✔ Basic test coverage  

Not implemented yet:
- Dynamic subscription management (via Telegram commands)
- HTTP API
- Metrics endpoint
- Horizontal scaling

This is currently a single-instance background service.

---

## Architecture Overview

For each subscription:

1. A cron service runs on a defined interval.
2. Weather alerts are fetched.
3. Already-sent alerts are filtered (SQLite-based dedup).
4. Only new alerts are sent.
5. Sent alerts are recorded to prevent duplicates.

SQLite tables:

### subscriptions

| Field | Description |
|--------|------------|
| chat_id | Telegram chat ID |
| interval | Run interval (nanoseconds) |
| start_at | Optional start time (HH:MM) |
| lat | Latitude |
| lon | Longitude |

### sent_alerts

| Field | Description |
|--------|------------|
| chat_id | Telegram chat ID |
| fingerprint | SHA256 of alert message |
| sent_at | Unix timestamp |

---

## How to Install

### Option 1 — Docker (Recommended)

1. Clone the repository:

```bash
git clone <repo_url>
cd cron-weather
````

2. Create `.env` file:

- `ENV=local` → debug level, human-readable console logs.
- `ENV=prod` → info level, structured JSON logs


```env
ENV=prod

INTERVAL=30s
START_AT=
TIMEZONE=UTC
DAILY_LIMIT=1000

WEATHER_API_KEY=your_openweather_api_key
WEATHER_LAT=your_latitude
WEATHER_LON=your_longitude
WEATHER_HTTP_TIMEOUT=10s

TG_TOKEN=your_telegram_bot_token
TG_CHAT_ID=your_chat_id

DB_PATH=/data/subscriptions.db
```

3. Run:

```bash
docker compose up --build -d
```

4. View logs:

```bash
docker compose logs -f
```

---

### Option 2 — Local Run

Requirements:

* Go 1.25+

Run:

```bash
make run
```

Run tests:

```bash
make test
```

---

## Configuration

All configuration is environment-based.

### Core

| Variable    | Description                       |
| ----------- | --------------------------------- |
| ENV         | Environment name                  |
| INTERVAL    | Default scheduler interval        |
| START_AT    | Optional first run time (HH:MM)   |
| TIMEZONE    | Timezone used for scheduling      |
| DAILY_LIMIT | Approximate daily API request cap |
| DB_PATH     | SQLite database path              |

---

### OpenWeather

| Variable             | Description       |
| -------------------- | ----------------- |
| WEATHER_API_KEY      | Required API key  |
| WEATHER_LAT          | Default latitude  |
| WEATHER_LON          | Default longitude |
| WEATHER_HTTP_TIMEOUT | HTTP timeout      |

---

### Telegram

| Variable   | Description                   |
| ---------- | ----------------------------- |
| TG_TOKEN   | Telegram bot token            |
| TG_CHAT_ID | Used for default subscription |

---

## Tests

Run all tests:

```bash
make test
```

Tests cover:

* Scheduler behavior
* Alert fetching
* Telegram sender logic
* SQLite repository
* Deduplication logic

---

## Graceful Shutdown

On SIGINT or SIGTERM:

* Stops accepting new jobs
* Waits for active jobs
* Closes SQLite connection

---

## Environments

Supported environments:

* Local development
* Docker container
* Production (single instance)

---

## Limitations

* No distributed locking
* No API layer
* Manual subscription management only
* No rate limit auto-detection from OpenWeather

