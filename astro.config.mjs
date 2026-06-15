import { defineConfig } from 'astro/config';
import tailwindcss from '@tailwindcss/vite';
import sitemap from '@astrojs/sitemap';

// Served at https://lookout.dosanjhlabs.com/
export default defineConfig({
  site: 'https://lookout.dosanjhlabs.com',
  base: '/',
  // Build into ./docs so the static output can be deployed from /docs.
  outDir: './docs',
  integrations: [sitemap()],
  vite: { plugins: [tailwindcss()] },
});
