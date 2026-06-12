# lookout-site

Marketing website for **Lookout** — commercial IT infrastructure monitoring built for humans.

Built with **Astro + Tailwind CSS**. The built site is served at
**https://dosanjhlabs.com/lookout/**.

## Develop

```bash
npm install
npm run dev        # local dev server
npm run build      # production build → ./docs
```

## Deploy

The production build is written to `./docs` on the `main` branch.
Because the site is served under a sub-path, `astro.config.mjs` sets `base: '/lookout'`.

## Pricing

Lookout is a commercial product with three paid plans — Starter, Pro, and Enterprise —
priced per monitored server. There is no free tier. See `src/pages/pricing.astro`.

## Contact

Sales and support: use the contact form at `/lookout/contact/`.
