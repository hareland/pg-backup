# pg-backup

A lightweight PostgreSQL backup scheduler that automatically creates database backups and uploads them to S3-compatible storage (AWS S3, MinIO, Cloudflare R2, etc.).

## Features

- **Scheduled Backups**: Use cron expressions to schedule automatic backups
- **S3-Compatible Storage**: Supports AWS S3, MinIO, Cloudflare R2, and other S3-compatible services
- **Custom Format**: Creates PostgreSQL custom format dumps (`pg_dump -Fc`) for efficient compression and restore
- **Environment Variable Support**: Flexible configuration with environment variable substitution
- **Docker Ready**: Available as a pre-built Docker image
- **Multiple Databases**: Support for backing up multiple databases to different destinations

## Quick Start

### Using Docker Compose

Create a `config.yaml` file:

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
    schedule: "0 2 * * *"  # Daily at 2 AM
```

Create a `docker-compose.yml` file:

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

Start the service:

```bash
docker-compose up -d
```

### Using Docker Run

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

## Configuration

The application is configured using a YAML file (default: `/config.yaml`). You can override the config file location using the `CONFIG_FILE` environment variable.

### Configuration Structure

```yaml
destinations:
  destination_name:
    bucket: string        # S3 bucket name
    prefix: string        # Optional prefix for backup files
    endpoint: string      # S3 endpoint URL (optional for AWS S3)
    accessKey: string     # S3 access key
    secretKey: string     # S3 secret key
    region: string        # S3 region

backups:
  - url: string          # PostgreSQL connection URL
    destination: string  # Reference to destination name
    schedule: string     # Cron expression for backup schedule
```

### Example Configurations

#### AWS S3

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
```

#### MinIO

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
```

#### Cloudflare R2

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
```

## Environment Variables

### Configuration

- `CONFIG_FILE`: Path to configuration file (default: `/config.yaml`)
- `TZ`: Timezone for scheduling (e.g., `Europe/Copenhagen`)

### AWS/S3 Credentials

These environment variables can be used as fallbacks when not specified in the destination configuration:

- `AWS_ACCESS_KEY_ID`: S3 access key
- `AWS_SECRET_ACCESS_KEY`: S3 secret key
- `AWS_DEFAULT_REGION`: S3 region
- `AWS_ENDPOINT_URL`: S3 endpoint URL

### Environment Variable Substitution

You can use `${VARIABLE_NAME}` syntax in your configuration file to substitute environment variables:

```yaml
destinations:
  prod:
    bucket: ${BACKUP_BUCKET}
    accessKey: ${AWS_ACCESS_KEY_ID}
    secretKey: ${AWS_SECRET_ACCESS_KEY}
```

## Cron Schedule Format

The application uses extended cron syntax supporting:

- **Standard 5-field**: `minute hour day month weekday`
- **6-field with seconds**: `second minute hour day month weekday`
- **Descriptive**: `@yearly`, `@monthly`, `@weekly`, `@daily`, `@hourly`

### Schedule Examples

```yaml
# Every 30 minutes
schedule: "*/30 * * * *"

# Daily at 2:30 AM
schedule: "30 2 * * *"

# Every Sunday at 3:15 AM
schedule: "15 3 * * 0"

# Every 6 hours
schedule: "0 */6 * * *"

# Monthly on the 1st at midnight
schedule: "0 0 1 * *"

# Using descriptive syntax
schedule: "@daily"
schedule: "@weekly"
```

## Backup File Format

Backups are created using PostgreSQL's custom format (`pg_dump -Fc`) and stored with the following naming convention:

```
s3://bucket/prefix/database_name/pgdump-YYYYMMDDTHHMMSSZ.dump
```

Example:
```
s3://my-backups/postgres/myapp/pgdump-20231225T030000Z.dump
```

## Docker Image

The application is available as a pre-built Docker image:

```
ghcr.io/hareland/pg-backup:latest
```

### Image Details

- **Base**: Alpine Linux with PostgreSQL client tools and AWS CLI
- **Architecture**: Multi-platform (amd64, arm64)
- **Size**: Optimized for minimal footprint
- **Updates**: Automatically built from the main branch

## Development and Testing

### Local Testing with MinIO

Use the provided test setup to run the application locally with MinIO:

```bash
# Start the test environment
docker-compose -f compose.test.yaml up -d

# View logs
docker-compose -f compose.test.yaml logs -f pg-backup

# Stop the test environment
docker-compose -f compose.test.yaml down -v
```

This will start:
- PostgreSQL database with test data
- MinIO S3-compatible storage
- pg-backup configured to backup every 2 minutes

### Building from Source

```bash
# Clone the repository
git clone https://github.com/hareland/pg-backup.git
cd pg-backup

# Build the Docker image
docker build -t pg-backup ./backup-runner

# Or build locally (requires Go 1.21+)
cd backup-runner
go build -o pg-backup .
```

## Monitoring and Logs

The application provides structured logging for monitoring backup operations:

```
[backup] start postgres://user:***@host:5432/database
[backup] uploaded s3://bucket/prefix/database/pgdump-20231225T030000Z.dump
```

### Log Levels

- **Info**: Successful backup operations and scheduling events
- **Error**: Failed backup operations, connection issues, or configuration errors

## Troubleshooting

### Common Issues

1. **Connection timeout**: Adjust `PGCONNECT_TIMEOUT` environment variable (default: 10 seconds)
2. **Permission denied**: Ensure the database user has sufficient privileges
3. **S3 upload fails**: Verify credentials, bucket permissions, and network connectivity
4. **Schedule not triggering**: Check cron syntax and timezone settings

### Debug Steps

1. **Test database connection**:
   ```bash
   docker run --rm ghcr.io/hareland/pg-backup:latest \
     pg_dump --version
   ```

2. **Validate configuration**:
   ```bash
   docker run --rm -v $(pwd)/config.yaml:/config.yaml:ro \
     ghcr.io/hareland/pg-backup:latest \
     cat /config.yaml
   ```

3. **Check logs**:
   ```bash
   docker logs pg-backup-container
   ```

## Contributing

Contributions are welcome! Please feel free to submit issues, feature requests, or pull requests.

## License

This project is open source and available under the MIT License.
