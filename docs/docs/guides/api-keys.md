# API Keys

API keys let external tools access Teldrive without using a browser session directly.

Use them for rclone, scripts, and external integrations.

## When to use an API key

Use an API key when a client needs long-lived programmatic access. Do not use browser cookies as the normal setup path for rclone or automation.

## Create an API key

In the Teldrive UI:

1. Open **Settings**
2. Go to **API Keys**
3. Create a new key
4. Copy the key value immediately

Store it securely. You may not be able to see the full value again later.

## Revoke an API key

Revoke the key from the same settings page when:

- a machine is retired
- a secret leaks
- an integration no longer needs access

Once revoked, clients using that key stop working immediately.

## Important behavior

- API keys are tied to your Teldrive account state
- if the underlying auth/session state becomes invalid, the key may stop working
- API keys are the recommended auth method for the Teldrive rclone backend

## Example: rclone

```toml
[teldrive]
type = teldrive
api_host = https://your-teldrive.example.com
api_key = your_api_key_here
```

See the full [rclone guide](/docs/guides/rclone) for all backend options.

## Troubleshooting

### `missing api_key`

Your client config is still using the old auth style. Add `api_key` explicitly.

### `invalid session`

The key is revoked, expired, or no longer linked to a valid Teldrive session state. Create a new one.
