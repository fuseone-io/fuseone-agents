/// <reference types="vitest/config" />
import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { precompressPlugin } from "./build/precompress";

// The build emits plain static assets. They are embedded into the Go binary
// via go:embed behind the `embedui` build tag, so the installation ships as a
// single artefact (PRD DE-01). Nothing here assumes a Node runtime in
// production.
export default defineConfig({
  // precompress writes .br and .gz copies the Go handler serves by
  // Accept-Encoding; see internal/web/spa.go.
  plugins: [react(), tailwindcss(), precompressPlugin()],
  resolve: {
    alias: { "@": path.resolve(__dirname, "./src") },
  },
  build: {
    outDir: "dist",
    // The Go embed expects a stable directory; hashed filenames inside it are
    // fine and give us long-lived caching behind the API's own cache headers.
    assetsDir: "assets",
    // "hidden" writes the map but omits the sourceMappingURL comment, so no
    // browser asks for it. The map stays a local build artefact for reading a
    // production stack trace; it is not copied into the binary, because the
    // original source carries comments that explain the Gate's checks, the
    // CSRF scheme and the bootstrap flow — free reconnaissance for anyone
    // probing an installation, and 2.5MB of the embedded bundle besides.
    sourcemap: "hidden",
    rollupOptions: {
      output: {
        // Lazy screens make Rollup cut a chunk at every module two screens
        // share — a lucide icon, a badge, a feature's api.ts — which left
        // dozens of sub-kilobyte files, each its own request. Chunks below
        // this size are folded into a neighbour instead; the screens
        // themselves stay separate.
        experimentalMinChunkSize: 20_000,
        // Icons are the exception the size floor cannot fold: each lucide
        // icon is its own module, shared by an arbitrary set of screens. They
        // are a few kilobytes all together, so they travel as one file.
        manualChunks: (id) =>
          id.includes("/node_modules/lucide-react/") ? "icons" : undefined,
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      // In development the SPA runs on Vite and talks to agentd directly.
      "/api": { target: "http://127.0.0.1:8080", changeOrigin: true },
      // Sign-in, sign-out and first-run setup live outside the API contract —
      // they are browser redirects and cookies — but they are just as much
      // part of the server the console talks to.
      "/auth": { target: "http://127.0.0.1:8080", changeOrigin: true },
      // Uploaded logos and icons, served by agentd itself so an air-gapped
      // installation needs no outside host to show its brand.
      "/branding/assets": { target: "http://127.0.0.1:8080", changeOrigin: true },
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    css: false,
  },
});
