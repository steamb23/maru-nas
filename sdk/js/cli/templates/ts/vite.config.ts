import { defineConfig } from 'vite'

// Builds the pages into addon/ui, the package's ui/ directory. Each view is one HTML page.
export default defineConfig({
  // The dashboard serves the files under a per-session path, so all URLs must be relative.
  base: './',
  build: {
    outDir: 'addon/ui',
    emptyOutDir: true,
    // Keep assets as files: the frame's Content-Security-Policy allows scripts only from the package itself.
    assetsInlineLimit: 0,
    modulePreload: { polyfill: false },
    rollupOptions: {
      input: { window: 'window.html' },
    },
  },
})
