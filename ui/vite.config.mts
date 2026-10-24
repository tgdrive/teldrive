import { readFile } from "node:fs/promises";
import path from "node:path";
import babel from "@rolldown/plugin-babel";
import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react, { reactCompilerPreset } from "@vitejs/plugin-react";
import Icons from "unplugin-icons/vite";
import { defineConfig } from "vite";

function foliatePatches() {
  return {
    name: "foliate-patches",
    enforce: "pre" as const,
    transform(code: string, id: string) {
      if (id.includes("/foliate-js/view.js")) {
        return code.replace(
          `    else if (await isPDF(file)) {
        const { makePDF } = await import('./pdf.js')
        book = await makePDF(file)
    }`,
          `    else if (await isPDF(file)) {
        throw new UnsupportedTypeError('PDF files use the Teldrive PDF reader')
    }`,
        );
      }
      if (id.includes("/foliate-js/paginator.js")) {
        let patched = code.replace(
          "#observer = new ResizeObserver(() => this.expand())",
          "#observer = new ResizeObserver(entries => { if (entries.some(entry => entry.target.isConnected)) this.expand() })",
        );
        patched = patched.replace(
          "#observer = new ResizeObserver(() => this.render())",
          "#observer = new ResizeObserver(entries => { if (entries.some(entry => entry.target.isConnected)) this.render() })",
        );
        patched = patched.replace(
          "    render(layout) {\n        if (!layout) return",
          "    render(layout) {\n        if (!layout || !this.document) return",
        );
        patched = patched.replace(
          "    render() {\n        if (!this.#view) return",
          "    render() {\n        if (!this.#view?.document) return",
        );
        return patched;
      }
    },
  };
}

function embedPdfNotices() {
  return {
    name: "embedpdf-notices",
    async generateBundle() {
      this.emitFile({
        type: "asset",
        fileName: "licenses/embedpdf-LICENSE.txt",
        source: await readFile(
          path.resolve(import.meta.dirname, "node_modules/@embedpdf/engine-runtime-wasm32/LICENSE"),
        ),
      });
      this.emitFile({
        type: "asset",
        fileName: "licenses/embedpdf-NOTICE.txt",
        source:
          "EmbedPDF 3.0.0-next.15 (Apache-2.0)\nhttps://github.com/embedpdf/embed-pdf-viewer\nPDFium WASM runtime: https://github.com/embedpdf/runtime\n",
      });
    },
  };
}

export default defineConfig(() => {
  const backendAddress = process.env.TELDRIVE_HTTP_ADDRESS ?? "127.0.0.1:8080";
  const backendHost = backendAddress.startsWith("0.0.0.0:")
    ? `127.0.0.1:${backendAddress.slice("0.0.0.0:".length)}`
    : backendAddress;

  return {
    define: {
      "import.meta.env.VITE_APP_VERSION": JSON.stringify(process.env.VERSION?.trim() || "dev"),
    },
    plugins: [
      foliatePatches(),
      embedPdfNotices(),
      tanstackRouter(),
      react(),
      babel({ presets: [reactCompilerPreset()] }),
      tailwindcss(),
      Icons({
        compiler: "jsx",
        jsx: "react",
        autoInstall: true,
        iconCustomizer(_1, _2, props) {
          props.width = "1.25rem";
          props.height = "1.25rem";
          props.className = "pointer-events-none";
        },
      }),
    ],
    resolve: {
      dedupe: ["react", "react-dom"],
      alias: {
        "@": path.resolve(import.meta.dirname, "./src"),
      },
    },
    optimizeDeps: {
      exclude: ["foliate-js"],
      // Loaded on demand when "Audio tracks and subtitles from the file" is on and a Matroska/WebM
      // video is opened; prebundling them keeps the dev server from reloading the page when it first
      // finds them.
      include: ["mediabunny", "@mediabunny/ac3"],
    },
    server: {
      cors: true,
      proxy: {
        "/api": {
          target: `http://${backendHost}`,
        },
      },
    },
  };
});
