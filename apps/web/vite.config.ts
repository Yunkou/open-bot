import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";

// Shared by Web browser and Tauri desktop (apps/desktop points here).
export default defineConfig({
  appType: "spa",
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  // Avoid clearing Rust/Tauri compile output when tauri runs beforeDevCommand.
  clearScreen: false,
  server: {
    port: 5173,
    host: "127.0.0.1",
    // Tauri expects a fixed port matching tauri.conf.json devUrl.
    strictPort: true,
  },
});
