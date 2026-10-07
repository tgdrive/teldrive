import { QueryClientProvider } from "@tanstack/react-query";
import { createRouter, RouterProvider } from "@tanstack/react-router";
import { ThemeProvider, useTheme } from "next-themes";
import ReactDOM from "react-dom/client";
import { Toaster } from "sonner";
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

// Toasts follow the chosen theme, drawn with its own overlay and border tokens.
function ThemedToaster() {
  const { resolvedTheme } = useTheme();
  return (
    <Toaster
      position="bottom-right"
      richColors
      closeButton
      theme={resolvedTheme === "light" ? "light" : "dark"}
      toastOptions={{
        style: {
          background: "color-mix(in oklch, var(--overlay) 85%, transparent)",
          border: "1px solid var(--border)",
          backdropFilter: "blur(16px)",
        },
      }}
    />
  );
}

async function startApp() {
  const rootElement = document.getElementById("root")!;

  if (!rootElement.innerHTML) {
    const root = ReactDOM.createRoot(rootElement);
    root.render(
      <QueryClientProvider client={queryClient}>
        {/* index.html starts with both the class and data-theme set to dark, and the dark tokens
            are selected by either: both have to be switched, or the page stays dark. */}
        <ThemeProvider attribute={["class", "data-theme"]} defaultTheme="dark" enableSystem={false}>
          <RouterProvider router={router} />
          <ThemedToaster />
        </ThemeProvider>
      </QueryClientProvider>,
    );
  }
}

startApp();
