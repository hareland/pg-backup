# pg-backup

A lightweight PostgreSQL backup scheduler that **automagically** creates database dumps and stores them in S3-compatible storage (AWS S3, MinIO, Cloudflare R2, etc.).

## ✨ Features

- **Cron Scheduling** – define when backups run using familiar cron expressions
- **S3-Compatible Storage** – works with AWS S3, MinIO, Cloudflare R2, and others
- **Custom Dump Format** – uses `pg_dump -Fc` for compressed, efficient backups and restores
- **Retention Policy** – optional `maxHistory` to keep only the latest *N* backups per database
- **Environment Variable Substitution** – `${VAR}` placeholders expand from environment variables
- **Docker Ready** – run as a container with a simple YAML config
- **Multiple Databases** – back up many databases to different destinations with one config

---

## 🚀 Quick Start

### 1. Create `config.yaml`

```yaml
destinations:
s3:
bucket: my-backup-bucket
prefix: postgres-backups
endpoint: https://s3.amazonaws.com
accessKey: ${AWS_ACCESS_KEY_ID}
secretKey: ${AWS_SECRET_ACCESS_KEY}
region: us-east-1

backups:
- url: postgres://postgres:password@localhost:5432/mydb
  destination: s3
  schedule: "0 2 * * *"   # Daily at 2 AM
  maxHistory: 7           # Keep last 7 backups
  ```

### 2. Docker Compose

```yaml
services:
pg-backup:
image: ghcr.io/hareland/pg-backup:latest
volumes:
- ./config.yaml:/config.yaml:ro
environment:
AWS_ACCESS_KEY_ID: your-access-key
AWS_SECRET_ACCESS_KEY: your-secret-key
TZ: Europe/Copenhagen
restart: unless-stopped
```

```bash
docker-compose up -d
```

### 3. Docker Run

```bash
docker run -d \
--name pg-backup \
-v $(pwd)/config.yaml:/config.yaml:ro \
-e AWS_ACCESS_KEY_ID=your-access-key \
-e AWS_SECRET_ACCESS_KEY=your-secret-key \
-e TZ=Europe/Copenhagen \
--restart unless-stopped \
ghcr.io/hareland/pg-backup:latest
```

---

## ⚙️ Configuration

- Default config file: `/config.yaml`
- Override with: `CONFIG_FILE=/path/to/config.yaml`

### Schema

```yaml
destinations:
name:
bucket: string
prefix: string        # optional
endpoint: string      # optional for AWS
accessKey: string
secretKey: string
region: string

backups:
- url: string
  destination: string   # reference to a destination
  schedule: string      # cron expression
  maxHistory: int       # keep latest N backups (optional)
  ```

---

## 🔑 Example Configs

### AWS S3

```yaml
destinations:
aws:
bucket: my-backup-bucket
prefix: database-backups
region: us-east-1
accessKey: ${AWS_ACCESS_KEY_ID}
secretKey: ${AWS_SECRET_ACCESS_KEY}

backups:
- url: postgres://user:pass@db.example.com:5432/production
  destination: aws
  schedule: "0 3 * * *"  # Daily at 3 AM
  maxHistory: 14
  ```

### MinIO

```yaml
destinations:
minio:
bucket: backups
prefix: postgres
endpoint: http://minio:9000
accessKey: minio
secretKey: minio123
region: us-east-1

backups:
- url: postgres://postgres:postgres@postgres:5432/app
  destination: minio
  schedule: "0 */6 * * *"  # Every 6 hours
  maxHistory: 10
  ```

### Cloudflare R2

```yaml
destinations:
r2:
bucket: my-r2-bucket
prefix: db-backups
endpoint: https://your-account-id.r2.cloudflarestorage.com
accessKey: ${R2_ACCESS_KEY}
secretKey: ${R2_SECRET_KEY}
region: auto

backups:
- url: postgres://postgres:password@db:5432/myapp
  destination: r2
  schedule: "0 1 * * 0"  # Weekly on Sunday at 1 AM
  maxHistory: 7
  ```

---

## 🌍 Environment Variables

### Core

- `CONFIG_FILE` – path to config file (default: `/config.yaml`)
- `TZ` – timezone for cron schedule (e.g. `Europe/Copenhagen`)

### AWS/S3 Fallbacks

- `AWS_ACCESS_KEY_ID`
- `AWS_SECRET_ACCESS_KEY`
- `AWS_DEFAULT_REGION`
- `AWS_ENDPOINT_URL`

### Substitution Example

```yaml
destinations:
prod:
bucket: ${BACKUP_BUCKET}
accessKey: ${AWS_ACCESS_KEY_ID}
secretKey: ${AWS_SECRET_ACCESS_KEY}
```

---

## ⏰ Cron Syntax

Supports:

- 5-field (`minute hour day month weekday`)
- 6-field (with seconds)
- Descriptive (`@daily`, `@weekly`, etc.)

```yaml
schedule: "*/30 * * * *"   # every 30 min
schedule: "30 2 * * *"     # daily at 2:30
schedule: "15 3 * * 0"     # Sundays 3:15
schedule: "@daily"
```

---

## 📦 Backup File Format

```
s3://bucket/prefix/database/pgdump-YYYYMMDDTHHMMSSZ.dump
```

Example:

```
s3://my-backups/postgres/myapp/pgdump-20231225T030000Z.dump
```

---

## 🔄 Restore

You can restore any backup using `aws s3 cp` (or your S3-compatible CLI) together with `pg_restore`.

### Steps

1. Copy the backup file locally:

```bash
aws s3 cp s3://my-backup-bucket/postgres/mydb/pgdump-20231225T030000Z.dump ./backup.dump
```

2. Restore into a database (must exist beforehand):

```bash
createdb -h localhost -U postgres mydb_restored
pg_restore -h localhost -U postgres -d mydb_restored ./backup.dump
```

3. Or overwrite an existing database (careful: destructive):

```bash
pg_restore -h localhost -U postgres -d mydb --clean --if-exists ./backup.dump
```

---

## 🐳 Docker Image

```
ghcr.io/hareland/pg-backup:latest
```

- Alpine base, PostgreSQL client + AWS CLI
- Multi-arch (amd64, arm64)
- Optimized, minimal footprint

---

## 🧪 Development & Testing

Local testing with MinIO:

```bash
docker-compose -f compose.test.yaml up -d
docker-compose -f compose.test.yaml logs -f pg-backup
docker-compose -f compose.test.yaml down -v
```

Build from source:

```bash
git clone https://github.com/hareland/pg-backup.git
cd pg-backup
docker build -t pg-backup ./backup-runner

# Or with Go 1.21+
cd backup-runner
go build -o pg-backup .
```

---

## 📊 Logs

```
[backup] start postgres://user:***@host:5432/db
[backup] uploaded s3://bucket/prefix/db/pgdump-20231225T030000Z.dump
[prune] deleting 3 old backups under s3://bucket/prefix/db/
```

---

## 🛠 Troubleshooting

- **Timeouts** – increase `PGCONNECT_TIMEOUT`
- **Permission denied** – check DB user rights
- **Upload fails** – verify credentials/bucket/endpoint
- **Cron not firing** – check timezone + syntax

Debug tips:

```bash
docker run --rm ghcr.io/hareland/pg-backup:latest pg_dump --version
docker run --rm -v $(pwd)/config.yaml:/config.yaml:ro ghcr.io/hareland/pg-backup:latest cat /config.yaml
docker logs pg-backup
```

---

## 🤝 Contributing

Issues, PRs, and feature requests welcome!

---

## 📄 License

MIT
