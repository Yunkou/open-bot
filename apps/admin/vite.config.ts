import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath, URL } from "node:url";

export default defineConfig({
  appType: "spa",
  plugins: [react()],
  clearScreen: false,
  resolve: {
    dedupe: ["react", "react-dom", "antd", "@ant-design/icons"],
    alias: {
      // pnpm: pro-* packages import @ant-design/icons without declaring it;
      // pin to this app's direct dependency so Vite/Rollup can resolve.
      "@ant-design/icons": fileURLToPath(
        new URL("./node_modules/@ant-design/icons", import.meta.url),
      ),
    },
  },
  server: {
    port: 5174,
    host: "127.0.0.1",
    strictPort: true,
  },
});
