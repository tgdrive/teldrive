# Usage

This guide covers the shortest path to a working Teldrive server.

## 1. Start from the generated sample config

Teldrive ships generated sample config files:

- [`config.sample.toml`](https://github.com/tgdrive/teldrive/blob/main/config.sample.toml)
- [`config.sample.yml`](https://github.com/tgdrive/teldrive/blob/main/config.sample.yml)

Copy one and adjust only the values you need first.

## 2. Minimal config

```toml
[db]
data-source = "postgres://teldrive:secret@localhost/postgres"

[jwt]
allowed-users = ["your_telegram_username"]
secret = "replace-with-a-random-secret"

[tg.uploads]
encryption-key = "replace-with-a-random-key"
```

Use your actual database connection string and Telegram username.

## 3. Generate secrets

Use the tool below to generate secure random values:

<SecretGenerator />

You can also generate a JWT secret manually:

```sh
openssl rand -hex 32
```

## 4. Run Teldrive

### With Docker

::: code-group

```yml [docker-compose.yml]
services:
  teldrive:
    image: ghcr.io/tgdrive/teldrive
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./config.toml:/config.toml:ro
```

:::

```sh
docker compose up -d
```

### From the binary

```sh
./teldrive run --config ./config.toml
```

You can also place the config at `$HOME/.teldrive/config.toml` and run:

```sh
teldrive run
```

## 5. First-time setup in the UI

1. Open `http://localhost:8080`
2. Log in with Telegram
3. Sync your channels in **Settings**
4. Select a default upload channel
5. Create an API key if you plan to use rclone or external automation

## Notes

- For local PostgreSQL setup, see [Prerequisites](/docs/getting-started/prerequisites).
- For exact flags and defaults, use the generated [CLI reference](/docs/cli/run).
- For rclone setup, use the dedicated [rclone guide](/docs/guides/rclone).

> [!NOTE]
> If login seems broken, check system time first. Telegram is sensitive to clock drift. You can enable automatic synchronization with `tg.ntp = true` or `--tg-ntp`.
