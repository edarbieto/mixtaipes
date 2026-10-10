import { defineConfig } from "astro/config";
import react from "@astrojs/react";

// In development the Go server runs separately; proxy API and probe calls to it.
const apiURL = process.env.API_URL ?? "http://localhost:8080";

export default defineConfig({
  output: "static",
  integrations: [react()],
  i18n: {
    defaultLocale: "en",
    locales: ["en", "es"],
    routing: { prefixDefaultLocale: false },
  },
  vite: {
    server: {
      proxy: {
        "/v1": apiURL,
        "/healthz": apiURL,
      },
    },
  },
});
