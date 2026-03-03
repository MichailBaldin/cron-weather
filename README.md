# cron-weather

`cron-weather` is a lightweight Go service that runs persistent cron schedules, fetches weather alerts from the OpenWeather **One Call 3.0** API, and delivers notifications to Telegram chats.

Key properties:

- Telegram-managed schedules (create/list/stop)
- PostgreSQL persistence (subscriptions, endpoints, schedules, runs)
- Schedules survive restarts (active schedules are bootstrapped on startup)
- Alert deduplication across restarts (fingerprint-based)
- Hard daily API request cap per subscription (persisted counter)

---

## Tech stack

- Go
- PostgreSQL
- Telegram Bot API
- OpenWeather One Call API 3.0
- `github.com/robfig/cron/v3`
- Docker / Docker Compose

---

## Architecture overview

At a high level the service is split into two parts:

- **Scheduler runtime** (`internal/scheduler`): owns *when* jobs run (robfig/cron), registers/bootstraps schedules, records runs.
- **Task runners** (`internal/task/...`): own *what* happens on each run (weather fetch, formatting, dedup, etc).

A schedule has a `kind` field (e.g. `weather`). The runtime engine routes each run to a matching task runner. Replacing the API or adding new types of work is done by adding a new runner and registering it by `kind`, without rewriting the scheduler.

---

## Database schema (PostgreSQL)

Migrations are located in `internal/storage/postgres/migrations`.

Core tables:

- `subscriptions` — one subscription per Telegram chat (`owner_ref` is chat ID as string), plus per-subscription coordinates (`lat`, `lon`).
- `endpoints` — delivery targets (currently only `telegram`).
- `subscription_endpoints` — links a subscription to its endpoint(s).
- `schedules` — persisted cron schedules (`expr`, `kind`, `starts_at`, `ends_at`, `active`, `next_run_at`).
- `runs` — execution history for observability/debugging.

Weather-specific tables:

- `daily_usage` — persisted per-day request counter (guarantees the daily limit across restarts).
- `sent_alerts` — per-subscription alert fingerprints to prevent duplicate deliveries.

---

## Telegram commands

The bot controls subscriptions and schedules.

### Subscription

Enable a subscription for the current chat:

```
/start_scheduler
```

Disable the subscription (and stop all schedules for the chat):

```
/stop_scheduler
```

Set coordinates for the subscription (required for weather task):

```
/set_location <lat> <lon>
```

### Schedules

Create a schedule:

```
/start <cron expr> <start_at> <end_at>
```

- `start_at` / `end_at` are RFC3339 timestamps or `-` (meaning “unset”).
- If `start_at` is `-`, the schedule starts immediately.
- If `end_at` is `-`, the schedule runs indefinitely.

List active schedules for the chat:

```
/list_scheduler
```

Stop a schedule by ID:

```
/stop <schedule_id>
```

---

## Cron expressions

The runtime uses `cron.WithSeconds()`, so expressions support seconds (6 fields):

```
second minute hour day month weekday
```

Examples:

- Every 10 seconds:

```
*/10 * * * * *
```

- Every 30 seconds:

```
*/30 * * * * *
```

To start immediately and run forever, use `- -` for the time window:

```
/start */10 * * * * * - -
```

---

## Weather task behavior

On each run, the `weather` task:

1. Reserves one request from the daily limit (`daily_usage`).
2. Calls OpenWeather One Call 3.0 API for the subscription coordinates.
3. Extracts alerts and formats them for Telegram.
4. Deduplicates each alert using a SHA256 fingerprint stored in `sent_alerts`.
5. Checks `current.weather[].id` for urgent codes and, if present, appends the phrase:
   - `позвони срочно родителям`
   and logs the matched codes.

### Retry policy

- `400`, `401`, `404` — **no retry**
- `429` — retry using `Retry-After` (if present), otherwise backoff
- `5xx` — retry with backoff

---

## Configuration

Configuration is loaded from environment variables.

Required:

- `TG_BOT_TOKEN` — Telegram bot token
- `OWM_API_KEY` — OpenWeather API key
- `PG_*` — PostgreSQL connection settings (see `.env` and `docker-compose.yml`)

Optional:

- `OWM_DAILY_LIMIT` — daily request cap per subscription (default: `1000`)

---

## Running with Docker

Create/update `.env` (see the repo `example.env` for an example), then:
Example .env

```txt
ENV=<local|prod>

TG_BOT_TOKEN=<your telegram bot token>

PG_HOST=postgres
PG_PORT=5432
PG_USER=<your postgres user>
PG_PASSWORD=<your user postgres password>
PG_DB=<your postgres database name>
PG_SSLMODE=disable

OWM_API_KEY=<your api key for open weather api>

#IANA Time Zone Database
TZ=Europe/Vilnius
```


```bash
make up
```

Follow logs:

```bash
make logs
```

Restart only the app container:

```bash
make restart
```

Open a shell inside the app container:

```bash
make app-shell
```

Open a `psql` shell:

```bash
make db-shell
```

Full wipe (removes containers **and volumes**):

```bash
make db-reset
```

---

## Adding a new scheduled task kind

1. Create a new package under `internal/task/<kind>` implementing `task.Runner`.
2. Register the runner in application wiring (where runners map is built) under key `<kind>`.
3. Create schedules with `kind=<kind>` (currently the bot creates `weather` schedules).

This keeps the separation explicit: the scheduler engine stays unchanged.

---

## Logging

In production, the service logs:

- schedule creation / stop
- runtime schedule registration (important for restarts)
- every run start + finish (status and duration)

Telegram debug output is opt-in via `TG_DEBUG=true` to avoid noisy JSON logs.
