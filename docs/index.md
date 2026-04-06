---
layout: home

hero:
  name: "Teldrive"
  tagline: Run Telegram-backed file storage with a web UI, API keys, sync jobs, and an rclone backend.
  actions:
    - theme: brand
      text: Get Started
      link: /docs/
    - theme: alt
      text: Use with rclone
      link: /docs/guides/rclone/
    - theme: alt
      text: API Reference
      link: /docs/api
    - theme: alt
      text: CLI Reference
      link: /docs/cli/
  image:
    src: /images/logo.png
    alt: Teldrive

features:
  - icon: 🚀
    title: Self-host fast
    details: Start with generated config, PostgreSQL, and a single Go binary or container image.
  - icon: 🔄
    title: Connect rclone
    details: Use API keys, mount the backend, sync data, and automate transfers with current backend options.
  - icon: ⚙️
    title: Run background jobs
    details: Queue-based sync and transfer jobs with configurable retries, timeouts, and worker capacity.
  - icon: 🔐
    title: Protect data
    details: Optional encrypted uploads, resumable sync transfers, and server-controlled chunk naming.
---

## What to do next

- [Install and run Teldrive](/docs/getting-started/installation)
- [Configure the server](/docs/getting-started/usage)
- [Create API keys](/docs/guides/api-keys)
- [Connect rclone](/docs/guides/rclone)
- [Understand jobs and sync](/docs/guides/jobs-and-sync)
