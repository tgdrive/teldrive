# Teldrive

Telegram-backed file storage with a web interface, an HTTP API, and rclone integration.

## Install

Teldrive requires PostgreSQL and a Telegram account. Choose a deployment method:

- [Linux and Windows binaries](https://tgdrive.github.io/teldrive/installation/binary)
- [Nix, NixOS, and Home Manager](https://tgdrive.github.io/teldrive/installation/nix)
- [Docker or Podman](https://tgdrive.github.io/teldrive/getting-started/quick-start)
- [Build from source](https://tgdrive.github.io/teldrive/installation/from-source)

See the [documentation](https://tgdrive.github.io/teldrive/) for configuration, upgrades, and operations.

## Public share links from a local server

Set `http.public-url` to the public origin of your tunnel or reverse proxy so links
copied from localhost or Teldrive Desktop use your domain:

```toml
[http]
address = "127.0.0.1:8080"
public-url = "https://drive.example.com"
trusted-proxies = ["127.0.0.1/32", "::1/128"]
```

The trusted proxy addresses above apply when the proxy runs on the same machine.
Configure Cloudflare Tunnel (or your reverse proxy) to forward the public hostname
to `http://127.0.0.1:8080`, then restart Teldrive. In Desktop, edit the existing
`[http]` section under **Configuración → Configuración avanzada** and save it.
This setting changes newly generated share URLs; it does not create a DNS record,
a tunnel, or HTTPS. Existing share tokens remain valid under the new domain.
An empty `public-url` keeps using the browser origin. Only an HTTP or HTTPS origin
is accepted; credentials, subpaths, queries, and fragments are not supported.
The equivalent CLI option is `--http-public-url`; the environment variable is
`TELDRIVE_HTTP_PUBLIC_URL`.

## Data safety

Follow Telegram's terms and API limits. Accounts and stored files can become unavailable; keep independent backups. Back up database credentials, signing keys, and encryption keys before upgrading.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Supported development commands are in `justfile`.

## Donate

If you find Teldrive useful, consider a [PayPal donation](https://paypal.me/redux234).

## Star History

<a href="https://www.star-history.com/#tgdrive/teldrive&Date">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=tgdrive/teldrive&type=Date&theme=dark" />
    <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=tgdrive/teldrive&type=Date" />
    <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=tgdrive/teldrive&type=Date" />
  </picture>
</a>
