import node from "@astrojs/node";
import react from "@astrojs/react";
import { defineConfig } from "astro/config";

// Pages are rendered on the server (decision 24 in docs/decisions.md), by the
// standalone Node server of @astrojs/node, which Bun runs. The languages are
// handled in src/i18n/locales.ts.
export default defineConfig({
  output: "server",
  adapter: node({ mode: "standalone" }),
  // React renders the map, in the browser only.
  integrations: [react()],
  server: {
    // This machine only. Pass --host to `astro dev` only on purpose.
    host: "127.0.0.1",
    port: 4321,
  },
  vite: {
    // MapLibre GL alone is about 1.1 MB once minified, 300 KB compressed.
    build: { chunkSizeWarningLimit: 1200 },
  },
  devToolbar: { enabled: false },
});
