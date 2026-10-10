# Teldrive

Telegram-backed file storage with a web interface, an HTTP API, and rclone integration.

## Install

Teldrive requires PostgreSQL and a Telegram account. Choose a deployment method:

- [Linux and Windows binaries](https://tgdrive.github.io/teldrive/installation/binary)
- [Nix, NixOS, and Home Manager](https://tgdrive.github.io/teldrive/installation/nix)
- [Docker or Podman](https://tgdrive.github.io/teldrive/getting-started/quick-start)
- [Build from source](https://tgdrive.github.io/teldrive/installation/from-source)

See the [documentation](https://tgdrive.github.io/teldrive/) for configuration, upgrades, and operations.

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
