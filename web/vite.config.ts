import { resolve } from "node:path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig, loadEnv } from "vite"

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  // Dev-only: where the proxy sends /api. Read from web/.env* so a plain
  // `pnpm dev` needs no exported variables; ARGUS_SERVER_URL in the shell still
  // wins for one-off overrides. It is deliberately not VITE_-prefixed: the
  // browser always calls same-origin /api, so this must not reach the bundle.
  const env = loadEnv(mode, import.meta.dirname, "")
  const apiTarget =
    process.env.ARGUS_SERVER_URL ??
    env.ARGUS_SERVER_URL ??
    "http://127.0.0.1:8095"

  return {
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
          target: apiTarget,
          changeOrigin: false,
        },
      },
    },
  }
})
