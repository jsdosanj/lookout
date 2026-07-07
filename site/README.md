# lookout-site

Marketing website for **Lookout** — commercial IT infrastructure monitoring built for humans.

Built with **Astro + Tailwind CSS**. The built site is served at
**https://lookout.dosanjhlabs.com/**.

## Develop

```bash
npm install
npm run dev        # local dev server
npm run build      # production build → ./docs
```

## Deploy

The production build is written to `./docs` and deployed via Cloudflare Pages
(git-connected: merging to `main` publishes to `lookout.dosanjhlabs.com`).
The site is served from the domain root, so `astro.config.mjs` sets `base: '/'`.

## AI-crawler policy & SEO

`public/robots.txt`, `public/ai.txt`, `public/llms.txt`, `public/.well-known/tdmrep.json`,
and `public/_headers` declare that AI answer/search engines may crawl and cite
the site, while AI/ML training and dataset collection is reserved. See
`src/pages/ai-policy.astro` (served at `/ai-policy/`) for the human-readable policy.

## Pricing

Lookout is a commercial product with three paid plans — Starter, Pro, and Enterprise —
priced per monitored server. There is no free tier. See `src/pages/pricing.astro`.

## Contact

Sales and support: use the contact form at `/contact/`.
