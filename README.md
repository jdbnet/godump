# GoDump

GoDump is a lightweight, standalone MariaDB backup application written in Go. It manages multiple MariaDB instances concurrently, automatically discovers databases, runs scheduled backups, enforces retention policies, and presents a sleek, embedded web UI to manage operations.

## Features

- **Multi-Instance Support**: Manage multiple MariaDB servers independently.
- **Auto-Discovery**: Automatically discovers all non-system databases (`information_schema`, `performance_schema`, `mysql`, `sys` are ignored) before backing up.
- **Isolated Backups**: Each database is backed up via `mysqldump` in a dedicated subprocess, compressed instantly with `gzip`, and stored independently. Failure of one database does not disrupt others.
- **Retention Policies**: Configurable retention period (in days) per instance. Old backups are automatically groomed after every run.
- **Embedded Web UI**: Single-page modern interface served directly from the Go binary. No external CDN dependencies, fully functional offline. View statuses, trigger manual backups, browse backup files, and read real-time logs.
- **Cron Scheduling**: Uses standard cron expressions to schedule automated jobs.

## Requirements

The machine running GoDump must have the following installed in its system PATH:
- `mysqldump`
- `gzip`

## Building from Source

Ensure you have Go 1.20+ installed.

```bash
git clone <repository_url>
cd godump
go mod tidy
GOOS=linux GOARCH=amd64 go build -o godump
```

## Configuration

GoDump uses a YAML configuration file. By default, it looks for `/etc/godump/config.yaml`, but you can specify a custom path using the `--config` flag.

### Example `config.yaml`

```yaml
server:
  port: 8080

logging:
  file: ""

instances:
  - name: primary
    host: 192.168.1.10
    port: 3306
    user: backup
    password: secret
    backup_dir: /backups/primary
    retention_days: 14
    schedule: "0 2 * * *"

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
- `logging.file`: The path where log files should be written. 
- `instances`: An array of MariaDB instances. Each requires its own name, connection details, backup directory, retention configuration (in days), and cron `schedule`.

> **Note:** Make sure the user specified in the configuration has `SELECT`, `LOCK TABLES`, `SHOW VIEW`, and `TRIGGER` permissions to properly perform `mysqldump` operations across all databases.

## Usage

1. Create a `config.yaml` using the template above.
2. Run the application:
   ```bash
   ./godump --config /path/to/your/config.yaml
   ```
3. Open a web browser and navigate to `http://<your_server_ip>:<configured_port>`.

From the UI, you can:
- See the overall status of all configured instances.
- Observe discovered databases.
- Click **Run All Now** to manually trigger backups across all servers at once, or use the **Run Now** button on individual cards for targeted runs.
- Monitor real-time progress via the auto-refreshing log console at the bottom of the page.
- Review your backup inventory grouped by instance and database.