import cp from "node:child_process";
import { TanStackRouterVite } from "@tanstack/router-vite-plugin";
import react from "@vitejs/plugin-react";
import Icons from "unplugin-icons/vite";
import { defineConfig, loadEnv } from "vite";
import tsconfigPaths from "vite-tsconfig-paths";

const commitHash = cp
  .execSync("git rev-parse --short HEAD")
  .toString()
  .replace("\n", "");

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  return {
    plugins: [
      TanStackRouterVite({
        quoteStyle: "double",
      }),
      react(),
      tsconfigPaths({
        projects: ["./tsconfig.json"],
      }),
      Icons({
        compiler: "jsx",
        jsx: "react",
        iconCustomizer(_1, _2, props) {
          props.width = "1.5rem";
          props.height = "1.5rem";
        },
      }),
    ],

    server: {
      proxy: {
        "/api": {
          target: env.VITE_API_URL || "http://localhost:5000",
          ws: true,
          headers: {
            Cookie: env.VITE_API_COOKIE || "",
          },
        },
      },
    },
    build: {
      minify: "terser",
      terserOptions: {
        compress: {
          drop_console: true,
          drop_debugger: true,
        },
        format: {
          comments: false,
        },
      },
    },
    define: {
      "import.meta.env.UI_VERSION": JSON.stringify(`${commitHash}-drive-es`),
    },
  };
});
