# Jobs and Sync

Teldrive uses background jobs for long-running work like sync, transfer, and maintenance.

## Main job types

- `sync.run`
  - plans and coordinates a sync workflow
- `sync.transfer`
  - uploads individual files
- maintenance jobs
  - cleanup and retention tasks

## How sync works

At a high level:

1. `sync.run` scans the source and decides what needs work
2. it enqueues `sync.transfer` jobs for files that must be uploaded
3. `sync.transfer` uploads file parts and finalizes the file in Teldrive

`sync.run` is a coordinator. It may move between active queue states while it waits for child transfers.

## Supported source patterns

Teldrive currently supports these source schemes:

- `local://`
- `dav://`
- `davs://`
- `sftp://`
- `rclone://`
- `rclones://`

Examples:

```text
local:///srv/media
dav://user:pass@example.com/remote/path
davs://user:pass@example.com/remote/path
sftp://user:pass@example.com:22/remote/path
rclone://remote:/path
rclones://remote:/path
```

Notes:

- `dav://` uses WebDAV over HTTP.
- `davs://` uses WebDAV over HTTPS.
- `rclone://` and `rclones://` depend on the configured rclone-backed source implementation.

## Progress

- `sync.transfer` progress is byte-based
- task UI progress comes from live job metadata updates
- for large files, progress should move while the upload is in flight

## Resume behavior

Sync uploads now keep partial upload state instead of deleting it on failure. That means:

- retries can resume from already uploaded parts
- stale abandoned upload state is cleaned later by maintenance jobs

## Tuning

There are two main layers of tuning.

### Queue capacity

These control worker capacity:

- `queue.default-workers`
- `queue.upload-workers`

### Job policy

These control sync job behavior:

- `jobs.sync-run.max-attempts`
- `jobs.sync-transfer.max-attempts`
- `jobs.sync-transfer.timeout`

## Chunk sizing

Sync transfer chunk size follows the same backend logic as rclone:

- default `512Mi`
- minimum `64Mi`
- maximum `2000Mi`
- aligned to `16Mi` boundaries

## Encryption

If sync uploads are encrypted:

- the server must have `tg.uploads.encryption-key`
- sync/rclone/client settings must agree about encryption usage

## Troubleshooting

### `repository: not found`

This usually points to destination path issues. Teldrive now lazily creates nested destination directories for sync transfers.

### `context deadline exceeded`

This usually indicates a long-running transfer hitting the configured timeout or a slow source/upload path.

### Progress not visible

If the backend is updating progress but the UI does not show it, check the task list response and browser rendering path. The backend stores progress in live job metadata.

## Related docs

- [Usage](/docs/getting-started/usage)
- [Advanced Usage](/docs/getting-started/advanced)
- [CLI Reference](/docs/cli/run)
