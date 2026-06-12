import { defineConfig } from 'astro/config';
import tailwindcss from '@tailwindcss/vite';
import sitemap from '@astrojs/sitemap';

// Served at https://dosanjhlabs.com/lookout/
export default defineConfig({
  site: 'https://dosanjhlabs.com',
  base: '/lookout',
  // Build into ./docs so the static output can be deployed from /docs.
  outDir: './docs',
  integrations: [sitemap()],
  vite: { plugins: [tailwindcss()] },
});
