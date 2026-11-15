import { resolve } from "node:path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": resolve(import.meta.dirname, "./src"),
    },
  },
  server: {
    // During local development the browser talks only to Vite (:8096); API
    // calls and the SSE stream are proxied to a locally running argus server.
    // Proxying (rather than CORS) keeps the session cookie same-origin, so
    // login/CSRF work exactly as they do in production.
    proxy: {
      "/api": {
        target: process.env.ARGUS_SERVER_URL ?? "http://127.0.0.1:8095",
        changeOrigin: false,
      },
    },
  },
})
