# Prerequisites

Before you run Teldrive, make sure you have these basics in place.

## Required

1. A Telegram account
2. A PostgreSQL database
3. A way to run Teldrive
   - Docker is optional
   - running the binary directly is also supported

## Recommended

- a dedicated private Telegram channel for uploads
- a reverse proxy with HTTPS if you plan to expose Teldrive publicly
- a stable system clock or `tg.ntp = true`

## PostgreSQL options

You can use either:

- Supabase or another managed PostgreSQL provider
- a local PostgreSQL instance

### Managed PostgreSQL

If you use Supabase or another hosted provider:

1. Create the database
2. Copy the connection string
3. Make sure the required extensions are available for your deployment

Use that connection string as `db.data-source`.

### Local PostgreSQL with Docker

This is the simplest local setup:

::: code-group

```yml [docker-compose.yml]
services:
  postgres:
    image: groonga/pgroonga:latest-alpine-17
    restart: unless-stopped
    environment:
      POSTGRES_USER: teldrive
      POSTGRES_PASSWORD: secret
      POSTGRES_DB: teldrive
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data

volumes:
  postgres_data:
```

:::

Start it with:

```sh
docker compose up -d
```

Example connection string:

```text
postgres://teldrive:secret@localhost:5432/teldrive?sslmode=disable
```

## Docker

Docker is useful if you want:

- a local PostgreSQL quickly
- a containerized Teldrive deployment
- a simple repeatable setup on a server

Verify Docker if you plan to use it:

```sh
docker --version
docker compose version
```

## Telegram setup

You do not need to create everything up front, but it helps to know:

- Teldrive stores uploads in Telegram channels
- after first login, you should sync channels in the UI
- then choose a default upload channel in Settings

## Next steps

- [Installation](/docs/getting-started/installation)
- [Usage](/docs/getting-started/usage)
- [Jobs and Sync](/docs/guides/jobs-and-sync)
