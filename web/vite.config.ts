/// <reference types="vitest/config" />
import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// The build emits plain static assets. They are embedded into the Go binary
// via go:embed behind the `embedui` build tag, so the installation ships as a
// single artefact (PRD DE-01). Nothing here assumes a Node runtime in
// production.
export default defineConfig({
  plugins: [react(), tailwindcss()],
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
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    css: false,
  },
});
