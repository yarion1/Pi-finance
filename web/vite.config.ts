import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// Em desenvolvimento a API Go roda em 127.0.0.1:3100 com PUBLIC_URL=http://localhost:5173.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    proxy: { "/api": "http://127.0.0.1:3100" },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    sourcemap: false,
    // nada inline: a CSP do servidor proíbe scripts e estilos embutidos
    assetsInlineLimit: 0,
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
  },
});
