import { defineConfig } from 'astro/config';
import tailwindcss from '@tailwindcss/vite';
import sitemap from '@astrojs/sitemap';

// Served at https://lookout.dosanjhlabs.com/
const BUILD_DATE = new Date('2026-07-07');

// Sensible, hand-picked priority/changefreq per section (Google/Bing largely
// ignore these now, but they cost nothing and some crawlers still use them
// as a weak hint). lastmod reflects this build's date.
function sitemapPriority(url) {
  const path = new URL(url).pathname;
  if (path === '/') return { changefreq: 'weekly', priority: 1.0 };
  if (path === '/pricing/' || path === '/why/') return { changefreq: 'monthly', priority: 0.8 };
  if (path.startsWith('/docs/')) return { changefreq: 'monthly', priority: 0.6 };
  if (path === '/contact/') return { changefreq: 'monthly', priority: 0.5 };
  if (path === '/ai-policy/') return { changefreq: 'yearly', priority: 0.3 };
  return { changefreq: 'monthly', priority: 0.5 };
}

export default defineConfig({
  site: 'https://lookout.dosanjhlabs.com',
  base: '/',
  // Build into ./docs so the static output can be deployed from /docs.
  outDir: './docs',
  integrations: [
    sitemap({
      serialize(item) {
        return { ...item, lastmod: BUILD_DATE, ...sitemapPriority(item.url) };
      },
    }),
  ],
  vite: { plugins: [tailwindcss()] },
});
