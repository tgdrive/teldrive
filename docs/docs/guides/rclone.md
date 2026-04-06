# Use Teldrive with rclone

This is the recommended way to connect rclone to Teldrive.

## Before you start

You need:

- a running Teldrive server
- a working Teldrive login
- an API key created from the Teldrive UI

Use API keys for rclone.

## 1. Create an API key

In the Teldrive UI:

1. Open **Settings**
2. Go to **API Keys**
3. Create a new key
4. Copy the key value and store it safely

If you revoke the key later, rclone access stops immediately.

## 2. Install the Teldrive rclone backend

::: code-group

```sh [macOS/Linux]
curl -sSL instl.vercel.app/rclone | bash
```

```powershell [Windows]
powershell -c "irm https://instl.vercel.app/rclone?platform=windows|iex"
```

:::

## 3. Minimal config

Add a remote to your rclone config file:

```toml
[teldrive]
type = teldrive
api_host = https://your-teldrive.example.com
api_key = your_api_key_here
chunk_size = 512Mi
upload_concurrency = 4
encrypt_files = false
```

## 4. Optional settings

| Option | Description |
| --- | --- |
| `type` | Must be `teldrive`. |
| `api_host` | Base URL of your Teldrive server. |
| `api_key` | API key created in the Teldrive UI. |
| `chunk_size` | Upload chunk size. Default is `512Mi`, clamped and aligned by the backend. |
| `upload_concurrency` | Concurrent upload workers used by rclone. Default is `4`. |
| `encrypt_files` | Enable native Teldrive encryption. Default is `false`. |
| `hash_enabled` | Enable Teldrive BLAKE3 tree hashing. Default is `true`. |
| `channel_id` | Upload to a specific Telegram channel instead of automatic selection. |
| `page_size` | Page size for file listing. Default is `500`. |
| `threaded_streams` | Enable threaded reads for streaming-heavy workloads. |
| `upload_host` | Separate host for upload traffic if it differs from `api_host`. |
| `link_password` | Default password for created public links. |

## 5. Optional config examples

### Separate upload host

```toml
[teldrive]
type = teldrive
api_host = https://teldrive.example.com
upload_host = https://uploads.teldrive.example.com
api_key = your_api_key_here
chunk_size = 512Mi
upload_concurrency = 4
```

### Encrypted uploads

```toml
[teldrive]
type = teldrive
api_host = https://your-teldrive.example.com
api_key = your_api_key_here
encrypt_files = true
chunk_size = 512Mi
```

When `encrypt_files = true`, your Teldrive server must also be configured with `tg.uploads.encryption-key`.

## 6. Common commands

```sh
rclone lsd teldrive:
rclone ls teldrive:
rclone copy ./local-folder teldrive:/backup
rclone sync ./media teldrive:/media
rclone mount teldrive:/ ~/mnt/teldrive
rclone link teldrive:/path/to/file
```

## Notes

- Chunk naming is controlled by the Teldrive server with `tg.uploads.chunk-naming`.
- `random_chunk_name` is no longer used.
- Teldrive sync and upload flows now keep resumable server-side upload state.
- `chunk_size` is normalized to the backend limits and 16 MiB boundaries.

## Troubleshooting

### `missing api_key`

Your rclone config is still using the old auth style. Replace it with `api_key`.

### `invalid session`

The API key is revoked, expired, or no longer tied to a valid Teldrive session. Create a new key in the UI.

### Uploads fail on encrypted mode

Make sure both sides agree:

- rclone: `encrypt_files = true`
- Teldrive server: `tg.uploads.encryption-key` set

### Uploads go to the wrong host

Set `upload_host` only if your deployment serves upload traffic from a different host than `api_host`.

### Slow listing or paging behavior

Tune `page_size` for your workload, but keep the default unless you have a specific reason to change it.

## Related docs

- [Usage](/docs/getting-started/usage)
- [Advanced Usage](/docs/getting-started/advanced)
- [CLI Reference](/docs/cli/run)
