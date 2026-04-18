import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  base: "./",
  plugins: [
    react(),
    {
      name: "joicetyper-local-webview-html",
      transformIndexHtml: {
        order: "post",
        handler(html: string) {
          return html.replace(/\s+crossorigin(?=[\s>])/g, "");
        },
      },
    },
  ],
  build: {
    outDir: "dist",
    emptyOutDir: true
  }
});
