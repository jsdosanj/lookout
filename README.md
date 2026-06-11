# lookout-site

Marketing website for **Lookout** — open-source IT infrastructure monitoring.

Built with **Astro + Tailwind CSS**. The built site deploys to GitHub Pages at
**https://jsdosanj.github.io/lookout-site/**.

## Develop

```bash
npm install
npm run dev        # local dev server
npm run build      # production build → ./dist
```

## Deploy

The production build is published to the `gh-pages` branch (GitHub Pages source).
Because this is a project site, `astro.config.mjs` sets `base: '/lookout-site'`.

## Pricing

The pricing model lives in [`docs/pricing.md`](docs/pricing.md).

## Links

- Product / agent + dashboard: https://github.com/jsdosanj/servmonitor
- Live demo dashboard: https://jsdosanj.github.io/servmonitor/
