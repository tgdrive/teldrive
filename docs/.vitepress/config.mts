import { defineConfig } from 'vitepress'

export default defineConfig({
  lang: "en-US",
  title: "Teldrive",
  description: "Telegram-backed file storage with a web UI, sync jobs, and an rclone backend.",
  lastUpdated: true,
  ignoreDeadLinks: true,
  cleanUrls: true,
  sitemap: {
    hostname: 'https://teldrive-docs.pages.dev'
  },
  themeConfig: {
    logo: '/images/logo.png',
    siteTitle: 'Teldrive',
    socialLinks: [
      { icon: 'github', link: 'https://github.com/tgdrive/teldrive' },
      { icon: 'discord', link: 'https://discord.gg/8QAeCvTK7G' },
    ],
    editLink: {
      pattern: 'https://github.com/tgdrive/teldrive/edit/main/docs/:path',
      text: 'Edit this page on GitHub',
    },
    footer: {
      message: 'Released under the MIT License.',
    },
    nav: [
      { text: 'Home', link: '/' },
    ],

    sidebar: [
      {
        text: 'Getting Started',
        collapsed: false,
        items: [
          { text: 'Prerequisites', link: '/docs/getting-started/prerequisites.md' },
          { text: 'Installation', link: '/docs/getting-started/installation.md' },
          { text: 'Usage', link: '/docs/getting-started/usage.md' },
          { text: 'Advanced', link: '/docs/getting-started/advanced.md' },
        ]
      },
      {
        text: 'Integrations',
        collapsed: false,
        items: [
          { text: 'API Keys', link: '/docs/guides/api-keys.md' },
          { text: 'rclone', link: '/docs/guides/rclone.md' },
          { text: 'Media Servers', link: '/docs/guides/jellyfin.md' },
        ]
      },
      {
        text: 'Operations',
        collapsed: false,
        items: [
          { text: 'Jobs and Sync', link: '/docs/guides/jobs-and-sync.md' },
          { text: 'Deploy with Caddy and Cloudflare', link: '/docs/guides/caddy-cloudflare.md' },
          { text: 'Database Backup', link: '/docs/guides/db-backup.md' },
        ]
      },
      {
        text: 'CLI',
        collapsed: false,
        items: [
          { text: 'Overview', link: '/docs/cli/' },
          { text: 'run', link: '/docs/cli/run.md' },
          { text: 'check', link: '/docs/cli/check.md' },
          { text: 'upgrade', link: '/docs/cli/upgrade.md' },
          { text: 'version', link: '/docs/cli/version.md' },
        ]
      },
      {
        text: 'API Reference',
        link: '/docs/api.md',
      },
    ],
    
  },
  head: [
    [
      'meta',
      {
        property: 'og:image',
        content: '/images/logo.png',
      },
    ],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'twitter:domain', content: 'teldrive-docs.pages.dev' }],
    [
      'meta',
      {
        property: 'twitter:image',
        content: '/images/logo.png',
      },
    ],
    [
      'meta',
      { property: 'twitter:card', content: 'summary_large_image' },
    ],
    ['link', { rel: 'shortcut icon', href: '/favicon.ico' }],
  ]
})
