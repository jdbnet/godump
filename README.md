<div align="center">
  <img src="web/public/favicon.png" alt="GoDump" width="128" />

  # GoDump

GoDump is a lightweight, standalone MariaDB backup application written in Go. It manages multiple MariaDB instances concurrently, automatically discovers databases, runs scheduled backups, enforces retention policies, and presents a sleek, embedded web UI to manage operations.

</div>

## Features

- **Multi-Instance Support**: Manage multiple MariaDB servers independently.
- **Auto-Discovery**: Automatically discovers all non-system databases (`information_schema`, `performance_schema`, `mysql`, `sys` are ignored) before backing up.
- **Isolated Backups**: Each database is backed up via `mariadb-dump` (or `mysqldump`) in a dedicated subprocess, compressed instantly with `gzip`, and stored independently. Failure of one database does not disrupt others.
- **Retention Policies**: Configurable retention period (in days) per instance. Old backups are automatically groomed after every run.
- **Embedded Web UI**: Single-page modern interface served directly from the Go binary. No external CDN dependencies, fully functional offline. View statuses, trigger manual backups, browse backup files, and read real-time logs.
- **Optional Authentication**: Secure your dashboard and API with a simple, cookie-based session login.
- **Read-only status API**: API keys for `GET /api/v1/health` and `GET /api/v1/backups/status`, so an external dashboard can show backup health without being able to start or change anything.
- **Notifications**: Receive instant alerts when backup jobs complete via HTML Emails (SMTP) or JSON Webhooks (perfect for Ntfy, Gotify, Discord, Slack, Zapier, etc.).
- **Cron Scheduling**: Uses standard cron expressions to schedule automated jobs.

## Requirements

The machine running GoDump must have the following installed in its system PATH:
- `mariadb-dump` (or `mysqldump`)
- `gzip`

## Installation

Add the JDB-NET apt repository, then install the package:

```bash
curl -fsSL https://apt.jdbnet.co.uk/install/stable.sh | sudo bash
sudo apt update
sudo apt install godump
```

The package installs the binary to `/usr/local/bin/godump`, a default config at `/etc/godump/config.yaml`, and a systemd unit. Edit the config, then enable and start the service:

```bash
sudo systemctl enable godump
sudo systemctl start godump
```

Updates are delivered through the apt repository:

```bash
sudo apt update && sudo apt upgrade godump
```

### Docker

Images are published to `ghcr.io/jdbnet/godump` for `linux/amd64` and `linux/arm64`. Bind-mount your config and backup directory:

```yaml
services:
  godump:
    image: ghcr.io/jdbnet/godump:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./config.yaml:/etc/godump/config.yaml:ro
      - ./backups:/backups
```

Point `backup_dir` in `config.yaml` at a path under `/backups` (for example `/backups/primary`). If you use `temp_dir`, mount that too, or set it to a path under `/backups`.

## Configuration

GoDump uses a YAML configuration file. By default, it looks for `/etc/godump/config.yaml`, but you can specify a custom path using the `--config` flag.

### Example `config.yaml`

```yaml
server:
  port: 8080

auth:
  enabled: true
  username: admin
  password: password

notifications:
  events:
    on_success: false
    on_failure: true
  email:
    enabled: true
    # You can override events at the channel level
    # events:
    #   on_success: false
    #   on_failure: true
    host: smtp.example.com
    port: 587
    username: myuser
    password: mypassword
    from: godump@example.com
    to: you@example.com
  webhooks:
    - enabled: true
      url: https://hook.example.com/success
      events:
        on_success: true
        on_failure: false
    - enabled: true
      url: https://hook.example.com/failure
      events:
        on_success: false
        on_failure: true
      headers:
        Authorization: "Bearer your_token_here"

logging:
  file: ""

# api_keys:
#   - name: dashboard
#     hash: "<sha256 hex of the key>"
#     prefix: "gd_0123abcd"
# api_keys_file: /backups/api-keys.yaml

instances:
  - name: primary
    host: 192.168.1.10
    port: 3306
    user: backup
    password: secret
    backup_dir: /backups/primary
    # Optional: write dumps to local disk first, then copy the gzip to backup_dir.
    # Useful when backup_dir is a remote/S3 mount that is slow or unreliable for streaming writes.
    # temp_dir: /tmp/godump
    retention_days: 14
    schedule: "0 2 * * *"
    # Optional: explicitly include or exclude specific databases
    # include:
    #   - my_app_db
    # exclude:
    #   - temp_db

  - name: secondary
    host: 192.168.1.20
    port: 3306
    user: backup
    password: secret
    backup_dir: /backups/secondary
    retention_days: 7
    schedule: "0 3 * * *"
```

- `server.port`: The HTTP port for the web UI.
- `auth`: Optional authentication for the Web UI. `enabled` to turn it on, along with `username` and `password`.
- `api_keys`: Optional read-only keys for the status API. Each entry has a `name` and a `hash` (hex SHA-256 of the full key). The key itself is not stored. `prefix` is optional and is only used to recognise the key in the UI.
- `api_keys_file`: Optional path for keys created in the web UI. Defaults to `api-keys.yaml` next to this file. Use a writable path when the configuration file is mounted read-only.
- `notifications`: Optional post-run notifications. 
  - `events`: Control what triggers notifications globally (`on_success`, `on_failure`).
  - `email`: SMTP details for sending HTML-formatted email alerts. Can have its own `events` block.
  - `webhooks`: An array of webhook endpoints. Each can have its own `events` block to fire only on specific outcomes.
- `logging.file`: The path where log files should be written. 
- `instances`: An array of MariaDB instances. Each requires its own name, connection details, backup directory, retention configuration (in days), and cron `schedule`.
  - `temp_dir`: (Optional) If set, dumps are written here first, then the finished `.sql.gz` is copied to `backup_dir` and the temp file is removed. Use this when `backup_dir` is a remote or S3 mount.
  - `include`: (Optional) If specified, ONLY the listed databases will be backed up.
  - `exclude`: (Optional) If specified, the listed databases will be ignored. System databases are ALWAYS excluded automatically.

> **Note:** Make sure the user specified in the configuration has `SELECT`, `LOCK TABLES`, `SHOW VIEW`, and `TRIGGER` permissions to properly perform dumps across all databases.

## Status API

The status API is read-only and always requires an API key, including when dashboard login is turned off. Existing UI routes keep using the session cookie (or no login, when `auth.enabled` is false). An API key cannot start backups, delete files, download backups, or manage keys.

Send the key as `Authorization: Bearer <key>` or `X-API-Key: <key>`. A missing or invalid key returns `401` and `{"error":"unauthorised"}`.

A job is one discovered database on a configured MariaDB instance. The stable `id` is `<instance>/<database>`. If an instance has not discovered any databases, it is reported as one job so a connection failure is still visible. `target` is `<host>:<port>/<database>` (or `<host>:<port>` for that instance-level job). `last_size_bytes` is the size of that database's latest backup file when GoDump knows it. Times the app does not know are JSON `null`.

`stale` is true when the last successful backup is older than twice the gap between the next two scheduled runs, or older than 48 hours when the instance has no schedule. `overall` is `failing` if any enabled job's last run failed, `warning` if any enabled job is stale, partial, or has never run, `ok` when every enabled job is healthy, and `unknown` when there are no jobs.

### `GET /api/v1/health`

```json
{"app":"godump","version":"1.2.3","status":"ok"}
```

### `GET /api/v1/backups/status`

```json
{
  "app": "godump",
  "version": "1.2.3",
  "generated_at": "2026-06-02T12:00:00Z",
  "overall": "ok",
  "jobs": [
    {
      "id": "primary/app",
      "name": "app",
      "target": "192.168.1.10:3306/app",
      "enabled": true,
      "last_run_at": "2026-06-02T02:00:04Z",
      "last_status": "success",
      "last_success_at": "2026-06-02T02:00:04Z",
      "last_duration_seconds": 12,
      "last_size_bytes": 1048576,
      "last_error": null,
      "next_run_at": "2026-06-03T02:00:00Z",
      "stale": false
    }
  ]
}
```

`last_status` is `success`, `failed`, `running`, `partial`, or `never_run`.

### Creating a key

In the web UI, open **API keys**, enter a name, and choose **Create key**. Copy the key immediately. GoDump stores only its SHA-256 hash and will not show the key again. Revoking a key stops it working. Keys declared in the configuration file are listed there, and are removed by editing the configuration and restarting.

To provision a key in the configuration instead:

```bash
key="gd_$(openssl rand -hex 32)"
printf '%s\n' "$key"
printf '%s' "$key" | sha256sum
```

Put the hash in `config.yaml`, then restart GoDump. Keep the printed key somewhere safe. It is not written to the configuration.

```yaml
api_keys:
  - name: dashboard
    hash: "<sha256 hex from the command above>"
    prefix: "gd_0123abcd"
```

```bash
curl -s -H "Authorization: Bearer gd_..." http://127.0.0.1:8080/api/v1/health
curl -s -H "X-API-Key: gd_..." http://127.0.0.1:8080/api/v1/backups/status
```

## Usage

After editing `/etc/godump/config.yaml`, restart the service:

```bash
sudo systemctl restart godump
```

Open a web browser and navigate to `http://<your_server_ip>:<configured_port>`.

### Development

Build the embedded web UI, then run the Go server:

```bash
cd web && npm install && npm run build && cd ..
go run . --config config.yaml
```

For frontend development with hot reload, run the API and Vite dev server in separate terminals:

```bash
# Terminal 1 - API
go run . --config config.yaml

# Terminal 2 - Vite (proxies /api to the Go server)
cd web && npm run dev
```

Check the installed version:

```bash
godump --version
```

From the UI, you can:
- View an overview dashboard with instance and backup totals.
- Manage MariaDB instances, schedules, and per-database status.
- Browse backup files and download or delete them.
- Monitor recent logs with auto-refresh.
- Toggle light or dark theme from the sidebar.
