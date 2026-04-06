# `teldrive check`

Check file integrity in Telegram channels by comparing database records
with the actual Telegram messages. Missing files can be exported and optional cleanup
removes missing files and orphan channel messages.

Examples:
  teldrive check --user alice --dry-run
  teldrive check --export-file missing_files.json
  teldrive check --concurrent 8

## Usage

```sh
teldrive check [flags]
```

## Flags

### General

| Flag | Default | Description |
| --- | --- | --- |
| `-c, --config` | `—` | Config file path (default $HOME/.teldrive/config.toml) |

### Dry

| Flag | Default | Description |
| --- | --- | --- |
| `--dry-run` | `false` | Simulate check/clean process without making changes |

### Export

| Flag | Default | Description |
| --- | --- | --- |
| `--export-file` | `results.json` | Path for exported JSON file |

### Concurrent

| Flag | Default | Description |
| --- | --- | --- |
| `--concurrent` | `4` | Number of concurrent channel processing |

### User

| Flag | Default | Description |
| --- | --- | --- |
| `--user` | `—` | Telegram username to check (prompts if not specified) |

> Duration flags accept values like `30s`, `5m`, `1h`, or `7d`. Flags can also be set through the config file or environment-variable mapping where applicable.
