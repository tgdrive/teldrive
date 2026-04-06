# Deploy Teldrive with Caddy and Cloudflare

This is the recommended reverse-proxy deployment path for Teldrive.

It gives you:

- Cloudflare DNS and edge protection
- automatic HTTPS with Caddy
- a clean setup for Teldrive and optional imgproxy

## What you need

- a server with Docker and Docker Compose
- a domain managed in Cloudflare
- ports `80` and `443` open to the server
- a working Teldrive `config.toml`

Before continuing, finish the base server setup in [Usage](/docs/getting-started/usage).

## 1. Configure Cloudflare

In Cloudflare:

1. Add your domain
2. point an `A` record for your app host to the server IP
3. keep the record proxied if you want Cloudflare protection

Recommended settings:

- **SSL/TLS mode:** `Full` or `Full (strict)`
- **Always Use HTTPS:** enabled
- **Minimum TLS version:** 1.2 or newer

Example hostnames used in this guide:

- `teldrive.example.com`
- `imgproxy.example.com`

## 2. Create the Docker network

```sh
docker network create web
```

## 3. Run Teldrive

Create `docker-compose.yml`:

::: code-group

```yml [docker-compose.yml]
services:
  teldrive:
    image: ghcr.io/tgdrive/teldrive
    restart: unless-stopped
    networks:
      - web
    volumes:
      - ./config.toml:/config.toml:ro

  caddy:
    image: caddy:2
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
      - "443:443/udp"
    networks:
      - web
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config

  imgproxy:
    image: darthsim/imgproxy
    restart: unless-stopped
    networks:
      - web
    environment:
      IMGPROXY_ALLOW_ORIGIN: "*"
      IMGPROXY_ENFORCE_WEBP: true

networks:
  web:
    external: true

volumes:
  caddy_data:
  caddy_config:
```

:::

## 4. Add a Caddyfile

```caddyfile
teldrive.example.com {
  encode zstd gzip
  reverse_proxy teldrive:8080
}

imgproxy.example.com {
  encode zstd gzip
  reverse_proxy imgproxy:8080
}
```

If you do not use `imgproxy`, remove that site block and container.

## 5. Start the stack

```sh
docker compose up -d
```

## 6. Final Teldrive settings

After the stack is running:

1. open `https://teldrive.example.com`
2. log in
3. if you enabled imgproxy, set the resizer URL in the Teldrive UI to:

```text
https://imgproxy.example.com
```

## Notes

- You do not need a separate upload host unless your deployment routes uploads differently.
- If you add a separate uploads hostname later, update clients like rclone with `upload_host`.
- Caddy handles certificate management automatically.

## Troubleshooting

### Site does not load

- confirm DNS points to your server
- confirm ports `80` and `443` are open
- check logs:

```sh
docker logs <caddy-container>
docker logs <teldrive-container>
```

### TLS problems behind Cloudflare

Make sure Cloudflare is set to `Full` or `Full (strict)`, not `Flexible`.

### Bad Gateway from Caddy

This usually means Teldrive is not reachable from the `web` Docker network.

Check:

- both containers are running
- both are on the same Docker network
- Caddy is proxying to `teldrive:8080`

### Large uploads fail through the proxy

Start with the basic config above first. If you add custom proxy buffering or request limits later, make sure they still allow large uploads.

## Advanced

If you already know you need it, you can expand this setup with:

- additional site blocks
- a separate uploads hostname
- caching or CDN rules in Cloudflare
- hardened headers and access rules in Caddy

For most users, the basic setup above is the right place to stop.
