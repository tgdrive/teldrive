# Automated Database Backups with Rclone

This guide shows how to back up your Teldrive PostgreSQL database to any rclone-supported remote.

## Prerequisites

- A configured database from the [prerequisites guide](/docs/getting-started/prerequisites.md)
- A cloud storage account with any provider supported by Rclone
- Rclone configured with access to your desired cloud storage

## 1. Run the backup container

Create a Docker Compose file for the backup service:

::: code-group

```yml [docker-compose.yml]
services:
  rclone-backup:
    image: ghcr.io/tgdrive/rclone-backup:17 # for postgres 16 use tag 16
    container_name: rclone-backup
    environment:
      - RCLONE_REMOTE_NAME=remote
      - BACKUP_KEEP_DAYS=10
      - CRON=0 0 * * * # backup frequency (every 24 hours in this example)
      - ZIP_ENABLE=true # enable backup compression
      - PG_CONNECTION_STRING=postgres://user:pass@postgres/postgres # database string
      - ZIP_PASSWORD=zippass # password to protect backup archives
    restart: always
    networks:
     - postgres
    volumes:
      - /path/to/rclone/configdir/:/config/rclone # mount your rclone config directory

networks:
  postgres:                                 
    external: true
```
:::

> [!NOTE]
> If you're using Supabase, remove the networks block from the compose file and update the database connection details accordingly.

Start the backup service:

```sh
docker compose up -d
```

## 2. Main settings

| Variable | Description | Default |
| --- | --- | --- |
| `RCLONE_REMOTE_NAME` | Name of your Rclone remote | *required* |
| `BACKUP_KEEP_DAYS` | Days to keep backup history | 7 |
| `CRON` | Backup schedule in cron format | "0 0 * * *" (daily) |
| `ZIP_ENABLE` | Enable backup compression | false |
| `ZIP_PASSWORD` | Password for encrypted backups | *empty* |
| `PG_CONNECTION_STRING` | PostgreSQL Connection string | *required* |

## 3. Verify backups

Verify backups by:

1. Checking the backup service logs: `docker logs rclone-backup`
2. Listing your backups in the remote storage: `rclone ls remote:path/to/backups`

## 4. Restore

1. Download the backup from your remote
2. Extract it if needed
3. Restore with `pg_restore`:

```bash
pg_restore --dbname="your_postgres_url" --create --no-owner --disable-triggers backup_file.dump
```

If PostgreSQL runs in Docker:

```bash
# Copy the backup file into the container
docker cp backup_file.dump postgres_container:/tmp/

docker exec -it postgres_container pg_restore --dbname="your_postgres_url" --create --no-owner --disable-triggers /tmp/backup_file.dump
```
