import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";
export default defineConfig({
  plugins: [tailwindcss()],
  resolve: { alias: { "@": new URL("./src", import.meta.url).pathname } },
  server: { proxy: { "/api": "http://localhost:8080" } },
});
