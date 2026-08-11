import { QueryClientProvider } from "@tanstack/react-query";
import { createRouter, RouterProvider } from "@tanstack/react-router";
import { ThemeProvider } from "next-themes";
import ReactDOM from "react-dom/client";
import { Toaster } from "sonner";
import { CommandPaletteProvider } from "./components/command-palette-context";
import { getQueryClient } from "./lib/queryClient";
import { routeTree } from "./routeTree.gen";
import "./styles/globals.css";

const queryClient = getQueryClient();

const router = createRouter({
  routeTree,
  context: { queryClient },
  defaultPreload: "intent",
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}

async function startApp() {
  const rootElement = document.getElementById("root")!;

  if (!rootElement.innerHTML) {
    const root = ReactDOM.createRoot(rootElement);
    root.render(
      <QueryClientProvider client={queryClient}>
        <ThemeProvider attribute="class" defaultTheme="dark" enableSystem={false}>
          <CommandPaletteProvider>
            <RouterProvider router={router} />
          </CommandPaletteProvider>
          <Toaster
            position="bottom-right"
            richColors
            closeButton
            theme="dark"
            toastOptions={{
              style: {
                background: "oklch(0.21 0.008 70 / 0.85)",
                border: "1px solid oklch(0.95 0.02 70 / 0.1)",
                backdropFilter: "blur(16px)",
              },
            }}
          />
        </ThemeProvider>
      </QueryClientProvider>,
    );
  }
}

startApp();
