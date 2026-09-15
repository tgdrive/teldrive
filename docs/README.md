# Teldrive documentation

Static documentation site built with Astro and Fumadocs UI.

## Development

From the repository root:

```bash
just docs-dev
```

Or directly:

```bash
cd docs
bun install --frozen-lockfile
bun run dev
```

## Production build

```bash
just docs-build
```

Astro writes the complete static site to `docs/dist/`. It can be served by any static host or CDN; no Node.js or Next.js runtime is required.

## Content

Documentation lives under `content/docs/` and is organized into:

- `getting-started/` — beginner setup and first-run verification;
- `installation/` — container, binary, and source builds;
- `configuration/` — security, Telegram, PostgreSQL, uploads, and events;
- `deployment/` — HTTPS/reverse proxy and production operations;
- `advanced/` — architecture, encryption, performance, and multi-instance operation;
- `troubleshooting.mdx` — symptom-driven diagnostics.

The docs are intentionally grounded in the current Teldrive configuration and repository workflows. Update the relevant page when configuration defaults or deployment behavior changes.
