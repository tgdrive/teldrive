# `teldrive run`

Start Teldrive Server

## Usage

```sh
teldrive run [flags]
```

## Flags

### General

| Flag | Default | Description |
| --- | --- | --- |
| `-c, --config` | `—` | Config file path (default $HOME/.teldrive/config.toml) |

### Db

| Flag | Default | Description |
| --- | --- | --- |
| `--db-data-source` | `—` | Database connection string |
| `--db-pool-enable` | `true` | Enable connection pooling |
| `--db-pool-max-idle-connections` | `25` | Maximum number of idle connections |
| `--db-pool-max-lifetime` | `10m0s` | Maximum connection lifetime |
| `--db-pool-max-open-connections` | `25` | Maximum number of open connections |
| `--db-prepare-stmt` | `true` | Use prepared statements |

### Events

| Flag | Default | Description |
| --- | --- | --- |
| `--events-db-buffer-size` | `1000` | Size of DB worker queue buffer |
| `--events-db-workers` | `10` | Number of DB worker goroutines for event persistence |
| `--events-deduplication-ttl` | `5s` | Event deduplication time-to-live |
| `--events-poll-interval` | `10s` | Event polling interval for single-instance mode |

### Cache

| Flag | Default | Description |
| --- | --- | --- |
| `--cache-max-size` | `10485760` | Maximum cache size in bytes (used for memory cache) |

### Jobs

| Flag | Default | Description |
| --- | --- | --- |
| `--jobs-sync-run-max-attempts` | `8` | Maximum retry attempts for sync.run jobs |
| `--jobs-sync-transfer-max-attempts` | `2` | Maximum retry attempts for sync.transfer jobs |
| `--jobs-sync-transfer-timeout` | `3h0m0s` | Maximum execution time for sync.transfer jobs |

### Jwt

| Flag | Default | Description |
| --- | --- | --- |
| `--jwt-allowed-users` | `[]` | List of allowed usernames |
| `--jwt-secret` | `—` | JWT signing secret key |
| `--jwt-session-time` | `30d` | JWT token validity duration |

### Log

| Flag | Default | Description |
| --- | --- | --- |
| `--log-db-level` | `error` | Database logging level (silent, error, warn, info, debug) |
| `--log-db-log-sql` | `false` | LogSQL |
| `--log-db-slow-threshold` | `1s` | Log queries slower than this threshold |
| `--log-file` | `—` | Log file path, if empty logs to stdout only |
| `--log-http-enabled` | `true` | Enable HTTP request logging |
| `--log-http-log-queries` | `false` | Log full query strings (use with caution) |
| `--log-http-log-request-body-size` | `true` | Log request Content-Length |
| `--log-http-log-response-size` | `true` | Log response bytes written |
| `--log-http-log-user-agent` | `true` | Log user agent (truncated) |
| `--log-http-max-query-length` | `100` | Maximum length of query preview |
| `--log-http-sanitize-queries` | `true` | Remove sensitive params from query preview |
| `--log-http-skip-paths` | `[/health,/metrics]` | Paths to skip from logging |
| `--log-level` | `info` | Global logging level (debug, info, warn, error) |
| `--log-tg-enabled` | `false` | Enable Telegram client internal logging |
| `--log-tg-level` | `warn` | Telegram client logging level (debug, info, warn, error) |
| `--log-time-format` | `2006-01-02 15:04:05` | Log time format |

### Queue

| Flag | Default | Description |
| --- | --- | --- |
| `--queue-default-workers` | `50` | Maximum number of workers for the default task queue |
| `--queue-upload-workers` | `4` | Maximum number of workers for the uploads task queue |

### Redis

| Flag | Default | Description |
| --- | --- | --- |
| `--redis-addr` | `—` | Redis server address (empty to disable Redis) |
| `--redis-conn-max-idle-time` | `5m0s` | Redis connection maximum idle time |
| `--redis-conn-max-lifetime` | `1h0m0s` | Redis connection maximum lifetime |
| `--redis-max-idle-conns` | `10` | Redis maximum idle connections |
| `--redis-min-idle-conns` | `5` | Redis minimum idle connections |
| `--redis-password` | `—` | Redis server password |
| `--redis-pool-size` | `10` | Redis connection pool size |

### Server

| Flag | Default | Description |
| --- | --- | --- |
| `--server-enable-pprof` | `false` | Enable pprof debugging endpoints |
| `--server-graceful-shutdown` | `10s` | Grace period for server shutdown |
| `--server-port` | `8080` | HTTP port for the server to listen on |
| `--server-read-timeout` | `1h0m0s` | Maximum duration for reading entire request |
| `--server-write-timeout` | `1h0m0s` | Maximum duration for writing response |

### Tg

| Flag | Default | Description |
| --- | --- | --- |
| `--tg-app-hash` | `8da85b0d5bfe62527e5b244c209159c3` | Telegram app hash |
| `--tg-app-id` | `2496` | Telegram app ID |
| `--tg-app-version` | `6.1.4 K` | App version |
| `--tg-auto-channel-create` | `true` | Auto Create Channel |
| `--tg-channel-limit` | `500000` | Channel message limit before auto channel creation |
| `--tg-device-model` | `Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/116.0` | Device model |
| `--tg-enable-logging` | `false` | Enable Telegram client logging (deprecated: use logging.tg.enabled instead) |
| `--tg-lang-code` | `en` | Language code |
| `--tg-lang-pack` | `webk` | Language pack |
| `--tg-mtproxy-addr` | `—` | MTProto proxy address in host:port format |
| `--tg-mtproxy-secret` | `—` | MTProto proxy secret as hex string |
| `--tg-ntp` | `false` | Use NTP for time synchronization |
| `--tg-pool-size` | `8` | Session pool size |
| `--tg-proxy` | `—` | HTTP/SOCKS5 proxy URL |
| `--tg-rate` | `100` | Rate limit in requests per minute |
| `--tg-rate-burst` | `5` | Maximum burst size for rate limiting |
| `--tg-rate-limit` | `true` | Enable rate limiting for API calls |
| `--tg-reconnect-timeout` | `5m0s` | Client reconnection timeout |
| `--tg-session-bolt-no-grow-sync` | `false` | Disable grow sync for performance |
| `--tg-session-bolt-path` | `—` | Path to BoltDB session file (empty for auto-detect) |
| `--tg-session-bolt-timeout` | `1s` | Timeout for opening BoltDB |
| `--tg-session-instance` | `teldrive` | Bot session instance name for multi-instance deployments |
| `--tg-session-key` | `session` | Key prefix for session storage |
| `--tg-session-type` | `postgres` | Session storage type: postgres, bolt, memory |
| `--tg-stream-bots-limit` | `0` | Maximum number of bots for streaming (0 = use all bots) |
| `--tg-stream-buffers` | `8` | Number of stream buffers |
| `--tg-stream-chunk-timeout` | `30s` | Chunk download timeout |
| `--tg-stream-concurrency` | `1` | Number of concurrent threads for concurrent reader |
| `--tg-system-lang-code` | `en-US` | System language code |
| `--tg-system-version` | `Win32` | System version |
| `--tg-uploads-chunk-naming` | `random` | Upload chunk naming mode (random, deterministic) |
| `--tg-uploads-encryption-key` | `—` | Encryption key for uploads |
| `--tg-uploads-max-retries` | `10` | Maximum upload retry attempts |
| `--tg-uploads-retention` | `7d` | Upload retention period |
| `--tg-uploads-threads` | `8` | Number of upload threads |

> Duration flags accept values like `30s`, `5m`, `1h`, or `7d`. Flags can also be set through the config file or environment-variable mapping where applicable.
