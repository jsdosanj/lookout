# lookout-site

Marketing website for **Lookout** — commercial IT infrastructure monitoring built for humans.

Built with **Astro + Tailwind CSS**. The built site deploys to GitHub Pages at
**https://jsdosanj.github.io/lookout-site/**.

## Develop

```bash
npm install
npm run dev        # local dev server
npm run build      # production build → ./docs
```

## Deploy

The production build is written to `./docs` on the `main` branch (GitHub Pages source).
Because this is a project site, `astro.config.mjs` sets `base: '/lookout-site'`.

## Pricing

Lookout is a commercial product with three paid plans — Starter, Pro, and Enterprise —
priced per monitored server. There is no free tier. See `src/pages/pricing.astro`.

## Contact

Sales and support: jasvantdosanjh@outlook.com
