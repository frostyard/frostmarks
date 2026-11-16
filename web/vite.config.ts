/// <reference types="vitest/config" />
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// lodge serves the build from /usr/share/lodge/apps/marks/web/ at /app/marks/
// (docs/specs/lodge-app.md); its policy loads scripts and styles only from
// assets/, which is Vite's default directory.
export default defineConfig({
  base: "/app/marks/",
  plugins: [react(), tailwindcss()],
  clearScreen: false,
  build: { target: "es2022", outDir: "dist", emptyOutDir: true },
  test: {
    environment: "happy-dom",
    env: { TZ: "UTC" },
    include: ["src/**/*.test.{ts,tsx}"],
    setupFiles: ["src/testing/setup.ts"],
  },
});
