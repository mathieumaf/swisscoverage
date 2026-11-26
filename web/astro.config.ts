import node from "@astrojs/node";
import react from "@astrojs/react";
import { defineConfig } from "astro/config";
import { defaultLocale, locales } from "./src/i18n/locales";

// Pages are rendered on the server (decision 24 in docs/decisions.md), by the
// standalone Node server of @astrojs/node, which Bun runs.
export default defineConfig({
  output: "server",
  adapter: node({ mode: "standalone" }),
  integrations: [react()],
  i18n: {
    locales: [...locales],
    defaultLocale,
    routing: {
      prefixDefaultLocale: true,
      redirectToDefaultLocale: false,
    },
  },
  server: {
    // This machine only. Pass --host to `astro dev` only on purpose.
    host: "127.0.0.1",
    port: 4321,
  },
  devToolbar: { enabled: false },
});
