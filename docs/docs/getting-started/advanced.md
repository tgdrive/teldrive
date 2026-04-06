# Advanced Usage

This page covers the main tuning and security settings you may adjust after setup.

## Stream tuning

If playback or large reads feel slow, tune the Telegram stream settings.

Relevant server flags are documented in the generated [CLI reference](/docs/cli/run), especially:

- `tg.stream-buffers`
- `tg.stream-concurrency`
- `tg.stream-chunk-timeout`

For rclone-based reads, a good starting point is:

```sh
--vfs-read-chunk-size=32M
--vfs-read-chunk-streams=4
--teldrive-threaded-streams=true
```

Increase values only after testing.

## Upload tuning

Useful knobs live in both server config and rclone config:

- server queue capacity
  - `queue.default-workers`
  - `queue.upload-workers`
- sync job behavior
  - `jobs.sync-run.max-attempts`
  - `jobs.sync-transfer.max-attempts`
  - `jobs.sync-transfer.timeout`
- rclone client behavior
  - `chunk_size`
  - `upload_concurrency`

Teldrive normalizes upload chunk sizes to backend-safe values. Default is `512Mi`.

## Encryption

Enable native Teldrive encryption on the server:

```toml
[tg.uploads]
encryption-key = "your-key"
```

Then enable it in rclone only when you want encrypted uploads from that client:

```toml
encrypt_files = true
```

- Store the encryption key safely. You cannot recover encrypted files without it.
- Sync and upload jobs now use the same server-side staging flow.
- Teldrive encryption remains compatible with the web UI and sync workflows.

## Bot tokens

For better Telegram throughput:

1. Create bots with `@BotFather`
2. Add their tokens in the Teldrive UI settings
3. Let Teldrive add them to the selected channel, or add them manually if needed

> [!WARNING]
> Newly created Telegram sessions may need time before they can change channel admins. If you see `FRESH_CHANGE_ADMINS_FORBIDDEN`, wait and retry later.

## Image resizing

Teldrive can use `imgproxy` for thumbnail resizing.

::: code-group

```yml [docker-compose.yml]
services:
  imgproxy:
    image: darthsim/imgproxy
    restart: unless-stopped
    environment:
      IMGPROXY_ALLOW_ORIGIN: "*"
      IMGPROXY_ENFORCE_WEBP: true
    ports:
      - "8000:8080"
```

:::

```sh
docker compose up -d
```

For best results, place imgproxy behind a cache or reverse proxy and set that URL in the Teldrive UI.
