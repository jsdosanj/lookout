import { defineConfig } from 'astro/config';
import tailwindcss from '@tailwindcss/vite';
import sitemap from '@astrojs/sitemap';

// Served as a GitHub Pages project site at https://jsdosanj.github.io/lookout-site/
export default defineConfig({
  site: 'https://jsdosanj.github.io',
  base: '/lookout-site',
  integrations: [sitemap()],
  vite: { plugins: [tailwindcss()] },
});
