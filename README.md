# cron-weather

A lightweight Go service that periodically fetches weather alerts from OpenWeather and sends them to Telegram chats.

The service supports multiple chat subscriptions stored in SQLite and prevents duplicate alert delivery across restarts.

---

## Features

* Fetches alerts from **OpenWeather One Call API 3.0**
* Sends alerts to **Telegram**

  * HTML-safe formatting
  * Automatic message splitting (Telegram limit safe)
* Supports multiple chat subscriptions
* Subscriptions are stored in **SQLite**
* Alert deduplication persisted in SQLite
* Cron-style scheduler per subscription
* Graceful shutdown
* Configurable timezone
* Configurable daily API limit

---

## Architecture Overview

Each subscription:

* Has its own interval and optional `start_at`
* Runs as a scheduled job
* Fetches alerts
* Filters already-sent alerts
* Sends only new alerts

Data stored in SQLite:

* `subscriptions` — chat configuration
* `sent_alerts` — delivered alert fingerprints (for dedup)

---

## Requirements

* Go 1.25+
* Docker + Docker Compose (recommended)
* OpenWeather API key
* Telegram Bot Token

---

# Quick Start (Docker — Recommended)

### Create `.env`

```env
# local - DEBUG ON
# prod - ONLY INFO, WARN, ERROR
ENV=prod

# Scheduler
INTERVAL=30s
START_AT=
TIMEZONE=UTC
DAILY_LIMIT=1000

# OpenWeather
WEATHER_API_KEY=your_openweather_api_key
WEATHER_LAT=55.60732906526543
WEATHER_LON=38.168394018791
WEATHER_HTTP_TIMEOUT=10s

# Telegram
TG_TOKEN=your_telegram_bot_token
TG_CHAT_ID=123456789

# SQLite
DB_PATH=/data/subscriptions.db

# Optional default subscription (only used if DB is empty)
DEFAULT_SUB_CHAT_ID=
DEFAULT_SUB_INTERVAL=
DEFAULT_SUB_START_AT=
DEFAULT_SUB_LAT=
DEFAULT_SUB_LON=
```

---

### Run

```bash
docker compose up --build -d
```

---

### View logs

```bash
docker compose logs -f
```

---

## Persistence

SQLite database is stored in:

```
./data/subscriptions.db
```

Docker volume:

```yaml
volumes:
  - ./data:/data
```

Your subscriptions and dedup state survive container restarts.

---

# Run Locally (Without Docker)

### Install dependencies

```bash
go mod download
```

### Create `.env`

Same as above.

### Run

```bash
make run
```

### Tests

```bash
make test
```

---

# Configuration Details

## Scheduler

* `INTERVAL` — default interval (e.g. `30s`, `5m`)
* `START_AT` — first execution time in `HH:MM`
* `TIMEZONE` — used for:

  * interpreting `START_AT`
  * daily limiter reset

Example:

```
TIMEZONE=Europe/Vilnius
START_AT=09:00
```

---

## OpenWeather

* `WEATHER_API_KEY` — required
* `WEATHER_LAT`, `WEATHER_LON` — default coordinates
* `WEATHER_HTTP_TIMEOUT` — request timeout
* `DAILY_LIMIT` — approximate daily request cap

---

## Telegram

* `TG_TOKEN` — bot token
* `TG_CHAT_ID` — used for default subscription only

Telegram specifics:

* Messages are HTML-escaped
* Messages are automatically split if too long
* Duplicate alerts are not re-sent

---

# Subscriptions

Subscriptions are stored in SQLite table:

```sql
subscriptions (
  chat_id INTEGER PRIMARY KEY,
  interval INTEGER,
  start_at TEXT,
  lat REAL,
  lon REAL
)
```

Sent alerts dedup table:

```sql
sent_alerts (
  chat_id INTEGER,
  fingerprint TEXT,
  sent_at INTEGER,
  PRIMARY KEY(chat_id, fingerprint)
)
```

Currently subscriptions must be:

* seeded via environment (default subscription)
* or manually inserted into SQLite

Dynamic Telegram commands are not implemented yet.

---

# How Scheduling Works

Each subscription:

1. Scheduler waits for optional `start_at`
2. Runs job every `interval`
3. Fetches alerts
4. Filters already sent alerts (SQLite-based dedup)
5. Sends only new alerts
6. Marks them as sent

Overlapping runs are prevented.

---

# Graceful Shutdown

On `SIGINT` / `SIGTERM`:

* Stops accepting new runs
* Waits for active jobs
* Closes SQLite connection

---

# Troubleshooting

### No Telegram messages

* Check bot token
* Make sure bot has permission in the chat
* Verify `TG_CHAT_ID`

---

### Alerts are never sent again

Likely dedup is working correctly.
Delete rows from `sent_alerts` table to reset state.

---

### START_AT behaves unexpectedly

Ensure correct timezone:

```
TIMEZONE=Europe/Moscow
```

---

# Current Limitations

* No dynamic subscription management
* No HTTP API
* No metrics endpoint
* No distributed scheduling (single instance design)


